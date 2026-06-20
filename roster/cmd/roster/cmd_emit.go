package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

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
