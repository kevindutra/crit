package agent

import (
	"os"
	"path/filepath"
)

// Codex is the adapter for OpenAI's Codex CLI agent.
type Codex struct{}

func (c Codex) Name() string { return "Codex" }
func (c Codex) Slug() string { return "codex" }

func (c Codex) BannerText() string {
	return "Codex is paused — review the document, then press q to submit"
}

func (c Codex) Detect() bool {
	return os.Getenv("CODEX_HOME") != ""
}

func (c Codex) SetupBasePath(scope SetupScope) string {
	if scope == ScopeProject {
		return "."
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "skills")
}

func (c Codex) SetupFiles(scope SetupScope) []SetupFile {
	if scope == ScopeProject {
		return []SetupFile{
			{RelPath: "AGENTS.md", Content: AgentsSection(), Mode: ModeAppendSection},
		}
	}
	review, codeReview, planReview := RenderSkills(SkillSetupConfig)
	return []SetupFile{
		{RelPath: "crit-review/SKILL.md", Content: review},
		{RelPath: "crit-plan-review/SKILL.md", Content: planReview},
		{RelPath: "crit-code-review/SKILL.md", Content: codeReview},
	}
}
