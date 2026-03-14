package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSectionCreatesNewFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "AGENTS.md")

	content := []byte("<!-- crit:start -->\n## Crit\nHello\n<!-- crit:end -->\n")
	if err := writeSection(path, content); err != nil {
		t.Fatalf("writeSection() error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}

	if string(got) != string(content) {
		t.Errorf("file content = %q, want %q", string(got), string(content))
	}
}

func TestWriteSectionAppendsToExisting(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "AGENTS.md")

	existing := "# My Project Agents\n\nSome existing content.\n"
	os.WriteFile(path, []byte(existing), 0644)

	section := []byte("<!-- crit:start -->\n## Crit\nHello\n<!-- crit:end -->\n")
	if err := writeSection(path, section); err != nil {
		t.Fatalf("writeSection() error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}

	if !strings.Contains(string(got), existing) {
		t.Error("existing content should be preserved")
	}
	if !strings.Contains(string(got), "<!-- crit:start -->") {
		t.Error("section should be appended")
	}
}

func TestWriteSectionReplacesExisting(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "AGENTS.md")

	existing := "# Agents\n\n<!-- crit:start -->\n## Old Crit\nOld content\n<!-- crit:end -->\n\nOther stuff.\n"
	os.WriteFile(path, []byte(existing), 0644)

	newSection := []byte("<!-- crit:start -->\n## New Crit\nNew content\n<!-- crit:end -->\n")
	if err := writeSection(path, newSection); err != nil {
		t.Fatalf("writeSection() error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}

	if strings.Contains(string(got), "Old Crit") {
		t.Error("old section should be replaced")
	}
	if !strings.Contains(string(got), "New Crit") {
		t.Error("new section should be present")
	}
	if !strings.Contains(string(got), "Other stuff") {
		t.Error("content outside section should be preserved")
	}
}

func TestWriteSectionIsIdempotent(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "AGENTS.md")

	section := []byte("<!-- crit:start -->\n## Crit\nHello\n<!-- crit:end -->\n")

	// Write twice
	writeSection(path, section)
	writeSection(path, section)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}

	count := strings.Count(string(got), "<!-- crit:start -->")
	if count != 1 {
		t.Errorf("found %d crit sections, want 1 (idempotent)", count)
	}
}

func TestResolveAgentAlias(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"claude-code", "claude"},
		{"Claude-Code", "claude"},
		{"claude", "claude"},
		{"codex", "codex"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		got := resolveAgentAlias(tt.input)
		if got != tt.want {
			t.Errorf("resolveAgentAlias(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
