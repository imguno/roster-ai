package hub

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os/exec"
	"time"

	"github.com/roster-io/roster/internal/store/observe"
	"github.com/roster-io/roster/pkg/types"
)

// startTrigger starts a background goroutine for an exec or poll trigger.
func (h *Hub) startTrigger(ctx context.Context, subscriberID string, trig types.TriggerConfig) {
	interval := parseTriggerInterval(trig.Interval)
	if interval < 10*time.Second {
		interval = 30 * time.Second
	}
	eventType := trig.Event
	if eventType == "" {
		eventType = subscriberID + ".triggered"
	}

	h.ensureQueue(subscriberID)
	h.recorder.Record(observe.Event{StepID: subscriberID, Type: observe.EventType("trigger." + trig.Type + ".registered")})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fired, payload := checkTrigger(ctx, trig)
				if fired {
					h.Emit(ctx, types.Event{
						Type:    eventType,
						Source:  "trigger:" + subscriberID,
						Payload: payload,
					})
				}
			}
		}
	}()
}

func checkTrigger(ctx context.Context, trig types.TriggerConfig) (bool, []byte) {
	switch trig.Type {
	case "exec":
		cmd := exec.CommandContext(ctx, "sh", "-c", trig.Command)
		out, err := cmd.Output()
		if err != nil {
			return false, nil
		}
		return true, bytes.TrimSpace(out)
	case "poll":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, trig.URL, nil)
		if err != nil {
			return false, nil
		}
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return false, nil
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return true, body
		}
		return false, nil
	default:
		return false, nil
	}
}

func parseTriggerInterval(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}
