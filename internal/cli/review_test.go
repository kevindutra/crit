package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildTmuxPaneCommand(t *testing.T) {
	cmd := buildTmuxPaneCommand("/usr/local/bin/crit", "/home/user/doc.md", "crit-review-1234")

	if !strings.Contains(cmd, "'/usr/local/bin/crit'") {
		t.Errorf("expected escaped crit binary in command, got: %s", cmd)
	}
	if !strings.Contains(cmd, "'/home/user/doc.md'") {
		t.Errorf("expected escaped file path in command, got: %s", cmd)
	}
	if !strings.Contains(cmd, "tmux wait-for -S crit-review-1234") {
		t.Errorf("expected wait-for signal in command, got: %s", cmd)
	}
}

func TestBuildTmuxPaneCommandEscapesQuotes(t *testing.T) {
	cmd := buildTmuxPaneCommand("/bin/crit", "/home/user/it's a file.md", "ch-1")

	if !strings.Contains(cmd, "'/home/user/it'\\''s a file.md'") {
		t.Errorf("expected escaped single quotes in path, got: %s", cmd)
	}
}

func TestSplitWindowArgsTargetsInvokingPane(t *testing.T) {
	t.Setenv("TMUX_PANE", "%42")

	args := splitWindowArgs(true, "crit review")
	want := []string{"split-window", "-h", "-t", "%42", "-p", "70", "crit review"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Errorf("splitWindowArgs = %v, want %v", args, want)
	}

	args = splitWindowArgs(false, "crit review")
	want = []string{"split-window", "-h", "-t", "%42", "crit review"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Errorf("splitWindowArgs = %v, want %v", args, want)
	}
}

func TestSplitWindowArgsWithoutPaneEnv(t *testing.T) {
	t.Setenv("TMUX_PANE", "")

	args := splitWindowArgs(true, "crit review")
	if containsArg(args, "-t") {
		t.Errorf("expected no -t flag when TMUX_PANE is unset, got: %v", args)
	}
}

