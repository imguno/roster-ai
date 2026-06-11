package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/roster-io/roster/internal/config"
	"github.com/roster-io/roster/internal/exec/desk"
	"github.com/roster-io/roster/internal/hub"
	"github.com/roster-io/roster/internal/store/observe"
	"github.com/roster-io/roster/internal/event/queue"
	"github.com/roster-io/roster/internal/exec/runner"
	runnerapi "github.com/roster-io/roster/internal/exec/runner/api"
	"github.com/roster-io/roster/internal/agent/skill"
	"github.com/roster-io/roster/internal/store/state"
	"github.com/roster-io/roster/internal/validate"
	"github.com/roster-io/roster/internal/web"
	"github.com/roster-io/roster/pkg/types"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	switch os.Args[1] {
	case "hub":
		dir, uiAddr, storeBackend, pythonBin, nodeBin := ".", ":8080", "", "", ""
		for i := 2; i < len(os.Args); i++ {
			switch os.Args[i] {
			case "--dir":
				if i+1 < len(os.Args) {
					dir = os.Args[i+1]
					i++
				}
			case "--ui":
				if i+1 < len(os.Args) {
					uiAddr = os.Args[i+1]
					i++
				}
			case "--store":
				if i+1 < len(os.Args) {
					storeBackend = os.Args[i+1]
					i++
				}
			case "--python":
				if i+1 < len(os.Args) {
					pythonBin = os.Args[i+1]
					i++
				}
			case "--node":
				if i+1 < len(os.Args) {
					nodeBin = os.Args[i+1]
					i++
				}
			}
		}
		if err := startHub(ctx, dir, uiAddr, storeBackend, pythonBin, nodeBin); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	case "worker":
		addr := ":50051"
		if len(os.Args) >= 3 {
			addr = os.Args[2]
		}
		if err := startWorker(ctx, addr); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	case "emit":
		if err := runEmit(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	case "logs":
		if err := runLogs(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	case "load":
		if err := runLoad(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	case "summarize":
		if err := runSummarize(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	case "vacuum":
		if err := runVacuum(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	case "status":
		if err := runStatus(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	case "dry-run":
		if err := runDryRun(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	case "runs":
		if err := runRuns(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	case "init":
		if err := runInit(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func startHub(ctx context.Context, dir, uiAddr, storeBackend, pythonBin, nodeBin string) error {
	// --- Phase 1: Hub infrastructure (always succeeds) ---
	dataDir := filepath.Join(dir, ".roster", "data")

	storeCfg := types.StoreConfig{}
	if storeBackend != "" {
		storeCfg.Backend = storeBackend
	}

	store, err := state.NewStore(storeCfg, dir)
	if err != nil {
		return fmt.Errorf("state store: %w", err)
	}

	logFile := filepath.Join(dataDir, "events.jsonl")
	recorder, err := observe.NewFileRecorder(logFile)
	if err != nil {
		return fmt.Errorf("recorder: %w", err)
	}
	defer recorder.Close()

	// Build executor registry (independent of project config).
	inner := runner.NewRegistry()
	inner.Register(types.ExecutorTypeAPI, runnerapi.New())
	inner.Register(types.ExecutorTypeExec, runner.NewExecRunner())
	inner.Register(types.ExecutorTypeDocker, runner.NewDockerRunner())
	inner.Register(types.ExecutorTypeRemote, runner.NewRemoteRunner(""))
	reg := runner.NewConcurrentRegistry(inner)

	resolver := skill.NewResolver(dir)
	h := hub.New(reg, store, resolver, recorder)
	h.SetProjectDir(dir)
	h.SetQueueDir(dataDir)
	if pythonBin != "" {
		h.SetSDKPython(pythonBin)
	}
	if nodeBin != "" {
		h.SetSDKNode(nodeBin)
	}

	// --- Phase 2: Load project (non-fatal — hub starts regardless) ---
	orgName := "(empty hub)"
	var validationWarnings []string
	var project *config.Project

	loaded, loadErr := config.LoadProject(dir)
	if loadErr != nil {
		validationWarnings = append(validationWarnings, fmt.Sprintf("project load: %v", loadErr))
	} else {
		project = loaded

		// Validation: collect warnings but don't stop.
		if err := validate.Project(project); err != nil {
			msg := err.Error()
			// Strip the "validation failed:\n" prefix, keep individual lines.
			if idx := strings.Index(msg, "\n"); idx >= 0 {
				for _, line := range strings.Split(msg[idx+1:], "\n") {
					line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
					if line != "" {
						validationWarnings = append(validationWarnings, line)
					}
				}
			}
		}

		// Re-create store if org config specifies a different backend.
		if project.Organization != nil && project.Organization.Store.Backend != "" && storeBackend == "" {
			if newStore, err := state.NewStore(project.Organization.Store, dir); err == nil {
				store = newStore
				// Rebuild hub with new store.
				h = hub.New(reg, store, resolver, recorder)
				h.SetProjectDir(dir)
				h.SetQueueDir(dataDir)
			}
			storeCfg = project.Organization.Store
		}

		// Load config into hub (even with validation warnings — partial config is fine).
		h.Load(project.Organization, project.Agents, project.Desks, project.Groups, project.Resources, project.Policies)

		// Configure per-desk concurrency.
		for id, d := range project.Desks {
			if d.Concurrency.Mode != "" {
				reg.ConfigureDesk(id, d.Concurrency)
			}
		}

		if project.Organization != nil {
			orgName = project.Organization.Name
			if orgName == "" {
				orgName = project.Organization.ID
			}
		}
	}

	// --- Phase 3: Start hub (always) ---
	if err := h.Start(ctx); err != nil {
		return fmt.Errorf("hub start: %w", err)
	}

	// If this startup follows a binary upgrade, emit upgrade.done.
	// The previous process wrote this marker before syscall.Exec.
	upgradeDoneMarker := filepath.Join(dataDir, "upgrade-done")
	if _, err := os.Stat(upgradeDoneMarker); err == nil {
		if removeErr := os.Remove(upgradeDoneMarker); removeErr == nil {
			fmt.Println("post-upgrade startup detected — emitting upgrade.done")
			h.Emit(ctx, types.Event{Type: "upgrade.done", Source: "hub"})
		}
	}

	// Merge validation warnings + skill warnings for banner.
	var allWarnings []types.Warning
	for _, msg := range validationWarnings {
		allWarnings = append(allWarnings, types.Warning{Level: "warn", Source: "validate", Message: msg})
	}
	allWarnings = append(allWarnings, h.Warnings()...)

	backendName := storeCfg.Backend
	if backendName == "" {
		backendName = "file"
	}
	printBanner(orgName, uiAddr, dataDir, backendName, project, allWarnings)

	ui := web.New(h, dir)
	srv := &http.Server{Addr: uiAddr, Handler: ui}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, "web server:", err)
		}
	}()

	// Watch project directory for config changes and hot-reload.
	go watchAndReload(ctx, dir, h)

	// Watch for restart marker written by upgrade.sh.
	// When found, gracefully shut down the HTTP server, then replace
	// this process with a fresh hub using the new binary (syscall.Exec).
	restartMarker := filepath.Join(dataDir, "restart-requested")
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			if _, err := os.Stat(restartMarker); err != nil {
				continue
			}
			if err := os.Remove(restartMarker); err != nil {
				fmt.Fprintln(os.Stderr, "restart: could not remove marker:", err)
				continue
			}
			fmt.Println("restart marker detected — shutting down for binary replace")

			// Graceful shutdown: release the port before exec.
			shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			srv.Shutdown(shutCtx)
			cancel()

			// Leave a marker so the new process knows to emit upgrade.done.
			upgradeDoneMarker := filepath.Join(dataDir, "upgrade-done")
			if err := os.WriteFile(upgradeDoneMarker, []byte(time.Now().Format(time.RFC3339)), 0644); err != nil {
				fmt.Fprintln(os.Stderr, "restart: could not write upgrade-done marker:", err)
			}

			self, err := os.Executable()
			if err != nil {
				fmt.Fprintln(os.Stderr, "restart: could not resolve executable:", err)
				continue
			}
			fmt.Println("exec:", self, os.Args)
			if err := syscall.Exec(self, os.Args, os.Environ()); err != nil {
				fmt.Fprintln(os.Stderr, "restart: exec failed:", err)
			}
		}
	}()

	<-ctx.Done()
	fmt.Println("\n  Shutting down…")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutCancel()
	return srv.Shutdown(shutCtx)
}

func startWorker(ctx context.Context, addr string) error {
	reg := runner.NewRegistry()
	reg.Register(types.ExecutorTypeAPI, runnerapi.New())
	reg.Register(types.ExecutorTypeExec, runner.NewExecRunner())
	reg.Register(types.ExecutorTypeDocker, runner.NewDockerRunner())
	s := desk.NewServer(reg)
	fmt.Printf("roster worker listening on %s\n", addr)
	go func() {
		<-ctx.Done()
		s.Stop()
	}()
	return s.Listen(addr)
}

// runEmit sends an event to the running hub via the API.
// Usage: roster emit <event-type> [payload] [--hub URL]
func runEmit(args []string) error {
	hubURL := "http://localhost:8080"
	var positional []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--hub" && i+1 < len(args) {
			hubURL = args[i+1]
			i++
		} else {
			positional = append(positional, args[i])
		}
	}
	if len(positional) == 0 {
		return fmt.Errorf("usage: roster emit <event-type> [payload] [--hub URL]")
	}
	eventType := positional[0]
	payload := ""
	if len(positional) > 1 {
		payload = strings.Join(positional[1:], " ")
	}

	// Payload must be base64-encoded because types.Event.Payload is []byte.
	encodedPayload := base64.StdEncoding.EncodeToString([]byte(payload))
	body := fmt.Sprintf(`{"type":%q,"payload":%q}`, eventType, encodedPayload)
	resp, err := http.Post(hubURL+"/api/events", "application/json", strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("connect to hub at %s: %w", hubURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hub returned %d", resp.StatusCode)
	}
	fmt.Printf("Event %q emitted.\n", eventType)
	return nil
}

// runLogs prints events from the JSONL log file.
// Usage: roster logs [--dir .] [--type <eventType>] [--follow] [--output]
func runLogs(args []string) error {
	dir, typeFilter := ".", ""
	follow, showOutput := false, false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dir":
			if i+1 < len(args) {
				dir = args[i+1]
				i++
			}
		case "--type":
			if i+1 < len(args) {
				typeFilter = args[i+1]
				i++
			}
		case "--follow", "-f":
			follow = true
		case "--output", "-o":
			showOutput = true
		}
	}

	logFile := filepath.Join(dir, ".roster", "data", "events.jsonl")
	return tailLog(logFile, typeFilter, follow, showOutput)
}

func tailLog(logFile, typeFilter string, follow, showOutput bool) error {
	printEvent := func(e observe.Event) {
		if typeFilter != "" && string(e.Type) != typeFilter {
			return
		}
		dur := ""
		if e.DurationMs > 0 {
			dur = fmt.Sprintf("  %dms", e.DurationMs)
		}
		model := ""
		if e.Model != "" {
			model = "  " + e.Model
		}
		out := ""
		if e.OutputBytes > 0 {
			out = fmt.Sprintf("  %dB out", e.OutputBytes)
		}
		errStr := ""
		if e.Error != "" {
			errStr = "  ERROR: " + e.Error
		}
		fmt.Printf("%s  %-25s  %-20s%s%s%s%s\n",
			e.At.Format(time.RFC3339),
			e.StepID,
			string(e.Type),
			model, dur, out, errStr,
		)
		if showOutput && e.Output != "" {
			fmt.Printf("  │ %s\n", strings.ReplaceAll(strings.TrimSpace(e.Output), "\n", "\n  │ "))
		}
	}

	data, err := os.ReadFile(logFile)
	if os.IsNotExist(err) {
		if !follow {
			fmt.Println("no log file found:", logFile)
		}
	} else if err != nil {
		return err
	} else {
		for _, line := range splitLines(data) {
			var e observe.Event
			if json.Unmarshal(line, &e) == nil {
				printEvent(e)
			}
		}
	}

	if !follow {
		return nil
	}

	// Follow mode: poll for new lines.
	offset := int64(len(data))
	for {
		time.Sleep(500 * time.Millisecond)
		f, err := os.Open(logFile)
		if err != nil {
			continue
		}
		fi, _ := f.Stat()
		if fi.Size() <= offset {
			f.Close()
			continue
		}
		buf := make([]byte, fi.Size()-offset)
		f.ReadAt(buf, offset)
		f.Close()
		offset += int64(len(buf))
		for _, line := range splitLines(buf) {
			var e observe.Event
			if json.Unmarshal(line, &e) == nil {
				printEvent(e)
			}
		}
	}
}

func watchAndReload(ctx context.Context, dir string, h *hub.Hub) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintln(os.Stderr, "hot-reload: watcher init failed:", err)
		return
	}
	defer watcher.Close()

	// Load initial project to get the exact set of config files to watch.
	watchedFiles := map[string]struct{}{}
	registerFiles := func(project *config.Project) {
		for _, f := range project.SourceFiles {
			if _, ok := watchedFiles[f]; ok {
				continue
			}
			watchedFiles[f] = struct{}{}
			watcher.Add(f) //nolint:errcheck
		}
	}
	if initial, err := config.LoadProject(dir); err == nil {
		registerFiles(initial)
	}

	var debounce <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-watcher.Events:
			if !ok {
				return
			}
			if _, watched := watchedFiles[ev.Name]; !watched {
				continue
			}
			debounce = time.After(500 * time.Millisecond)
		case <-watcher.Errors:
			continue
		case <-debounce:
			debounce = nil
			project, err := config.LoadProject(dir)
			if err != nil {
				fmt.Fprintln(os.Stderr, "hot-reload: config error:", err)
				continue
			}
			if err := validate.Project(project); err != nil {
				fmt.Fprintln(os.Stderr, "hot-reload: validation error:", err)
				continue
			}
			// Register any newly added config files.
			registerFiles(project)
			h.Reload(ctx, project.Organization, project.Agents, project.Desks, project.Groups, project.Resources, project.Policies)
			fmt.Println("  ↺ Config reloaded")
		}
	}
}



