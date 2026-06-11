package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/roster-io/roster/internal/config"
	"github.com/roster-io/roster/internal/validate"
	"github.com/roster-io/roster/pkg/types"
)

func (s *Server) handleQueues(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.hub.QueueStatus())
}

func (s *Server) handleWarnings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	warnings := s.hub.Warnings()
	if warnings == nil {
		warnings = []types.Warning{}
	}
	json.NewEncoder(w).Encode(warnings)
}

// handleLoad loads (or reloads) an organization from a directory.
// POST /api/load  {"dir": "./my-org"}
func (s *Server) handleBudget(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.hub.BudgetStatus())
}

func (s *Server) handleLoad(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Dir string `json:"dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Dir == "" {
		http.Error(w, `"dir" is required`, http.StatusBadRequest)
		return
	}

	project, err := config.LoadProject(body.Dir)
	if err != nil {
		http.Error(w, "load error: "+err.Error(), http.StatusBadRequest)
		return
	}

	var warnings []string
	if err := validate.Project(project); err != nil {
		for _, line := range strings.Split(err.Error(), "\n") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
			if line != "" && !strings.HasPrefix(line, "validation") {
				warnings = append(warnings, line)
			}
		}
	}

	s.hub.Reload(context.Background(), project.Organization, project.Agents, project.Desks, project.Groups, project.Resources)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":   "loaded",
		"dir":      body.Dir,
		"desks":    len(project.Desks),
		"groups":   len(project.Groups),
		"warnings": warnings,
	})
}

func (s *Server) handleHumanInput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	deskID := strings.TrimPrefix(r.URL.Path, "/api/human/")
	if deskID == "" {
		http.Error(w, "desk ID required", http.StatusBadRequest)
		return
	}

	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Content == "" {
		http.Error(w, `"content" is required`, http.StatusBadRequest)
		return
	}

	if !s.hub.SubmitHumanInput(deskID, body.Content) {
		http.Error(w, "no waiting human desk: "+deskID, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
