package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runInit(args []string) error {
	dir := "."
	name := ""
	template := ""
	hasName := false
	hasTemplate := false
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		dir = args[0]
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			if i+1 < len(args) {
				name = args[i+1]
				hasName = true
				i++
			}
		case "--template":
			if i+1 < len(args) {
				template = args[i+1]
				hasTemplate = true
				i++
			}
		}
	}

	// Interactive mode: prompt when stdin is a terminal and flags were not provided.
	if (!hasName || !hasTemplate) && isTTY() {
		fmt.Println()
		if !hasName {
			name = promptWithDefault("Organization name", "my-org")
		}
		if !hasTemplate {
			fmt.Println()
			fmt.Println("  Available templates:")
			fmt.Println("    minimal          — single API desk, ready to run (default)")
			fmt.Println("    product-team     — strategy → dev → review → ops")
			fmt.Println("    content-pipeline — research → writing → editorial")
			fmt.Println("    code-review      — parallel security + quality review")
			fmt.Println()
			choice := promptWithDefault("Template", "minimal")
			if choice != "minimal" {
				template = choice
				hasTemplate = true
			}
		}
		fmt.Println()
	}

	// Always prompt for API key in interactive mode — without it the default
	// template won't run at all.
	if isTTY() {
		fmt.Print("  Anthropic API key (press Enter to skip): ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			apiKey := strings.TrimSpace(scanner.Text())
			if apiKey != "" {
				_ = os.MkdirAll(dir, 0750)
				envPath := filepath.Join(dir, ".env")
				if _, err := os.Stat(envPath); os.IsNotExist(err) {
					os.WriteFile(envPath, []byte("ANTHROPIC_API_KEY="+apiKey+"\n"), 0600)
				}
				// Ensure .env is excluded from version control.
				gitignorePath := filepath.Join(dir, ".gitignore")
				existing, _ := os.ReadFile(gitignorePath)
				if !strings.Contains(string(existing), ".env") {
					f, ferr := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
					if ferr == nil {
						fmt.Fprintln(f, ".env")
						f.Close()
					}
				}
			}
		}
		fmt.Println()
	}

	if !hasName && name == "" {
		name = "my-org"
	}

	switch template {
	case "", "minimal":
		return initDefault(dir, name)
	case "product-team":
		return initProductTeam(dir, name)
	case "content-pipeline":
		return initContentPipeline(dir, name)
	case "code-review":
		return initCodeReview(dir, name)
	default:
		return fmt.Errorf("unknown template %q (available: product-team, content-pipeline, code-review)", template)
	}
}

// isTTY returns true if stdin is an interactive terminal.
func isTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// promptWithDefault prints a prompt and reads one line, returning defaultVal if empty.
func promptWithDefault(label, defaultVal string) string {
	fmt.Printf("  %s [%s]: ", label, defaultVal)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			return line
		}
	}
	return defaultVal
}

func initDefault(dir, name string) error {
	// Create directory structure
	dirs := []string{
		dir,
		filepath.Join(dir, "desks"),
		filepath.Join(dir, "agents"),
		filepath.Join(dir, "groups"),
		filepath.Join(dir, "skills"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0750); err != nil {
			return err
		}
	}

	// organization.yaml
	writeIfNotExists(filepath.Join(dir, "organization.yaml"), `kind: organization
name: `+name+`
`)

	// agents/developer.yaml
	writeIfNotExists(filepath.Join(dir, "agents", "developer.yaml"), `kind: agent
name: developer

skills:
  - coding
`)

	// skills/coding.md
	writeIfNotExists(filepath.Join(dir, "skills", "coding.md"), `You are a skilled software developer. Given a task, you:
- Write clean, idiomatic code with clear naming
- Explain your reasoning briefly before the code
- Keep solutions simple — the minimum needed, no more
- Point out any assumptions or edge cases worth noting
`)

	// desks/developer.yaml — uses Claude API directly
	writeIfNotExists(filepath.Join(dir, "desks", "developer.yaml"), `kind: desk
name: developer

agent: developer
groups:
  - dev-team

executor:
  type: api
  sdk: anthropic
  params:
    model: claude-haiku-4-5-20251001
    api_key: ${ANTHROPIC_API_KEY}

subscribe:
  - task.created

emit:
  - task.completed

timeout: 2m
`)

	// groups/dev-team.yaml
	writeIfNotExists(filepath.Join(dir, "groups", "dev-team.yaml"), `kind: group
name: dev-team
`)

	// .gitignore for .roster
	writeIfNotExists(filepath.Join(dir, ".gitignore"), ".roster/\n")

	printInitResult(name, dir, "")
	return nil
}