func printUsage() {
	fmt.Println("Usage: roster <command>")
	fmt.Println("  init [dir] [--name N] [--template T]")
	fmt.Println("                                Create organization (templates: product-team, content-pipeline, code-review)")
	fmt.Println("  dry-run [dir]                 Validate config and simulate routing")
	fmt.Println("  hub [--dir .] [--ui :8080] [--store file|sqlite|memory]")
	fmt.Println("                                Start hub server (loads --dir if provided)")
	fmt.Println("  load <dir> [--hub URL]        Load organization into running hub")
	fmt.Println("  worker [addr]                 Start remote worker (default: :50051)")
	fmt.Println("  emit <event-type> [payload] [--hub URL]")
	fmt.Println("                                Emit an event to the running hub")
	fmt.Println("  runs [--dir .] [--n 20] [--output]")
	fmt.Println("                                Show recent runs grouped by execution")
	fmt.Println("  logs [--dir .] [--type TYPE] [--follow] [--output]")
	fmt.Println("                                Query execution logs (--output shows desk responses)")
	fmt.Println("  summarize [--dir .] [--desk ID|--all]")
	fmt.Println("                                Compact session history")
	fmt.Println("  status [--hub URL]            Show hub status")
	fmt.Println("  vacuum [--dir .] [--keep 7d]  Clean up old data")
}

