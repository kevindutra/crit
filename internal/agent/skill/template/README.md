# Skill Templates

Shared templates for crit's agent instruction files. These are the single source of truth for all skill content — edit here, and all agents and the plugin marketplace get the update.

## How it works

Each `.tmpl` file contains the body of a skill instruction (everything after the YAML frontmatter). Templates use Go's `text/template` syntax for agent-specific values.

At build time, `content.go` in the parent package composes complete skill files by combining:

1. **Frontmatter** — defined per-agent in `SkillConfig` structs (name, description, allowed-tools, etc.)
2. **Template body** — the `.tmpl` file content, rendered with template variables

### Template variables

| Variable | Description | Example values |
|----------|-------------|----------------|
| `{{.CodeReviewSkill}}` | Reference to the code review skill | `/crit-code-review` (setup), `crit:code-review` (plugin) |
| `{{.PlanReviewSkill}}` | Reference to the plan review skill | `/crit-plan-review` (setup), `crit:plan-review` (plugin) |

Only `review.md.tmpl` uses these variables (to cross-reference other skills). The other templates have no variables — they're shared verbatim across all agents.

### Configs

Three predefined configs in `content.go` control how templates are rendered:

| Config | Used by | Differences |
|--------|---------|-------------|
| `ClaudeSetupConfig` | `crit setup --agent claude` | Adds `allowed-tools` frontmatter |
| `SkillSetupConfig` | `crit setup --agent codex/opencode` | No `allowed-tools` |
| `PluginConfig` | `go generate` → plugin marketplace | Uses `crit:` prefix naming |

## Files

| Template | Produces | Description |
|----------|----------|-------------|
| `review.md.tmpl` | Router skill | Asks user what to review, delegates to code-review or plan-review |
| `code-review.md.tmpl` | Code review skill | Multi-file TUI for reviewing git changes |
| `plan-review.md.tmpl` | Plan review skill | Single-file TUI for reviewing documents |
| `agents-section.md.tmpl` | AGENTS.md section | Appended to project-level AGENTS.md for Codex/OpenCode |

## Updating instructions

1. Edit the relevant `.tmpl` file
2. Run `go generate ./internal/agent/` to regenerate `plugin/crit/commands/`
3. Verify with `go test ./internal/agent/`
4. Commit both the template change and the regenerated plugin files

## Adding a new agent

No template changes needed. In the `internal/agent/` package:

1. Create a new adapter file (e.g., `cursor.go`) implementing the `Adapter` interface
2. Add it to the `registry` in `detect.go`
3. Choose `ClaudeSetupConfig` (if the agent supports `allowed-tools`) or `SkillSetupConfig` (generic)
4. For project-level setup, use `AgentsSection()` for the AGENTS.md content
