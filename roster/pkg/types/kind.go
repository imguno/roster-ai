package types

// Kind identifies the type of a Roster configuration file.
type Kind string

const (
	KindAgent        Kind = "agent"
	KindDesk         Kind = "desk"
	KindGroup        Kind = "group"
	KindSkill        Kind = "skill"
	KindOrg          Kind = "org"
	KindOrganization Kind = "organization" // v1 alias
	KindResource     Kind = "resource"
)
