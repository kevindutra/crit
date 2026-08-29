package git

import (
	"fmt"
	"os/exec"
	"strings"
	"unicode"

	"github.com/aymanbagabas/go-udiff/lcs"
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
	ChangedLines  map[int]bool            // added/modified line numbers (1-based) in new file
	InlineChanges map[int][]InlineSegment // inline changes keyed by new-file line number
	DeletedAfter  map[int][]DeletedLine   // deleted lines keyed by the new-file line they appear after (0 = before line 1)
}

// DeletedLine represents a line that was deleted from the old version.
type DeletedLine struct {
	OldLineNum int
	Content    string
	Inline     []InlineSegment
}

// InlineSegment is a changed or unchanged portion of a replacement line.
type InlineSegment struct {
	Content string
	Changed bool
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
		ChangedLines:  make(map[int]bool),
		InlineChanges: make(map[int][]InlineSegment),
		DeletedAfter:  make(map[int][]DeletedLine),
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
			annotateInlineChanges(info, frag)
		}
	}

	return info, nil
}

func annotateInlineChanges(info *DiffInfo, frag *gitdiff.TextFragment) {
	oldLine := int(frag.OldPosition)
	newLine := int(frag.NewPosition)
	for i := 0; i < len(frag.Lines); {
		if frag.Lines[i].Op != gitdiff.OpDelete {
			switch frag.Lines[i].Op {
			case gitdiff.OpAdd:
				newLine++
			case gitdiff.OpContext:
				oldLine++
				newLine++
			}
			i++
			continue
		}

		var deleted []struct {
			line    int
			content string
		}
		for i < len(frag.Lines) && frag.Lines[i].Op == gitdiff.OpDelete {
			deleted = append(deleted, struct {
				line    int
				content string
			}{oldLine, strings.TrimSuffix(frag.Lines[i].Line, "\n")})
			oldLine++
			i++
		}

		var added []struct {
			line    int
			content string
		}
		for i < len(frag.Lines) && frag.Lines[i].Op == gitdiff.OpAdd {
			added = append(added, struct {
				line    int
				content string
			}{newLine, strings.TrimSuffix(frag.Lines[i].Line, "\n")})
			newLine++
			i++
		}

		if len(deleted) != len(added) {
			continue
		}
		for j := range deleted {
			oldSegments, newSegments := inlineDiff(deleted[j].content, added[j].content)
			info.InlineChanges[added[j].line] = newSegments
			setDeletedInline(info, deleted[j].line, oldSegments)
		}
	}
}

func inlineDiff(before, after string) ([]InlineSegment, []InlineSegment) {
	beforeTokens := splitInlineTokens(before)
	afterTokens := splitInlineTokens(after)
	diffs := lcs.DiffLines(beforeTokens, afterTokens)
	if len(diffs) == 0 {
		return nil, nil
	}

	oldSegments := make([]InlineSegment, 0, len(diffs)*2+1)
	newSegments := make([]InlineSegment, 0, len(diffs)*2+1)
	lastEnd := 0
	for _, diff := range diffs {
		common := strings.Join(beforeTokens[lastEnd:diff.Start], "")
		oldSegments = appendInlineSegment(oldSegments, common, false)
		newSegments = appendInlineSegment(newSegments, common, false)
		oldSegments = appendInlineSegment(oldSegments, strings.Join(beforeTokens[diff.Start:diff.End], ""), true)
		newSegments = appendInlineSegment(newSegments, strings.Join(afterTokens[diff.ReplStart:diff.ReplEnd], ""), true)
		lastEnd = diff.End
	}
	common := strings.Join(beforeTokens[lastEnd:], "")
	oldSegments = appendInlineSegment(oldSegments, common, false)
	newSegments = appendInlineSegment(newSegments, common, false)
	return oldSegments, newSegments
}

// splitInlineTokens preserves each whitespace run with the word before it, so
// word-level matching can reconstruct the original line exactly.
func splitInlineTokens(text string) []string {
	if text == "" {
		return nil
	}

	var tokens []string
	start := 0
	previousWasSpace := false
	for i, r := range text {
		isSpace := unicode.IsSpace(r)
		if !isSpace && previousWasSpace {
			tokens = append(tokens, text[start:i])
			start = i
		}
		previousWasSpace = isSpace
	}
	return append(tokens, text[start:])
}

func appendInlineSegment(segments []InlineSegment, content string, changed bool) []InlineSegment {
	if content == "" {
		return segments
	}
	if len(segments) > 0 && segments[len(segments)-1].Changed == changed {
		segments[len(segments)-1].Content += content
		return segments
	}
	return append(segments, InlineSegment{Content: content, Changed: changed})
}

func setDeletedInline(info *DiffInfo, oldLine int, segments []InlineSegment) {
	for anchor := range info.DeletedAfter {
		for i := range info.DeletedAfter[anchor] {
			if info.DeletedAfter[anchor][i].OldLineNum == oldLine {
				info.DeletedAfter[anchor][i].Inline = segments
				return
			}
		}
	}
}

// diffNameStatus runs git diff --name-status and parses the output.
func diffNameStatus(ref string) ([]FileChange, error) {
	out, err := gitCommand("diff", ref, "--name-status")
	if err != nil {
		return nil, fmt.Errorf("git diff --name-status: %w", err)
	}

	var files []FileChange
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		status := parts[0]
		path := parts[1]

		fc := FileChange{Path: path}
		switch {
		case status == "M":
			fc.Status = StatusModified
		case status == "A":
			fc.Status = StatusAdded
		case status == "D":
			fc.Status = StatusDeleted
		case strings.HasPrefix(status, "R"):
			fc.Status = StatusRenamed
			if len(parts) >= 3 {
				fc.OldPath = parts[1]
				fc.Path = parts[2]
			}
		default:
			fc.Status = StatusModified
		}

		files = append(files, fc)
	}

	return files, nil
}

// detectBinaryFiles returns a set of paths that are binary.
func detectBinaryFiles(ref string) (map[string]bool, error) {
	out, err := gitCommand("diff", ref, "--numstat")
	if err != nil {
		return nil, fmt.Errorf("git diff --numstat: %w", err)
	}

	binaries := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		// Binary files show "-\t-\tpath"
		if strings.HasPrefix(line, "-\t-\t") {
			path := strings.TrimPrefix(line, "-\t-\t")
			binaries[path] = true
		}
	}

	return binaries, nil
}

// untrackedFiles returns untracked file paths.
func untrackedFiles() ([]string, error) {
	out, err := gitCommand("ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}

	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			paths = append(paths, line)
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
