package memory

import (
	"github.com/roster-io/roster/internal/store"
	"github.com/roster-io/roster/pkg/types"
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

func New() *Store {
	return &Store{
		desk:    &deskStore{data: make(map[string][]*types.Artifact)},
		session: &deskSessionStore{data: make(map[string][]store.SessionEntry)},
		group:   &groupStore{data: make(map[string][]store.Message)},
		run:     &runStore{data: make(map[string]*types.Artifact)},
		notes:   &noteStore{data: make(map[string]map[string][]byte)},
		metrics: &metricStore{},
	}
}

func (s *Store) Desk() store.DeskStore              { return s.desk }
func (s *Store) DeskSession() store.DeskSessionStore { return s.session }
func (s *Store) Group() store.GroupStore             { return s.group }
func (s *Store) Run() store.RunStore                 { return s.run }
func (s *Store) Notes() store.NoteStore              { return s.notes }
func (s *Store) Metrics() store.MetricStore          { return s.metrics }
