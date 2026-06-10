package hub

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/roster-io/roster/pkg/types"
)

// sdkRef is a parsed sdk: field value.
type sdkRef struct {
	prefix     string // "pip", "npm", "git", "local"
	pkg        string // package name, git URL, or local path
	pyVer      string // optional Python version (e.g. "3.11"), pip/local only
	gitRef     string // optional git branch/tag/commit, git only
	editable   bool   // local: always editable (-e)
}

// parseSDKRef parses sdk: field values:
//
//	pip:roster-sdk               PyPI package
//	pip:roster-sdk@3.11          PyPI + specific Python version
//	npm:roster-sdk               npm package (global install)
//	git:https://github.com/o/p   git URL (pip install git+...)
//	git:https://...@main         git URL + branch/tag
//	local:./path/to/sdk          local directory (pip install -e)
//	local:../sdk@3.11            local directory + Python version
func parseSDKRef(s string) (sdkRef, error) {
	idx := strings.Index(s, ":")
	if idx < 0 {
		return sdkRef{}, fmt.Errorf("sdk: invalid format %q (expected prefix:value)", s)
	}
	prefix := s[:idx]
	rest := s[idx+1:]
	ref := sdkRef{prefix: prefix}

	switch prefix {
	case "pip":
		// "roster-sdk" or "roster-sdk@3.11"
		if at := strings.LastIndex(rest, "@"); at >= 0 {
			ref.pkg = rest[:at]
			ref.pyVer = rest[at+1:]
		} else {
			ref.pkg = rest
		}
	case "npm":
		ref.pkg = rest
	case "git":
		// URL optionally with "@ref" suffix (no slashes or dots in ref part)
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
		// "./path" or "../path" or "/abs/path", optionally "@3.11"
		if at := strings.LastIndex(rest, "@"); at >= 0 {
			candidate := rest[at+1:]
			// looks like a version string if it starts with a digit
			if len(candidate) > 0 && candidate[0] >= '0' && candidate[0] <= '9' {
				ref.pkg = rest[:at]
				ref.pyVer = candidate
			} else {
				ref.pkg = rest
			}
		} else {
			ref.pkg = rest
		}
		ref.editable = true
	default:
		return sdkRef{}, fmt.Errorf("sdk: unknown prefix %q (use pip:, npm:, git:, or local:)", prefix)
	}
	return ref, nil
}

// collectSDKRefs scans agents and resources and returns unique sdkRefs.
func collectSDKRefs(agents map[string]*types.Agent, resources map[string]*types.Resource) []sdkRef {
	seen := map[string]bool{}
	var refs []sdkRef
	addRef := func(sdk string) {
		if sdk == "" || seen[sdk] {
			return
		}
		seen[sdk] = true
		ref, err := parseSDKRef(sdk)
		if err != nil {
			return
		}
		refs = append(refs, ref)
	}
	for _, a := range agents {
		addRef(a.SDK)
	}
	for _, r := range resources {
		addRef(r.SDK)
	}
	return refs
}

// EnsureSDK installs missing SDK packages and sets the runtime bin on the manager.
func (m *sdkProcessManager) EnsureSDK(ctx context.Context, agents map[string]*types.Agent, resources map[string]*types.Resource) error {
	refs := collectSDKRefs(agents, resources)
	if len(refs) == 0 {
		return nil
	}

	var hasPip, hasNpm bool
	for _, r := range refs {
		switch r.prefix {
		case "pip":
			hasPip = true
			pyBin := m.effectivePythonBin(r.pyVer)
			if err := pipInstall(ctx, pyBin, r.pkg); err != nil {
				return err
			}
			if m.pythonBin == "" {
				m.pythonBin = pyBin
			}
		case "npm":
			hasNpm = true
			if err := npmInstall(ctx, r.pkg); err != nil {
				return err
			}
			if m.nodeBin == "" {
				m.nodeBin = resolveNodeBin()
			}
		case "git":
			pyBin := m.effectivePythonBin(r.pyVer)
			if err := gitInstall(ctx, pyBin, r.pkg, r.gitRef); err != nil {
				return err
			}
			hasPip = true
			if m.pythonBin == "" {
				m.pythonBin = pyBin
			}
		case "local":
			pyBin := m.effectivePythonBin(r.pyVer)
			localPath := r.pkg
			if m.projectDir != "" && !filepath.IsAbs(localPath) {
				localPath = filepath.Join(m.projectDir, localPath)
			}
			if err := localInstall(ctx, pyBin, localPath); err != nil {
				return err
			}
			hasPip = true
			if m.pythonBin == "" {
				m.pythonBin = pyBin
			}
		}
	}
	_ = hasPip
	_ = hasNpm
	return nil
}

