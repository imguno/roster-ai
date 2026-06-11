package factory

import (
	"fmt"
	"path/filepath"

	"github.com/roster-io/roster/internal/store"
	filestore "github.com/roster-io/roster/internal/store/file"
	"github.com/roster-io/roster/internal/store/memory"
	"github.com/roster-io/roster/internal/store/sqlite"
	"github.com/roster-io/roster/pkg/types"
)

// New creates a Store from the given configuration.
// Supported backends: "file" (default), "sqlite", "memory".
func New(cfg types.StoreConfig, projectDir string) (store.Store, error) {
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
		return filestore.New(dir)

	case "sqlite":
		dbPath := cfg.Path
		if dbPath == "" {
			dbPath = filepath.Join(projectDir, ".roster", "data", "roster.db")
		}
		return sqlite.New(dbPath)

	case "memory":
		return memory.New(), nil

	default:
		return nil, fmt.Errorf("store: unknown backend %q (supported: file, sqlite, memory)", backend)
	}
}
