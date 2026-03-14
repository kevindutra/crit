package agent

import (
	"os"
	"strings"
	"testing"
)

func TestClaudeCodeDetect(t *testing.T) {
	clearAgentEnvVars(t)

	a := ClaudeCode{}

	// Not detected when env vars are unset
	if a.Detect() {
		t.Error("ClaudeCode.Detect() should return false when env vars are unset")
	}

	// Detected via CLAUDE_CODE
	os.Setenv("CLAUDE_CODE", "1")
	defer os.Unsetenv("CLAUDE_CODE")
	if !a.Detect() {
		t.Error("ClaudeCode.Detect() should return true when CLAUDE_CODE is set")
	}
}

func TestClaudeCodeDetectEntrypoint(t *testing.T) {
	clearAgentEnvVars(t)

	a := ClaudeCode{}
	os.Setenv("CLAUDE_CODE_ENTRYPOINT", "cli")
	defer os.Unsetenv("CLAUDE_CODE_ENTRYPOINT")

	if !a.Detect() {
		t.Error("ClaudeCode.Detect() should return true when CLAUDE_CODE_ENTRYPOINT is set")
	}
}

func TestCodexDetect(t *testing.T) {
	clearAgentEnvVars(t)

	a := Codex{}
	if a.Detect() {
		t.Error("Codex.Detect() should return false when CODEX_HOME is unset")
	}

	os.Setenv("CODEX_HOME", "/home/user/.codex")
	defer os.Unsetenv("CODEX_HOME")
	if !a.Detect() {
		t.Error("Codex.Detect() should return true when CODEX_HOME is set")
	}
}

func TestOpenCodeDetect(t *testing.T) {
	clearAgentEnvVars(t)

	a := OpenCode{}
	if a.Detect() {
		t.Error("OpenCode.Detect() should return false when OPENCODE is unset")
	}

	os.Setenv("OPENCODE", "1")
	defer os.Unsetenv("OPENCODE")
	if !a.Detect() {
		t.Error("OpenCode.Detect() should return true when OPENCODE is set")
	}
}

func TestDetectPriorityOrder(t *testing.T) {
	clearAgentEnvVars(t)

	// When multiple agents are detectable, Claude Code wins (first in registry)
	os.Setenv("CLAUDE_CODE", "1")
	os.Setenv("CODEX_HOME", "/tmp")
	defer os.Unsetenv("CLAUDE_CODE")
	defer os.Unsetenv("CODEX_HOME")

	a := Detect()
	if a == nil {
		t.Fatal("Detect() should return an adapter")
	}
	if a.Slug() != "claude" {
		t.Errorf("expected Claude Code (first in registry), got %s", a.Slug())
	}
}

func TestDetectReturnsNilWhenNoMatch(t *testing.T) {
	clearAgentEnvVars(t)

	if Detect() != nil {
		t.Error("Detect() should return nil when no agent is detected")
	}
}

func TestByName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"claude", "claude"},
		{"Claude", "claude"},
		{"CLAUDE", "claude"},
		{"codex", "codex"},
		{"opencode", "opencode"},
	}

	for _, tt := range tests {
		a := ByName(tt.input)
		if a == nil {
			t.Errorf("ByName(%q) returned nil", tt.input)
			continue
		}
		if a.Slug() != tt.want {
			t.Errorf("ByName(%q).Slug() = %q, want %q", tt.input, a.Slug(), tt.want)
		}
	}
}

func TestByNameReturnsNilForUnknown(t *testing.T) {
	if ByName("unknown") != nil {
		t.Error("ByName(\"unknown\") should return nil")
	}
}

func TestNames(t *testing.T) {
	names := Names()
	for _, expected := range []string{"claude", "codex", "opencode"} {
		if !strings.Contains(names, expected) {
			t.Errorf("Names() = %q, expected to contain %q", names, expected)
		}
	}
}

func TestAllReturnsAllAdapters(t *testing.T) {
	all := All()
	if len(all) != 3 {
		t.Errorf("All() returned %d adapters, want 3", len(all))
	}
}

func TestAdapterBannerText(t *testing.T) {
	tests := []struct {
		adapter Adapter
		want    string
	}{
		{ClaudeCode{}, "Claude Code"},
		{Codex{}, "Codex"},
		{OpenCode{}, "OpenCode"},
	}

	for _, tt := range tests {
		banner := tt.adapter.BannerText()
		if !strings.Contains(banner, tt.want) {
			t.Errorf("%s.BannerText() = %q, expected to contain %q", tt.adapter.Name(), banner, tt.want)
		}
	}
}

func TestClaudeSetupFilesGlobal(t *testing.T) {
	files := ClaudeCode{}.SetupFiles(ScopeGlobal)
	if len(files) != 3 {
		t.Fatalf("expected 3 setup files, got %d", len(files))
	}
	for _, f := range files {
		if len(f.Content) == 0 {
			t.Errorf("setup file %s has empty content", f.RelPath)
		}
		if f.Mode != ModeCreate {
			t.Errorf("setup file %s has mode %d, want ModeCreate", f.RelPath, f.Mode)
		}
	}
}

func TestCodexSetupFilesProject(t *testing.T) {
	files := Codex{}.SetupFiles(ScopeProject)
	if len(files) != 1 {
		t.Fatalf("expected 1 project setup file (AGENTS.md), got %d", len(files))
	}
	if files[0].RelPath != "AGENTS.md" {
		t.Errorf("expected AGENTS.md, got %s", files[0].RelPath)
	}
	if files[0].Mode != ModeAppendSection {
		t.Error("AGENTS.md should use ModeAppendSection")
	}
}

func TestOpenCodeSetupFilesProject(t *testing.T) {
	files := OpenCode{}.SetupFiles(ScopeProject)
	if len(files) != 1 {
		t.Fatalf("expected 1 project setup file (AGENTS.md), got %d", len(files))
	}
	if files[0].Mode != ModeAppendSection {
		t.Error("AGENTS.md should use ModeAppendSection")
	}
}

func TestXdgConfigHome(t *testing.T) {
	// Test with XDG_CONFIG_HOME set
	orig := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", "/custom/config")
	defer os.Setenv("XDG_CONFIG_HOME", orig)

	if got := xdgConfigHome(); got != "/custom/config" {
		t.Errorf("xdgConfigHome() = %q, want /custom/config", got)
	}

	// Test fallback
	os.Unsetenv("XDG_CONFIG_HOME")
	home, _ := os.UserHomeDir()
	if got := xdgConfigHome(); !strings.HasSuffix(got, ".config") {
		t.Errorf("xdgConfigHome() = %q, expected to end with .config (home=%s)", got, home)
	}
}

// clearAgentEnvVars unsets all agent-related env vars for test isolation.
func clearAgentEnvVars(t *testing.T) {
	t.Helper()
	vars := []string{"CLAUDE_CODE", "CLAUDE_CODE_ENTRYPOINT", "CODEX_HOME", "OPENCODE"}
	for _, v := range vars {
		orig := os.Getenv(v)
		os.Unsetenv(v)
		t.Cleanup(func() {
			if orig != "" {
				os.Setenv(v, orig)
			}
		})
	}
}
