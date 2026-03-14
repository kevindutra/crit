package agent

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed skill/template/*.tmpl
var templateFS embed.FS

// templateData is the data passed to skill templates.
type templateData struct {
	CodeReviewSkill string // e.g., "/crit-code-review" or "crit:code-review"
	PlanReviewSkill string // e.g., "/crit-plan-review" or "crit:plan-review"
}

// skillFrontmatter defines the YAML frontmatter for a skill file.
type skillFrontmatter struct {
	Name         string
	Description  string
	AllowedTools string // optional — Claude Code only
	ArgumentHint string // optional — plan-review only
}

// renderSkill composes a complete skill file from frontmatter + template body.
func renderSkill(fm skillFrontmatter, tmplName string, data templateData) []byte {
	// Build frontmatter
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.WriteString(fmt.Sprintf("name: %s\n", fm.Name))
	buf.WriteString(fmt.Sprintf("description: %s\n", fm.Description))
	if fm.AllowedTools != "" {
		buf.WriteString(fmt.Sprintf("allowed-tools: %s\n", fm.AllowedTools))
	}
	if fm.ArgumentHint != "" {
		buf.WriteString(fmt.Sprintf("argument-hint: %s\n", fm.ArgumentHint))
	}
	buf.WriteString("---\n\n")

	// Render template body
	tmpl, err := template.ParseFS(templateFS, "skill/template/"+tmplName)
	if err != nil {
		panic(fmt.Sprintf("parsing skill template %s: %v", tmplName, err))
	}
	if err := tmpl.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("executing skill template %s: %v", tmplName, err))
	}

	return buf.Bytes()
}

// renderTemplate renders a template without frontmatter (used for AGENTS.md sections).
func renderTemplate(tmplName string) []byte {
	tmpl, err := template.ParseFS(templateFS, "skill/template/"+tmplName)
	if err != nil {
		panic(fmt.Sprintf("parsing template %s: %v", tmplName, err))
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, nil); err != nil {
		panic(fmt.Sprintf("executing template %s: %v", tmplName, err))
	}
	return buf.Bytes()
}

// SkillConfig holds the naming conventions for a particular distribution channel.
type SkillConfig struct {
	// Skill name format (e.g., "crit-review" for setup, "crit:review" for plugin)
	ReviewName     string
	CodeReviewName string
	PlanReviewName string

	// Cross-references in the router skill body
	CodeReviewRef string // e.g., "/crit-code-review" or "crit:code-review"
	PlanReviewRef string // e.g., "/crit-plan-review" or "crit:plan-review"

	// Optional frontmatter fields (Claude Code only)
	AllowedToolsCodeReview string
	AllowedToolsPlanReview string
}

// RenderSkills generates the three skill files for a given config.
func RenderSkills(cfg SkillConfig) (review, codeReview, planReview []byte) {
	data := templateData{
		CodeReviewSkill: cfg.CodeReviewRef,
		PlanReviewSkill: cfg.PlanReviewRef,
	}

	review = renderSkill(skillFrontmatter{
		Name:        cfg.ReviewName,
		Description: "Open crit for review. Routes to code review (multi-file TUI for code changes) or plan/document review (single-file TUI).",
	}, "review.md.tmpl", data)

	codeReview = renderSkill(skillFrontmatter{
		Name:         cfg.CodeReviewName,
		Description:  "Review code changes in crit's multi-file TUI with syntax highlighting and diff markers. After the review, address any comments.",
		AllowedTools: cfg.AllowedToolsCodeReview,
	}, "code-review.md.tmpl", data)

	planReview = renderSkill(skillFrontmatter{
		Name:         cfg.PlanReviewName,
		Description:  "Open a document or plan in crit's interactive TUI for review. After the review, address any comments by editing the document. Use when a plan or document needs human review, when the user asks to review a document, or after generating/updating a plan.",
		AllowedTools: cfg.AllowedToolsPlanReview,
		ArgumentHint: "<file-path>",
	}, "plan-review.md.tmpl", data)

	return
}

// Predefined skill configs per distribution channel.
var (
	// SkillSetupConfig is used by `crit setup --agent <name>` (slash-command skills).
	SkillSetupConfig = SkillConfig{
		ReviewName:     "crit-review",
		CodeReviewName: "crit-code-review",
		PlanReviewName: "crit-plan-review",
		CodeReviewRef:  "/crit-code-review",
		PlanReviewRef:  "/crit-plan-review",
	}

	// ClaudeSetupConfig adds allowed-tools for Claude Code skills.
	ClaudeSetupConfig = SkillConfig{
		ReviewName:             "crit-review",
		CodeReviewName:         "crit-code-review",
		PlanReviewName:         "crit-plan-review",
		CodeReviewRef:          "/crit-code-review",
		PlanReviewRef:          "/crit-plan-review",
		AllowedToolsCodeReview: "Bash(crit *), Read, Edit, Grep, MultiEdit",
		AllowedToolsPlanReview: "Bash(crit *), Read, Edit, Grep",
	}

	// PluginConfig is used by `go generate` to produce plugin/crit/commands/*.md.
	PluginConfig = SkillConfig{
		ReviewName:             "crit:review",
		CodeReviewName:         "crit:code-review",
		PlanReviewName:         "crit:plan-review",
		CodeReviewRef:          "crit:code-review",
		PlanReviewRef:          "crit:plan-review",
		AllowedToolsCodeReview: "Bash(crit *), Read, Edit, Grep, MultiEdit",
		AllowedToolsPlanReview: "Bash(crit *), Read, Edit, Grep",
	}
)

// AgentsSection returns the AGENTS.md section content for project-level setup.
func AgentsSection() []byte {
	return renderTemplate("agents-section.md.tmpl")
}
