package queue

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/roster-io/roster/pkg/types"
)

// Entry is a queued event with metadata.
type Entry struct {
	ID       string      `json:"id"`
	RunID    string      `json:"run_id,omitempty"` // stable group run ID for checkpoint recovery
	Event    types.Event `json:"event"`
	QueuedAt time.Time   `json:"queued_at"`
	Status   string      `json:"status"` // "pending", "processing", "done", "failed"
	Error    string      `json:"error,omitempty"`
	DoneAt   *time.Time  `json:"done_at,omitempty"`
}

// Queue is the interface for event queues. Implementations may persist to file, SQLite, Redis, etc.
type Queue interface {
	Push(ev types.Event) *Entry
	Take() *Entry
	TakeAll() []*Entry
	Pending() []*Entry
	PendingCount() int
	Complete(entryID string)
	Fail(entryID, errMsg string)
	ContainsEventID(eventID string) bool
	// ContainsPendingType reports whether any pending or processing entry has the given event type.
	// Used to deduplicate ID-less events (e.g. hub.started) when one is already queued.
	ContainsPendingType(eventType string) bool
	RequeueProcessing() int
	// CollapseIDlessPending removes all but the latest pending entry for each ID-less event type
	// (events where Event.ID == ""). These are idempotent events like hub.started and cron ticks
	// where only the most recent occurrence matters. Returns the number of entries collapsed.
	// Called on startup after RequeueProcessing to drain stale accumulated events.
	CollapseIDlessPending() int
	// Signal returns a channel that receives a notification whenever a new item is pushed.
	// Workers should select on this instead of polling with time.Sleep.
	Signal() <-chan struct{}
	// GC removes completed and failed entries older than the given duration.
	GC(olderThan time.Duration) int
}

// Compile-time interface checks.
var _ Queue = (*FileQueue)(nil)
var _ Queue = (*MemoryQueue)(nil)

// ---------------------------------------------------------------------------
// FileQueue — persistent, per-subscriber event queue backed by a JSONL file.
// ---------------------------------------------------------------------------

// FileQueue is a persistent, per-subscriber event queue.
// Events are appended to a JSONL file and processed sequentially.
type FileQueue struct {
	mu      sync.Mutex
	id      string // subscriber ID (group or desk)
	dir     string
	entries []*Entry
	nextSeq int
	notify  chan struct{}
}

// NewQueue creates or loads a file-backed queue for the given subscriber.
func NewQueue(dir, subscriberID string) (*FileQueue, error) {
	qDir := filepath.Join(dir, "queues")
	if err := os.MkdirAll(qDir, 0750); err != nil {
		return nil, err
	}
	q := &FileQueue{
		id:     subscriberID,
		dir:    qDir,
		notify: make(chan struct{}, 1),
	}
	if err := q.load(); err != nil {
		return nil, err
	}
	return q, nil
}

func (q *FileQueue) filePath() string {
	return filepath.Join(q.dir, q.id+".jsonl")
}

// Signal returns a channel that is notified on every Push.
func (q *FileQueue) Signal() <-chan struct{} {
	return q.notify
}

// signal performs a non-blocking send on the notify channel.
func (q *FileQueue) signal() {
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

// Push adds an event to the queue.
func (q *FileQueue) Push(ev types.Event) *Entry {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.nextSeq++
	entry := &Entry{
		ID:       fmt.Sprintf("%s-%d", q.id, q.nextSeq),
		Event:    ev,
		QueuedAt: time.Now(),
		Status:   "pending",
	}
	q.entries = append(q.entries, entry)
	q.flush()
	q.signal()
	return entry
}

// Pending returns all pending events (oldest first).
func (q *FileQueue) Pending() []*Entry {
	q.mu.Lock()
	defer q.mu.Unlock()

	var out []*Entry
	for _, e := range q.entries {
		if e.Status == "pending" {
			out = append(out, e)
		}
	}
	return out
}

// PendingCount returns the number of pending events.
func (q *FileQueue) PendingCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	count := 0
	for _, e := range q.entries {
		if e.Status == "pending" {
			count++
		}
	}
	return count
}

// Take marks the next pending entry as "processing" and returns it.
// Returns nil if the queue is empty.
func (q *FileQueue) Take() *Entry {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, e := range q.entries {
		if e.Status == "pending" {
			e.Status = "processing"
			q.flush()
			return e
		}
	}
	return nil
}

