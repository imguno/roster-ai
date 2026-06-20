package hub

import (
	"sort"
	"sync"

	"github.com/roster-io/roster/pkg/types"
)

// Registry holds the in-memory configuration: desks, agents, groups, resources, org.
type Registry struct {
	mu           sync.RWMutex
	organization *types.Organization
	desks        map[string]*types.Desk
	agents       map[string]*types.Agent
	groups       map[string]*types.Group
	resources    map[string]*types.Resource
}

func newRegistry() *Registry {
	return &Registry{
		desks:     make(map[string]*types.Desk),
		agents:    make(map[string]*types.Agent),
		groups:    make(map[string]*types.Group),
		resources: make(map[string]*types.Resource),
	}
}

func (r *Registry) Load(org *types.Organization, agents map[string]*types.Agent, desks map[string]*types.Desk, groups map[string]*types.Group, resources map[string]*types.Resource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.organization = org
	for id, a := range agents {
		r.agents[id] = a
	}
	for id, d := range desks {
		r.desks[id] = d
	}
	for id, g := range groups {
		r.groups[id] = g
	}
	for id, res := range resources {
		r.resources[id] = res
	}
}

func (r *Registry) Desks() map[string]*types.Desk {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.desks
}

func (r *Registry) Groups() map[string]*types.Group {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.groups
}

func (r *Registry) Resources() map[string]*types.Resource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.resources
}

func (r *Registry) Organization() *types.Organization {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.organization
}

// GroupParent implements group.ParentLookup.
func (r *Registry) GroupParent(groupID string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.groups[groupID]
	if !ok {
		return "", false
	}
	return g.Parent, true
}

func (r *Registry) GroupDesks(groupID string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var ids []string
	for id, d := range r.desks {
		if d.BelongsTo(groupID) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func (r *Registry) GroupSubGroups(groupID string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var ids []string
	for id, g := range r.groups {
		if g.Parent == groupID {
			ids = append(ids, id)
		}
	}
	return ids
}

func (r *Registry) DeskInGroup(groupID, deskID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if desk, ok := r.desks[deskID]; ok {
		return desk.BelongsTo(groupID)
	}
	return false
}
