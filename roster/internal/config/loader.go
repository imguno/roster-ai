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
			switch d.Name() {
			case ".roster", ".venv", "venv", "node_modules", ".git":
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
	p.applyGroupDesks()
	p.applyDefaults()
	p.bindImplicitAgents()
	return p, nil
}

// applyGroupDesks sets desk.Parent for any desk listed in group.Desks that
// doesn't already have a parent set. This lets groups declare their members
// inline instead of requiring each desk file to carry a parent: field.
func (p *Project) applyGroupDesks() {
	for groupID, group := range p.Groups {
		for _, deskID := range group.Desks {
			if desk, ok := p.Desks[deskID]; ok && desk.Parent == "" {
				desk.Parent = groupID
			}
		}
	}
}

// applyDefaults applies org-level and group-level defaults to desks.
// Priority: desk-level config > group defaults > org defaults.
func (p *Project) applyDefaults() {
	var orgDefaults *types.DeskDefaults
	if p.Organization != nil {
		orgDefaults = p.Organization.Defaults
	}

	// Build group membership from desk.Parent declarations.
	deskGroup := map[string]*types.Group{}
	for _, desk := range p.Desks {
		if desk.Parent != "" {
			if g, ok := p.Groups[desk.Parent]; ok {
				deskGroup[desk.ID] = g
			}
		}
	}
	// Also map lead desks to their group.
	for _, g := range p.Groups {
		if g.Lead != nil {
			deskGroup[g.Lead.Desk] = g
		}
	}

	for id, desk := range p.Desks {
		var effective types.DeskDefaults
		if orgDefaults != nil {
			mergeDefaults(&effective, orgDefaults)
		}
		if g, ok := deskGroup[id]; ok && g.Defaults != nil {
			mergeDefaults(&effective, g.Defaults)
		}

		if desk.Executor.Type == "" && effective.Executor != nil {
			desk.Executor.Type = effective.Executor.Type
			desk.Executor.SDK = effective.Executor.SDK
			desk.Executor.Address = effective.Executor.Address
		}
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
		if desk.Policy == "" && effective.Policy != "" {
			desk.Policy = effective.Policy
		}
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

func mergeDefaults(dst, src *types.DeskDefaults) {
	if dst.Executor == nil && src.Executor != nil {
		cp := *src.Executor
		dst.Executor = &cp
	} else if dst.Executor != nil && src.Executor != nil {
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
func (p *Project) bindImplicitAgents() {
	for id, desk := range p.Desks {
		if desk.Agent.IsLocal() || desk.Agent.IsRemote() {
			continue
		}
		if _, ok := p.Agents[id]; ok {
			desk.Agent = types.AgentRef{ID: id}
		}
	}
}

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
		return nil
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
		var desk types.Desk
		if err := strictUnmarshal(data, &desk); err != nil {
			return fmt.Errorf("config: parse desk %s: %w", path, err)
		}
		if desk.ID == "" {
			desk.ID = fileID(path, types.KindDesk)
		}
		desk.SourcePath = filepath.Dir(path)
		p.Desks[desk.ID] = &desk

	case types.KindGroup:
		var v types.Group
		if err := strictUnmarshal(data, &v); err != nil {
			return fmt.Errorf("config: parse group %s: %w", path, err)
		}
		if v.ID == "" {
			v.ID = fileID(path, types.KindGroup)
		}
		p.Groups[v.ID] = &v

	case types.KindOrg, types.KindOrganization:
		var v types.Org
		if err := strictUnmarshal(data, &v); err != nil {
			return fmt.Errorf("config: parse org %s: %w", path, err)
		}
		if v.ID == "" {
			v.ID = fileID(path, header.Kind)
		}
		if p.Organization != nil {
			return fmt.Errorf("config: multiple orgs found (only one allowed): %s", path)
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

func (p *Project) resolveAgentRefs(projectDir string) error {
	for _, desk := range p.Desks {
		ref := desk.Agent
		if !ref.IsLocal() || !isPathRef(ref.ID) {
			continue
		}
		absPath := ref.ID
		if !filepath.IsAbs(ref.ID) {
			absPath = filepath.Join(desk.SourcePath, ref.ID)
		}
		agentID, err := p.agentIDForPath(absPath)
		if err != nil {
			return fmt.Errorf("config: desk %q: agent ref %q: %w", desk.ID, ref.ID, err)
		}
		desk.Agent = types.AgentRef{ID: agentID}
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
