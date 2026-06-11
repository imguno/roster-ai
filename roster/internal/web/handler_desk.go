package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/roster-io/roster/internal/store/observe"
)

func (s *Server) handleDesks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.hub.Desks())
}

func (s *Server) handleDeskSub(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/desks/"), "/")
	if r.Method != http.MethodGet || len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	deskID := parts[0]
	if deskID == "" {
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "profile":
		s.handleDeskProfile(w, r, deskID)
	case "artifact":
		content, found := s.hub.DeskArtifact(deskID)
		if !found {
			http.NotFound(w, r)
			return
		}
		if content == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(content))
	case "session":
		entries, found := s.hub.DeskSession(deskID)
		if !found {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entries)
	case "executor-file":
		desks := s.hub.Desks()
		desk, ok := desks[deskID]
		if !ok {
			http.NotFound(w, r)
			return
		}
		cmd := desk.Executor.Params["command"]
		if cmd == "" {
			http.NotFound(w, r)
			return
		}
		path := filepath.Join(s.projectDir, cmd)
		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(s.projectDir)) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(data)
	default:
		http.NotFound(w, r)
	}
}

// parseWindow converts a window string ("1h", "24h", "7d", "30d") to a duration.
// Returns 0 if the string is empty or unrecognized (meaning: no filter / all-time).
func parseWindow(s string) time.Duration {
	switch s {
	case "1h":
		return time.Hour
	case "6h":
		return 6 * time.Hour
	case "24h":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	case "30d":
		return 30 * 24 * time.Hour
	}
	return 0
}

func (s *Server) handleDeskProfile(w http.ResponseWriter, r *http.Request, deskID string) {
	events := s.hub.Events()

	// Optional time window filter: ?window=1h|6h|24h|7d|30d
	window := parseWindow(r.URL.Query().Get("window"))
	var cutoff time.Time
	if window > 0 {
		cutoff = time.Now().Add(-window)
	}

	type profile struct {
		DeskID            string         `json:"desk_id"`
		Window            string         `json:"window,omitempty"`
		TotalRuns         int            `json:"total_runs"`
		SuccessRate       float64        `json:"success_rate"`
		SkipRate          float64        `json:"skip_rate"`
		AvgDurationMs     int64          `json:"avg_duration_ms"`
		TotalInputTokens  int            `json:"total_input_tokens"`
		TotalOutputTokens int            `json:"total_output_tokens"`
		EstimatedCost     float64        `json:"estimated_cost"`
		ErrorCount        int            `json:"error_count"`
		LastRun           string         `json:"last_run,omitempty"`
		ModelsUsed        map[string]int `json:"models_used,omitempty"`
	}

	p := profile{DeskID: deskID, ModelsUsed: map[string]int{}}
	if windowParam := r.URL.Query().Get("window"); windowParam != "" && window > 0 {
		p.Window = windowParam
	}
	var totalDuration int64
	var skipCount float64

	for _, ev := range events {
		if ev.StepID != deskID {
			continue
		}
		if !cutoff.IsZero() && ev.At.Before(cutoff) {
			continue
		}

		switch ev.Type {
		case observe.EventStepStarted:
			p.TotalRuns++
			if p.LastRun == "" || ev.At.Format(time.RFC3339) > p.LastRun {
				p.LastRun = ev.At.Format(time.RFC3339)
			}
		case observe.EventStepCompleted:
			totalDuration += ev.DurationMs
			p.TotalInputTokens += ev.InputTokens
			p.TotalOutputTokens += ev.OutputTokens
			if ev.Model != "" {
				p.ModelsUsed[ev.Model]++
			}
		case observe.EventStepFailed:
			p.ErrorCount++
		case observe.EventStepSkipped:
			skipCount++
		}
	}

	if p.TotalRuns > 0 {
		completed := p.TotalRuns - p.ErrorCount - int(skipCount)
		if completed < 0 {
			completed = 0
		}
		p.SuccessRate = float64(completed) / float64(p.TotalRuns)
		p.SkipRate = skipCount / float64(p.TotalRuns)
		if completed > 0 {
			p.AvgDurationMs = totalDuration / int64(completed)
		}
	}

	// Rough cost estimate
	p.EstimatedCost = (float64(p.TotalInputTokens)*3.0 + float64(p.TotalOutputTokens)*15.0) / 1_000_000

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}
