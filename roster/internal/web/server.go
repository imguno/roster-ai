package web

import (
	"context"
	"io"
	"net/http"

	"github.com/roster-io/roster/internal/store/observe"
	"github.com/roster-io/roster/internal/store"
	"github.com/roster-io/roster/pkg/types"
)

// HubAPI is the interface the web server needs from the hub.
type HubAPI interface {
	Events() []observe.Event
	Subscribe() (chan observe.Event, func())
	Emit(ctx context.Context, ev types.Event)
	Reload(ctx context.Context, org *types.Organization, agents map[string]*types.Agent, desks map[string]*types.Desk, groups map[string]*types.Group, resources map[string]*types.Resource)
	SubmitHumanInput(deskID, content string) bool
	DeskSession(deskID string) ([]store.SessionEntry, bool)
	DeskLogs(deskID string) []store.LogEntry
	Desks() map[string]*types.Desk
	Groups() map[string]*types.Group
	Resources() map[string]*types.Resource
	Organization() *types.Organization
	QueueStatus() map[string]int
	Warnings() []types.Warning
	CancelRun(runID string) bool
	RecordMetrics(deskID string, metrics map[string]float64)
	GetMetrics(deskID string) map[string]map[string]float64
	BudgetStatus() map[string]float64
	SDKReady() bool
}

// Server is the hub management web UI and REST API.
type Server struct {
	hub        HubAPI
	mux        *http.ServeMux
	projectDir string
}

func New(h HubAPI, projectDir string) *Server {
	s := &Server{hub: h, mux: http.NewServeMux(), projectDir: projectDir}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/organization", s.handleOrganization)
	s.mux.HandleFunc("/api/desks", s.handleDesks)
	s.mux.HandleFunc("/api/desks/", s.handleDeskSub)
	s.mux.HandleFunc("/api/groups", s.handleGroups)
	s.mux.HandleFunc("/api/resources", s.handleResources)
	s.mux.HandleFunc("/api/events", s.handleEvents)
	s.mux.HandleFunc("/api/stream", s.handleStream)
	s.mux.HandleFunc("/api/queues", s.handleQueues)
	s.mux.HandleFunc("/api/human/", s.handleHumanInput)
	s.mux.HandleFunc("/api/warnings", s.handleWarnings)
	s.mux.HandleFunc("/api/load", s.handleLoad)
	s.mux.HandleFunc("/api/runs", s.handleRuns)
	s.mux.HandleFunc("/api/runs/", s.handleRunSub)
	s.mux.HandleFunc("/api/metrics", s.handleMetrics)
	s.mux.HandleFunc("/api/budget", s.handleBudget)
	s.mux.HandleFunc("/api/version", s.handleVersion)
	s.mux.HandleFunc("/api/ping", s.handlePing)
	s.mux.HandleFunc("/readiness", s.handleReadiness)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/ping", s.handlePing)
	s.mux.HandleFunc("/webhooks/", s.handleWebhook)
	s.mux.Handle("/static/", http.FileServer(http.FS(staticFS)))
	s.mux.HandleFunc("/", s.handleUI)
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pong"))
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"version":"0.1.0"}`))
}

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.hub.SDKReady() {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready"}`))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"not_ready"}`))
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	f, err := staticFS.Open("static/index.html")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.Copy(w, f)
}
