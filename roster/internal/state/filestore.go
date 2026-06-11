package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/roster-io/roster/pkg/types"
)

// FileStore is a production Store backed by human-readable files in a data directory.
// Sessions and group histories are stored as Markdown; run steps use YAML frontmatter + body.
type FileStore struct {
	dir         string
	desk        *fileDeskStore
	deskSession *fileDeskSessionStore
	group       *fileGroupStore
	run         *fileRunStore
	notes       *fileNoteStore
}

func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, err
	}
	fs := &FileStore{dir: dir}

	var err error
	fs.desk, err = newFileDeskStore(filepath.Join(dir, "artifacts.json"))
	if err != nil {
		return nil, err
	}
	fs.deskSession, err = newFileDeskSessionStore(filepath.Join(dir, "sessions"))
	if err != nil {
		return nil, err
	}
	fs.group, err = newFileGroupStore(filepath.Join(dir, "groups"))
	if err != nil {
		return nil, err
	}
	fs.run = newFileRunStore(filepath.Join(dir, "runs"))
	fs.notes = newFileNoteStore(filepath.Join(dir, "notes.json"))
	return fs, nil
}

func (f *FileStore) Desk() DeskStore              { return f.desk }
func (f *FileStore) DeskSession() DeskSessionStore { return f.deskSession }
func (f *FileStore) Group() GroupStore             { return f.group }
func (f *FileStore) Run() RunStore                 { return f.run }
func (f *FileStore) Notes() NoteStore              { return f.notes }

// --- fileDeskStore ---

type fileDeskStore struct {
	mu   sync.RWMutex
	path string
	data map[string][]*fileArtifact
}

type fileArtifact struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	Schema    string    `json:"schema"`
	Payload   []byte    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
}

func newFileDeskStore(path string) (*fileDeskStore, error) {
	s := &fileDeskStore{path: path, data: make(map[string][]*fileArtifact)}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *fileDeskStore) Save(deskID string, artifact *types.Artifact) {
	if artifact == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[deskID] = append(s.data[deskID], &fileArtifact{
		ID:        artifact.ID,
		AgentID:   artifact.AgentID,
		Schema:    artifact.Schema,
		Payload:   artifact.Payload,
		CreatedAt: artifact.CreatedAt,
	})
	_ = s.flush()
}

func (s *fileDeskStore) Get(deskID string) ([]*types.Artifact, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, ok := s.data[deskID]
	if !ok {
		return nil, false
	}
	out := make([]*types.Artifact, len(raw))
	for i, r := range raw {
		out[i] = &types.Artifact{
			ID: r.ID, AgentID: r.AgentID, Schema: r.Schema,
			Payload: r.Payload, CreatedAt: r.CreatedAt,
		}
	}
	return out, true
}

func (s *fileDeskStore) load() error {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.data)
}

func (s *fileDeskStore) flush() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0640)
}

// maxSessionEntries is the maximum number of session entries returned by Load.
// Older entries beyond this limit are silently dropped to prevent unbounded
// context growth across many runs.
const maxSessionEntries = 40

// --- fileDeskSessionStore ---
//
// Each desk+run gets its own Markdown file: sessions/{deskID}/{runID}.md
// A special summary file lives at: sessions/{deskID}/summary.md
//
// Format per entry:
//
//	## {role} · {RFC3339 timestamp}
//
//	{content}
//
//	---
//
// Load reads the summary file (if present) followed by the last N run files
// sorted by name (run IDs embed timestamps so lexicographic order = temporal order).
// The total is windowed to maxSessionEntries to prevent unbounded context growth.
type fileDeskSessionStore struct {
	mu  sync.RWMutex
	dir string
}

func newFileDeskSessionStore(dir string) (*fileDeskSessionStore, error) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, err
	}
	return &fileDeskSessionStore{dir: dir}, nil
}

// deskDir returns the per-desk session directory, validating deskID.
func (s *fileDeskSessionStore) deskDir(deskID string) (string, error) {
	if strings.ContainsAny(deskID, "/\\.") {
		return "", fmt.Errorf("state: invalid deskID %q", deskID)
	}
	return filepath.Join(s.dir, deskID), nil
}

// sessionPath returns the path for a specific run's session file, validating runID.
func (s *fileDeskSessionStore) sessionPath(deskID, runID string) (string, error) {
	if strings.ContainsAny(runID, "/\\.") {
		return "", fmt.Errorf("state: invalid runID %q", runID)
	}
	d, err := s.deskDir(deskID)
	if err != nil {
		return "", err
	}
	return filepath.Join(d, runID+".md"), nil
}

func (s *fileDeskSessionStore) Append(deskID, runID string, entry SessionEntry) {
	p, err := s.sessionPath(deskID, runID)
	if err != nil {
		return
	}
	block := fmt.Sprintf("## %s · %s\n\n%s\n\n---\n\n",
		entry.Role,
		entry.At.UTC().Format(time.RFC3339),
		entry.Content,
	)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(p), 0750); err != nil {
		return
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(block)
}

