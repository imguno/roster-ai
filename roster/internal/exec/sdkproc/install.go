package sdkproc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/roster-io/roster/pkg/types"
)

// sdkRef holds a parsed sdk: field value.
type sdkRef struct {
	prefix string // "pip", "npm", "git", "local"
	pkg    string // package name, git URL, or local path
	pyVer  string // optional Python version, pip/local only
	gitRef string // optional git ref, git only
}

// parseRef parses sdk: field values like "pip:roster-sdk", "local:./path", etc.
func parseRef(s string) (sdkRef, error) {
	idx := strings.Index(s, ":")
	if idx < 0 {
		return sdkRef{}, fmt.Errorf("sdk: invalid format %q (expected prefix:value)", s)
	}
	prefix := s[:idx]
	rest := s[idx+1:]
	ref := sdkRef{prefix: prefix}

	switch prefix {
	case "pip":
		if at := strings.LastIndex(rest, "@"); at >= 0 {
			ref.pkg = rest[:at]
			ref.pyVer = rest[at+1:]
		} else {
			ref.pkg = rest
		}
	case "npm":
		ref.pkg = rest
	case "git":
		if at := strings.LastIndex(rest, "@"); at >= 0 {
			candidate := rest[at+1:]
			if !strings.Contains(candidate, "/") && !strings.Contains(candidate, ".") {
				ref.pkg = rest[:at]
				ref.gitRef = candidate
			} else {
				ref.pkg = rest
			}
		} else {
			ref.pkg = rest
		}
	case "local":
		if at := strings.LastIndex(rest, "@"); at >= 0 {
			candidate := rest[at+1:]
			if len(candidate) > 0 && candidate[0] >= '0' && candidate[0] <= '9' {
				ref.pkg = rest[:at]
				ref.pyVer = candidate
			} else {
				ref.pkg = rest
			}
		} else {
			ref.pkg = rest
		}
	default:
		return sdkRef{}, fmt.Errorf("sdk: unknown prefix %q (use pip:, npm:, git:, or local:)", prefix)
	}
	return ref, nil
}

// collectRefs scans agents and resources and returns unique sdkRefs.
func collectRefs(agents map[string]*types.Agent, resources map[string]*types.Resource) []sdkRef {
	seen := map[string]bool{}
	var refs []sdkRef
	add := func(sdk string) {
		if sdk == "" || seen[sdk] {
			return
		}
		seen[sdk] = true
		ref, err := parseRef(sdk)
		if err != nil {
			return
		}
		refs = append(refs, ref)
	}
	for _, a := range agents {
		add(a.SDK)
	}
	return refs
}

// resolvePath joins projectDir and a possibly-relative path.
func resolvePath(projectDir, path string) string {
	if projectDir != "" && !filepath.IsAbs(path) {
		return filepath.Join(projectDir, path)
	}
	return path
}

// resolvePythonBin finds Python >= 3.11 by trying versioned names first,
// then falling back to python3/python.
func resolvePythonBin(ver string) string {
	if ver != "" {
		if path, err := exec.LookPath("python" + ver); err == nil {
			return path
		}
	}
	// Prefer higher versions that meet the >=3.11 requirement.
	for _, name := range []string{"python3.13", "python3.12", "python3.11", "python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return "python"
}

func resolveNodeBin() string {
	if path, err := exec.LookPath("node"); err == nil {
		return path
	}
	return "node"
}

// ensureVenv creates a .venv in projectDir if it doesn't exist and returns
// the path to the venv's python binary.  This avoids polluting the system
// python and sidesteps PEP 668 externally-managed-environment errors.
func ensureVenv(ctx context.Context, projectDir, pyBin string) (string, error) {
	venvDir := filepath.Join(projectDir, ".venv")

	// Windows venv uses Scripts/python.exe; Unix uses bin/python.
	venvPy := filepath.Join(venvDir, "bin", "python")
	if runtime.GOOS == "windows" {
		venvPy = filepath.Join(venvDir, "Scripts", "python.exe")
	}

	// Already exists — reuse.
	if _, err := os.Stat(venvPy); err == nil {
		return venvPy, nil
	}

	fmt.Printf("[sdk] creating venv at %s (using %s)\n", venvDir, pyBin)
	cmd := exec.CommandContext(ctx, pyBin, "-m", "venv", venvDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("sdk: create venv: %w", err)
	}
	return venvPy, nil
}

// installLocalDeps reads a local package's pyproject.toml and pre-installs
// any dependencies that look like sibling local packages (e.g. roster-sdk)
// which won't be found on PyPI.  It searches projectDir's parent for
// directories whose pyproject.toml defines a matching package name.
func installLocalDeps(ctx context.Context, pyBin, pkgDir, projectDir string) error {
	toml, err := os.ReadFile(filepath.Join(pkgDir, "pyproject.toml"))
	if err != nil {
		return nil // no pyproject.toml — skip
	}
	deps := parseDeps(string(toml))
	if len(deps) == 0 {
		return nil
	}

	// Search for sibling directories that provide the required packages.
	parentDir := filepath.Dir(projectDir)
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return nil
	}

	// Build a map: package-name → directory path.
	siblingPkgs := map[string]string{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sibDir := filepath.Join(parentDir, e.Name())
		sibToml, err := os.ReadFile(filepath.Join(sibDir, "pyproject.toml"))
		if err != nil {
			continue
		}
		if name := parsePkgName(string(sibToml)); name != "" {
			siblingPkgs[name] = sibDir
		}
	}

	for _, dep := range deps {
		if dir, ok := siblingPkgs[dep]; ok {
			if err := localInstall(ctx, pyBin, dir); err != nil {
				return fmt.Errorf("sdk: pre-install dep %s: %w", dep, err)
			}
		}
	}
	return nil
}

