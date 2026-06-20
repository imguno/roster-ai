package knowhow

import "testing"

func TestExtract_Found(t *testing.T) {
	output := `Some preamble.

## Knowhow
Always check error returns in Go.
Use sync.RWMutex for concurrent map access.

## Next Steps
Do something else.`

	kh := Extract(output)
	if kh == "" {
		t.Fatal("expected knowhow content")
	}
	if kh != "Always check error returns in Go.\nUse sync.RWMutex for concurrent map access." {
		t.Errorf("unexpected content: %q", kh)
	}
}

func TestExtract_NotFound(t *testing.T) {
	kh := Extract("No knowhow section here.")
	if kh != "" {
		t.Errorf("expected empty, got %q", kh)
	}
}

func TestExtract_AtEnd(t *testing.T) {
	output := "## Knowhow\nLearned something important."
	kh := Extract(output)
	if kh != "Learned something important." {
		t.Errorf("unexpected: %q", kh)
	}
}

func TestCap(t *testing.T) {
	items := make([]Knowhow, 5)
	for i := range items {
		items[i] = Knowhow{Content: string(rune('a' + i))}
	}

	capped := Cap(items, 3)
	if len(capped) != 3 {
		t.Fatalf("expected 3, got %d", len(capped))
	}
	if capped[0].Content != "c" {
		t.Errorf("expected most recent 3, first is %q", capped[0].Content)
	}
}

func TestCap_UnderLimit(t *testing.T) {
	items := []Knowhow{{Content: "a"}}
	capped := Cap(items, 10)
	if len(capped) != 1 {
		t.Errorf("expected 1, got %d", len(capped))
	}
}

func TestCap_ZeroLimit(t *testing.T) {
	items := []Knowhow{{Content: "a"}}
	capped := Cap(items, 0)
	if len(capped) != 1 {
		t.Errorf("zero limit should return all, got %d", len(capped))
	}
}
