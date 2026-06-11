package file

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/roster-io/roster/internal/store"
)

const (
	maxSessionEntries = 40
	maxRunFiles       = 10
)

type deskSessionStore struct {
	mu  sync.RWMutex
	dir string
}

func newDeskSessionStore(dir string) (*deskSessionStore, error) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, err
	}
	return &deskSessionStore{dir: dir}, nil
}

func (s *deskSessionStore) deskDir(deskID string) (string, error) {
	if strings.ContainsAny(deskID, "/\\.") {
		return "", fmt.Errorf("state: invalid deskID %q", deskID)
	}
	return filepath.Join(s.dir, deskID), nil
}

func (s *deskSessionStore) sessionPath(deskID, runID string) (string, error) {
	if strings.ContainsAny(runID, "/\\.") {
		return "", fmt.Errorf("state: invalid runID %q", runID)
	}
	d, err := s.deskDir(deskID)
	if err != nil {
		return "", err
	}
	return filepath.Join(d, runID+".md"), nil
}

func (s *deskSessionStore) Append(deskID, runID string, entry store.SessionEntry) {
	p, err := s.sessionPath(deskID, runID)
	if err != nil {
		return
	}
	block := fmt.Sprintf("## %s · %s\n\n%s\n\n---\n\n",
		entry.Role, entry.At.UTC().Format(time.RFC3339), entry.Content,
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

func (s *deskSessionStore) Load(deskID string) []store.SessionEntry {
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
	sort.Strings(runFiles)
	if len(runFiles) > maxRunFiles {
		runFiles = runFiles[len(runFiles)-maxRunFiles:]
	}

	var result []store.SessionEntry
	if hasSummary {
		data, err := os.ReadFile(filepath.Join(d, "summary.md"))
		if err == nil {
			result = append(result, parseSessionMd(string(data))...)
		}
	}
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

func (s *deskSessionStore) Summarize(deskID string, summary string) {
	d, err := s.deskDir(deskID)
	if err != nil {
		return
	}
	block := fmt.Sprintf("## system · %s\n\n%s\n\n---\n\n",
		time.Now().UTC().Format(time.RFC3339), summary,
	)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(d, 0750); err != nil {
		return
	}
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

func parseSessionMd(content string) []store.SessionEntry {
	var entries []store.SessionEntry
	for _, block := range strings.Split(content, "\n---\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		idx := strings.Index(block, "\n")
		if idx < 0 {
			continue
		}
		header := strings.TrimPrefix(block[:idx], "## ")
		body := strings.TrimSpace(block[idx+1:])
		parts := strings.SplitN(header, " · ", 2)
		if len(parts) != 2 {
			continue
		}
		at, _ := time.Parse(time.RFC3339, strings.TrimSpace(parts[1]))
		entries = append(entries, store.SessionEntry{
			Role: strings.TrimSpace(parts[0]), Content: body, At: at,
		})
	}
	return entries
}
