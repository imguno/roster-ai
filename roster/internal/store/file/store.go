// Package file provides a file-based store backed by sqlite.
// Legacy file-based implementations have been replaced by a unified
// sqlite database stored in the data directory.
package file

import (
	"path/filepath"

	"github.com/roster-io/roster/internal/store"
	"github.com/roster-io/roster/internal/store/sqlite"
)

// New creates a file-based store backed by sqlite in the given directory.
func New(dir string) (store.Store, error) {
	dbPath := filepath.Join(dir, "roster.db")
	return sqlite.New(dbPath)
}