// parseDeps extracts dependency names from pyproject.toml's dependencies list.
// Minimal parser — handles the common case: dependencies = ["pkg-name", "pkg>=1.0"].
func parseDeps(toml string) []string {
	inDeps := false
	var deps []string
	for _, line := range strings.Split(toml, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "dependencies") && strings.Contains(trimmed, "=") {
			inDeps = true
			// Handle single-line: dependencies = ["a", "b"]
			if idx := strings.Index(trimmed, "["); idx >= 0 {
				inner := trimmed[idx:]
				if end := strings.Index(inner, "]"); end >= 0 {
					for _, d := range parseBracketList(inner[:end+1]) {
						deps = append(deps, d)
					}
					return deps
				}
			}
			continue
		}
		if inDeps {
			if trimmed == "]" {
				break
			}
			if name := extractDepName(trimmed); name != "" {
				deps = append(deps, name)
			}
		}
	}
	return deps
}

// parsePkgName extracts the name = "..." value from [project] in pyproject.toml.
func parsePkgName(toml string) string {
	inProject := false
	for _, line := range strings.Split(toml, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[project]" {
			inProject = true
			continue
		}
		if inProject {
			if strings.HasPrefix(trimmed, "[") {
				break
			}
			if strings.HasPrefix(trimmed, "name") {
				if idx := strings.Index(trimmed, "="); idx >= 0 {
					val := strings.TrimSpace(trimmed[idx+1:])
					return strings.Trim(val, "\"' ")
				}
			}
		}
	}
	return ""
}

func parseBracketList(s string) []string {
	s = strings.Trim(s, "[]")
	var items []string
	for _, part := range strings.Split(s, ",") {
		if name := extractDepName(strings.TrimSpace(part)); name != "" {
			items = append(items, name)
		}
	}
	return items
}

func extractDepName(s string) string {
	s = strings.Trim(s, "\"', ")
	if s == "" || s == "]" || s == "[" {
		return ""
	}
	// Strip version specifiers: "roster-sdk>=1.0" → "roster-sdk"
	for i, c := range s {
		if c == '>' || c == '<' || c == '=' || c == '!' || c == '~' || c == '[' || c == ';' {
			s = s[:i]
			break
		}
	}
	return strings.TrimSpace(s)
}

func pipInstall(ctx context.Context, pyBin, pkg string) error {
	check := exec.CommandContext(ctx, pyBin, "-c",
		fmt.Sprintf("import importlib.util; assert importlib.util.find_spec(%q)", pipToModule(pkg)))
	if err := check.Run(); err == nil {
		return nil
	}
	fmt.Printf("[sdk] pip install %s\n", pkg)
	cmd := exec.CommandContext(ctx, pyBin, "-m", "pip", "install", "--quiet", pkg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func npmInstall(ctx context.Context, pkg string) error {
	if err := exec.CommandContext(ctx, "node", "-e", fmt.Sprintf("require(%q)", pkg)).Run(); err == nil {
		return nil
	}
	fmt.Printf("[sdk] npm install -g %s\n", pkg)
	cmd := exec.CommandContext(ctx, "npm", "install", "-g", "--quiet", pkg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func gitInstall(ctx context.Context, pyBin, url, ref string) error {
	target := url
	if ref != "" {
		target = url + "@" + ref
	}
	fmt.Printf("[sdk] pip install git+%s\n", target)
	cmd := exec.CommandContext(ctx, pyBin, "-m", "pip", "install", "--quiet", "git+"+target)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func localInstall(ctx context.Context, pyBin, dir string) error {
	fmt.Printf("[sdk] pip install -e %s\n", dir)
	cmd := exec.CommandContext(ctx, pyBin, "-m", "pip", "install", "--quiet", "-e", dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func pipToModule(pkg string) string {
	if idx := strings.IndexAny(pkg, "><=!~"); idx >= 0 {
		pkg = pkg[:idx]
	}
	return strings.ReplaceAll(pkg, "-", "_")
}
