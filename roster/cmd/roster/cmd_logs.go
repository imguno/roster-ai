package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/roster-io/roster/internal/store/observe"
)

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
			e.DeskID,
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
