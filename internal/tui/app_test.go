package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

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

func newApproveTestApp(t *testing.T, comments []review.Comment) AppModel {
	t.Helper()
	t.Chdir(t.TempDir())
	app := NewApp("test.go")
	app.tabs[0].state = &review.ReviewState{File: "test.go", Comments: comments}
	return app
}

func pressKeyCmd(app AppModel, code rune) (AppModel, tea.Cmd) {
	updated, cmd := app.Update(tea.KeyPressMsg{Code: code})
	switch v := updated.(type) {
	case AppModel:
		return v, cmd
	case *AppModel:
		return *v, cmd
	}
	panic("unexpected model type")
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func testComment() review.Comment {
	return review.Comment{ID: "c1", Line: 1, Body: "please fix", CreatedAt: time.Now()}
}

func TestQuitWithRemainingComments_PromptsApproval(t *testing.T) {
	app := newApproveTestApp(t, []review.Comment{testComment()})

	app, cmd := pressKeyCmd(app, 'q')

	if isQuit(cmd) {
		t.Fatal("expected quit to be intercepted by approval prompt")
	}
	if app.modal != approveModal {
		t.Fatalf("expected approveModal, got %v", app.modal)
	}
}

func TestApproveModal_ApproveClearsComments(t *testing.T) {
	app := newApproveTestApp(t, []review.Comment{testComment()})
	app, _ = pressKeyCmd(app, 'q')

	app, cmd := pressKeyCmd(app, 'y')

	if !isQuit(cmd) {
		t.Fatal("expected quit after approving")
	}
	if len(app.tabs[0].state.Comments) != 0 {
		t.Errorf("expected comments cleared, got %d", len(app.tabs[0].state.Comments))
	}
	state, err := review.Load("test.go")
	if err != nil {
		t.Fatalf("loading saved state: %v", err)
	}
	if len(state.Comments) != 0 {
		t.Errorf("expected persisted comments cleared, got %d", len(state.Comments))
	}
}

func TestApproveModal_DeclineKeepsComments(t *testing.T) {
	app := newApproveTestApp(t, []review.Comment{testComment()})
	app, _ = pressKeyCmd(app, 'q')

	app, cmd := pressKeyCmd(app, 'n')

	if !isQuit(cmd) {
		t.Fatal("expected quit after declining")
	}
	if len(app.tabs[0].state.Comments) != 1 {
		t.Errorf("expected comments kept, got %d", len(app.tabs[0].state.Comments))
	}
}

func TestApproveModal_EscReturnsToReview(t *testing.T) {
	app := newApproveTestApp(t, []review.Comment{testComment()})
	app, _ = pressKeyCmd(app, 'q')

	app, cmd := pressKeyCmd(app, tea.KeyEscape)

	if isQuit(cmd) {
		t.Fatal("expected esc to stay in the review")
	}
	if app.modal != noModal {
		t.Fatalf("expected modal dismissed, got %v", app.modal)
	}
	if len(app.tabs[0].state.Comments) != 1 {
		t.Errorf("expected comments kept, got %d", len(app.tabs[0].state.Comments))
	}
}

func TestQuitWithNewFeedback_NoPrompt(t *testing.T) {
	app := newApproveTestApp(t, []review.Comment{testComment()})
	app.newFeedback = true

	app, cmd := pressKeyCmd(app, 'q')

	if !isQuit(cmd) {
		t.Fatal("expected direct quit when new feedback was given")
	}
	if app.modal != noModal {
		t.Fatalf("expected no modal, got %v", app.modal)
	}
}

func TestQuitWithNoComments_NoPrompt(t *testing.T) {
	app := newApproveTestApp(t, nil)

	app, cmd := pressKeyCmd(app, 'q')

	if !isQuit(cmd) {
		t.Fatal("expected direct quit with no comments")
	}
	if app.modal != noModal {
		t.Fatalf("expected no modal, got %v", app.modal)
	}
}
