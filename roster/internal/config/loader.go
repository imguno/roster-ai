package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/roster-io/roster/pkg/types"
)

// Project holds all config loaded from a project directory.
type Project struct {
	Organization *types.Organization        // at most one
	Agents       map[string]*types.Agent    // key: agent ID
	Desks        map[string]*types.Desk     // key: desk ID
	Groups       map[string]*types.Group    // key: group ID
	Resources    map[string]*types.Resource // key: resource ID
	Policies     map[string]*types.Policy   // key: policy ID
	SourceFiles  []string                   // all config files loaded
}

// LoadProject recursively scans dir for YAML/JSON config files.
func LoadProject(dir string) (*Project, error) {
	p := &Project{
		Agents:    make(map[string]*types.Agent),
		Desks:     make(map[string]*types.Desk),
		Groups:    make(map[string]*types.Group),
		Resources: make(map[string]*types.Resource),
		Policies:  make(map[string]*types.Policy),
	}

	if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip .roster/ — it contains runtime data, not config
			if d.Name() == ".roster" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			return nil
		}
		if err := p.loadFile(path); err != nil {
			return err
		}
		p.SourceFiles = append(p.SourceFiles, path)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("config: scan %s: %w", dir, err)
	}

	if err := p.resolveAgentRefs(dir); err != nil {
		return nil, err
	}
	p.applyDefaults()
	p.bindImplicitAgents()
	return p, nil
}

// applyDefaults applies organization-level and group-level defaults to desks.
// Priority: desk-level config > group defaults > org defaults.
func (p *Project) applyDefaults() {
	var orgDefaults *types.DeskDefaults
	if p.Organization != nil {
		orgDefaults = p.Organization.Defaults
	}

	// Build group membership: deskID → group
	deskGroup := map[string]*types.Group{}
	for _, g := range p.Groups {
		for _, deskID := range g.Desks {
			deskGroup[deskID] = g
		}
		if g.Lead != nil {
			deskGroup[g.Lead.Desk] = g
		}
	}

	for id, desk := range p.Desks {
		// Resolve effective defaults: org → group → desk
		var effective types.DeskDefaults
		if orgDefaults != nil {
			mergeDefaults(&effective, orgDefaults)
		}
		if g, ok := deskGroup[id]; ok && g.Defaults != nil {
			mergeDefaults(&effective, g.Defaults)
		}

		// Apply executor defaults if desk has no executor type set.
		if desk.Executor.Type == "" && effective.Executor != nil {
			desk.Executor.Type = effective.Executor.Type
			desk.Executor.SDK = effective.Executor.SDK
			desk.Executor.Address = effective.Executor.Address
		}
		// Merge executor params: defaults first, desk overrides.
		if effective.Executor != nil && len(effective.Executor.Params) > 0 {
			if desk.Executor.Params == nil {
				desk.Executor.Params = make(map[string]string)
			}
			for k, v := range effective.Executor.Params {
				if _, exists := desk.Executor.Params[k]; !exists {
					desk.Executor.Params[k] = v
				}
			}
		}
		// Merge executor env: defaults first, desk overrides.
		if effective.Executor != nil && len(effective.Executor.Env) > 0 {
			if desk.Executor.Env == nil {
				desk.Executor.Env = make(map[string]string)
			}
			for k, v := range effective.Executor.Env {
				if _, exists := desk.Executor.Env[k]; !exists {
					desk.Executor.Env[k] = v
				}
			}
		}
		// Apply policy default.
		if desk.Policy == "" && effective.Policy != "" {
			desk.Policy = effective.Policy
		}
		// Merge tags (additive).
		if len(effective.Tags) > 0 {
			seen := make(map[string]bool, len(desk.Tags))
			for _, t := range desk.Tags {
				seen[t] = true
			}
			for _, t := range effective.Tags {
				if !seen[t] {
					desk.Tags = append(desk.Tags, t)
				}
			}
		}
	}
}

