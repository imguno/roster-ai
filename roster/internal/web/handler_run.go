package web

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/roster-io/roster/internal/observe"
)

var runIDGroupRe = regexp.MustCompile(`^(.+)-\d{8}-`)

type runSummary struct {
	RunID        string    `json:"run_id"`
	GroupID      string    `json:"group_id"`
	Desks        []string  `json:"desks"`
	Status       string    `json:"status"`
	StartedAt    time.Time `json:"started_at"`
	DurationMs   int64     `json:"total_step_ms"`
	InputTokens  int       `json:"input_tokens,omitempty"`
	OutputTokens int       `json:"output_tokens,omitempty"`
	TriggerType  string    `json:"trigger_type"`
}

type stepDetail struct {
	DeskID       string    `json:"desk_id"`
	Status       string    `json:"status"`
	StartedAt    time.Time `json:"started_at"`
	DurationMs   int64     `json:"duration_ms"`
	InputTokens  int       `json:"input_tokens,omitempty"`
	OutputTokens int       `json:"output_tokens,omitempty"`
	Model        string    `json:"model,omitempty"`
	Output       string    `json:"output,omitempty"`
	Error        string    `json:"error"`
}

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	events := s.hub.Events()
	runs := map[string]*runSummary{}
	deskSets := map[string]map[string]struct{}{}

	for _, ev := range events {
		rid := ev.RunID
		if rid == "" {
			continue
		}
		switch ev.Type {
		case observe.EventStepStarted:
			if _, ok := runs[rid]; !ok {
				gid := ""
				if m := runIDGroupRe.FindStringSubmatch(rid); len(m) == 2 {
					gid = m[1]
				}
				runs[rid] = &runSummary{RunID: rid, GroupID: gid, Status: "running", StartedAt: ev.At}
				deskSets[rid] = map[string]struct{}{}
			}
			entry := runs[rid]
			if ev.At.Before(entry.StartedAt) {
				entry.StartedAt = ev.At
			}
			if ev.StepID != "" {
				deskSets[rid][ev.StepID] = struct{}{}
			}
		case observe.EventStepCompleted:
			if entry, ok := runs[rid]; ok && entry.Status != "failed" {
				entry.Status = "completed"
				entry.DurationMs += ev.DurationMs
				entry.InputTokens += ev.InputTokens
				entry.OutputTokens += ev.OutputTokens
			}
		case observe.EventStepFailed:
			if entry, ok := runs[rid]; ok {
				entry.Status = "failed"
			}
		}
	}

	list := make([]*runSummary, 0, len(runs))
	for rid, entry := range runs {
		desks := make([]string, 0, len(deskSets[rid]))
		for d := range deskSets[rid] {
			desks = append(desks, d)
		}
		sort.Strings(desks)
		entry.Desks = desks
		list = append(list, entry)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].StartedAt.After(list[j].StartedAt) })
	if len(list) > 200 {
		list = list[:200]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (s *Server) handleRunSub(w http.ResponseWriter, r *http.Request) {
	tail := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	if tail == "" {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(tail, "/cancel") {
		s.handleRunCancel(w, r, strings.TrimSuffix(tail, "/cancel"))
		return
	}
	if strings.HasSuffix(tail, "/events") {
		s.handleRunEvents(w, r, strings.TrimSuffix(tail, "/events"))
		return
	}
	s.handleRunDetail(w, r, tail)
}

// handleRunEvents returns all observation events for a specific run in
// chronological order. This exposes the full group coordination flow
// (lead → member → lead) for debugging and audit purposes.
func (s *Server) handleRunEvents(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	if runID == "" || strings.Contains(runID, "/") {
		http.NotFound(w, r)
		return
	}

	var result []observe.Event
	for _, ev := range s.hub.Events() {
		if ev.RunID == runID {
			result = append(result, ev)
		}
	}

	if result == nil {
		http.NotFound(w, r)
		return
	}

	sort.Slice(result, func(i, j int) bool { return result[i].At.Before(result[j].At) })

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleRunCancel(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if runID == "" || strings.Contains(runID, "/") {
		http.NotFound(w, r)
		return
	}
	if !s.hub.CancelRun(runID) {
		runKnown := false
		for _, ev := range s.hub.Events() {
			if ev.RunID == runID {
				runKnown = true
				break
			}
		}
		if !runKnown {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"cancelled":false,"reason":"already_completed"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"cancelled":true}`))
}

func (s *Server) handleRunDetail(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	if runID == "" || strings.Contains(runID, "/") {
		http.NotFound(w, r)
		return
	}

	details := map[string]*stepDetail{}
	runKnown := false
	for _, ev := range s.hub.Events() {
		if ev.RunID != runID {
			continue
		}
		runKnown = true
		if ev.StepID == "" {
			continue
		}
		switch ev.Type {
		case observe.EventStepStarted:
			if _, ok := details[ev.StepID]; !ok {
				details[ev.StepID] = &stepDetail{DeskID: ev.StepID, Status: "running", StartedAt: ev.At}
			}
		case observe.EventStepCompleted:
			if d, ok := details[ev.StepID]; ok {
				d.Status = "completed"
				d.DurationMs = ev.DurationMs
				d.InputTokens = ev.InputTokens
				d.OutputTokens = ev.OutputTokens
				d.Model = ev.Model
				d.Output = ev.Output
			}
		case observe.EventStepFailed:
			if d, ok := details[ev.StepID]; ok {
				d.Status = "failed"
				d.Error = ev.Error
				d.DurationMs = ev.DurationMs
			}
		}
	}

	if !runKnown {
		http.NotFound(w, r)
		return
	}
	result := make([]*stepDetail, 0, len(details))
	for _, d := range details {
		result = append(result, d)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].StartedAt.Before(result[j].StartedAt) })

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