func runStatus(args []string) error {
	hubURL := "http://localhost:8080"
	for i := 0; i < len(args); i++ {
		if args[i] == "--hub" && i+1 < len(args) {
			hubURL = args[i+1]
			i++
		}
	}

	// Fetch org, desks, groups, queues, warnings in parallel
	type result struct {
		org      map[string]any
		desks    map[string]any
		groups   map[string]any
		queues   map[string]float64
		warnings []any
		events   []map[string]any
	}

	var r result
	var fetchErr error

	fetch := func(path string, target any) {
		resp, err := http.Get(hubURL + path)
		if err != nil {
			fetchErr = fmt.Errorf("connect to hub at %s: %w", hubURL, err)
			return
		}
		defer resp.Body.Close()
		json.NewDecoder(resp.Body).Decode(target)
	}

	fetch("/api/organization", &r.org)
	if fetchErr != nil {
		return fetchErr
	}
	fetch("/api/desks", &r.desks)
	fetch("/api/groups", &r.groups)
	fetch("/api/queues", &r.queues)
	fetch("/api/warnings", &r.warnings)
	fetch("/api/events", &r.events)

	// Organization
	orgName := "—"
	if name, ok := r.org["name"].(string); ok && name != "" {
		orgName = name
	}
	fmt.Printf("Organization: %s\n", orgName)
	fmt.Printf("Desks: %d  Groups: %d\n", len(r.desks), len(r.groups))
	fmt.Println()

	// Active work — scan events for current desk states
	deskStates := map[string]string{} // deskID → status
	for _, ev := range r.events {
		id, _ := ev["step_id"].(string)
		t, _ := ev["type"].(string)
		if id == "" {
			continue
		}
		switch t {
		case "step.started":
			deskStates[id] = "working"
		case "step.completed":
			deskStates[id] = "idle"
		case "step.failed":
			deskStates[id] = "error"
		case "human.waiting":
			deskStates[id] = "human"
		}
	}

	working := []string{}
	errors := []string{}
	human := []string{}
	for id, st := range deskStates {
		switch st {
		case "working":
			working = append(working, id)
		case "error":
			errors = append(errors, id)
		case "human":
			human = append(human, id)
		}
	}

	if len(working) > 0 {
		fmt.Printf("Working (%d):\n", len(working))
		for _, id := range working {
			fmt.Printf("  ⏳ %s\n", id)
		}
	}
	if len(human) > 0 {
		fmt.Printf("Waiting for human (%d):\n", len(human))
		for _, id := range human {
			fmt.Printf("  👤 %s\n", id)
		}
	}
	if len(errors) > 0 {
		fmt.Printf("Errors (%d):\n", len(errors))
		for _, id := range errors {
			fmt.Printf("  ✗ %s\n", id)
		}
	}
	if len(working) == 0 && len(human) == 0 && len(errors) == 0 {
		fmt.Println("All idle.")
	}

	// Queue depth
	totalQueued := 0.0
	for _, n := range r.queues {
		totalQueued += n
	}
	if totalQueued > 0 {
		fmt.Printf("\nQueued events: %.0f\n", totalQueued)
		for id, n := range r.queues {
			if n > 0 {
				fmt.Printf("  %s: %.0f pending\n", id, n)
			}
		}
	}

	// Warnings
	if len(r.warnings) > 0 {
		fmt.Printf("\nWarnings (%d):\n", len(r.warnings))
		for _, w := range r.warnings {
			if wm, ok := w.(map[string]any); ok {
				fmt.Printf("  ⚠ %v\n", wm["message"])
			}
		}
	}

	return nil
}

