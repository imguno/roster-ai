package file

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/roster-io/roster/pkg/types"
)

type runStore struct {
	mu  sync.RWMutex
	dir string
}

func newRunStore(dir string) *runStore {
	_ = os.MkdirAll(dir, 0750)
	return &runStore{dir: dir}
}

func (s *runStore) stepPath(runID, groupID, deskID string) (string, error) {
	for _, part := range []string{runID, groupID} {
		if strings.ContainsAny(part, "/\\.") {
			return "", fmt.Errorf("state: invalid run step key component %q", part)
		}
	}
	cleaned := filepath.Clean(deskID)
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("state: deskID %q escapes run directory", deskID)
	}
	base := filepath.Join(s.dir, runID, groupID)
	p := filepath.Join(base, cleaned+".md")
	if !strings.HasPrefix(filepath.Clean(p), filepath.Clean(base)+string(filepath.Separator)) {
		return "", fmt.Errorf("state: deskID %q escapes run directory", deskID)
	}
	return p, nil
}

func (s *runStore) SaveStep(runID, groupID, deskID string, artifact *types.Artifact) {
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
		artifact.ID, artifact.AgentID, artifact.Schema,
		artifact.CreatedAt.UTC().Format(time.RFC3339), artifact.Payload,
	)
	_ = os.WriteFile(p, []byte(content), 0640)
}

func (s *runStore) LoadStep(runID, groupID, deskID string) (*types.Artifact, bool) {
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
		ID: meta["id"], AgentID: meta["agent_id"], Schema: meta["schema"],
		Payload: []byte(body), CreatedAt: createdAt,
	}, true
}

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
	return meta, strings.TrimPrefix(rest[end+5:], "\n"), nil
}
