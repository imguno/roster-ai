package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/roster-io/roster/internal/config"
	"github.com/roster-io/roster/internal/hub"
	runnerapi "github.com/roster-io/roster/internal/exec/runner/api"
	"github.com/roster-io/roster/internal/agent/skill"
	"github.com/roster-io/roster/internal/exec/runner"
	"github.com/roster-io/roster/internal/store/factory"
	"github.com/roster-io/roster/internal/store/observe"
	"github.com/roster-io/roster/internal/validate"
	"github.com/roster-io/roster/internal/web"
	"github.com/roster-io/roster/pkg/types"
)

func startHub(ctx context.Context, dir, uiAddr, storeBackend, pythonBin, nodeBin string, sdkPort int) error {
	// --- Phase 1: Hub infrastructure (always succeeds) ---
	dataDir := filepath.Join(dir, ".roster", "data")

	storeCfg := types.StoreConfig{}
	if storeBackend != "" {
		storeCfg.Backend = storeBackend
	}

	store, err := factory.New(storeCfg, dir)
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
	if sdkPort > 0 {
		h.SetSDKPort(sdkPort)
	}
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
			if newStore, err := factory.New(project.Organization.Store, dir); err == nil {
				store = newStore
				// Rebuild hub with new store.
				h = hub.New(reg, store, resolver, recorder)
				h.SetProjectDir(dir)
				h.SetQueueDir(dataDir)
				if sdkPort > 0 {
					h.SetSDKPort(sdkPort)
				}
				if pythonBin != "" {
					h.SetSDKPython(pythonBin)
				}
				if nodeBin != "" {
					h.SetSDKNode(nodeBin)
				}
			}
			storeCfg = project.Organization.Store
		}

		// Load config into hub (even with validation warnings — partial config is fine).
		h.Load(project.Organization, project.Agents, project.Desks, project.Groups, project.Resources)

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

	// Open dashboard in browser.
	dashURL := "http://localhost" + uiAddr
	openBrowser(dashURL)

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
			h.Reload(ctx, project.Organization, project.Agents, project.Desks, project.Groups, project.Resources)
			fmt.Println("  ↺ Config reloaded")
		}
	}
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
