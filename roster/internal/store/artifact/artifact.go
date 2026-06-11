package artifact

import "github.com/roster-io/roster/pkg/types"

// Validator checks that an artifact conforms to its declared schema.
type Validator interface {
	Validate(a *types.Artifact) error
}