// effectivePythonBin returns the already-configured bin if set, otherwise resolves from PATH.
func (m *sdkProcessManager) effectivePythonBin(ver string) string {
	if ver == "" && m.pythonBin != "" {
		return m.pythonBin
	}
	return resolvePythonBin(ver)
}

// resolvePythonBin returns the python executable for the given version string.
// If ver is empty, it tries python3 then python.
func resolvePythonBin(ver string) string {
	if ver != "" {
		candidate := "python" + ver
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return "python"
}

// resolveNodeBin returns the node executable path.
func resolveNodeBin() string {
	if path, err := exec.LookPath("node"); err == nil {
		return path
	}
	return "node"
}

// pipInstall ensures the given package is installed. It checks first; skips if present.
func pipInstall(ctx context.Context, pyBin, pkg string) error {
	// Check if already importable.
	check := exec.CommandContext(ctx, pyBin, "-c", fmt.Sprintf("import importlib.util; assert importlib.util.find_spec(%q)", pipPkgToModule(pkg)))
	if err := check.Run(); err == nil {
		return nil // already installed
	}
	fmt.Printf("[sdk] pip install %s\n", pkg)
	cmd := exec.CommandContext(ctx, pyBin, "-m", "pip", "install", "--quiet", pkg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pip install %s: %w", pkg, err)
	}
	return nil
}

// npmInstall ensures the given npm package is installed globally. Checks first.
func npmInstall(ctx context.Context, pkg string) error {
	check := exec.CommandContext(ctx, "node", "-e", fmt.Sprintf("require(%q)", pkg))
	if err := check.Run(); err == nil {
		return nil
	}
	fmt.Printf("[sdk] npm install -g %s\n", pkg)
	cmd := exec.CommandContext(ctx, "npm", "install", "-g", "--quiet", pkg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("npm install %s: %w", pkg, err)
	}
	return nil
}

// gitInstall installs a package from a git URL via pip.
// ref is an optional branch/tag/commit (defaults to HEAD).
func gitInstall(ctx context.Context, pyBin, url, ref string) error {
	target := url
	if ref != "" {
		target = url + "@" + ref
	}
	pipURL := "git+" + target
	fmt.Printf("[sdk] pip install %s\n", pipURL)
	cmd := exec.CommandContext(ctx, pyBin, "-m", "pip", "install", "--quiet", pipURL)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git install %s: %w", url, err)
	}
	return nil
}

// localInstall installs a local directory as an editable pip package.
// Equivalent to: pip install -e ./path/to/sdk
func localInstall(ctx context.Context, pyBin, dir string) error {
	fmt.Printf("[sdk] pip install -e %s\n", dir)
	cmd := exec.CommandContext(ctx, pyBin, "-m", "pip", "install", "--quiet", "-e", dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("local install %s: %w", dir, err)
	}
	return nil
}

// pipPkgToModule converts a pip package name to a likely importable module name.
// "roster-sdk" → "roster_sdk", "my.pkg" → "my.pkg"
func pipPkgToModule(pkg string) string {
	// Strip version specifiers like roster-sdk>=1.0
	if idx := strings.IndexAny(pkg, "><=!~"); idx >= 0 {
		pkg = pkg[:idx]
	}
	return strings.ReplaceAll(pkg, "-", "_")
}
