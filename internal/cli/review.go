package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/kevindutra/crit/internal/git"
	"github.com/kevindutra/crit/internal/review"
	"github.com/kevindutra/crit/internal/tui"
)

var reviewDetach bool
var reviewWait bool
var reviewCode bool
var reviewBase string

// The following function variables allow tests to replace shell interactions
// without actually shelling out.
var runCommand = func(cmd *exec.Cmd) error {
	return cmd.Run()
}

var lookPath = exec.LookPath

var resolveExec = func() (string, error) {
	return resolveExecutable()
}

var reviewCmd = &cobra.Command{
	Use:   "review [file]",
	Short: "Launch interactive TUI review",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if reviewCode {
			return runCodeReview()
		}

		if len(args) == 0 {
			return fmt.Errorf("file argument required (use --code for code review mode)")
		}

		filePath := args[0]

		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", filePath)
		}

		// --wait without --detach: warn and ignore
		if reviewWait && !reviewDetach {
			fmt.Fprintln(os.Stderr, "crit: --wait requires --detach; ignoring --wait")
			reviewWait = false
		}

		if reviewDetach {
			return runDetachedReview(filePath)
		}

		model := tui.NewApp(filePath)
		p := tea.NewProgram(model)
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("TUI error: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(reviewCmd)
	reviewCmd.Flags().BoolVar(&reviewDetach, "detach", false, "open review in a tmux split pane")
	reviewCmd.Flags().BoolVar(&reviewWait, "wait", false, "block until the detached review completes (requires --detach)")
	reviewCmd.Flags().BoolVar(&reviewCode, "code", false, "review code changes (multi-file mode)")
	reviewCmd.Flags().StringVar(&reviewBase, "base", "", "base commit to diff against (used with --code)")
}

func runCodeReview() error {
	// --wait without --detach: warn and ignore
	if reviewWait && !reviewDetach {
		fmt.Fprintln(os.Stderr, "crit: --wait requires --detach; ignoring --wait")
		reviewWait = false
	}

	if reviewDetach {
		return runDetachedCodeReview()
	}

	if !git.IsGitRepo() {
		return fmt.Errorf("crit review --code requires a git repository")
	}

	var ref string
	var files []git.FileChange
	var err error

	if reviewBase != "" {
		ref = reviewBase
		files, err = git.ChangedFilesFrom(ref)
		if err != nil {
			return fmt.Errorf("detecting changed files from %s: %w", ref, err)
		}
		if len(files) == 0 {
			return fmt.Errorf("no changed files found relative to %s", ref)
		}
	} else {
		ref = "HEAD"
		files, err = git.ChangedFiles()
		if err != nil {
			return fmt.Errorf("detecting changed files: %w", err)
		}
		if len(files) == 0 {
			// Interactive fallback: try other refs
			var fallbackErr error
			ref, files, fallbackErr = interactiveFallback()
			if fallbackErr != nil {
				return fallbackErr
			}
		}
	}

	// Save session manifest
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	session := &review.CodeReviewSession{
		Files:     paths,
		DiffBase:  ref,
		CreatedAt: time.Now(),
	}
	if err := review.SaveSession(session); err != nil {
		fmt.Fprintf(os.Stderr, "crit: warning: could not save session: %v\n", err)
	}

	model := tui.NewCodeReviewApp(files, ref)
	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}