func TestShellEscape(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "'simple'"},
		{"with spaces", "'with spaces'"},
		{"it's", "'it'\\''s'"},
		{"", "''"},
	}

	for _, tt := range tests {
		got := shellEscape(tt.input)
		if got != tt.expected {
			t.Errorf("shellEscape(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestDetachRequiresTmux(t *testing.T) {
	tmp, err := os.CreateTemp("", "crit-test-*.md")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	orig := os.Getenv("TMUX")
	os.Setenv("TMUX", "")
	defer os.Setenv("TMUX", orig)

	err = runDetachedReview(tmp.Name())
	if err == nil {
		t.Fatal("expected error when TMUX is not set")
	}
	if !strings.Contains(err.Error(), "requires a tmux session") {
		t.Errorf("expected 'requires a tmux session' error, got: %s", err)
	}
}

func TestCodeReviewDetachRequiresTmux(t *testing.T) {
	orig := os.Getenv("TMUX")
	os.Setenv("TMUX", "")
	defer os.Setenv("TMUX", orig)

	// Simulate --detach --code flags
	reviewDetach = true
	reviewCode = true
	defer func() {
		reviewDetach = false
		reviewCode = false
	}()

	err := runCodeReview()
	if err == nil {
		t.Fatal("expected error when TMUX is not set")
	}
	if !strings.Contains(err.Error(), "requires a tmux session") {
		t.Errorf("expected 'requires a tmux session' error, got: %s", err)
	}
}

// TestCodeReviewDetachRoutesToDetachedPath verifies that runCodeReview with
// --detach enters the detached code path rather than launching the TUI directly.
// This is the fix for https://github.com/kevindutra/crit/issues/2 where
// --code --detach would bypass the detach check and fail with a TTY error.
func TestCodeReviewDetachRoutesToDetachedPath(t *testing.T) {
	origTmux := os.Getenv("TMUX")
	os.Setenv("TMUX", "/tmp/tmux-test/default,12345,0")
	defer os.Setenv("TMUX", origTmux)

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", origPath)

	reviewDetach = true
	reviewCode = true
	defer func() {
		reviewDetach = false
		reviewCode = false
	}()

	err := runCodeReview()
	if err == nil {
		t.Fatal("expected error from detached path (tmux not on PATH), but got nil")
	}

	// If we reach the detached path, the error will be about the tmux binary
	// not being found. If we had NOT entered the detached path, the error
	// would be about git repo or TUI instead.
	if !strings.Contains(err.Error(), "tmux binary not found") {
		t.Errorf("expected 'tmux binary not found' error (proving detached path was taken), got: %s", err)
	}
}

// TestWaitWithoutDetachIsIgnored verifies that --wait without --detach
// does not enter the tmux/detach path — it resets the flag and continues
// down the normal (non-detached) code path.
func TestWaitWithoutDetachIsIgnored(t *testing.T) {
	reviewDetach = false
	reviewWait = true
	reviewCode = true
	defer func() {
		reviewDetach = false
		reviewWait = false
		reviewCode = false
	}()

	err := runCodeReview()

	// --wait should have been ignored, so we should NOT get a tmux error.
	// The test environment will produce some other error (TUI/git), but
	// the key assertion is that we never entered the tmux detach path.
	if err != nil && strings.Contains(err.Error(), "tmux") {
		t.Errorf("--wait without --detach should not enter tmux path, got: %s", err)
	}

	// The flag should have been reset to false
	if reviewWait {
		t.Error("expected reviewWait to be reset to false")
	}
}

// stubExecDeps replaces lookPath, resolveExec, and runCommand with test
// doubles that avoid shelling out. It returns a pointer to a slice that
// accumulates the args of every command passed to runCommand, and a cleanup
// function that restores the originals.
func stubExecDeps(t *testing.T) (*[][]string, func()) {
	t.Helper()

	origRun := runCommand
	origLook := lookPath
	origResolve := resolveExec

	var commands [][]string

	lookPath = func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}
	resolveExec = func() (string, error) {
		return "/usr/local/bin/crit", nil
	}
	runCommand = func(cmd *exec.Cmd) error {
		commands = append(commands, cmd.Args)
		return nil
	}

	return &commands, func() {
		runCommand = origRun
		lookPath = origLook
		resolveExec = origResolve
	}
}

// TestDetachedCodeReviewWaitCallsWaitFor verifies that --detach --wait --code
// runs both the split-window command and the wait-for command.
func TestDetachedCodeReviewWaitCallsWaitFor(t *testing.T) {
	origTmux := os.Getenv("TMUX")
	os.Setenv("TMUX", "/tmp/tmux-test/default,12345,0")
	defer os.Setenv("TMUX", origTmux)

	commands, cleanup := stubExecDeps(t)
	defer cleanup()

	reviewDetach = true
	reviewWait = true
	reviewCode = true
	defer func() {
		reviewDetach = false
		reviewWait = false
		reviewCode = false
	}()

	err := runCodeReview()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if len(*commands) != 2 {
		t.Fatalf("expected 2 commands (split-window + wait-for), got %d: %v", len(*commands), *commands)
	}

	split := (*commands)[0]
	if !containsArg(split, "split-window") {
		t.Errorf("first command should be split-window, got: %v", split)
	}

	wait := (*commands)[1]
	if !containsArg(wait, "wait-for") {
		t.Errorf("second command should be wait-for, got: %v", wait)
	}
}

// TestDetachedCodeReviewNoWaitSkipsWaitFor verifies that --detach without
// --wait only runs the split-window command and does NOT call wait-for.
func TestDetachedCodeReviewNoWaitSkipsWaitFor(t *testing.T) {
	origTmux := os.Getenv("TMUX")
	os.Setenv("TMUX", "/tmp/tmux-test/default,12345,0")
	defer os.Setenv("TMUX", origTmux)

	commands, cleanup := stubExecDeps(t)
	defer cleanup()

	reviewDetach = true
	reviewWait = false
	reviewCode = true
	defer func() {
		reviewDetach = false
		reviewCode = false
	}()

	err := runCodeReview()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if len(*commands) != 1 {
		t.Fatalf("expected 1 command (split-window only), got %d: %v", len(*commands), *commands)
	}

	if !containsArg((*commands)[0], "split-window") {
		t.Errorf("expected split-window command, got: %v", (*commands)[0])
	}
}

// TestDetachedReviewWaitCallsWaitFor verifies the single-file --detach --wait
// path runs both split-window and wait-for.
func TestDetachedReviewWaitCallsWaitFor(t *testing.T) {
	origTmux := os.Getenv("TMUX")
	os.Setenv("TMUX", "/tmp/tmux-test/default,12345,0")
	defer os.Setenv("TMUX", origTmux)

	commands, cleanup := stubExecDeps(t)
	defer cleanup()

	reviewWait = true
	defer func() { reviewWait = false }()

	tmp, err := os.CreateTemp("", "crit-test-*.md")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	err = runDetachedReview(tmp.Name())
	// The wait path tries to Load() the review state file, which won't exist.
	// That's fine — we just care that wait-for was called before that error.
	if err != nil && !strings.Contains(err.Error(), "reading review state") {
		t.Fatalf("unexpected error: %s", err)
	}

	if len(*commands) != 2 {
		t.Fatalf("expected 2 commands (split-window + wait-for), got %d: %v", len(*commands), *commands)
	}

	if !containsArg((*commands)[1], "wait-for") {
		t.Errorf("second command should be wait-for, got: %v", (*commands)[1])
	}
}

// TestDetachedCodeReviewFallsBackWithoutPercentage verifies that when the
// initial split-window with -p 70 fails, it retries without the percentage flag.
func TestDetachedCodeReviewFallsBackWithoutPercentage(t *testing.T) {
	origTmux := os.Getenv("TMUX")
	os.Setenv("TMUX", "/tmp/tmux-test/default,12345,0")
	defer os.Setenv("TMUX", origTmux)

	origRun := runCommand
	origLook := lookPath
	origResolve := resolveExec
	defer func() {
		runCommand = origRun
		lookPath = origLook
		resolveExec = origResolve
	}()

	var commands [][]string
	lookPath = func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}
	resolveExec = func() (string, error) {
		return "/usr/local/bin/crit", nil
	}
	runCommand = func(cmd *exec.Cmd) error {
		commands = append(commands, cmd.Args)
		// Fail if -p is present (simulating the tmux percentage error)
		if containsArg(cmd.Args, "-p") {
			return fmt.Errorf("exit status 1")
		}
		return nil
	}

	reviewDetach = true
	reviewWait = false
	reviewCode = true
	defer func() {
		reviewDetach = false
		reviewCode = false
	}()

	err := runCodeReview()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if len(commands) != 2 {
		t.Fatalf("expected 2 commands (failed split + retry), got %d: %v", len(commands), commands)
	}

	// First attempt should have -p 70
	if !containsArg(commands[0], "-p") {
		t.Errorf("first attempt should include -p, got: %v", commands[0])
	}

	// Retry should NOT have -p
	if containsArg(commands[1], "-p") {
		t.Errorf("retry should not include -p, got: %v", commands[1])
	}
}