// runRuns shows recent runs grouped by run ID.
// Usage: roster runs [--dir .] [--n 20] [--output]
func runRuns(args []string) error {
	dir, n, showOutput := ".", 20, false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dir":
			if i+1 < len(args) {
				dir = args[i+1]
				i++
			}
		case "--n":
			if i+1 < len(args) {
				if v, err := strconv.Atoi(args[i+1]); err == nil {
					n = v
				}
				i++
			}
		case "--output", "-o":
			showOutput = true
		}
	}

	logFile := filepath.Join(dir, ".roster", "data", "events.jsonl")
	data, err := os.ReadFile(logFile)
	if os.IsNotExist(err) {
		fmt.Println("no log file found:", logFile)
		return nil
	} else if err != nil {
		return err
	}

	type runInfo struct {
		id      string
		group   string
		startAt time.Time
		endAt   time.Time
		status  string // started, completed, failed, skipped
		output  string
		desks   []string
		deskSet map[string]struct{}
	}

	// Parse events and group by RunID.
	runs := map[string]*runInfo{}
	var order []string // insertion order (first seen)
	for _, line := range splitLines(data) {
		var e observe.Event
		if json.Unmarshal(line, &e) != nil || e.RunID == "" {
			continue
		}
		r, ok := runs[e.RunID]
		if !ok {
			r = &runInfo{id: e.RunID, status: "started", deskSet: map[string]struct{}{}}
			runs[e.RunID] = r
			order = append(order, e.RunID)

			// Extract group from run ID prefix (e.g. "strategy-team-20260609-…")
			// Run IDs follow: <prefix>-YYYYMMDD-HHMMSS-<hex>
			// Everything before the date segment is the group/desk name.
			if idx := findRunIDDateIndex(e.RunID); idx > 0 {
				r.group = e.RunID[:idx-1]
			} else {
				r.group = e.StepID
			}
		}
		if e.At.Before(r.startAt) || r.startAt.IsZero() {
			r.startAt = e.At
		}
		if e.At.After(r.endAt) {
			r.endAt = e.At
		}
		switch e.Type {
		case observe.EventStepCompleted:
			r.status = "completed"
			if e.Output != "" {
				r.output = e.Output
			}
		case observe.EventStepFailed:
			r.status = "failed"
		case observe.EventStepSkipped:
			if r.status == "started" {
				r.status = "skipped"
			}
		}
		if e.StepID != "" {
			if _, seen := r.deskSet[e.StepID]; !seen {
				r.deskSet[e.StepID] = struct{}{}
				r.desks = append(r.desks, e.StepID)
			}
		}
	}

	// Show the last n runs (most recent first).
	start := len(order) - n
	if start < 0 {
		start = 0
	}
	subset := order[start:]
	// Print most-recent first.
	for i := len(subset) - 1; i >= 0; i-- {
		r := runs[subset[i]]
		dur := ""
		if !r.endAt.IsZero() && !r.startAt.IsZero() {
			d := r.endAt.Sub(r.startAt).Round(time.Millisecond)
			if d >= time.Minute {
				dur = fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
			} else {
				dur = fmt.Sprintf("%.1fs", d.Seconds())
			}
		}
		statusSymbol := map[string]string{
			"completed": "✓",
			"failed":    "✗",
			"skipped":   "~",
			"started":   "⏳",
		}[r.status]
		if statusSymbol == "" {
			statusSymbol = "?"
		}
		groupStr := r.group
		if len(r.desks) > 0 && r.group != r.desks[0] {
			groupStr = r.group
		}
		fmt.Printf("%s  %s  %-30s  %-10s  %s\n",
			r.startAt.Format("2006-01-02 15:04:05"),
			statusSymbol,
			truncate(groupStr, 30),
			r.status,
			dur,
		)
		if showOutput && r.output != "" {
			lines := strings.SplitN(strings.TrimSpace(r.output), "\n", 6)
			for j, l := range lines {
				if j == 5 {
					fmt.Printf("  │ …\n")
					break
				}
				fmt.Printf("  │ %s\n", l)
			}
		}
	}
	if len(order) == 0 {
		fmt.Println("no runs found")
	}
	return nil
}

// findRunIDDateIndex returns the index of the date segment (YYYYMMDD) in a run ID,
// or -1 if not found. This lets us strip the date suffix to get the group/desk prefix.
func findRunIDDateIndex(runID string) int {
	// Date segment is 8 digits: YYYYMMDD
	for i := 0; i+8 <= len(runID); i++ {
		allDigits := true
		for _, c := range runID[i : i+8] {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return i
		}
	}
	return -1
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func runDryRun(args []string) error {
	dir := "."
	for i := 0; i < len(args); i++ {
		if args[i] == "--dir" && i+1 < len(args) {
			dir = args[i+1]
			i++
		} else if !strings.HasPrefix(args[i], "-") {
			dir = args[i]
		}
	}

	fmt.Println("Dry-run: validating", dir)
	fmt.Println()

	// 1. Load config
	project, err := config.LoadProject(dir)
	if err != nil {
		return fmt.Errorf("config load failed: %w", err)
	}
	fmt.Printf("  ✓ Config loaded: %d desks, %d groups, %d resources, %d policies\n",
		len(project.Desks), len(project.Groups), len(project.Resources), len(project.Policies))

	// 2. Validate
	if err := validate.Project(project); err != nil {
		fmt.Printf("  ✗ Validation errors:\n")
		for _, line := range strings.Split(err.Error(), "\n") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
			if line != "" && !strings.HasPrefix(line, "validation") {
				fmt.Printf("      %s\n", line)
			}
		}
	} else {
		fmt.Println("  ✓ Validation passed")
	}

	// 3. Check skill resolution
	resolver := skill.NewResolver(dir)
	ctx := context.Background()
	skillIssues := 0
	for id, agent := range project.Agents {
		for _, ref := range agent.Skills {
			if _, err := resolver.Resolve(ctx, ref); err != nil {
				fmt.Printf("  ⚠ Agent %q: skill %q not found\n", id, ref)
				skillIssues++
			}
		}
	}
	if skillIssues == 0 {
		fmt.Println("  ✓ All skills resolved")
	}

	// 4. Simulate event flow (subscribe/emit based)
	{
		fmt.Println()
		fmt.Println("  Event flow:")

		// Build emit map: event → emitters
		emitMap := map[string][]string{}
		for id, g := range project.Groups {
			for _, ev := range g.Emit {
				emitMap[ev] = append(emitMap[ev], id)
			}
		}
		for id, d := range project.Desks {
			for _, ev := range d.Emit {
				emitMap[ev] = append(emitMap[ev], id)
			}
		}

		// Build subscribe map: event → subscribers
		subMap := map[string][]string{}
		for id, g := range project.Groups {
			for _, ev := range g.Subscribe {
				subMap[ev] = append(subMap[ev], id)
			}
		}
		for id, d := range project.Desks {
			for _, ev := range d.Subscribe {
				subMap[ev] = append(subMap[ev], id)
			}
		}

		// Print connections
		allEvents := map[string]bool{}
		for ev := range emitMap {
			allEvents[ev] = true
		}
		for ev := range subMap {
			allEvents[ev] = true
		}
		evList := make([]string, 0, len(allEvents))
		for ev := range allEvents {
			evList = append(evList, ev)
		}
		sort.Strings(evList)
		for _, ev := range evList {
			emitters := emitMap[ev]
			subs := subMap[ev]
			emStr := "(external)"
			if len(emitters) > 0 {
				emStr = strings.Join(emitters, ", ")
			}
			subStr := "(none)"
			if len(subs) > 0 {
				subStr = strings.Join(subs, ", ")
			}
			if len(subs) == 0 {
				fmt.Printf("    ⚠ [%s] emitted by %s — no subscribers\n", ev, emStr)
			} else {
				fmt.Printf("    [%s]: %s → %s\n", ev, emStr, subStr)
			}
		}
	}

	// 5. Check executor connectivity (basic)
	fmt.Println()
	for id, desk := range project.Desks {
		switch desk.Executor.Type {
		case "exec":
			cmd := desk.Executor.Params["command"]
			if cmd == "" {
				fmt.Printf("  ✗ Desk %q: exec type but no command\n", id)
			}
		case "api":
			if desk.Executor.SDK == "" {
				fmt.Printf("  ✗ Desk %q: api type but no sdk\n", id)
			}
		case "remote":
			addr := desk.Executor.Params["address"]
			if addr == "" {
				fmt.Printf("  ✗ Desk %q: remote type but no address\n", id)
			}
		}
	}

	// 6. Summary
	fmt.Println()
	cronCount := 0
	triggerCount := 0
	for _, d := range project.Desks {
		if d.Cron != "" {
			cronCount++
		}
		triggerCount += len(d.Triggers)
	}
	for _, g := range project.Groups {
		if g.Cron != "" {
			cronCount++
		}
		triggerCount += len(g.Triggers)
	}
	fmt.Printf("  Summary: %d desks, %d groups, %d cron schedules, %d triggers\n",
		len(project.Desks), len(project.Groups), cronCount, triggerCount)
	fmt.Println("  ✓ Dry-run complete")

	return nil
}

