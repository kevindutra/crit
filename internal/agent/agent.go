//go:generate go run -C ../.. ./cmd/generate-plugin

package agent

// SetupScope controls whether instruction files are installed globally or per-project.
type SetupScope int

const (
	ScopeGlobal  SetupScope = iota
	ScopeProject
)

// WriteMode controls how a setup file is written.
type WriteMode int

const (
	// ModeCreate writes a new file (or overwrites if --force).
	ModeCreate WriteMode = iota
	// ModeAppendSection appends or replaces a delimited section in an existing file.
	ModeAppendSection
)

// SetupFile represents a single instruction file to write during setup.
type SetupFile struct {
	RelPath string // Relative path from the scope root (e.g., "skills/crit-review/SKILL.md")
	Content []byte
	Mode    WriteMode
}

// Adapter encapsulates per-agent differences.
type Adapter interface {
	Name() string                            // "Claude Code", "Codex", "OpenCode"
	Slug() string                            // "claude", "codex", "opencode" — used for --agent flag
	Detect() bool                            // Is this agent likely running?
	BannerText() string                      // TUI banner when paused
	SetupFiles(scope SetupScope) []SetupFile // Files to write during setup
	SetupBasePath(scope SetupScope) string   // Root directory for setup files
}
