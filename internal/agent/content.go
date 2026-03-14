package agent

import "embed"

// Claude Code SKILL.md files
//
//go:embed skill/claude/crit-review/SKILL.md
var claudeSkillReview []byte

//go:embed skill/claude/crit-plan-review/SKILL.md
var claudeSkillPlanReview []byte

//go:embed skill/claude/crit-code-review/SKILL.md
var claudeSkillCodeReview []byte

// Codex SKILL.md files
//
//go:embed skill/codex/crit-review/SKILL.md
var codexSkillReview []byte

//go:embed skill/codex/crit-plan-review/SKILL.md
var codexSkillPlanReview []byte

//go:embed skill/codex/crit-code-review/SKILL.md
var codexSkillCodeReview []byte

//go:embed skill/codex/AGENTS.md
var codexAgentsMD []byte

// OpenCode SKILL.md files
//
//go:embed skill/opencode/crit-review/SKILL.md
var opencodeSkillReview []byte

//go:embed skill/opencode/crit-plan-review/SKILL.md
var opencodeSkillPlanReview []byte

//go:embed skill/opencode/crit-code-review/SKILL.md
var opencodeSkillCodeReview []byte

//go:embed skill/opencode/AGENTS.md
var opencodeAgentsMD []byte

// Ensure embed import is used.
var _ embed.FS
