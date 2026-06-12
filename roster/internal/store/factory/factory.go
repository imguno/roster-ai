package factory

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/roster-io/roster/internal/store"
	"github.com/roster-io/roster/internal/store/memory"
	"github.com/roster-io/roster/internal/store/sqlite"
	"github.com/roster-io/roster/pkg/types"
)

// New creates a Store from the given configuration.
// Supported backends: "sqlite" (default), "memory".
// The legacy "file" backend now maps to sqlite.
func New(cfg types.StoreConfig, projectDir string) (store.Store, error) {
	backend := cfg.Backend
	if backend == "" {
		backend = "sqlite"
	}

	switch backend {
	case "file", "sqlite":
		dbPath := cfg.Path
		if dbPath == "" {
			dir := filepath.Join(projectDir, ".roster", "data")
			_ = os.MkdirAll(dir, 0750)
			dbPath = filepath.Join(dir, "roster.db")
		}
		return sqlite.New(dbPath)

	case "memory":
		return memory.New(), nil

	default:
		return nil, fmt.Errorf("store: unknown backend %q (supported: sqlite, memory)", backend)
	}
}
