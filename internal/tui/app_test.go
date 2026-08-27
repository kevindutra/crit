package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kevindutra/crit/internal/document"
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
	app.tabs[0].isMarkdown = isMarkdown
	if !isMarkdown {
		// Stand in for highlightCode output: one rendered line per doc line.
		app.tabs[0].chromaLines = lines
	}
	app.contentViewport.SetWidth(width)
	app.contentViewport.SetHeight(height)
	return app
}

func TestExtraLinesPerDocLine_ChromaLinesNotWrapped(t *testing.T) {
	longLine := strings.Repeat("x", 500)
	lines := []string{"short", longLine, "short"}
	app := newScrollTestApp("test.go", lines, false, 80, 24)

	counts := app.extraLinesPerDocLine()
	if len(counts) != 0 {
		t.Errorf("expected no extra lines for chroma-highlighted content, got %v", counts)
	}
}

func TestExtraLinesPerDocLine_MarkdownWraps(t *testing.T) {
	longLine := strings.Repeat("word ", 100) // 500 chars, wraps in markdown
	lines := []string{"short", longLine, "short"}
	app := newScrollTestApp("test.md", lines, true, 80, 24)

	counts := app.extraLinesPerDocLine()
	if counts[2] == 0 {
		t.Error("expected extra lines for wrapped markdown line, got none")
	}
	if counts[1] != 0 || counts[3] != 0 {
		t.Errorf("expected no extra lines for short lines, got %v", counts)
	}
}

func TestScrollToChunk_SourceWithLongLines(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = strings.Repeat("x", 500) // longer than the viewport width
	}
	app := newScrollTestApp("test.go", lines, false, 80, 10)
	app.tabs[0].changedLines = map[int]bool{30: true}
	app.rebuildContent()

	app.scrollToChunk(changeChunk{startLine: 30, endLine: 30})

	// Chunk start (line 30) minus chunkScrollPadding should sit at the top:
	// rendered offset = 25 since each doc line renders as exactly one line.
	want := 30 - chunkScrollPadding - 1
	if got := app.contentViewport.YOffset(); got != want {
		t.Errorf("expected YOffset %d, got %d", want, got)
	}
}
