// Package group contains domain logic for session scope groups.
// No external dependencies — pure Go types and functions.
package group

// ParentLookup provides parent information for groups.
type ParentLookup interface {
	GroupParent(groupID string) (parent string, ok bool)
}

// AncestorChain returns the group itself plus all ancestor group IDs
// by walking Parent links. The returned slice starts with groupID.
func AncestorChain(groupID string, lookup ParentLookup) []string {
	var chain []string
	for cur := groupID; cur != ""; {
		chain = append(chain, cur)
		parent, ok := lookup.GroupParent(cur)
		if !ok || parent == "" {
			break
		}
		cur = parent
	}
	return chain
}

// AllAncestors returns deduplicated ancestor group IDs for multiple groups.
func AllAncestors(groupIDs []string, lookup ParentLookup) []string {
	seen := make(map[string]bool)
	var result []string
	for _, gid := range groupIDs {
		for _, ancestor := range AncestorChain(gid, lookup) {
			if !seen[ancestor] {
				seen[ancestor] = true
				result = append(result, ancestor)
			}
		}
	}
	return result
}
