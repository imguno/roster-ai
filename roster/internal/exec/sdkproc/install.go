package sdkproc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/roster-io/roster/pkg/types"
)

// sdkRef holds a parsed sdk: field value.
type sdkRef struct {
	prefix  string // "pip", "npm", "git", "local"
	pkg     string // package name, git URL, or local path
	pyVer   string // optional Python version, pip/local only
	gitRef  string // optional git ref, git only
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

func resolvePythonBin(ver string) string {
	if ver != "" {
		if path, err := exec.LookPath("python" + ver); err == nil {
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

func resolveNodeBin() string {
	if path, err := exec.LookPath("node"); err == nil {
		return path
	}
	return "node"
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
