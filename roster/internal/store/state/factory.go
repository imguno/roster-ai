package state

import (
	"fmt"
	"path/filepath"

	"github.com/roster-io/roster/pkg/types"
)

// NewStore creates a Store from the given configuration.
// Supported backends: "file" (default), "sqlite", "memory".
func NewStore(cfg types.StoreConfig, projectDir string) (Store, error) {
	backend := cfg.Backend
	if backend == "" {
		backend = "file"
	}

	switch backend {
	case "file":
		dir := cfg.Path
		if dir == "" {
			dir = filepath.Join(projectDir, ".roster", "data")
		}
		return NewFileStore(dir)

	case "sqlite":
		dbPath := cfg.Path
		if dbPath == "" {
			dbPath = filepath.Join(projectDir, ".roster", "data", "roster.db")
		}
		return NewSQLiteStore(dbPath)

	case "memory":
		return NewMemoryStore(), nil

	default:
		return nil, fmt.Errorf("state: unknown store backend %q (supported: file, sqlite, memory)", backend)
	}
}
