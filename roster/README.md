# Roster (Go Module)

This directory contains the Roster framework source code.

```
cmd/roster/    CLI entrypoint
pkg/types/     Domain models (pure types, no external deps)
pkg/sdk/       Port interfaces (Executor, Task, Trigger)
internal/      Implementation (hub, runners, web dashboard, etc.)
proto/         gRPC worker protocol
```

## Build

```bash
go build -o roster ./cmd/roster
```

## Test

```bash
go test ./...
```

## Module

```
github.com/roster-io/roster
```

See the [root README](../README.md) for full documentation.
