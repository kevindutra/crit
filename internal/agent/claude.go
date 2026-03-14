package agent

import (
	"os"
	"path/filepath"
)

// ClaudeCode is the adapter for Anthropic's Claude Code agent.
type ClaudeCode struct{}

func (c ClaudeCode) Name() string { return "Claude Code" }
func (c ClaudeCode) Slug() string { return "claude" }

func (c ClaudeCode) BannerText() string {
	return "Claude Code is paused — review the document, then press q to submit"
}

func (c ClaudeCode) Detect() bool {
	return os.Getenv("CLAUDE_CODE") != "" || os.Getenv("CLAUDE_CODE_ENTRYPOINT") != ""
}

func (c ClaudeCode) SetupBasePath(scope SetupScope) string {
	if scope == ScopeProject {
		return filepath.Join(".claude", "skills")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "skills")
}

func (c ClaudeCode) SetupFiles(scope SetupScope) []SetupFile {
	review, codeReview, planReview := RenderSkills(ClaudeSetupConfig)
	return []SetupFile{
		{RelPath: "crit-review/SKILL.md", Content: review},
		{RelPath: "crit-plan-review/SKILL.md", Content: planReview},
		{RelPath: "crit-code-review/SKILL.md", Content: codeReview},
	}
}
