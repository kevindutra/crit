package agent

import (
	"os"
	"path/filepath"
)

// OpenCode is the adapter for the OpenCode agent.
type OpenCode struct{}

func (c OpenCode) Name() string { return "OpenCode" }
func (c OpenCode) Slug() string { return "opencode" }

func (c OpenCode) BannerText() string {
	return "OpenCode is paused — review the document, then press q to submit"
}

func (c OpenCode) Detect() bool {
	return os.Getenv("OPENCODE") != ""
}

func (c OpenCode) SetupBasePath(scope SetupScope) string {
	if scope == ScopeProject {
		return "."
	}
	return filepath.Join(xdgConfigHome(), "opencode", "skills")
}

func (c OpenCode) SetupFiles(scope SetupScope) []SetupFile {
	if scope == ScopeProject {
		return []SetupFile{
			{RelPath: "AGENTS.md", Content: opencodeAgentsMD, Mode: ModeAppendSection},
		}
	}
	return []SetupFile{
		{RelPath: "crit-review/SKILL.md", Content: opencodeSkillReview},
		{RelPath: "crit-plan-review/SKILL.md", Content: opencodeSkillPlanReview},
		{RelPath: "crit-code-review/SKILL.md", Content: opencodeSkillCodeReview},
	}
}

func xdgConfigHome() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}
