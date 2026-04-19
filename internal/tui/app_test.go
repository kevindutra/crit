package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kevindutra/crit/internal/review"
)

func setupAppWithDoc(t *testing.T, content string) AppModel {
	t.Helper()
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	app := NewApp(testFile)
	app.tabs = []FileTab{{path: testFile}}
	app.activeTab = 0
	updated, _ := app.Update(docRenderedMsg{})
	a := updated.(AppModel)
	a.contentViewport.SetHeight(5)
	return a
}

func pressKey(app AppModel, code rune) AppModel {
	updated, _ := app.Update(tea.KeyPressMsg{Code: code})
	switch v := updated.(type) {
	case AppModel:
		return v
	case *AppModel:
		return *v
	}
	panic("unexpected model type")
}

func TestNavigationHome(t *testing.T) {
	lines := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	app := setupAppWithDoc(t, lines)
	app.tabs[0].cursorLine = 8

	app = pressKey(app, tea.KeyHome)

	if app.tabs[0].cursorLine != 1 {
		t.Errorf("Home: expected cursorLine 1, got %d", app.tabs[0].cursorLine)
	}
}

func TestNavigationEnd(t *testing.T) {
	lines := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	app := setupAppWithDoc(t, lines)

	app = pressKey(app, tea.KeyEnd)

	tab := app.tabs[0]
	if tab.cursorLine != tab.doc.LineCount() {
		t.Errorf("End: expected cursorLine %d, got %d", tab.doc.LineCount(), tab.cursorLine)
	}
}

func TestNavigationPgDown(t *testing.T) {
	lines := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	app := setupAppWithDoc(t, lines)
	app.tabs[0].cursorLine = 1

	app = pressKey(app, tea.KeyPgDown)

	tab := app.tabs[0]
	expected := 1 + app.contentViewport.Height()/2
	if tab.cursorLine != expected {
		t.Errorf("PgDown: expected cursorLine %d, got %d", expected, tab.cursorLine)
	}
}

func TestNavigationPgUp(t *testing.T) {
	lines := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	app := setupAppWithDoc(t, lines)
	app.tabs[0].cursorLine = 3

	app = pressKey(app, tea.KeyPgUp)

	tab := app.tabs[0]
	if tab.cursorLine != 1 {
		t.Errorf("PgUp: expected cursorLine 1 (clamped), got %d", tab.cursorLine)
	}
}

func TestNavigationPgDownClampsAtBottom(t *testing.T) {
	lines := "line1\nline2\nline3\n"
	app := setupAppWithDoc(t, lines)
	app.tabs[0].cursorLine = 3

	app = pressKey(app, tea.KeyPgDown)

	tab := app.tabs[0]
	if tab.cursorLine != tab.doc.LineCount() {
		t.Errorf("PgDown clamp: expected cursorLine %d, got %d", tab.doc.LineCount(), tab.cursorLine)
	}
}

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
