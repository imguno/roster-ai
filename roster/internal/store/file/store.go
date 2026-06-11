package file

import (
	"os"
	"path/filepath"

	"github.com/roster-io/roster/internal/store"
)

var _ store.Store = (*Store)(nil)

type Store struct {
	desk    *deskStore
	session *deskSessionStore
	group   *groupStore
	run     *runStore
	notes   *noteStore
	metrics *metricStore
}

func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, err
	}
	desk, err := newDeskStore(filepath.Join(dir, "artifacts.json"))
	if err != nil {
		return nil, err
	}
	session, err := newDeskSessionStore(filepath.Join(dir, "sessions"))
	if err != nil {
		return nil, err
	}
	group, err := newGroupStore(filepath.Join(dir, "groups"))
	if err != nil {
		return nil, err
	}
	return &Store{
		desk:    desk,
		session: session,
		group:   group,
		run:     newRunStore(filepath.Join(dir, "runs")),
		notes:   newNoteStore(filepath.Join(dir, "notes.json")),
		metrics: newMetricStore(filepath.Join(dir, "metrics.jsonl")),
	}, nil
}

func (s *Store) Desk() store.DeskStore              { return s.desk }
func (s *Store) DeskSession() store.DeskSessionStore { return s.session }
func (s *Store) Group() store.GroupStore             { return s.group }
func (s *Store) Run() store.RunStore                 { return s.run }
func (s *Store) Notes() store.NoteStore              { return s.notes }
func (s *Store) Metrics() store.MetricStore          { return s.metrics }