// TakeAll marks all pending entries as "processing" and returns them.
// Used by lead desks to batch-process queued work.
func (q *FileQueue) TakeAll() []*Entry {
	q.mu.Lock()
	defer q.mu.Unlock()

	var batch []*Entry
	for _, e := range q.entries {
		if e.Status == "pending" {
			e.Status = "processing"
			batch = append(batch, e)
		}
	}
	if len(batch) > 0 {
		q.flush()
	}
	return batch
}

// Complete marks an entry as done.
func (q *FileQueue) Complete(entryID string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, e := range q.entries {
		if e.ID == entryID {
			e.Status = "done"
			now := time.Now()
			e.DoneAt = &now
			q.flush()
			return
		}
	}
}

// Fail marks an entry as failed with an error message.
func (q *FileQueue) Fail(entryID, errMsg string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, e := range q.entries {
		if e.ID == entryID {
			e.Status = "failed"
			e.Error = errMsg
			now := time.Now()
			e.DoneAt = &now
			q.flush()
			return
		}
	}
}

// ContainsEventID reports whether any pending or processing entry carries the given event ID.
// Used by the hub to prevent double-enqueue when multiple routing paths deliver the same event.
func (q *FileQueue) ContainsEventID(eventID string) bool {
	if eventID == "" {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, e := range q.entries {
		if e.Status != "done" && e.Status != "failed" && e.Event.ID == eventID {
			return true
		}
	}
	return false
}

// ContainsPendingType reports whether any pending or processing entry has the given event type.
func (q *FileQueue) ContainsPendingType(eventType string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, e := range q.entries {
		if e.Status != "done" && e.Status != "failed" && e.Event.Type == eventType {
			return true
		}
	}
	return false
}

// RequeueProcessing moves any "processing" entries back to "pending".
// Called on startup to recover from interrupted runs.
func (q *FileQueue) RequeueProcessing() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	count := 0
	for _, e := range q.entries {
		if e.Status == "processing" {
			e.Status = "pending"
			count++
		}
	}
	if count > 0 {
		q.flush()
	}
	return count
}

// CollapseIDlessPending removes all but the latest pending entry for each ID-less event type.
func (q *FileQueue) CollapseIDlessPending() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	type indexed struct {
		idx int
		e   *Entry
	}
	byType := map[string][]indexed{}
	for i, e := range q.entries {
		if e.Status == "pending" && e.Event.ID == "" {
			byType[e.Event.Type] = append(byType[e.Event.Type], indexed{i, e})
		}
	}

	now := time.Now()
	removed := 0
	for _, entries := range byType {
		if len(entries) <= 1 {
			continue
		}
		// Keep the last entry (most recent), mark all earlier ones as done.
		for _, ie := range entries[:len(entries)-1] {
			ie.e.Status = "done"
			ie.e.DoneAt = &now
			removed++
		}
	}
	if removed > 0 {
		q.flush()
	}
	return removed
}

// GC removes completed and failed entries older than the given duration, then flushes.
func (q *FileQueue) GC(olderThan time.Duration) int {
	q.mu.Lock()
	defer q.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	removed := 0
	kept := make([]*Entry, 0, len(q.entries))
	for _, e := range q.entries {
		if (e.Status == "done" || e.Status == "failed") && e.DoneAt != nil && e.DoneAt.Before(cutoff) {
			removed++
			continue
		}
		kept = append(kept, e)
	}
	if removed > 0 {
		q.entries = kept
		q.flush()
	}
	return removed
}