func runInit(args []string) error {
	dir := "."
	name := ""
	template := ""
	hasName := false
	hasTemplate := false
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		dir = args[0]
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			if i+1 < len(args) {
				name = args[i+1]
				hasName = true
				i++
			}
		case "--template":
			if i+1 < len(args) {
				template = args[i+1]
				hasTemplate = true
				i++
			}
		}
	}

	// Interactive mode: prompt when stdin is a terminal and flags were not provided.
	if (!hasName || !hasTemplate) && isTTY() {
		fmt.Println()
		if !hasName {
			name = promptWithDefault("Organization name", "my-org")
		}
		if !hasTemplate {
			fmt.Println()
			fmt.Println("  Available templates:")
			fmt.Println("    minimal          — single API desk, ready to run (default)")
			fmt.Println("    product-team     — strategy → dev → review → ops")
			fmt.Println("    content-pipeline — research → writing → editorial")
			fmt.Println("    code-review      — parallel security + quality review")
			fmt.Println()
			choice := promptWithDefault("Template", "minimal")
			if choice != "minimal" {
				template = choice
				hasTemplate = true
			}
		}
		fmt.Println()
	}

	// Always prompt for API key in interactive mode — without it the default
	// template won't run at all.
	if isTTY() {
		fmt.Print("  Anthropic API key (press Enter to skip): ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			apiKey := strings.TrimSpace(scanner.Text())
			if apiKey != "" {
				_ = os.MkdirAll(dir, 0750)
				envPath := filepath.Join(dir, ".env")
				if _, err := os.Stat(envPath); os.IsNotExist(err) {
					os.WriteFile(envPath, []byte("ANTHROPIC_API_KEY="+apiKey+"\n"), 0600)
				}
				// Ensure .env is excluded from version control.
				gitignorePath := filepath.Join(dir, ".gitignore")
				existing, _ := os.ReadFile(gitignorePath)
				if !strings.Contains(string(existing), ".env") {
					f, ferr := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
					if ferr == nil {
						fmt.Fprintln(f, ".env")
						f.Close()
					}
				}
			}
		}
		fmt.Println()
	}

	if !hasName && name == "" {
		name = "my-org"
	}

	switch template {
	case "":
		return initDefault(dir, name)
	case "product-team":
		return initProductTeam(dir, name)
	case "content-pipeline":
		return initContentPipeline(dir, name)
	case "code-review":
		return initCodeReview(dir, name)
	default:
		return fmt.Errorf("unknown template %q (available: product-team, content-pipeline, code-review)", template)
	}
}

// isTTY returns true if stdin is an interactive terminal.
func isTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// promptWithDefault prints a prompt and reads one line, returning defaultVal if empty.
func promptWithDefault(label, defaultVal string) string {
	fmt.Printf("  %s [%s]: ", label, defaultVal)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			return line
		}
	}
	return defaultVal
}

func initDefault(dir, name string) error {
	// Create directory structure
	dirs := []string{
		dir,
		filepath.Join(dir, "desks"),
		filepath.Join(dir, "agents"),
		filepath.Join(dir, "groups"),
		filepath.Join(dir, "skills"),
		filepath.Join(dir, "policies"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0750); err != nil {
			return err
		}
	}

	// organization.yaml
	writeIfNotExists(filepath.Join(dir, "organization.yaml"), `kind: organization
name: `+name+`

groups:
  - dev-team

routing:
  - on: task.created
    to: dev-team
`)

	// agents/developer.yaml
	writeIfNotExists(filepath.Join(dir, "agents", "developer.yaml"), `kind: agent
name: developer

skills:
  - coding
`)

	// skills/coding.md
	writeIfNotExists(filepath.Join(dir, "skills", "coding.md"), `You are a skilled software developer. Given a task, you:
- Write clean, idiomatic code with clear naming
- Explain your reasoning briefly before the code
- Keep solutions simple — the minimum needed, no more
- Point out any assumptions or edge cases worth noting
`)

	// desks/developer.yaml — uses Claude API directly
	writeIfNotExists(filepath.Join(dir, "desks", "developer.yaml"), `kind: desk
name: developer

agent: developer

executor:
  type: api
  sdk: anthropic
  params:
    model: claude-haiku-4-5-20251001
    api_key: ${ANTHROPIC_API_KEY}

policy: standard
`)

	// groups/dev-team.yaml
	writeIfNotExists(filepath.Join(dir, "groups", "dev-team.yaml"), `kind: group
name: dev-team

desks:
  - developer

subscribe:
  - task.created

emit:
  - task.completed
`)

	// policies/standard.yaml
	writeIfNotExists(filepath.Join(dir, "policies", "standard.yaml"), `kind: policy
name: standard

retry: 1
timeout: 2m
`)

	// .gitignore for .roster
	writeIfNotExists(filepath.Join(dir, ".gitignore"), ".roster/\n")

	printInitResult(name, dir, "")
	return nil
}