// maxRunFiles is the maximum number of per-run session files read by Load.
// Reading more files than this is unnecessary since we window to maxSessionEntries anyway.
const maxRunFiles = 10

func (s *fileDeskSessionStore) Load(deskID string) []SessionEntry {
	d, err := s.deskDir(deskID)
	if err != nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(d)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return nil
	}

	// Separate summary.md from run files.
	var runFiles []string
	hasSummary := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if e.Name() == "summary.md" {
			hasSummary = true
		} else {
			runFiles = append(runFiles, e.Name())
		}
	}

	// Sort run files lexicographically (run IDs contain timestamps).
	sort.Strings(runFiles)

	// Take the last maxRunFiles run files.
	if len(runFiles) > maxRunFiles {
		runFiles = runFiles[len(runFiles)-maxRunFiles:]
	}

	var result []SessionEntry

	// Prepend summary entries if a summary file exists.
	if hasSummary {
		data, err := os.ReadFile(filepath.Join(d, "summary.md"))
		if err == nil {
			result = append(result, parseSessionMd(string(data))...)
		}
	}

	// Append entries from each run file in order.
	for _, name := range runFiles {
		data, err := os.ReadFile(filepath.Join(d, name))
		if err != nil {
			continue
		}
		result = append(result, parseSessionMd(string(data))...)
	}

	if len(result) > maxSessionEntries {
		result = result[len(result)-maxSessionEntries:]
	}
	return result
}

func (s *fileDeskSessionStore) Summarize(deskID string, summary string) {
	d, err := s.deskDir(deskID)
	if err != nil {
		return
	}
	block := fmt.Sprintf("## system · %s\n\n%s\n\n---\n\n",
		time.Now().UTC().Format(time.RFC3339),
		summary,
	)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(d, 0750); err != nil {
		return
	}
	// Write the summary file, then remove all per-run files so the summary
	// becomes the sole base context.
	summaryPath := filepath.Join(d, "summary.md")
	if err := os.WriteFile(summaryPath, []byte(block), 0640); err != nil {
		return
	}
	runEntries, _ := os.ReadDir(d)
	for _, e := range runEntries {
		if e.Name() != "summary.md" && strings.HasSuffix(e.Name(), ".md") {
			_ = os.Remove(filepath.Join(d, e.Name()))
		}
	}
}

// parseSessionMd parses session Markdown into SessionEntry slice.
func parseSessionMd(content string) []SessionEntry {
	var entries []SessionEntry
	for _, block := range strings.Split(content, "\n---\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		idx := strings.Index(block, "\n")
		if idx < 0 {
			continue
		}
		headerLine := block[:idx]
		body := strings.TrimSpace(block[idx+1:])
		header := strings.TrimPrefix(headerLine, "## ")
		parts := strings.SplitN(header, " · ", 2)
		if len(parts) != 2 {
			continue
		}
		role := strings.TrimSpace(parts[0])
		at, _ := time.Parse(time.RFC3339, strings.TrimSpace(parts[1]))
		entries = append(entries, SessionEntry{Role: role, Content: body, At: at})
	}
	return entries
}

// --- fileGroupStore ---
//
// Each group gets its own Markdown file: groups/{groupID}.md
// Format per message:
//
//	## {deskID} · {role}
//
//	{content}
//
//	---
type fileGroupStore struct {
	mu  sync.RWMutex
	dir string
}

func newFileGroupStore(dir string) (*fileGroupStore, error) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, err
	}
	return &fileGroupStore{dir: dir}, nil
}

func (s *fileGroupStore) groupPath(groupID string) (string, error) {
	p := filepath.Clean(filepath.Join(s.dir, groupID+".md"))
	base := filepath.Clean(s.dir) + string(filepath.Separator)
	if !strings.HasPrefix(p, base) {
		return "", fmt.Errorf("state: groupID %q escapes store directory", groupID)
	}
	return p, nil
}

func (s *fileGroupStore) Append(groupID string, msg Message) error {
	p, err := s.groupPath(groupID)
	if err != nil {
		return err
	}
	block := fmt.Sprintf("## %s · %s\n\n%s\n\n---\n\n", msg.DeskID, msg.Role, msg.Content)
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(block)
	return err
}

func (s *fileGroupStore) History(groupID string) ([]Message, error) {
	p, err := s.groupPath(groupID)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("state: read group %q: %w", groupID, err)
	}
	return parseGroupMd(string(data)), nil
}

func (s *fileGroupStore) Clear(groupID string) {
	p, err := s.groupPath(groupID)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = os.Remove(p)
}