func (q *FileQueue) load() error {
	data, err := os.ReadFile(q.filePath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	for _, line := range splitLines(data) {
		var e Entry
		if json.Unmarshal(line, &e) == nil {
			q.entries = append(q.entries, &e)
			q.nextSeq++
		}
	}
	return nil
}

func (q *FileQueue) flush() {
	f, err := os.Create(q.filePath())
	if err != nil {
		return
	}
	defer f.Close()
	for _, e := range q.entries {
		data, _ := json.Marshal(e)
		f.Write(data)
		f.Write([]byte("\n"))
	}
}

// ---------------------------------------------------------------------------
// MemoryQueue — in-memory queue without file persistence (for tests / ephemeral use).
// ---------------------------------------------------------------------------

// MemoryQueue is an in-memory queue that implements Queue without file persistence.
type MemoryQueue struct {
	mu      sync.Mutex
	id      string
	entries []*Entry
	nextSeq int
	notify  chan struct{}
}

// NewMemoryQueue creates a new in-memory queue.
func NewMemoryQueue(id string) *MemoryQueue {
	return &MemoryQueue{
		id:     id,
		notify: make(chan struct{}, 1),
	}
}

// Signal returns a channel that is notified on every Push.
func (q *MemoryQueue) Signal() <-chan struct{} {
	return q.notify
}

func (q *MemoryQueue) signal() {
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

// Push adds an event to the queue.
func (q *MemoryQueue) Push(ev types.Event) *Entry {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.nextSeq++
	entry := &Entry{
		ID:       fmt.Sprintf("%s-%d", q.id, q.nextSeq),
		Event:    ev,
		QueuedAt: time.Now(),
		Status:   "pending",
	}
	q.entries = append(q.entries, entry)
	q.signal()
	return entry
}

// Pending returns all pending events (oldest first).
func (q *MemoryQueue) Pending() []*Entry {
	q.mu.Lock()
	defer q.mu.Unlock()

	var out []*Entry
	for _, e := range q.entries {
		if e.Status == "pending" {
			out = append(out, e)
		}
	}
	return out
}

// PendingCount returns the number of pending events.
func (q *MemoryQueue) PendingCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	count := 0
	for _, e := range q.entries {
		if e.Status == "pending" {
			count++
		}
	}
	return count
}

// Take marks the next pending entry as "processing" and returns it.
func (q *MemoryQueue) Take() *Entry {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, e := range q.entries {
		if e.Status == "pending" {
			e.Status = "processing"
			return e
		}
	}
	return nil
}

// TakeAll marks all pending entries as "processing" and returns them.
func (q *MemoryQueue) TakeAll() []*Entry {
	q.mu.Lock()
	defer q.mu.Unlock()

	var batch []*Entry
	for _, e := range q.entries {
		if e.Status == "pending" {
			e.Status = "processing"
			batch = append(batch, e)
		}
	}
	return batch
}

// Complete marks an entry as done.
func (q *MemoryQueue) Complete(entryID string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, e := range q.entries {
		if e.ID == entryID {
			e.Status = "done"
			now := time.Now()
			e.DoneAt = &now
			return
		}
	}
}

// Fail marks an entry as failed with an error message.
func (q *MemoryQueue) Fail(entryID, errMsg string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, e := range q.entries {
		if e.ID == entryID {
			e.Status = "failed"
			e.Error = errMsg
			now := time.Now()
			e.DoneAt = &now
			return
		}
	}
}

// ContainsEventID reports whether any pending or processing entry carries the given event ID.
func (q *MemoryQueue) ContainsEventID(eventID string) bool {
	if eventID == "" {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, e := range q.entries {
		if e.Status != "done" && e.Status != "failed" && e.Event.ID == eventID {
			return true
		}
	}
	return false
}

// ContainsPendingType reports whether any pending or processing entry has the given event type.
func (q *MemoryQueue) ContainsPendingType(eventType string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, e := range q.entries {
		if e.Status != "done" && e.Status != "failed" && e.Event.Type == eventType {
			return true
		}
	}
	return false
}

// CollapseIDlessPending removes all but the latest pending entry for each ID-less event type.
func (q *MemoryQueue) CollapseIDlessPending() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	type indexed struct {
		idx int
		e   *Entry
	}
	byType := map[string][]indexed{}
	for i, e := range q.entries {
		if e.Status == "pending" && e.Event.ID == "" {
			byType[e.Event.Type] = append(byType[e.Event.Type], indexed{i, e})
		}
	}

	now := time.Now()
	removed := 0
	for _, entries := range byType {
		if len(entries) <= 1 {
			continue
		}
		for _, ie := range entries[:len(entries)-1] {
			ie.e.Status = "done"
			ie.e.DoneAt = &now
			removed++
		}
	}
	return removed
}

// RequeueProcessing moves any "processing" entries back to "pending".
func (q *MemoryQueue) RequeueProcessing() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	count := 0
	for _, e := range q.entries {
		if e.Status == "processing" {
			e.Status = "pending"
			count++
		}
	}
	return count
}

// GC removes completed and failed entries older than the given duration.
func (q *MemoryQueue) GC(olderThan time.Duration) int {
	q.mu.Lock()
	defer q.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	removed := 0
	kept := make([]*Entry, 0, len(q.entries))
	for _, e := range q.entries {
		if (e.Status == "done" || e.Status == "failed") && e.DoneAt != nil && e.DoneAt.Before(cutoff) {
			removed++
			continue
		}
		kept = append(kept, e)
	}
	if removed > 0 {
		q.entries = kept
	}
	return removed
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