func initProductTeam(dir, name string) error {
	dirs := []string{dir, filepath.Join(dir, "desks"), filepath.Join(dir, "agents"),
		filepath.Join(dir, "groups"), filepath.Join(dir, "skills"), filepath.Join(dir, "scripts")}
	for _, d := range dirs {
		os.MkdirAll(d, 0750)
	}

	writeIfNotExists(filepath.Join(dir, "organization.yaml"),
		`kind: organization
name: `+name+`
`)

	// Agents
	writeIfNotExists(filepath.Join(dir, "agents", "architect.yaml"),
		`kind: agent
name: architect
skills:
  - product-strategy
  - system-design
`)
	writeIfNotExists(filepath.Join(dir, "agents", "developer.yaml"),
		`kind: agent
name: developer
skills:
  - coding
`)
	writeIfNotExists(filepath.Join(dir, "agents", "reviewer.yaml"),
		`kind: agent
name: reviewer
skills:
  - code-review
`)

	// Skills
	writeIfNotExists(filepath.Join(dir, "skills", "product-strategy.md"),
		`You are a product strategist. Analyze requirements, identify priorities, and create actionable implementation plans. Focus on user value and technical feasibility.`)
	writeIfNotExists(filepath.Join(dir, "skills", "system-design.md"),
		`You are a system architect. Design clean, maintainable solutions. Consider scalability, error handling, and separation of concerns.`)
	writeIfNotExists(filepath.Join(dir, "skills", "coding.md"),
		`You are a skilled developer. Write clean, well-tested code. Follow best practices and keep solutions simple.`)
	writeIfNotExists(filepath.Join(dir, "skills", "code-review.md"),
		`You are a code reviewer. Check for correctness, security, performance, and maintainability. Be specific and actionable in feedback. Reply with "APPROVED" or "CHANGES REQUESTED: <reason>".`)

	// Desks — subscribe/emit live on the desk in v2
	writeIfNotExists(filepath.Join(dir, "desks", "architect.yaml"),
		`kind: desk
name: architect
agent: architect
groups:
  - strategy-team
executor:
  type: exec
  params:
    command: echo "implement your executor here"
subscribe:
  - hub.started
emit:
  - plan.approved
timeout: 10m
`)
	writeIfNotExists(filepath.Join(dir, "desks", "developer.yaml"),
		`kind: desk
name: developer
agent: developer
groups:
  - dev-team
executor:
  type: exec
  params:
    command: echo "implement your executor here"
subscribe:
  - plan.approved
  - review.rejected
  - build.failed
  - test.failed
emit:
  - code.ready
timeout: 10m
`)
	writeIfNotExists(filepath.Join(dir, "desks", "reviewer.yaml"),
		`kind: desk
name: reviewer
agent: reviewer
groups:
  - review-squad
executor:
  type: exec
  params:
    command: echo "implement your executor here"
subscribe:
  - code.ready
emit:
  - review.approved
  - review.rejected
timeout: 10m
`)
	writeIfNotExists(filepath.Join(dir, "desks", "builder.yaml"),
		`kind: desk
name: builder
groups:
  - ops-team
executor:
  type: exec
  params:
    command: echo "build script here"
subscribe:
  - review.approved
emit:
  - build.succeeded
  - build.failed
timeout: 10m
`)
	writeIfNotExists(filepath.Join(dir, "desks", "tester.yaml"),
		`kind: desk
name: tester
groups:
  - ops-team
executor:
  type: exec
  params:
    command: echo "test script here"
subscribe:
  - review.approved
emit:
  - test.passed
  - test.failed
timeout: 10m
`)

	// Groups — v2 groups are session scopes only
	writeIfNotExists(filepath.Join(dir, "groups", "strategy-team.yaml"),
		`kind: group
name: strategy-team
`)
	writeIfNotExists(filepath.Join(dir, "groups", "dev-team.yaml"),
		`kind: group
name: dev-team
`)
	writeIfNotExists(filepath.Join(dir, "groups", "review-squad.yaml"),
		`kind: group
name: review-squad
`)
	writeIfNotExists(filepath.Join(dir, "groups", "ops-team.yaml"),
		`kind: group
name: ops-team
`)

	writeIfNotExists(filepath.Join(dir, ".gitignore"), ".roster/\n")

	printInitResult(name, dir, "product-team")
	return nil
}

