package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadProject_SkipsRosterDir verifies that .roster/ subdirectories are not walked,
// so runtime files inside them are never parsed as config.
func TestLoadProject_SkipsRosterDir(t *testing.T) {
	dir := t.TempDir()

	// Write a valid organization config at the project root.
	validYAML := []byte("kind: organization\nid: my-org\nname: Test Org\n")
	if err := os.WriteFile(filepath.Join(dir, "organization.yaml"), validYAML, 0o644); err != nil {
		t.Fatal(err)
	}

	// Write an invalid YAML inside .roster/ — if the directory is not skipped,
	// LoadProject will return an error parsing it.
	rosterDir := filepath.Join(dir, ".roster")
	if err := os.Mkdir(rosterDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rosterDir, "state.yaml"), []byte("{{{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	proj, err := LoadProject(dir)
	if err != nil {
		t.Fatalf("LoadProject returned error (probably walked .roster/): %v", err)
	}
	if proj.Organization == nil || proj.Organization.ID != "my-org" {
		t.Errorf("expected organization 'my-org', got %v", proj.Organization)
	}
}

// TestLoadProject_SkipsNestedRosterDir verifies the skip applies to .roster/ at any depth.
func TestLoadProject_SkipsNestedRosterDir(t *testing.T) {
	dir := t.TempDir()

	sub := filepath.Join(dir, "subproject")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	rosterDir := filepath.Join(sub, ".roster")
	if err := os.Mkdir(rosterDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Unparseable file — would cause an error if walked.
	if err := os.WriteFile(filepath.Join(rosterDir, "run-123.yaml"), []byte("{{{"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadProject(dir)
	if err != nil {
		t.Fatalf("LoadProject walked nested .roster/: %v", err)
	}
}

// TestLoadProject_EmptyDir verifies an empty directory returns an empty project, not an error.
func TestLoadProject_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	proj, err := LoadProject(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proj.Organization != nil || len(proj.Agents) != 0 || len(proj.Desks) != 0 {
		t.Error("expected empty project for empty directory")
	}
}

// TestLoadProject_LoadsResource verifies resource kind is parsed.
func TestLoadProject_LoadsResource(t *testing.T) {
	dir := t.TempDir()
	yaml := []byte(`kind: resource
name: codebase
type: github
watch:
  - pull_request
  - issue
actions:
  commit:
    exec: scripts/commit.sh
`)
	if err := os.WriteFile(filepath.Join(dir, "codebase.yaml"), yaml, 0o644); err != nil {
		t.Fatal(err)
	}

	proj, err := LoadProject(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(proj.Resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(proj.Resources))
	}
	r := proj.Resources["codebase"]
	if r == nil || r.Type != "github" {
		t.Errorf("expected github resource, got %v", r)
	}
	if len(r.Watch) != 2 {
		t.Errorf("expected 2 watch events, got %d", len(r.Watch))
	}
}