func runDetachedCodeReview() error {
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("--detach requires a tmux session (TMUX environment variable not set)")
	}

	tmuxBin, err := lookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux binary not found on PATH: %w", err)
	}

	critBin, err := resolveExec()
	if err != nil {
		return fmt.Errorf("resolving crit binary path: %w", err)
	}

	channel := fmt.Sprintf("crit-review-%d", os.Getpid())
	baseFlag := ""
	if reviewBase != "" {
		baseFlag = " --base " + shellEscape(reviewBase)
	}
	critCmd := fmt.Sprintf("CRIT_DETACHED=1 %s review --code%s ; tmux wait-for -S %s",
		shellEscape(critBin), baseFlag, channel)

	splitCmd := exec.Command(tmuxBin, "split-window", "-h", "-p", "70", critCmd)
	if err := runCommand(splitCmd); err != nil {
		// Retry without -p flag — percentage sizing fails when parent pane
		// size isn't available (e.g. invoked from a subprocess like Claude Code)
		splitCmd = exec.Command(tmuxBin, "split-window", "-h", critCmd)
		if err := runCommand(splitCmd); err != nil {
			return fmt.Errorf("failed to open tmux pane: %w", err)
		}
	}

	fmt.Fprintln(os.Stderr, "Opened code review in tmux pane")

	if reviewWait {
		waitCmd := exec.Command(tmuxBin, "wait-for", channel)
		if err := runCommand(waitCmd); err != nil {
			return fmt.Errorf("review pane terminated abnormally")
		}

		fmt.Fprintln(os.Stdout, "Code review complete.")
	}

	return nil
}

func interactiveFallback() (string, []git.FileChange, error) {
	// Try common alternatives in order
	alternatives := []struct {
		label string
		ref   string
	}{
		{"last commit (HEAD~1)", "HEAD~1"},
		{"base branch (main)", "main"},
	}

	for _, alt := range alternatives {
		files, err := git.ChangedFilesFrom(alt.ref)
		if err != nil {
			continue
		}
		if len(files) > 0 {
			fmt.Fprintf(os.Stderr, "No unstaged changes found. Using %s.\n", alt.label)
			return alt.ref, files, nil
		}
	}

	return "", nil, fmt.Errorf("no changed files found")
}

func runDetachedReview(filePath string) error {
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("--detach requires a tmux session (TMUX environment variable not set)")
	}

	tmuxBin, err := lookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux binary not found on PATH: %w", err)
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("resolving absolute path: %w", err)
	}

	critBin, err := resolveExec()
	if err != nil {
		return fmt.Errorf("resolving crit binary path: %w", err)
	}

	channel := fmt.Sprintf("crit-review-%d", os.Getpid())
	critCmd := buildTmuxPaneCommand(critBin, absPath, channel)

	splitCmd := exec.Command(tmuxBin, "split-window", "-h", "-p", "70", critCmd)
	if err := runCommand(splitCmd); err != nil {
		// Retry without -p flag — percentage sizing fails when parent pane
		// size isn't available (e.g. invoked from a subprocess like Claude Code)
		splitCmd = exec.Command(tmuxBin, "split-window", "-h", critCmd)
		if err := runCommand(splitCmd); err != nil {
			return fmt.Errorf("failed to open tmux pane: %w", err)
		}
	}

	fmt.Fprintln(os.Stderr, "Opened review in tmux pane")

	if reviewWait {
		waitCmd := exec.Command(tmuxBin, "wait-for", channel)
		if err := runCommand(waitCmd); err != nil {
			return fmt.Errorf("review pane terminated abnormally")
		}

		state, err := review.Load(absPath)
		if err != nil {
			return fmt.Errorf("reading review state: %w", err)
		}
		fmt.Fprint(os.Stdout, formatReviewSummary(state))
	}

	return nil
}

// formatReviewSummary returns the trailing user-facing line printed after a
// detached --wait review completes. Resolved comments are reported separately
// from new (unresolved) ones so callers can tell which counts matter.
func formatReviewSummary(state *review.ReviewState) string {
	unresolved, resolved := review.Partition(state)
	if len(resolved) > 0 {
		return fmt.Sprintf("Review complete. %d new · %d resolved.\n", len(unresolved), len(resolved))
	}
	return fmt.Sprintf("Review complete. %d new.\n", len(unresolved))
}

// resolveExecutable returns the absolute path to the currently running binary.
func resolveExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// buildTmuxPaneCommand constructs the shell command string to run inside a tmux split pane.
// It runs crit review for the given file, then signals the wait-for channel on completion.
func buildTmuxPaneCommand(critBin, absPath, channel string) string {
	return fmt.Sprintf("CRIT_DETACHED=1 %s review %s ; tmux wait-for -S %s",
		shellEscape(critBin), shellEscape(absPath), channel)
}

// shellEscape escapes a string for safe embedding in a POSIX shell command.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