func initContentPipeline(dir, name string) error {
	dirs := []string{dir, filepath.Join(dir, "desks"), filepath.Join(dir, "agents"),
		filepath.Join(dir, "groups"), filepath.Join(dir, "skills")}
	for _, d := range dirs {
		os.MkdirAll(d, 0750)
	}

	writeIfNotExists(filepath.Join(dir, "organization.yaml"),
		`kind: organization
name: `+name+`
`)

	writeIfNotExists(filepath.Join(dir, "agents", "researcher.yaml"),
		`kind: agent
name: researcher
skills:
  - research
`)
	writeIfNotExists(filepath.Join(dir, "agents", "writer.yaml"),
		`kind: agent
name: writer
skills:
  - writing
`)
	writeIfNotExists(filepath.Join(dir, "agents", "editor.yaml"),
		`kind: agent
name: editor
skills:
  - editing
`)

	writeIfNotExists(filepath.Join(dir, "skills", "research.md"),
		`You are a researcher. Gather relevant information, identify key facts, and organize findings into a clear brief.`)
	writeIfNotExists(filepath.Join(dir, "skills", "writing.md"),
		`You are a professional writer. Create clear, engaging content based on research briefs. Match the target audience and tone.`)
	writeIfNotExists(filepath.Join(dir, "skills", "editing.md"),
		`You are an editor. Review content for clarity, accuracy, grammar, and style. Reply with "APPROVED" or "REVISION NEEDED: <feedback>".`)

	writeIfNotExists(filepath.Join(dir, "desks", "researcher.yaml"),
		`kind: desk
name: researcher
agent: researcher
groups:
  - research-team
executor:
  type: exec
  params:
    command: echo "implement research executor"
subscribe:
  - hub.started
emit:
  - research.done
timeout: 5m
`)
	writeIfNotExists(filepath.Join(dir, "desks", "writer.yaml"),
		`kind: desk
name: writer
agent: writer
groups:
  - writing-team
executor:
  type: exec
  params:
    command: echo "implement writing executor"
subscribe:
  - research.done
  - revision.needed
emit:
  - draft.ready
timeout: 5m
`)
	writeIfNotExists(filepath.Join(dir, "desks", "editor.yaml"),
		`kind: desk
name: editor
agent: editor
groups:
  - editorial
executor:
  type: exec
  params:
    command: echo "implement editor executor"
subscribe:
  - draft.ready
emit:
  - content.published
  - revision.needed
timeout: 5m
`)

	writeIfNotExists(filepath.Join(dir, "groups", "research-team.yaml"),
		`kind: group
name: research-team
`)
	writeIfNotExists(filepath.Join(dir, "groups", "writing-team.yaml"),
		`kind: group
name: writing-team
`)
	writeIfNotExists(filepath.Join(dir, "groups", "editorial.yaml"),
		`kind: group
name: editorial
`)

	writeIfNotExists(filepath.Join(dir, ".gitignore"), ".roster/\n")

	printInitResult(name, dir, "content-pipeline")
	return nil
}

func initCodeReview(dir, name string) error {
	dirs := []string{dir, filepath.Join(dir, "desks"), filepath.Join(dir, "agents"),
		filepath.Join(dir, "groups"), filepath.Join(dir, "skills")}
	for _, d := range dirs {
		os.MkdirAll(d, 0750)
	}

	writeIfNotExists(filepath.Join(dir, "organization.yaml"),
		`kind: organization
name: `+name+`
`)

	writeIfNotExists(filepath.Join(dir, "agents", "security-reviewer.yaml"),
		`kind: agent
name: security-reviewer
skills:
  - security-review
`)
	writeIfNotExists(filepath.Join(dir, "agents", "quality-reviewer.yaml"),
		`kind: agent
name: quality-reviewer
skills:
  - quality-review
`)

	writeIfNotExists(filepath.Join(dir, "skills", "security-review.md"),
		`You are a security reviewer. Check code for vulnerabilities: injection, XSS, auth bypass, secrets exposure, OWASP top 10. Be specific about the risk and fix.`)
	writeIfNotExists(filepath.Join(dir, "skills", "quality-review.md"),
		`You are a code quality reviewer. Check for readability, maintainability, test coverage, error handling, and performance. Suggest concrete improvements.`)

	writeIfNotExists(filepath.Join(dir, "desks", "security-reviewer.yaml"),
		`kind: desk
name: security-reviewer
agent: security-reviewer
groups:
  - review-team
executor:
  type: exec
  params:
    command: echo "implement security review executor"
subscribe:
  - code.submitted
emit:
  - review.completed
timeout: 5m
`)
	writeIfNotExists(filepath.Join(dir, "desks", "quality-reviewer.yaml"),
		`kind: desk
name: quality-reviewer
agent: quality-reviewer
groups:
  - review-team
executor:
  type: exec
  params:
    command: echo "implement quality review executor"
subscribe:
  - code.submitted
emit:
  - review.completed
timeout: 5m
`)

	writeIfNotExists(filepath.Join(dir, "groups", "review-team.yaml"),
		`kind: group
name: review-team
`)

	writeIfNotExists(filepath.Join(dir, ".gitignore"), ".roster/\n")

	printInitResult(name, dir, "code-review")
	return nil
}

func printInitResult(name, dir, template string) {
	fmt.Printf("Initialized %q organization in %s", name, dir)
	if template != "" {
		fmt.Printf(" (template: %s)", template)
	}
	fmt.Println()
	fmt.Println()
	fmt.Println("  Next steps:")
	fmt.Printf("    cd %s\n", dir)
	// Show the right API key step depending on whether .env was written.
	if _, err := os.Stat(filepath.Join(dir, ".env")); err == nil {
		fmt.Println("    source .env")
	} else if template == "" {
		fmt.Println("    export ANTHROPIC_API_KEY=<your-key>")
	}
	fmt.Println("    roster dry-run .")
	fmt.Println("    roster hub --ui :8080")
	fmt.Println("    roster emit task.created '{\"task\": \"write a hello world in Go\"}'")
	fmt.Println("    roster logs --follow")
	fmt.Println()
}

func writeIfNotExists(path, content string) {
	if _, err := os.Stat(path); err == nil {
		return // file exists, don't overwrite
	}
	os.WriteFile(path, []byte(content), 0640)
}
