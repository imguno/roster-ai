package types

// Skill is the resolved content of a skill definition.
// Community members publish skills in this format to git repos or web URLs.
type Skill struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	Prompt  string `yaml:"prompt"` // the instruction set handed to the runner
}

// SkillRef is a raw reference string from an agent definition.
// It can be:
//   - a plain name ("product-planning-v2")            → resolved from local registry
//   - a git path ("github.com/org/repo/skill-name")  → fetched from git
//   - an https URL ("https://example.com/skill.yaml") → fetched over HTTP
type SkillRef = string