func initProductTeam(dir, name string) error {
	dirs := []string{dir, filepath.Join(dir, "desks"), filepath.Join(dir, "agents"),
		filepath.Join(dir, "groups"), filepath.Join(dir, "skills"), filepath.Join(dir, "policies"), filepath.Join(dir, "scripts")}
	for _, d := range dirs {
		os.MkdirAll(d, 0750)
	}

	writeIfNotExists(filepath.Join(dir, "organization.yaml"),
		`kind: organization
name: `+name+`

groups:
  - strategy-team
  - dev-team
  - review-squad
  - ops-team

routing:
  - on: plan.approved
    to: dev-team
  - on: code.ready
    to: review-squad
  - on: review.approved
    to: ops-team
  - on: review.rejected
    to: dev-team
    when: "changes requested"
  - on: build.failed
    to: dev-team
  - on: test.failed
    to: dev-team
`)

	// Agents
	writeIfNotExists(filepath.Join(dir, "agents", "architect.yaml"),
		`kind: agent
name: architect
skills:
  - product-strategy
  - system-design
`)
	writeIfNotExists(filepath.Join(dir, "agents", "developer.yaml"),
		`kind: agent
name: developer
skills:
  - coding
`)
	writeIfNotExists(filepath.Join(dir, "agents", "reviewer.yaml"),
		`kind: agent
name: reviewer
skills:
  - code-review
`)

	// Skills
	writeIfNotExists(filepath.Join(dir, "skills", "product-strategy.md"),
		`You are a product strategist. Analyze requirements, identify priorities, and create actionable implementation plans. Focus on user value and technical feasibility.`)
	writeIfNotExists(filepath.Join(dir, "skills", "system-design.md"),
		`You are a system architect. Design clean, maintainable solutions. Consider scalability, error handling, and separation of concerns.`)
	writeIfNotExists(filepath.Join(dir, "skills", "coding.md"),
		`You are a skilled developer. Write clean, well-tested code. Follow best practices and keep solutions simple.`)
	writeIfNotExists(filepath.Join(dir, "skills", "code-review.md"),
		`You are a code reviewer. Check for correctness, security, performance, and maintainability. Be specific and actionable in feedback. Reply with "APPROVED" or "CHANGES REQUESTED: <reason>".`)

	// Desks
	writeIfNotExists(filepath.Join(dir, "desks", "architect.yaml"),
		`kind: desk
name: architect
agent: architect
executor:
  type: exec
  params:
    command: echo "implement your executor here"
`)
	writeIfNotExists(filepath.Join(dir, "desks", "developer.yaml"),
		`kind: desk
name: developer
agent: developer
executor:
  type: exec
  params:
    command: echo "implement your executor here"
`)
	writeIfNotExists(filepath.Join(dir, "desks", "reviewer.yaml"),
		`kind: desk
name: reviewer
agent: reviewer
executor:
  type: exec
  params:
    command: echo "implement your executor here"
`)
	writeIfNotExists(filepath.Join(dir, "desks", "builder.yaml"),
		`kind: desk
name: builder
executor:
  type: exec
  params:
    command: echo "build script here"
emit:
  - build.succeeded
  - build.failed
`)
	writeIfNotExists(filepath.Join(dir, "desks", "tester.yaml"),
		`kind: desk
name: tester
executor:
  type: exec
  params:
    command: echo "test script here"
emit:
  - test.passed
  - test.failed
`)

	// Groups
	writeIfNotExists(filepath.Join(dir, "groups", "strategy-team.yaml"),
		`kind: group
name: strategy-team
lead:
  desk: architect
  position: first
desks: []
subscribe:
  - hub.started
emit:
  - plan.approved
cron: "0 */6 * * *"
`)
	writeIfNotExists(filepath.Join(dir, "groups", "dev-team.yaml"),
		`kind: group
name: dev-team
lead:
  desk: developer
desks: []
subscribe:
  - plan.approved
  - build.failed
  - test.failed
emit:
  - code.ready
`)
	writeIfNotExists(filepath.Join(dir, "groups", "review-squad.yaml"),
		`kind: group
name: review-squad
desks:
  - reviewer
subscribe:
  - code.ready
emit:
  - review.approved
  - review.rejected
`)
	writeIfNotExists(filepath.Join(dir, "groups", "ops-team.yaml"),
		`kind: group
name: ops-team
desks:
  - builder
  - tester
subscribe:
  - review.approved
emit:
  - build.succeeded
  - test.passed
`)

	// Policies
	writeIfNotExists(filepath.Join(dir, "policies", "standard.yaml"),
		`kind: policy
name: standard
retry: 2
timeout: 10m
`)

	writeIfNotExists(filepath.Join(dir, ".gitignore"), ".roster/\n")

	printInitResult(name, dir, "product-team")
	return nil
}

func initContentPipeline(dir, name string) error {
	dirs := []string{dir, filepath.Join(dir, "desks"), filepath.Join(dir, "agents"),
		filepath.Join(dir, "groups"), filepath.Join(dir, "skills"), filepath.Join(dir, "policies")}
	for _, d := range dirs {
		os.MkdirAll(d, 0750)
	}

	writeIfNotExists(filepath.Join(dir, "organization.yaml"),
		`kind: organization
name: `+name+`

groups:
  - research-team
  - writing-team
  - editorial

routing:
  - on: research.done
    to: writing-team
  - on: draft.ready
    to: editorial
  - on: revision.needed
    to: writing-team
`)

	writeIfNotExists(filepath.Join(dir, "agents", "researcher.yaml"),
		`kind: agent
name: researcher
skills:
  - research
`)
	writeIfNotExists(filepath.Join(dir, "agents", "writer.yaml"),
		`kind: agent
name: writer
skills:
  - writing
`)
	writeIfNotExists(filepath.Join(dir, "agents", "editor.yaml"),
		`kind: agent
name: editor
skills:
  - editing
`)

	writeIfNotExists(filepath.Join(dir, "skills", "research.md"),
		`You are a researcher. Gather relevant information, identify key facts, and organize findings into a clear brief.`)
	writeIfNotExists(filepath.Join(dir, "skills", "writing.md"),
		`You are a professional writer. Create clear, engaging content based on research briefs. Match the target audience and tone.`)
	writeIfNotExists(filepath.Join(dir, "skills", "editing.md"),
		`You are an editor. Review content for clarity, accuracy, grammar, and style. Reply with "APPROVED" or "REVISION NEEDED: <feedback>".`)

	writeIfNotExists(filepath.Join(dir, "desks", "researcher.yaml"),
		`kind: desk
name: researcher
agent: researcher
executor:
  type: exec
  params:
    command: echo "implement research executor"
`)
	writeIfNotExists(filepath.Join(dir, "desks", "writer.yaml"),
		`kind: desk
name: writer
agent: writer
executor:
  type: exec
  params:
    command: echo "implement writing executor"
`)
	writeIfNotExists(filepath.Join(dir, "desks", "editor.yaml"),
		`kind: desk
name: editor
agent: editor
executor:
  type: exec
  params:
    command: echo "implement editor executor"
`)

	writeIfNotExists(filepath.Join(dir, "groups", "research-team.yaml"),
		`kind: group
name: research-team
desks:
  - researcher
subscribe:
  - hub.started
emit:
  - research.done
`)
	writeIfNotExists(filepath.Join(dir, "groups", "writing-team.yaml"),
		`kind: group
name: writing-team
desks:
  - writer
subscribe:
  - research.done
  - revision.needed
emit:
  - draft.ready
`)
	writeIfNotExists(filepath.Join(dir, "groups", "editorial.yaml"),
		`kind: group
name: editorial
desks:
  - editor
subscribe:
  - draft.ready
emit:
  - content.published
  - revision.needed
`)

	writeIfNotExists(filepath.Join(dir, ".gitignore"), ".roster/\n")

	printInitResult(name, dir, "content-pipeline")
	return nil
}

