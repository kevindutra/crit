package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/kevindutra/crit/internal/document"
	"github.com/kevindutra/crit/internal/review"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns
// everything written to it. Errors from the pipe are reported via t.Fatalf.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan []byte)
	go func() {
		b, _ := io.ReadAll(r)
		done <- b
	}()

	fn()
	w.Close()
	return string(<-done)
}

func TestStatusSingleFileShape(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Ensure --code flag from a prior test doesn't leak in.
	statusCode = false
	defer func() { statusCode = false }()

	docPath := filepath.Join(dir, "plan.md")
	os.WriteFile(docPath, []byte("# Test\n"), 0644)

	document.EnsureDirs()

	if err := review.Save(&review.ReviewState{File: docPath, Comments: []review.Comment{
		{ID: "c1", Line: 1, Body: "new"},
		{ID: "c2", Line: 2, Body: "old", Resolved: true},
	}}); err != nil {
		t.Fatalf("seeding review: %v", err)
	}

	out := captureStdout(t, func() {
		statusCmd.SetArgs([]string{docPath})
		if err := statusCmd.Execute(); err != nil {
			t.Fatalf("status: %v", err)
		}
	})

	var got review.CodeFileStatus
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decoding JSON: %v\nraw: %s", err, out)
	}

	if got.File != docPath {
		t.Errorf("File = %q, want %q", got.File, docPath)
	}
	if len(got.Comments) != 1 || got.Comments[0].ID != "c1" {
		t.Errorf("Comments = %+v, want only c1", got.Comments)
	}
	if len(got.ResolvedComments) != 1 || got.ResolvedComments[0].ID != "c2" {
		t.Errorf("ResolvedComments = %+v, want only c2", got.ResolvedComments)
	}

	// Single-file output must be a CodeFileStatus, not the aggregate wrapper.
	var asAggregate map[string]any
	_ = json.Unmarshal([]byte(out), &asAggregate)
	if _, hasFiles := asAggregate["files"]; hasFiles {
		t.Error(`single-file status should not have "files" wrapper`)
	}
}

func TestStatusCodeAggregateShape(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	document.EnsureDirs()

	fileA := filepath.Join(dir, "a.go")
	fileB := filepath.Join(dir, "b.go")
	os.WriteFile(fileA, []byte("package a\n"), 0644)
	os.WriteFile(fileB, []byte("package b\n"), 0644)

	if err := review.Save(&review.ReviewState{File: fileA, Comments: []review.Comment{
		{ID: "a1"}, {ID: "a2", Resolved: true},
	}}); err != nil {
		t.Fatalf("seeding a: %v", err)
	}
	if err := review.Save(&review.ReviewState{File: fileB, Comments: []review.Comment{
		{ID: "b1"},
	}}); err != nil {
		t.Fatalf("seeding b: %v", err)
	}
	if err := review.SaveSession(&review.CodeReviewSession{Files: []string{fileA, fileB}}); err != nil {
		t.Fatalf("saving session: %v", err)
	}

	defer func() { statusCode = false }()
	out := captureStdout(t, func() {
		statusCmd.SetArgs([]string{"--code"})
		if err := statusCmd.Execute(); err != nil {
			t.Fatalf("status --code: %v", err)
		}
	})

	var got review.CodeReviewStatus
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decoding JSON: %v\nraw: %s", err, out)
	}

	if len(got.Files) != 2 {
		t.Fatalf("Files: got %d, want 2", len(got.Files))
	}
	if got.TotalComments != 2 {
		t.Errorf("TotalComments = %d, want 2", got.TotalComments)
	}
	if got.TotalResolvedComments != 1 {
		t.Errorf("TotalResolvedComments = %d, want 1", got.TotalResolvedComments)
	}

	// Each per-file entry has the same shape as single-file output.
	for _, f := range got.Files {
		if f.File == "" {
			t.Errorf("Files entry missing File field: %+v", f)
		}
	}
}

func TestFormatReviewSummary(t *testing.T) {
	cases := []struct {
		name string
		in   *review.ReviewState
		want string
	}{
		{
			name: "no resolved",
			in: &review.ReviewState{Comments: []review.Comment{
				{ID: "a"}, {ID: "b"},
			}},
			want: "Review complete. 2 new.\n",
		},
		{
			name: "mixed",
			in: &review.ReviewState{Comments: []review.Comment{
				{ID: "a"},
				{ID: "b", Resolved: true},
				{ID: "c", Resolved: true},
			}},
			want: "Review complete. 1 new · 2 resolved.\n",
		},
		{
			name: "all resolved",
			in: &review.ReviewState{Comments: []review.Comment{
				{ID: "a", Resolved: true},
			}},
			want: "Review complete. 0 new · 1 resolved.\n",
		},
		{
			name: "empty",
			in:   &review.ReviewState{},
			want: "Review complete. 0 new.\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatReviewSummary(tc.in)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
