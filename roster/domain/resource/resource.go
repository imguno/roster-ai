// Package resource contains domain logic for resource management.
// No external dependencies — pure Go types and functions.
package resource

import "time"

// WatchType classifies how a resource should be monitored.
type WatchType string

const (
	WatchNone  WatchType = ""
	WatchLocal WatchType = "local" // fsnotify
	WatchSDK   WatchType = "sdk"   // gRPC stream from SDK process
	WatchMCP   WatchType = "mcp"   // gRPC stream via MCP bridge in SDK
)

// NeedsWatch returns true if the resource has watch configuration.
func NeedsWatch(watchPatterns []string, configPath string) bool {
	return len(watchPatterns) > 0 || configPath != ""
}

// ClassifyWatch determines the watch strategy based on resource type.
func ClassifyWatch(resourceType string) WatchType {
	switch resourceType {
	case "mcp":
		return WatchMCP
	case "sdk":
		return WatchSDK
	case "", "local":
		return WatchLocal
	default:
		return WatchLocal
	}
}

// EventType returns the canonical event type for a resource change.
// Format: "resource.{resourceID}.changed"
func EventType(resourceID string) string {
	return "resource." + resourceID + ".changed"
}

// ChangeEvent represents a detected change in a resource.
type ChangeEvent struct {
	ResourceID string
	ChangeType string // "file_modified", "file_created", "content_updated"
	Path       string // affected file path (for local resources)
	At         time.Time
}