// mergeDefaults copies src fields into dst where dst is empty.
func mergeDefaults(dst, src *types.DeskDefaults) {
	if dst.Executor == nil && src.Executor != nil {
		cp := *src.Executor
		dst.Executor = &cp
	} else if dst.Executor != nil && src.Executor != nil {
		// Merge params/env from src where dst doesn't have them.
		if dst.Executor.Type == "" {
			dst.Executor.Type = src.Executor.Type
		}
		if dst.Executor.SDK == "" {
			dst.Executor.SDK = src.Executor.SDK
		}
		for k, v := range src.Executor.Params {
			if dst.Executor.Params == nil {
				dst.Executor.Params = make(map[string]string)
			}
			if _, exists := dst.Executor.Params[k]; !exists {
				dst.Executor.Params[k] = v
			}
		}
		for k, v := range src.Executor.Env {
			if dst.Executor.Env == nil {
				dst.Executor.Env = make(map[string]string)
			}
			if _, exists := dst.Executor.Env[k]; !exists {
				dst.Executor.Env[k] = v
			}
		}
	}
	if dst.Policy == "" {
		dst.Policy = src.Policy
	}
	if len(dst.Tags) == 0 {
		dst.Tags = src.Tags
	}
}

// bindImplicitAgents auto-binds desk.Agent when desk name matches an agent ID.
// This eliminates the need for `agent: reviewer` when the desk is named "reviewer".
func (p *Project) bindImplicitAgents() {
	for id, desk := range p.Desks {
		if desk.Agent != "" {
			continue // explicitly set
		}
		if _, ok := p.Agents[id]; ok {
			desk.Agent = id
		}
	}
}

// fileID derives an ID from the file path.
// If the stem matches the kind name (e.g. "agent.yaml" in "researcher/"),
// uses the parent directory name instead.
func fileID(path string, kind types.Kind) string {
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if strings.EqualFold(stem, string(kind)) {
		parent := filepath.Base(filepath.Dir(path))
		if parent != "." && parent != "" {
			return parent
		}
	}
	return stem
}

// strictUnmarshal decodes YAML data into v and returns an error for any
// unrecognised field names. This catches typos like "subscribes" for "subscribe"
// at config load time rather than silently at runtime.
func strictUnmarshal(data []byte, v interface{}) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	return dec.Decode(v)
}

func (p *Project) loadFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config: read %s: %w", path, err)
	}
	data := []byte(os.ExpandEnv(string(raw)))

	var header struct {
		Kind types.Kind `yaml:"kind"`
	}
	if err := yaml.Unmarshal(data, &header); err != nil {
		return fmt.Errorf("config: parse %s: %w", path, err)
	}
	if header.Kind == "" {
		return nil // not a Roster config file
	}

	switch header.Kind {
	case types.KindAgent:
		var v types.Agent
		if err := strictUnmarshal(data, &v); err != nil {
			return fmt.Errorf("config: parse agent %s: %w", path, err)
		}
		if v.ID == "" {
			v.ID = fileID(path, types.KindAgent)
		}
		p.Agents[v.ID] = &v

	case types.KindDesk:
		desk, inlineAgent, err := parseDeskFile(data, path)
		if err != nil {
			return fmt.Errorf("config: parse desk %s: %w", path, err)
		}
		if desk.ID == "" {
			desk.ID = fileID(path, types.KindDesk)
		}
		desk.SourcePath = filepath.Dir(path)
		if inlineAgent != nil {
			if inlineAgent.ID == "" {
				inlineAgent.ID = desk.ID
			}
			p.Agents[inlineAgent.ID] = inlineAgent
			desk.Agent = inlineAgent.ID
		}
		p.Desks[desk.ID] = desk

	case types.KindGroup:
		var v types.Group
		if err := strictUnmarshal(data, &v); err != nil {
			return fmt.Errorf("config: parse group %s: %w", path, err)
		}
		if v.ID == "" {
			v.ID = fileID(path, types.KindGroup)
		}
		p.Groups[v.ID] = &v

	case types.KindOrganization:
		var v types.Organization
		if err := strictUnmarshal(data, &v); err != nil {
			return fmt.Errorf("config: parse organization %s: %w", path, err)
		}
		if v.ID == "" {
			v.ID = fileID(path, types.KindOrganization)
		}
		if p.Organization != nil {
			return fmt.Errorf("config: multiple organizations found (only one allowed): %s", path)
		}
		p.Organization = &v

	case types.KindResource:
		var v types.Resource
		if err := strictUnmarshal(data, &v); err != nil {
			return fmt.Errorf("config: parse resource %s: %w", path, err)
		}
		if v.ID == "" {
			v.ID = fileID(path, types.KindResource)
		}
		p.Resources[v.ID] = &v

	case types.KindPolicy:
		var v types.Policy
		if err := strictUnmarshal(data, &v); err != nil {
			return fmt.Errorf("config: parse policy %s: %w", path, err)
		}
		if v.ID == "" {
			v.ID = fileID(path, types.KindPolicy)
		}
		p.Policies[v.ID] = &v
	}

	return nil
}

