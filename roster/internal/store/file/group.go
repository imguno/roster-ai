package file

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/roster-io/roster/internal/store"
)

type groupStore struct {
	mu  sync.RWMutex
	dir string
}

func newGroupStore(dir string) (*groupStore, error) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, err
	}
	return &groupStore{dir: dir}, nil
}

func (s *groupStore) groupPath(groupID string) (string, error) {
	p := filepath.Clean(filepath.Join(s.dir, groupID+".md"))
	base := filepath.Clean(s.dir) + string(filepath.Separator)
	if !strings.HasPrefix(p, base) {
		return "", fmt.Errorf("state: groupID %q escapes store directory", groupID)
	}
	return p, nil
}

func (s *groupStore) Append(groupID string, msg store.Message) error {
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

func (s *groupStore) History(groupID string) ([]store.Message, error) {
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

func (s *groupStore) Clear(groupID string) {
	p, err := s.groupPath(groupID)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = os.Remove(p)
}

func parseGroupMd(content string) []store.Message {
	var msgs []store.Message
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
		msgs = append(msgs, store.Message{
			DeskID: strings.TrimSpace(parts[0]),
			Role:   strings.TrimSpace(parts[1]),
			Content: body,
		})
	}
	return msgs
}
