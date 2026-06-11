package web

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/roster-io/roster/internal/store/observe"
)

// handleMetrics handles both reporting and querying metrics.
//
//	POST /api/metrics — report metrics: {"desk":"builder","metrics":{"tokens":1234,"cost":0.05}}
//	GET  /api/metrics — query aggregated metrics per desk
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	switch r.Method {
	case http.MethodPost:
		s.postMetrics(w, r)
	case http.MethodGet:
		s.getMetrics(w, r)
	case http.MethodOptions:
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "GET or POST only", http.StatusMethodNotAllowed)
	}
}

func (s *Server) postMetrics(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Desk    string             `json:"desk"`
		RunID   string             `json:"run_id"`
		Metrics map[string]float64 `json:"metrics"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Desk == "" {
		http.Error(w, `"desk" is required`, http.StatusBadRequest)
		return
	}
	if len(body.Metrics) == 0 {
		http.Error(w, `"metrics" must contain at least one key`, http.StatusBadRequest)
		return
	}
	s.hub.RecordMetrics(body.Desk, body.Metrics)
	w.WriteHeader(http.StatusAccepted)
}

// deskMetricsSummary holds aggregated metrics for one desk.
type deskMetricsSummary struct {
	DeskID  string             `json:"desk_id"`
	Runs    int                `json:"runs"`
	Totals  map[string]float64 `json:"totals"`
}

func (s *Server) getMetrics(w http.ResponseWriter, r *http.Request) {
	filterDesk := r.URL.Query().Get("desk")
	events := s.hub.Events()

	// Aggregate metrics per desk from all events that carry them.
	agg := map[string]*deskMetricsSummary{}
	for _, ev := range events {
		id := ev.StepID
		if id == "" {
			continue
		}
		if filterDesk != "" && id != filterDesk {
			continue
		}

		// Count runs.
		if ev.Type == observe.EventStepCompleted {
			if _, ok := agg[id]; !ok {
				agg[id] = &deskMetricsSummary{DeskID: id, Totals: map[string]float64{}}
			}
			agg[id].Runs++
			// Built-in token metrics.
			if ev.InputTokens > 0 {
				agg[id].Totals["input_tokens"] += float64(ev.InputTokens)
			}
			if ev.OutputTokens > 0 {
				agg[id].Totals["output_tokens"] += float64(ev.OutputTokens)
			}
		}

		// Custom metrics from any event.
		if len(ev.Metrics) > 0 {
			if _, ok := agg[id]; !ok {
				agg[id] = &deskMetricsSummary{DeskID: id, Totals: map[string]float64{}}
			}
			for k, v := range ev.Metrics {
				agg[id].Totals[k] += v
			}
		}
	}

	result := make([]*deskMetricsSummary, 0, len(agg))
	for _, s := range agg {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].DeskID < result[j].DeskID })

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
