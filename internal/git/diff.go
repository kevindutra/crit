package git

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

// ChangeStatus represents the type of change to a file.
type ChangeStatus int

const (
	StatusModified ChangeStatus = iota
	StatusAdded
	StatusDeleted
	StatusRenamed
	StatusUntracked
	StatusBinary
)

// FileChange describes a single changed file.
type FileChange struct {
	Path    string
	Status  ChangeStatus
	OldPath string // for renames
}

// ChangedFiles returns files with changes relative to HEAD (staged + unstaged + untracked).
func ChangedFiles() ([]FileChange, error) {
	return ChangedFilesFrom("HEAD")
}

// ChangedFilesFrom returns files changed relative to a ref (branch, commit, HEAD~N).
func ChangedFilesFrom(ref string) ([]FileChange, error) {
	files, err := diffNameStatus(ref)
	if err != nil {
		return nil, err
	}

	// Detect binary files via --numstat (binary shows - - for counts)
	binaries, err := detectBinaryFiles(ref)
	if err != nil {
		return nil, err
	}

	for i := range files {
		if binaries[files[i].Path] {
			files[i].Status = StatusBinary
		}
	}

	// Add untracked files (only for HEAD-based diffs)
	if ref == "HEAD" {
		untracked, err := untrackedFiles()
		if err != nil {
			return nil, err
		}
		for _, path := range untracked {
			files = append(files, FileChange{
				Path:   path,
				Status: StatusUntracked,
			})
		}
	}

	return files, nil
}

// DiffInfo contains parsed diff information for a single file.
type DiffInfo struct {
	ChangedLines map[int]bool          // added/modified line numbers (1-based) in new file
	DeletedAfter map[int][]DeletedLine // deleted lines keyed by the new-file line they appear after (0 = before line 1)
}

// DeletedLine represents a line that was deleted from the old version.
type DeletedLine struct {
	OldLineNum int
	Content    string
}

// DiffFile returns full diff information for a file relative to the given ref.
func DiffFile(path string, ref string) (*DiffInfo, error) {
	out, err := gitCommand("diff", ref, "--", path)
	if err != nil {
		return nil, fmt.Errorf("git diff for %s: %w", path, err)
	}

	if strings.TrimSpace(out) == "" {
		return nil, nil
	}

	files, _, err := gitdiff.Parse(strings.NewReader(out))
	if err != nil {
		return nil, fmt.Errorf("parsing diff for %s: %w", path, err)
	}

	info := &DiffInfo{
		ChangedLines: make(map[int]bool),
		DeletedAfter: make(map[int][]DeletedLine),
	}

	for _, f := range files {
		for _, frag := range f.TextFragments {
			newLine := frag.NewPosition
			oldLine := frag.OldPosition
			// Track the last new-file line we've seen (for anchoring deletions)
			lastNewLine := int(newLine) - 1 // before the hunk starts

			for _, line := range frag.Lines {
				switch line.Op {
				case gitdiff.OpAdd:
					info.ChangedLines[int(newLine)] = true
					lastNewLine = int(newLine)
					newLine++
				case gitdiff.OpContext:
					lastNewLine = int(newLine)
					newLine++
					oldLine++
				case gitdiff.OpDelete:
					content := strings.TrimSuffix(line.Line, "\n")
					info.DeletedAfter[lastNewLine] = append(info.DeletedAfter[lastNewLine], DeletedLine{
						OldLineNum: int(oldLine),
						Content:    content,
					})
					oldLine++
				}
			}
		}
	}

	return info, nil
}

// diffNameStatus runs git diff --name-status and parses the output.
// It uses -z so paths with spaces, tabs, or non-ASCII characters arrive
// unquoted and NUL-separated.
func diffNameStatus(ref string) ([]FileChange, error) {
	out, err := gitCommand("diff", ref, "--name-status", "-z")
	if err != nil {
		return nil, fmt.Errorf("git diff --name-status: %w", err)
	}
	return parseNameStatusZ(out), nil
}

// parseNameStatusZ parses NUL-separated `git diff --name-status -z` output:
// each entry is a status token followed by one path, or two paths for
// renames and copies.
func parseNameStatusZ(out string) []FileChange {
	tokens := strings.Split(out, "\x00")

	var files []FileChange
	for i := 0; i+1 < len(tokens); {
		status := tokens[i]
		if status == "" {
			i++
			continue
		}

		fc := FileChange{Path: tokens[i+1]}
		i += 2
		switch {
		case status == "A":
			fc.Status = StatusAdded
		case status == "D":
			fc.Status = StatusDeleted
		case strings.HasPrefix(status, "R"), strings.HasPrefix(status, "C"):
			fc.Status = StatusRenamed
			if i < len(tokens) {
				fc.OldPath = fc.Path
				fc.Path = tokens[i]
				i++
			}
		default:
			fc.Status = StatusModified
		}

		files = append(files, fc)
	}

	return files
}

// detectBinaryFiles returns a set of paths that are binary.
func detectBinaryFiles(ref string) (map[string]bool, error) {
	out, err := gitCommand("diff", ref, "--numstat", "-z")
	if err != nil {
		return nil, fmt.Errorf("git diff --numstat: %w", err)
	}
	return parseNumstatZ(out), nil
}

// parseNumstatZ parses NUL-separated `git diff --numstat -z` output.
// Each entry is "added\tdeleted\tpath"; binary files show "-" for both
// counts. For renames and copies the path field is empty and the old and
// new paths follow as two separate NUL-terminated tokens.
func parseNumstatZ(out string) map[string]bool {
	binaries := make(map[string]bool)
	tokens := strings.Split(out, "\x00")
	for i := 0; i < len(tokens); i++ {
		parts := strings.SplitN(tokens[i], "\t", 3)
		if len(parts) < 3 {
			continue
		}

		binary := parts[0] == "-" && parts[1] == "-"
		path := parts[2]
		if path == "" && i+2 < len(tokens) {
			path = tokens[i+2]
			i += 2
		}
		if binary && path != "" {
			binaries[path] = true
		}
	}

	return binaries
}

// untrackedFiles returns untracked file paths.
func untrackedFiles() ([]string, error) {
	out, err := gitCommand("ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}

	var paths []string
	for _, path := range strings.Split(out, "\x00") {
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

// gitCommand runs a git command and returns stdout.
func gitCommand(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%s: %s", strings.Join(args, " "), string(exitErr.Stderr))
		}
		return "", err
	}
	return string(out), nil
}

// IsGitRepo checks if the current directory is inside a git repository.
func IsGitRepo() bool {
	_, err := gitCommand("rev-parse", "--is-inside-work-tree")
	return err == nil
}
