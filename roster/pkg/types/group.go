package types

// GroupLead configures a lead desk for a group.
// The lead desk runs twice: first to plan and assign work, last to synthesize results.
// Position "both" (default): plan → members → synthesize.
// Position "first": lead decomposes, members execute.
// Position "last": members run, lead synthesizes.
type GroupLead struct {
	Desk     string `yaml:"desk" json:"desk"`
	Position string `yaml:"position,omitempty" json:"position,omitempty"`
}

// Group is a team. Desks in a group share an event stream — every desk sees
// what the others produce.
//
// The lead desk runs twice: first to plan and assign work, last to synthesize results.
//
//	lead (plan)  →  members (work)  →  lead (synthesize)  →  output event
type Group struct {
	Kind        Kind            `yaml:"kind" json:"kind"`
	ID          string          `yaml:"id,omitempty" json:"id,omitempty"`
	Name        string          `yaml:"name,omitempty" json:"name,omitempty"`
	Description string          `yaml:"description,omitempty" json:"description,omitempty"`
	Lead        *GroupLead      `yaml:"lead,omitempty" json:"lead,omitempty"`
	Desks       []string        `yaml:"desks,omitempty" json:"desks,omitempty"`
	Groups      []string        `yaml:"groups,omitempty" json:"groups,omitempty"`
	Resources   []string        `yaml:"resources,omitempty" json:"resources,omitempty"`

	// Event subscriptions: which event types this group listens to.
	Subscribe []string `yaml:"subscribe,omitempty" json:"subscribe,omitempty"`
	// Event emissions: which event types this group produces on completion.
	Emit []string `yaml:"emit,omitempty" json:"emit,omitempty"`

	// Cron schedule expression (e.g. "0 */3 * * *" = every 3 hours).
	// When set, the group auto-triggers on this schedule.
	Cron string `yaml:"cron,omitempty" json:"cron,omitempty"`

	Policy   string `yaml:"policy,omitempty" json:"policy,omitempty"`
	Dispatch string `yaml:"dispatch,omitempty" json:"dispatch,omitempty"`

	// Triggers define automated event sources for this group.
	Triggers []TriggerConfig `yaml:"triggers,omitempty" json:"triggers,omitempty"`

	// Defaults override organization-level defaults for desks in this group.
	// Group defaults take priority over org defaults; desk-level config takes priority over both.
	Defaults *DeskDefaults `yaml:"defaults,omitempty" json:"defaults,omitempty"`
}
