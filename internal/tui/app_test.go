package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
