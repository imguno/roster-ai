package skill

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/roster-io/roster/pkg/types"
)

// Resolver fetches and parses a Skill from any supported source.
// Remote skills are cached locally so repeated resolutions don't hit the network.
type Resolver struct {
	localDir string      // project skills/ folder
	cacheDir string      // ~/.roster/skills or .roster/skills
	http     *http.Client
}

func NewResolver(localDir string) *Resolver {
	cacheDir := filepath.Join(localDir, ".roster", "skills")
	return &Resolver{
		localDir: localDir,
		cacheDir: cacheDir,
		http:     &http.Client{},
	}
}

// Resolve fetches a skill by its ref string.
func (r *Resolver) Resolve(ctx context.Context, ref types.SkillRef) (*types.Skill, error) {
	switch {
	case strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "http://"):
		return r.fetchWithCache(ctx, ref, ref)
	case looksLikeGit(ref):
		rawURL, err := toRawURL(ref)
		if err != nil {
			return nil, fmt.Errorf("skill: git ref %s: %w", ref, err)
		}
		return r.fetchWithCache(ctx, ref, rawURL)
	default:
		return r.fetchLocal(ref)
	}
}

// fetchWithCache downloads a skill and caches it by ref hash.
// It tries the base URL with .yaml, .md, and bare extensions.
func (r *Resolver) fetchWithCache(ctx context.Context, cacheKey, baseURL string) (*types.Skill, error) {
	// Check cache (with TTL).
	cached, err := r.loadCache(cacheKey)
	if err == nil {
		return cached, nil
	}

	// Try multiple extensions.
	urls := []string{baseURL + ".yaml", baseURL + ".md", baseURL}
	for _, url := range urls {
		skill, data, err := r.fetchHTTP(ctx, url)
		if err == nil {
			_ = r.saveCache(cacheKey, data) // best-effort
			return skill, nil
		}
	}
	return nil, fmt.Errorf("skill: fetch %s: all extensions failed", baseURL)
}

func (r *Resolver) fetchHTTP(ctx context.Context, url string) (*types.Skill, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("skill: http request %s: %w", url, err)
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("skill: http fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("skill: http fetch %s: status %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("skill: http read %s: %w", url, err)
	}
	skill, err := parseSkill(url, data)
	return skill, data, err
}

func (r *Resolver) fetchLocal(name string) (*types.Skill, error) {
	candidates := []string{
		filepath.Join(r.localDir, name+".yaml"),
		filepath.Join(r.localDir, "skills", name+".yaml"),
		filepath.Join(r.localDir, "knowhow", name+".yaml"),
		filepath.Join(r.localDir, name+".md"),
		filepath.Join(r.localDir, "skills", name+".md"),
		filepath.Join(r.localDir, "knowhow", name+".md"),
		filepath.Join(r.localDir, name+".txt"),
		filepath.Join(r.localDir, "skills", name+".txt"),
		filepath.Join(r.localDir, "knowhow", name+".txt"),
		filepath.Join(r.localDir, name),
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil {
			return parseSkill(path, data)
		}
	}
	return nil, fmt.Errorf("skill: %q not found in %s", name, r.localDir)
}

func (r *Resolver) cacheKeyToPath(key string) string {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(key)))
	return filepath.Join(r.cacheDir, hash+".yaml")
}

func (r *Resolver) loadCache(key string) (*types.Skill, error) {
	p := r.cacheKeyToPath(key)
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	// Versioned refs (containing @) are cached indefinitely.
	// Unversioned refs expire after 1 hour.
	if !strings.Contains(key, "@") && time.Since(info.ModTime()) > time.Hour {
		return nil, fmt.Errorf("cache expired")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return parseSkill(p, data)
}

func (r *Resolver) saveCache(key string, data []byte) error {
	if err := os.MkdirAll(r.cacheDir, 0750); err != nil {
		return err
	}
	return os.WriteFile(r.cacheKeyToPath(key), data, 0640)
}

func parseSkill(path string, data []byte) (*types.Skill, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".txt", "":
		// Plain text / markdown: entire content is the prompt.
		return &types.Skill{Prompt: strings.TrimSpace(string(data))}, nil
	case ".yaml", ".yml", ".json":
		var s types.Skill
		if err := yaml.Unmarshal(data, &s); err != nil {
			return nil, fmt.Errorf("skill: parse: %w", err)
		}
		if s.Prompt == "" {
			return nil, fmt.Errorf("skill: missing required field 'prompt'")
		}
		return &s, nil
	default:
		// Unknown extension: treat as plain text.
		return &types.Skill{Prompt: strings.TrimSpace(string(data))}, nil
	}
}

func looksLikeGit(ref string) bool {
	return strings.HasPrefix(ref, "github.com/") ||
		strings.HasPrefix(ref, "gitlab.com/") ||
		strings.HasPrefix(ref, "bitbucket.org/")
}

func toRawURL(ref string) (string, error) {
	ref = strings.TrimPrefix(ref, "github.com/")
	// Split off @version if present.
	version := "main"
	if idx := strings.LastIndex(ref, "@"); idx > 0 {
		version = ref[idx+1:]
		ref = ref[:idx]
	}
	parts := strings.SplitN(ref, "/", 3)
	if len(parts) < 3 {
		return "", fmt.Errorf("expected github.com/<org>/<repo>/<path>[@version]")
	}
	org, repo, path := parts[0], parts[1], parts[2]
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", org, repo, version, path), nil
}