func initCodeReview(dir, name string) error {
	dirs := []string{dir, filepath.Join(dir, "desks"), filepath.Join(dir, "agents"),
		filepath.Join(dir, "groups"), filepath.Join(dir, "skills")}
	for _, d := range dirs {
		os.MkdirAll(d, 0750)
	}

	writeIfNotExists(filepath.Join(dir, "organization.yaml"),
		`kind: organization
name: `+name+`

groups:
  - review-team

routing:
  - on: code.submitted
    to: review-team
`)

	writeIfNotExists(filepath.Join(dir, "agents", "security-reviewer.yaml"),
		`kind: agent
name: security-reviewer
skills:
  - security-review
`)
	writeIfNotExists(filepath.Join(dir, "agents", "quality-reviewer.yaml"),
		`kind: agent
name: quality-reviewer
skills:
  - quality-review
`)

	writeIfNotExists(filepath.Join(dir, "skills", "security-review.md"),
		`You are a security reviewer. Check code for vulnerabilities: injection, XSS, auth bypass, secrets exposure, OWASP top 10. Be specific about the risk and fix.`)
	writeIfNotExists(filepath.Join(dir, "skills", "quality-review.md"),
		`You are a code quality reviewer. Check for readability, maintainability, test coverage, error handling, and performance. Suggest concrete improvements.`)

	writeIfNotExists(filepath.Join(dir, "desks", "security-reviewer.yaml"),
		`kind: desk
name: security-reviewer
agent: security-reviewer
executor:
  type: exec
  params:
    command: echo "implement security review executor"
`)
	writeIfNotExists(filepath.Join(dir, "desks", "quality-reviewer.yaml"),
		`kind: desk
name: quality-reviewer
agent: quality-reviewer
executor:
  type: exec
  params:
    command: echo "implement quality review executor"
`)

	writeIfNotExists(filepath.Join(dir, "groups", "review-team.yaml"),
		`kind: group
name: review-team
desks:
  - security-reviewer
  - quality-reviewer
dispatch: parallel
subscribe:
  - code.submitted
emit:
  - review.completed
`)

	writeIfNotExists(filepath.Join(dir, ".gitignore"), ".roster/\n")

	printInitResult(name, dir, "code-review")
	return nil
}

func printInitResult(name, dir, template string) {
	fmt.Printf("Initialized %q organization in %s", name, dir)
	if template != "" {
		fmt.Printf(" (template: %s)", template)
	}
	fmt.Println()
	fmt.Println()
	fmt.Println("  Next steps:")
	fmt.Printf("    cd %s\n", dir)
	// Show the right API key step depending on whether .env was written.
	if _, err := os.Stat(filepath.Join(dir, ".env")); err == nil {
		fmt.Println("    source .env")
	} else if template == "" {
		fmt.Println("    export ANTHROPIC_API_KEY=<your-key>")
	}
	fmt.Println("    roster dry-run .")
	fmt.Println("    roster hub --ui :8080")
	fmt.Println("    roster emit task.created '{\"task\": \"write a hello world in Go\"}'")
	fmt.Println("    roster logs --follow")
	fmt.Println()
}

func writeIfNotExists(path, content string) {
	if _, err := os.Stat(path); err == nil {
		return // file exists, don't overwrite
	}
	os.WriteFile(path, []byte(content), 0640)
}

func printBanner(orgName, uiAddr, dataDir, storeBackend string, project *config.Project, warnings []types.Warning) {
	const logo = `
  ██████╗  ██████╗ ███████╗████████╗███████╗██████╗
  ██╔══██╗██╔═══██╗██╔════╝╚══██╔══╝██╔════╝██╔══██╗
  ██████╔╝██║   ██║███████╗   ██║   █████╗  ██████╔╝
  ██╔══██╗██║   ██║╚════██║   ██║   ██╔══╝  ██╔══██╗
  ██║  ██║╚██████╔╝███████║   ██║   ███████╗██║  ██║
  ╚═╝  ╚═╝ ╚═════╝ ╚══════╝   ╚═╝   ╚══════╝╚═╝  ╚═╝`

	fmt.Println(logo)
	fmt.Println("  Organization as Code")
	fmt.Println()

	// Group names, truncated if many
	groupsStr := "(none)"
	desksCount := 0
	resourcesCount := 0
	if project != nil {
		groupNames := make([]string, 0, len(project.Groups))
		for id := range project.Groups {
			groupNames = append(groupNames, id)
		}
		groupsStr = strings.Join(groupNames, ", ")
		if len(groupNames) > 6 {
			groupsStr = strings.Join(groupNames[:6], ", ") + fmt.Sprintf(", +%d more", len(groupNames)-6)
		}
		if len(groupNames) == 0 {
			groupsStr = "(none)"
		}
		desksCount = len(project.Desks)
		resourcesCount = len(project.Resources)
	}

	fmt.Printf("  %-14s %s\n", "Organization:", orgName)
	fmt.Printf("  %-14s %s\n", "Groups:", groupsStr)
	fmt.Printf("  %-14s %d loaded\n", "Desks:", desksCount)
	fmt.Printf("  %-14s %d connected\n", "Resources:", resourcesCount)
	fmt.Printf("  %-14s %s\n", "Store:", storeBackend)

	if len(warnings) == 0 {
		fmt.Printf("  %-14s %d\n", "Warnings:", 0)
	} else {
		fmt.Printf("  %-14s \033[33m%d\033[0m\n", "Warnings:", len(warnings))
		for _, w := range warnings {
			fmt.Printf("    \033[33m⚠\033[0m  %s\n", w.Message)
		}
	}

	fmt.Println()

	dashURL := "http://localhost" + uiAddr
	fmt.Printf("  %-14s %s\n", "Dashboard:", dashURL)
	fmt.Printf("  %-14s %s\n", "Queue dir:", filepath.Join(dataDir, "queues"))
	fmt.Println()
	fmt.Println("  ✓ Hub started — listening for events")
	fmt.Println()
}