// TestDetachedReviewFallsBackWithoutPercentage verifies the same fallback
// behavior for single-file review mode.
func TestDetachedReviewFallsBackWithoutPercentage(t *testing.T) {
	origTmux := os.Getenv("TMUX")
	os.Setenv("TMUX", "/tmp/tmux-test/default,12345,0")
	defer os.Setenv("TMUX", origTmux)

	origRun := runCommand
	origLook := lookPath
	origResolve := resolveExec
	defer func() {
		runCommand = origRun
		lookPath = origLook
		resolveExec = origResolve
	}()

	var commands [][]string
	lookPath = func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}
	resolveExec = func() (string, error) {
		return "/usr/local/bin/crit", nil
	}
	runCommand = func(cmd *exec.Cmd) error {
		commands = append(commands, cmd.Args)
		if containsArg(cmd.Args, "-p") {
			return fmt.Errorf("exit status 1")
		}
		return nil
	}

	reviewWait = false
	defer func() { reviewWait = false }()

	tmp, err := os.CreateTemp("", "crit-test-*.md")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	err = runDetachedReview(tmp.Name())
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if len(commands) != 2 {
		t.Fatalf("expected 2 commands (failed split + retry), got %d: %v", len(commands), commands)
	}

	if !containsArg(commands[0], "-p") {
		t.Errorf("first attempt should include -p, got: %v", commands[0])
	}
	if containsArg(commands[1], "-p") {
		t.Errorf("retry should not include -p, got: %v", commands[1])
	}
}

func containsArg(args []string, target string) bool {
	for _, a := range args {
		if a == target {
			return true
		}
	}
	return false
}

// TestDetachedCodeReviewPassesBaseFlag verifies that --base is forwarded
// to the child crit process in the tmux split command.
func TestDetachedCodeReviewPassesBaseFlag(t *testing.T) {
	origTmux := os.Getenv("TMUX")
	os.Setenv("TMUX", "/tmp/tmux-test/default,12345,0")
	defer os.Setenv("TMUX", origTmux)

	commands, cleanup := stubExecDeps(t)
	defer cleanup()

	reviewDetach = true
	reviewWait = false
	reviewCode = true
	reviewBase = "abc123"
	defer func() {
		reviewDetach = false
		reviewCode = false
		reviewBase = ""
	}()

	err := runCodeReview()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if len(*commands) != 1 {
		t.Fatalf("expected 1 command, got %d: %v", len(*commands), *commands)
	}

	// The last arg to split-window is the shell command string
	shellCmd := (*commands)[0][len((*commands)[0])-1]
	if !strings.Contains(shellCmd, "--base 'abc123'") {
		t.Errorf("expected --base flag in tmux command, got: %s", shellCmd)
	}
}

// TestDetachedCodeReviewOmitsBaseFlagWhenEmpty verifies that --base is NOT
// included in the tmux command when reviewBase is empty.
func TestDetachedCodeReviewOmitsBaseFlagWhenEmpty(t *testing.T) {
	origTmux := os.Getenv("TMUX")
	os.Setenv("TMUX", "/tmp/tmux-test/default,12345,0")
	defer os.Setenv("TMUX", origTmux)

	commands, cleanup := stubExecDeps(t)
	defer cleanup()

	reviewDetach = true
	reviewWait = false
	reviewCode = true
	reviewBase = ""
	defer func() {
		reviewDetach = false
		reviewCode = false
	}()

	err := runCodeReview()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if len(*commands) != 1 {
		t.Fatalf("expected 1 command, got %d: %v", len(*commands), *commands)
	}

	shellCmd := (*commands)[0][len((*commands)[0])-1]
	if strings.Contains(shellCmd, "--base") {
		t.Errorf("expected no --base flag in tmux command, got: %s", shellCmd)
	}
}

func TestPathResolution(t *testing.T) {
	rel := "relative/path/doc.md"
	abs, err := filepath.Abs(rel)
	if err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	expected := filepath.Join(cwd, rel)
	if abs != expected {
		t.Errorf("filepath.Abs(%q) = %q, want %q", rel, abs, expected)
	}
}
