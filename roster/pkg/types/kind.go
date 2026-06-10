package types

// Kind identifies the type of a Roster configuration file.
// Every config file must declare its kind so the hub can discover and load
// files from any directory structure without relying on folder names.
type Kind string

const (
	KindAgent        Kind = "agent"
	KindDesk         Kind = "desk"
	KindGroup        Kind = "group"
	KindSkill        Kind = "skill"
	KindOrg          Kind = "org"
	KindOrganization Kind = "organization" // v1 alias, kept for backwards compatibility
	KindResource     Kind = "resource"
	KindPolicy       Kind = "policy"
)
