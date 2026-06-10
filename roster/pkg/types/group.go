package types

// GroupLead configures a lead desk for a group.
// Position "both" (default): lead(plan) → members(work) → lead(synthesize).
// Position "first": lead decomposes, members execute.
// Position "last": members run, lead synthesizes.
type GroupLead struct {
	Desk     string `yaml:"desk" json:"desk"`
	Position string `yaml:"position,omitempty" json:"position,omitempty"`
}

// Group is a team container. Desks and sub-groups declare membership
// via their `parent` field — the group does not list its children.
type Group struct {
	Kind        Kind       `yaml:"kind" json:"kind"`
	ID          string     `yaml:"id,omitempty" json:"id,omitempty"`
	Name        string     `yaml:"name,omitempty" json:"name,omitempty"`
	Description string     `yaml:"description,omitempty" json:"description,omitempty"`
	Parent      string     `yaml:"parent,omitempty" json:"parent,omitempty"`
	Lead        *GroupLead `yaml:"lead,omitempty" json:"lead,omitempty"`
	Resources   []string   `yaml:"resources,omitempty" json:"resources,omitempty"`

	// Event subscriptions: which event types this group listens to.
	Subscribe []string `yaml:"subscribe,omitempty" json:"subscribe,omitempty"`
	// Event emissions: which event types this group produces on completion.
	Emit []string `yaml:"emit,omitempty" json:"emit,omitempty"`

	// Cron schedule expression (e.g. "0 */3 * * *" = every 3 hours).
	Cron string `yaml:"cron,omitempty" json:"cron,omitempty"`

	Policy   string `yaml:"policy,omitempty" json:"policy,omitempty"`
	Dispatch string `yaml:"dispatch,omitempty" json:"dispatch,omitempty"`

	Triggers []TriggerConfig `yaml:"triggers,omitempty" json:"triggers,omitempty"`

	// Defaults override org-level defaults for desks in this group.
	Defaults *DeskDefaults `yaml:"defaults,omitempty" json:"defaults,omitempty"`
}
