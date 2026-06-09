package web

import (
	"encoding/json"
	"net/http"

	"github.com/roster-io/roster/pkg/types"
)

func (s *Server) handleCrons(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	infos := s.hub.CronStatus()
	if infos == nil {
		infos = []types.CronInfo{}
	}
	json.NewEncoder(w).Encode(infos)
}
