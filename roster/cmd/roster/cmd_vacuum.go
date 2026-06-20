package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/roster-io/roster/internal/event/queue"
)

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
