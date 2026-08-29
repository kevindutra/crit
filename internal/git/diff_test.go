package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInlineDiff(t *testing.T) {
	before := "prefix.  `tfil` maintains a screen model of the output and enables mouse reporting.  Hovering continues."
	after := "prefix.  When Codex starts, `tfil` begins maintaining a screen model of the output and enables mouse reporting.  Non-interactive commands stay quiet.  Hovering continues."
	oldSegments, newSegments := inlineDiff(before, after)

	assertSegments := func(name, want string, segments []InlineSegment) string {
		t.Helper()
		var got strings.Builder
		var changedText strings.Builder
		changed := false
		unchanged := false
		for _, segment := range segments {
			got.WriteString(segment.Content)
			if segment.Changed {
				changed = true
				changedText.WriteString(segment.Content)
			} else {
				unchanged = true
			}
		}
		if got.String() != want {
			t.Errorf("%s reconstructed as %q, want %q", name, got.String(), want)
		}
		if !changed || !unchanged {
			t.Errorf("%s segments did not separate changed and common text: %+v", name, segments)
		}
		return changedText.String()
	}

	oldChanged := assertSegments("old", before, oldSegments)
	newChanged := assertSegments("new", after, newSegments)
	if oldChanged != "maintains " {
		t.Errorf("old changed text = %q, want %q", oldChanged, "maintains ")
	}
	if strings.Contains(newChanged, "screen model") || strings.Contains(newChanged, "Hovering") {
		t.Errorf("new changed text contains common words: %q", newChanged)
	}
	for _, want := range []string{"When Codex starts", "begins maintaining", "Non-interactive commands"} {
		if !strings.Contains(newChanged, want) {
			t.Errorf("new changed text %q does not contain %q", newChanged, want)
		}
	}
}

func TestDiffFileAnnotatesReplacementLines(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")

	writeFile(t, dir, "README.md", "prefix old suffix\n")
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-q", "-m", "initial")
	writeFile(t, dir, "README.md", "prefix new suffix\n")

	t.Chdir(dir)
	info, err := DiffFile("README.md", "HEAD")
	if err != nil {
		t.Fatalf("DiffFile: %v", err)
	}
	if len(info.InlineChanges[1]) == 0 {
		t.Fatal("expected inline changes for added replacement line")
	}
	deleted := info.DeletedAfter[0]
	if len(deleted) != 1 || len(deleted[0].Inline) == 0 {
		t.Fatalf("expected inline changes for deleted replacement line, got %+v", deleted)
	}
}
