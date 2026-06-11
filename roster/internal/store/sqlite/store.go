package sqlite

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"

	"github.com/roster-io/roster/internal/store"
)

var _ store.Store = (*Store)(nil)

type Store struct {
	db      *sql.DB
	desk    *deskStore
	session *deskSessionStore
	group   *groupStore
	run     *runStore
	notes   *noteStore
	metrics *metricStore
}

func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %s: %w", dbPath, err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: enable WAL: %w", err)
	}
	if err := createSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{
		db:      db,
		desk:    &deskStore{db: db},
		session: &deskSessionStore{db: db},
		group:   &groupStore{db: db},
		run:     &runStore{db: db},
		notes:   &noteStore{db: db},
		metrics: &metricStore{db: db},
	}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Desk() store.DeskStore              { return s.desk }
func (s *Store) DeskSession() store.DeskSessionStore { return s.session }
func (s *Store) Group() store.GroupStore             { return s.group }
func (s *Store) Run() store.RunStore                 { return s.run }
func (s *Store) Notes() store.NoteStore              { return s.notes }
func (s *Store) Metrics() store.MetricStore          { return s.metrics }