// parseGroupMd parses group Markdown into Message slice.
func parseGroupMd(content string) []Message {
	var msgs []Message
	for _, block := range strings.Split(content, "\n---\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		idx := strings.Index(block, "\n")
		if idx < 0 {
			continue
		}
		headerLine := block[:idx]
		body := strings.TrimSpace(block[idx+1:])
		header := strings.TrimPrefix(headerLine, "## ")
		parts := strings.SplitN(header, " · ", 2)
		if len(parts) != 2 {
			continue
		}
		msgs = append(msgs, Message{
			DeskID:  strings.TrimSpace(parts[0]),
			Role:    strings.TrimSpace(parts[1]),
			Content: body,
		})
	}
	return msgs
}

// --- fileRunStore ---
//
// Persists per-desk step artifacts under runs/{runID}/{groupID}/{deskID}.md
// Format: YAML frontmatter (id, agent_id, schema, created_at) + blank line + payload body.
// Sub-keys like "architect/plan" are allowed and create subdirectories.
type fileRunStore struct {
	mu  sync.RWMutex
	dir string
}

func newFileRunStore(dir string) *fileRunStore {
	_ = os.MkdirAll(dir, 0750)
	return &fileRunStore{dir: dir}
}

func (s *fileRunStore) stepPath(runID, groupID, deskID string) (string, error) {
	// runID and groupID must not contain path separators or dots.
	for _, part := range []string{runID, groupID} {
		if strings.ContainsAny(part, "/\\.") {
			return "", fmt.Errorf("state: invalid run step key component %q", part)
		}
	}
	// deskID may contain "/" for sub-keys like "architect/plan"; block ".." traversal only.
	cleaned := filepath.Clean(deskID)
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("state: deskID %q escapes run directory", deskID)
	}
	base := filepath.Join(s.dir, runID, groupID)
	p := filepath.Join(base, cleaned+".md")
	absBase := filepath.Clean(base)
	absP := filepath.Clean(p)
	if !strings.HasPrefix(absP, absBase+string(filepath.Separator)) {
		return "", fmt.Errorf("state: deskID %q escapes run directory", deskID)
	}
	return p, nil
}

func (s *fileRunStore) SaveStep(runID, groupID, deskID string, artifact *types.Artifact) {
	if artifact == nil {
		return
	}
	p, err := s.stepPath(runID, groupID, deskID)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(p), 0750); err != nil {
		return
	}
	content := fmt.Sprintf("---\nid: %s\nagent_id: %s\nschema: %s\ncreated_at: %s\n---\n\n%s",
		artifact.ID,
		artifact.AgentID,
		artifact.Schema,
		artifact.CreatedAt.UTC().Format(time.RFC3339),
		artifact.Payload,
	)
	_ = os.WriteFile(p, []byte(content), 0640)
}

func (s *fileRunStore) LoadStep(runID, groupID, deskID string) (*types.Artifact, bool) {
	p, err := s.stepPath(runID, groupID, deskID)
	if err != nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	meta, body, err := parseMarkdownFrontmatter(string(data))
	if err != nil {
		return nil, false
	}
	createdAt, _ := time.Parse(time.RFC3339, meta["created_at"])
	return &types.Artifact{
		ID:        meta["id"],
		AgentID:   meta["agent_id"],
		Schema:    meta["schema"],
		Payload:   []byte(body),
		CreatedAt: createdAt,
	}, true
}

// --- fileNoteStore ---
//
// Persists notes as a flat JSON map: notes/{scopeID → {key → base64(value)}}.
// The file is rewritten on every mutation (notes are infrequent).
type fileNoteStore struct {
	mu   sync.RWMutex
	path string
	data map[string]map[string][]byte // scopeID → key → value
}

func newFileNoteStore(path string) *fileNoteStore {
	s := &fileNoteStore{path: path, data: make(map[string]map[string][]byte)}
	_ = s.load()
	return s
}

func (s *fileNoteStore) Set(scopeID, key string, value []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data[scopeID] == nil {
		s.data[scopeID] = make(map[string][]byte)
	}
	s.data[scopeID][key] = value
	_ = s.flush()
}

func (s *fileNoteStore) Get(scopeID, key string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[scopeID][key]
	return v, ok
}

func (s *fileNoteStore) Delete(scopeID, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data[scopeID], key)
	_ = s.flush()
}

func (s *fileNoteStore) All(scopeID string) map[string][]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.data[scopeID]
	out := make(map[string][]byte, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func (s *fileNoteStore) load() error {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.data)
}

func (s *fileNoteStore) flush() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0640)
}

// parseMarkdownFrontmatter extracts YAML frontmatter and body from Markdown content.
// Frontmatter is delimited by "---\n" at the start and "\n---\n" at the end.
func parseMarkdownFrontmatter(content string) (map[string]string, string, error) {
	meta := make(map[string]string)
	if !strings.HasPrefix(content, "---\n") {
		return meta, content, nil
	}
	rest := content[4:]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return meta, content, nil
	}
	for _, line := range strings.Split(rest[:end], "\n") {
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) == 2 {
			meta[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	body := strings.TrimPrefix(rest[end+5:], "\n")
	return meta, body, nil
}