// runLoad sends a load request to a running hub via the API.
// Usage: roster load --dir ./my-org [--hub http://localhost:8080]
func runLoad(args []string) error {
	dir := ""
	hubURL := "http://localhost:8080"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dir":
			if i+1 < len(args) {
				dir = args[i+1]
				i++
			}
		case "--hub":
			if i+1 < len(args) {
				hubURL = args[i+1]
				i++
			}
		default:
			if dir == "" {
				dir = args[i] // positional arg
			}
		}
	}
	if dir == "" {
		return fmt.Errorf("usage: roster load <dir> or roster load --dir <dir>")
	}

	body := fmt.Sprintf(`{"dir":%q}`, dir)
	resp, err := http.Post(hubURL+"/api/load", "application/json", strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("connect to hub at %s: %w", hubURL, err)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	if resp.StatusCode != http.StatusOK {
		if msg, ok := result["error"]; ok {
			return fmt.Errorf("hub: %v", msg)
		}
		return fmt.Errorf("hub returned %d", resp.StatusCode)
	}

	fmt.Printf("Loaded %s into hub:\n", dir)
	if d, ok := result["desks"]; ok {
		fmt.Printf("  Desks:  %.0f\n", d)
	}
	if g, ok := result["groups"]; ok {
		fmt.Printf("  Groups: %.0f\n", g)
	}
	if w, ok := result["warnings"]; ok {
		if ws, ok := w.([]any); ok && len(ws) > 0 {
			fmt.Printf("  Warnings: %d\n", len(ws))
			for _, ww := range ws {
				fmt.Printf("    ⚠ %v\n", ww)
			}
		}
	}
	return nil
}

func runVacuum(args []string) error {
	dir := "."
	keepStr := "7d"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dir":
			if i+1 < len(args) {
				dir = args[i+1]
				i++
			}
		case "--keep":
			if i+1 < len(args) {
				keepStr = args[i+1]
				i++
			}
		}
	}

	retention, err := parseDuration(keepStr)
	if err != nil {
		return fmt.Errorf("invalid --keep value: %w", err)
	}

	dataDir := filepath.Join(dir, ".roster", "data")
	cutoff := time.Now().Add(-retention)

	// 1. Queue GC
	queueDir := filepath.Join(dataDir, "queues")
	entries, _ := os.ReadDir(queueDir)
	totalQueueCleaned := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		subID := strings.TrimSuffix(e.Name(), ".jsonl")
		q, err := queue.NewQueue(dataDir, subID)
		if err != nil {
			continue
		}
		totalQueueCleaned += q.GC(retention)
	}

	// 2. Run checkpoint cleanup
	runDir := filepath.Join(dataDir, "runs")
	totalRunsCleaned := 0
	runEntries, _ := os.ReadDir(runDir)
	for _, e := range runEntries {
		if !e.IsDir() {
			continue
		}
		info, _ := e.Info()
		if info != nil && info.ModTime().Before(cutoff) {
			os.RemoveAll(filepath.Join(runDir, e.Name()))
			totalRunsCleaned++
		}
	}

	// 3. Session file cleanup (keep summary.md, remove old run files)
	sessionDir := filepath.Join(dataDir, "sessions")
	totalSessionsCleaned := 0
	deskEntries, _ := os.ReadDir(sessionDir)
	for _, deskEntry := range deskEntries {
		if !deskEntry.IsDir() {
			continue
		}
		deskPath := filepath.Join(sessionDir, deskEntry.Name())
		runFiles, _ := os.ReadDir(deskPath)
		for _, rf := range runFiles {
			if rf.Name() == "summary.md" {
				continue
			}
			info, _ := rf.Info()
			if info != nil && info.ModTime().Before(cutoff) {
				os.Remove(filepath.Join(deskPath, rf.Name()))
				totalSessionsCleaned++
			}
		}
	}

	fmt.Printf("Vacuum complete:\n")
	fmt.Printf("  Queue entries removed: %d\n", totalQueueCleaned)
	fmt.Printf("  Run checkpoints removed: %d\n", totalRunsCleaned)
	fmt.Printf("  Session files removed: %d\n", totalSessionsCleaned)
	return nil
}

// runSummarize compacts a desk's session history into a summary.
// Usage: roster summarize [--dir .] [--desk <id>] [--all]
func runSummarize(args []string) error {
	dir := "."
	deskID := ""
	all := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dir":
			if i+1 < len(args) {
				dir = args[i+1]
				i++
			}
		case "--desk":
			if i+1 < len(args) {
				deskID = args[i+1]
				i++
			}
		case "--all":
			all = true
		}
	}

	if deskID == "" && !all {
		return fmt.Errorf("usage: roster summarize --desk <id> or --all")
	}

	dataDir := filepath.Join(dir, ".roster", "data")
	store, err := state.NewFileStore(dataDir)
	if err != nil {
		return fmt.Errorf("state store: %w", err)
	}

	summarizeDesk := func(id string) {
		entries := store.DeskSession().Load(id)
		if len(entries) == 0 {
			fmt.Printf("  %s: no session data\n", id)
			return
		}

		// Build a simple extractive summary: keep first and last entries,
		// count total, list roles involved.
		var roles []string
		roleSet := map[string]bool{}
		for _, e := range entries {
			if !roleSet[e.Role] {
				roleSet[e.Role] = true
				roles = append(roles, e.Role)
			}
		}

		summary := fmt.Sprintf("Session summary: %d entries from %s to %s. Roles: %s.\n\nMost recent exchange:\n\n",
			len(entries),
			entries[0].At.Format(time.RFC3339),
			entries[len(entries)-1].At.Format(time.RFC3339),
			strings.Join(roles, ", "),
		)

		// Keep last 4 entries as context.
		start := len(entries) - 4
		if start < 0 {
			start = 0
		}
		for _, e := range entries[start:] {
			summary += fmt.Sprintf("## %s\n%s\n\n", e.Role, e.Content)
		}

		store.DeskSession().Summarize(id, summary)
		fmt.Printf("  %s: %d entries → summarized\n", id, len(entries))
	}

	if all {
		// Summarize all desks that have session data.
		sessionDir := filepath.Join(dataDir, "sessions")
		entries, err := os.ReadDir(sessionDir)
		if err != nil {
			return fmt.Errorf("no session data found")
		}
		fmt.Println("Summarizing all desks:")
		for _, e := range entries {
			if e.IsDir() {
				summarizeDesk(e.Name())
			}
		}
	} else {
		fmt.Println("Summarizing desk:")
		summarizeDesk(deskID)
	}

	return nil
}

func parseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