// deskRaw handles the `agent` field as either a string reference or inline object.
type deskRaw struct {
	Kind        types.Kind              `yaml:"kind"`
	ID          string                  `yaml:"id"`
	Name        string                  `yaml:"name"`
	Description string                  `yaml:"description"`
	Agent       yaml.Node               `yaml:"agent"`
	Executor    types.ExecutorConfig    `yaml:"executor"`
	Concurrency types.ConcurrencyConfig `yaml:"concurrency"`
	Subscribe   []string                `yaml:"subscribe"`
	Emit        []string                `yaml:"emit"`
	Cron        string                  `yaml:"cron"`
	Resources   []string                `yaml:"resources"`
	Tags        []string                `yaml:"tags"`
	Policy      string                  `yaml:"policy"`
	Session     types.SessionConfig     `yaml:"session"`
	Triggers    []types.TriggerConfig   `yaml:"triggers"`
}

func parseDeskFile(data []byte, path string) (*types.Desk, *types.Agent, error) {
	var raw deskRaw
	if err := strictUnmarshal(data, &raw); err != nil {
		return nil, nil, err
	}

	desk := &types.Desk{
		Kind:        raw.Kind,
		ID:          raw.ID,
		Name:        raw.Name,
		Description: raw.Description,
		Executor:    raw.Executor,
		Concurrency: raw.Concurrency,
		Subscribe:   raw.Subscribe,
		Emit:        raw.Emit,
		Cron:        raw.Cron,
		Resources:   raw.Resources,
		Tags:        raw.Tags,
		Policy:      raw.Policy,
		Session:     raw.Session,
		Triggers:    raw.Triggers,
	}

	var inlineAgent *types.Agent
	switch raw.Agent.Kind {
	case yaml.ScalarNode:
		desk.Agent = raw.Agent.Value
	case yaml.MappingNode:
		var a types.Agent
		if err := raw.Agent.Decode(&a); err != nil {
			return nil, nil, fmt.Errorf("inline agent: %w", err)
		}
		inlineAgent = &a
	}

	return desk, inlineAgent, nil
}

func (p *Project) resolveAgentRefs(projectDir string) error {
	for _, desk := range p.Desks {
		ref := desk.Agent
		if ref == "" || !isPathRef(ref) {
			continue
		}
		absPath := ref
		if !filepath.IsAbs(ref) {
			absPath = filepath.Join(desk.SourcePath, ref)
		}
		agentID, err := p.agentIDForPath(absPath)
		if err != nil {
			return fmt.Errorf("config: desk %q: agent ref %q: %w", desk.ID, ref, err)
		}
		desk.Agent = agentID
	}
	return nil
}

func (p *Project) agentIDForPath(absPath string) (string, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("read agent file: %w", err)
	}
	var a types.Agent
	if err := yaml.Unmarshal(data, &a); err != nil {
		return "", fmt.Errorf("parse agent file: %w", err)
	}
	if a.ID == "" {
		a.ID = fileID(absPath, types.KindAgent)
	}
	return a.ID, nil
}

func isPathRef(s string) bool {
	ext := filepath.Ext(s)
	return ext == ".yaml" || ext == ".yml" || ext == ".json" ||
		len(s) > 0 && (s[0] == '.' || s[0] == '/')
}
