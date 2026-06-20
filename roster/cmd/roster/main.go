package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	switch os.Args[1] {
	case "hub":
		dir, uiAddr, storeBackend, pythonBin, nodeBin, sdkPort := ".", ":8080", "", "", "", 0
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
			case "--sdk-port":
				if i+1 < len(os.Args) {
					if v, err := strconv.Atoi(os.Args[i+1]); err == nil {
						sdkPort = v
					}
					i++
				}
			}
		}
		if err := startHub(ctx, dir, uiAddr, storeBackend, pythonBin, nodeBin, sdkPort); err != nil {
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

	case "help", "--help", "-h":
		printUsage()

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: roster <command>")
	fmt.Println("  init [dir] [--name N] [--template T]")
	fmt.Println("                                Create organization (templates: product-team, content-pipeline, code-review)")
	fmt.Println("  dry-run [dir]                 Validate config and simulate routing")
	fmt.Println("  hub [--dir .] [--ui :8080] [--store file|sqlite|memory] [--sdk-port auto]")
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
