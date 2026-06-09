package observe

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// Recorder collects events and writes them to an append-only JSONL log file.
// Subscribers can register channels to receive events in real-time.
type Recorder struct {
	mu     sync.RWMutex
	events []Event
	file   *os.File // nil = memory-only mode
	subs   []chan Event
}

// NewRecorder creates an in-memory recorder (for tests / ephemeral use).
func NewRecorder() *Recorder {
	return &Recorder{}
}

// NewFileRecorder creates a recorder that appends to logFile (JSONL format).
func NewFileRecorder(logFile string) (*Recorder, error) {
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		return nil, err
	}
	r := &Recorder{file: f}

	existing, _ := os.ReadFile(logFile)
	for _, line := range splitLines(existing) {
		var e Event
		if json.Unmarshal(line, &e) == nil {
			r.events = append(r.events, e)
		}
	}
	return r, nil
}

func (r *Recorder) Record(e Event) {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	r.mu.Lock()
	r.events = append(r.events, e)
	if r.file != nil {
		data, _ := json.Marshal(e)
		data = append(data, '\n')
		_, _ = r.file.Write(data)
	}
	subs := make([]chan Event, len(r.subs))
	copy(subs, r.subs)
	r.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// Subscribe returns a channel that receives events in real-time.
// Call the returned cancel func to unsubscribe.
func (r *Recorder) Subscribe() (chan Event, func()) {
	ch := make(chan Event, 64)
	r.mu.Lock()
	r.subs = append(r.subs, ch)
	r.mu.Unlock()
	cancel := func() {
		r.mu.Lock()
		for i, s := range r.subs {
			if s == ch {
				r.subs = append(r.subs[:i], r.subs[i+1:]...)
				break
			}
		}
		r.mu.Unlock()
		close(ch)
	}
	return ch, cancel
}

// Events returns all recorded events.
func (r *Recorder) Events() []Event {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Event, len(r.events))
	copy(out, r.events)
	return out
}

func (r *Recorder) Close() error {
	if r.file != nil {
		return r.file.Close()
	}
	return nil
}

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
