package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kevindutra/crit/internal/review"
)

func TestNewApp(t *testing.T) {
	app := NewApp("test.md")
	if app.filePath != "test.md" {
		t.Errorf("expected filePath 'test.md', got %s", app.filePath)
	}
}

func TestDocRenderedMsg_MarksLoadedCommentsResolvedAndPersists(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	testFile := filepath.Join(tmpDir, "test.go")
	if err := os.WriteFile(testFile, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	state := &review.ReviewState{
		File: testFile,
		Comments: []review.Comment{{
			ID:             "test-comment-1",
			Line:           1,
			ContentSnippet: "package main",
			Body:           "This is a test comment",
			CreatedAt:      time.Now(),
		}},
	}
	if err := review.Save(state); err != nil {
		t.Fatalf("failed to save review state: %v", err)
	}

	app := NewApp(testFile)
	app.tabs = []FileTab{{path: testFile}}
	app.activeTab = 0

	updatedModel, _ := app.Update(docRenderedMsg{})
	updatedApp := updatedModel.(AppModel)
	if updatedApp.err != nil {
		t.Fatalf("unexpected err after docRenderedMsg: %v", updatedApp.err)
	}

	tab := updatedApp.tabs[0]
	if tab.state == nil {
		t.Fatal("expected tab.state to be non-nil after docRenderedMsg")
	}
	if len(tab.state.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(tab.state.Comments))
	}
	if !tab.state.Comments[0].Resolved {
		t.Error("expected loaded comment to be marked Resolved=true after migration")
	}

	reloaded, err := review.Load(testFile)
	if err != nil {
		t.Fatalf("reloading state: %v", err)
	}
	if len(reloaded.Comments) != 1 || !reloaded.Comments[0].Resolved {
		t.Errorf("on-disk state not migrated: %+v", reloaded.Comments)
	}
}

func TestDocRenderedMsg_PlaceholderTabsSkippedNoSave(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	realFile := filepath.Join(tmpDir, "real.go")
	binaryFile := filepath.Join(tmpDir, "image.png")
	deletedFile := filepath.Join(tmpDir, "removed.go")
	if err := os.WriteFile(realFile, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("write real: %v", err)
	}

	if err := review.Save(&review.ReviewState{
		File: realFile,
		Comments: []review.Comment{
			{ID: "r1", Line: 1, Body: "from prior session"},
		},
	}); err != nil {
		t.Fatalf("seeding real: %v", err)
	}

	app := NewApp(realFile)
	app.tabs = []FileTab{
		{path: realFile},
		{path: binaryFile, isBinary: true},
		{path: deletedFile, isDeleted: true},
	}
	app.activeTab = 0

	updatedModel, _ := app.Update(docRenderedMsg{})
	updatedApp := updatedModel.(AppModel)
	if updatedApp.err != nil {
		t.Fatalf("unexpected err: %v", updatedApp.err)
	}

	if !updatedApp.tabs[0].state.Comments[0].Resolved {
		t.Error("real tab: expected migration to mark comment resolved")
	}
	for _, idx := range []int{1, 2} {
		st := updatedApp.tabs[idx].state
		if st == nil {
			t.Errorf("tab %d: state should be empty placeholder, got nil", idx)
			continue
		}
		if len(st.Comments) != 0 {
			t.Errorf("tab %d: placeholder should have no comments, got %d", idx, len(st.Comments))
		}
	}
}

func TestVisibleCommentsFiltersResolved(t *testing.T) {
	state := &review.ReviewState{Comments: []review.Comment{
		{ID: "new1", Line: 1},
		{ID: "old1", Line: 2, Resolved: true},
		{ID: "new2", Line: 3},
		{ID: "old2", Line: 4, Resolved: true},
	}}
	tab := &FileTab{state: state}

	hidden := AppModel{showResolved: false}
	got := hidden.visibleComments(tab)
	if len(got) != 2 || got[0].ID != "new1" || got[1].ID != "new2" {
		t.Errorf("hidden mode: expected only new1 and new2, got %+v", got)
	}

	shown := AppModel{showResolved: true}
	got = shown.visibleComments(tab)
	if len(got) != 4 {
		t.Errorf("shown mode: expected all 4 comments, got %d", len(got))
	}
}

func TestVisibleCommentsNilState(t *testing.T) {
	m := AppModel{}
	if got := m.visibleComments(&FileTab{}); got != nil {
		t.Errorf("nil state should yield nil slice, got %+v", got)
	}
}

func TestExtraLinesPerDocLineSkipsResolvedWhenHidden(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	docPath := filepath.Join(tmpDir, "f.go")
	if err := os.WriteFile(docPath, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("writing doc: %v", err)
	}

	app := NewApp(docPath)
	app.tabs = []FileTab{{path: docPath}}
	app.activeTab = 0

	updatedModel, _ := app.Update(docRenderedMsg{})
	app = updatedModel.(AppModel)

	app.tabs[0].state.Comments = []review.Comment{
		{ID: "new", Line: 1, Body: "still active"},
		{ID: "old", Line: 1, Body: "from prior round", Resolved: true},
	}
	app.contentViewport.SetWidth(80)

	app.showResolved = false
	hiddenCounts := app.extraLinesPerDocLine()
	app.showResolved = true
	shownCounts := app.extraLinesPerDocLine()

	if hiddenCounts[1] >= shownCounts[1] {
		t.Errorf("hidden mode should reserve fewer rows for line 1: hidden=%d shown=%d",
			hiddenCounts[1], shownCounts[1])
	}
}

func TestCommentCountsAndSummary(t *testing.T) {
	cases := []struct {
		name         string
		comments     []review.Comment
		showResolved bool
		wantNew      int
		wantResolved int
		wantSummary  string
	}{
		{
			name:        "empty",
			comments:    nil,
			wantSummary: "0 new",
		},
		{
			name:        "only new",
			comments:    []review.Comment{{ID: "a"}, {ID: "b"}},
			wantNew:     2,
			wantSummary: "2 new",
		},
		{
			name: "mixed hidden",
			comments: []review.Comment{
				{ID: "a"}, {ID: "b"}, {ID: "r1", Resolved: true},
			},
			wantNew:      2,
			wantResolved: 1,
			wantSummary:  "2 new · 1 hidden",
		},
		{
			name: "mixed shown",
			comments: []review.Comment{
				{ID: "a"}, {ID: "r1", Resolved: true}, {ID: "r2", Resolved: true},
			},
			showResolved: true,
			wantNew:      1,
			wantResolved: 2,
			wantSummary:  "1 new · 2 resolved",
		},
		{
			name:         "all resolved hidden",
			comments:     []review.Comment{{ID: "r1", Resolved: true}},
			wantResolved: 1,
			wantSummary:  "0 new · 1 hidden",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tab := &FileTab{state: &review.ReviewState{Comments: tc.comments}}
			m := AppModel{showResolved: tc.showResolved}
			n, r := m.commentCounts(tab)
			if n != tc.wantNew || r != tc.wantResolved {
				t.Errorf("counts: got (%d,%d), want (%d,%d)", n, r, tc.wantNew, tc.wantResolved)
			}
			if got := m.formatCommentSummary(tab); got != tc.wantSummary {
				t.Errorf("summary: got %q, want %q", got, tc.wantSummary)
			}
		})
	}
}

func TestCommentCountsNilState(t *testing.T) {
	m := AppModel{}
	n, r := m.commentCounts(&FileTab{})
	if n != 0 || r != 0 {
		t.Errorf("nil state: got (%d,%d), want (0,0)", n, r)
	}
}
