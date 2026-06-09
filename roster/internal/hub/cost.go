package hub

import (
	"strconv"
	"strings"
)

// parseCostLimit parses a cost string like "$0.10" to a float64.
func parseCostLimit(s string) float64 {
	s = strings.TrimPrefix(s, "$")
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// estimateCost calculates approximate cost based on model and token counts.
// Prices are approximate and may not reflect current pricing.
func estimateCost(model string, inputTokens, outputTokens int) float64 {
	// Price per million tokens (input, output).
	type pricing struct{ input, output float64 }
	prices := map[string]pricing{
		"claude-sonnet-4-5": {3.0, 15.0},
		"claude-sonnet-4-6": {3.0, 15.0},
		"claude-opus-4-5":   {15.0, 75.0},
		"claude-opus-4-6":   {15.0, 75.0},
		"claude-haiku-3-5":  {0.80, 4.0},
		"gpt-4o":            {2.50, 10.0},
		"gpt-4o-mini":       {0.15, 0.60},
		"gemini-1.5-pro":    {1.25, 5.0},
		"gemini-2.0-flash":  {0.10, 0.40},
	}
	p, ok := prices[model]
	if !ok {
		p = pricing{3.0, 15.0} // default to mid-range
	}
	return (float64(inputTokens)*p.input + float64(outputTokens)*p.output) / 1_000_000
}
