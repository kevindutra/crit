package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/kevindutra/crit/internal/document"
	gitpkg "github.com/kevindutra/crit/internal/git"
	"github.com/kevindutra/crit/internal/review"
)

func TestNewApp(t *testing.T) {
	app := NewApp("test.md")
	if app.filePath != "test.md" {
		t.Errorf("expected filePath 'test.md', got %s", app.filePath)
	}
}

func TestDocRenderedMsg_LoadsExistingComments(t *testing.T) {
	// Create a temp directory and test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	if err := os.WriteFile(testFile, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Save a review state with comments for that file
	comment := review.Comment{
		ID:             "test-comment-1",
		Line:           1,
		ContentSnippet: "package main",
		Body:           "This is a test comment",
		CreatedAt:      time.Now(),
	}
	state := &review.ReviewState{
		File:     testFile,
		Comments: []review.Comment{comment},
	}
	if err := review.Save(state); err != nil {
		t.Fatalf("failed to save review state: %v", err)
	}

	// Create an AppModel with a tab for the test file
	app := NewApp(testFile)
	app.tabs = []FileTab{
		{path: testFile},
	}
	app.activeTab = 0

	// Process the docRenderedMsg
	updatedModel, _ := app.Update(docRenderedMsg{})
	updatedApp := updatedModel.(AppModel)

	// Assert the tab's state contains the previously saved comment
	tab := updatedApp.tabs[0]
	if tab.state == nil {
		t.Fatal("expected tab.state to be non-nil after docRenderedMsg")
	}
	if len(tab.state.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(tab.state.Comments))
	}
	if tab.state.Comments[0].ID != "test-comment-1" {
		t.Errorf("expected comment ID 'test-comment-1', got %s", tab.state.Comments[0].ID)
	}
	if tab.state.Comments[0].Body != "This is a test comment" {
		t.Errorf("expected comment body 'This is a test comment', got %s", tab.state.Comments[0].Body)
	}
}

func newScrollTestApp(path string, lines []string, isMarkdown bool, width, height int) AppModel {
	app := NewApp(path)
	app.tabs[0].doc = &document.Document{
		Path:    path,
		Content: strings.Join(lines, "\n"),
		Lines:   lines,
	}
	app.tabs[0].state = &review.ReviewState{}
	app.tabs[0].isMarkdown = isMarkdown
	if !isMarkdown {
		app.tabs[0].chromaLines = lines
	}
	app.contentViewport.SetWidth(width)
	app.contentViewport.SetHeight(height)
	return app
}

func TestExtraLinesPerDocLineChromaLinesWrap(t *testing.T) {
	longLine := strings.Repeat("x", 500)
	lines := []string{"short", longLine, "short"}
	app := newScrollTestApp("test.go", lines, false, 80, 24)
	app.tabs[0].chromaLines[1] = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(longLine)
	app.tabs[0].changedLines = map[int]bool{2: true}

	counts := app.extraLinesPerDocLine()
	if counts[2] == 0 {
		t.Error("expected extra lines for wrapped Chroma-highlighted line")
	}
	if counts[1] != 0 || counts[3] != 0 {
		t.Errorf("expected no extra lines for short lines, got %v", counts)
	}

	app.rebuildContent()
	if got := strings.Count(app.contentViewport.View(), "x"); got != len(longLine) {
		t.Errorf("rendered %d of %d highlighted characters", got, len(longLine))
	}
}

func TestExtraLinesPerDocLineMarkdownWraps(t *testing.T) {
	longLine := strings.Repeat("word ", 100)
	lines := []string{"short", longLine, "short"}
	app := newScrollTestApp("test.md", lines, true, 80, 24)

	counts := app.extraLinesPerDocLine()
	if counts[2] == 0 {
		t.Error("expected extra lines for wrapped Markdown line")
	}
	if counts[1] != 0 || counts[3] != 0 {
		t.Errorf("expected no extra lines for short lines, got %v", counts)
	}
}

func TestScrollToChunkSourceWithLongLines(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = strings.Repeat("x", 500)
	}
	app := newScrollTestApp("test.go", lines, false, 80, 10)
	app.tabs[0].changedLines = map[int]bool{30: true}
	app.rebuildContent()

	app.scrollToChunk(changeChunk{startLine: 30, endLine: 30})

	wrappedLines := strings.Count(lipgloss.Wrap(lines[0], app.contentViewport.Width()-8, ""), "\n") + 1
	want := (30 - chunkScrollPadding - 1) * wrappedLines
	if got := app.contentViewport.YOffset(); got != want {
		t.Errorf("expected YOffset %d, got %d", want, got)
	}
}

func TestDeletedMarkdownLinesWrap(t *testing.T) {
	longLine := strings.Repeat("word ", 100)
	lines := deletedDisplayLines(longLine, "", nil, true, 72)
	if len(lines) < 2 {
		t.Fatalf("expected deleted Markdown line to wrap, got %d display line", len(lines))
	}
}

func TestInlineDiffDisplayLinesUseNeutralCommonBackground(t *testing.T) {
	initAdaptiveStyles(true)
	segments := []gitpkg.InlineSegment{
		{Content: "common ", Changed: false},
		{Content: "added", Changed: true},
	}

	rendered := strings.Join(inlineDiffDisplayLines(segments, true, 80, diffCommonTextBg, diffAddedTextBg), "\n")
	commonBackground := bgToAnsi(diffCommonTextBg.GetBackground())
	changedBackground := bgToAnsi(diffAddedTextBg.GetBackground())
	if commonBackground == changedBackground {
		t.Fatal("expected common and changed text to use distinct backgrounds")
	}
	if commonBackground == bgToAnsi(diffChangedLineBg.GetBackground()) ||
		commonBackground == bgToAnsi(diffDeletedLineBg.GetBackground()) {
		t.Fatal("expected replacement-line common text to use a neutral background")
	}
	if !strings.Contains(rendered, commonBackground) || !strings.Contains(rendered, changedBackground) {
		t.Errorf("rendered line does not contain both backgrounds: %q", rendered)
	}
}
