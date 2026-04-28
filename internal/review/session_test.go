package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevindutra/crit/internal/document"
)

func TestPartition(t *testing.T) {
	cases := []struct {
		name           string
		comments       []Comment
		wantUnresolved int
		wantResolved   int
	}{
		{name: "empty"},
		{
			name:           "all_unresolved",
			comments:       []Comment{{ID: "a"}, {ID: "b"}},
			wantUnresolved: 2,
		},
		{
			name:         "all_resolved",
			comments:     []Comment{{ID: "a", Resolved: true}, {ID: "b", Resolved: true}},
			wantResolved: 2,
		},
		{
			name: "mixed",
			comments: []Comment{
				{ID: "a"},
				{ID: "b", Resolved: true},
				{ID: "c"},
				{ID: "d", Resolved: true},
			},
			wantUnresolved: 2,
			wantResolved:   2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := &ReviewState{Comments: tc.comments}
			unresolved, resolved := Partition(state)
			if len(unresolved) != tc.wantUnresolved {
				t.Errorf("unresolved: got %d, want %d", len(unresolved), tc.wantUnresolved)
			}
			if len(resolved) != tc.wantResolved {
				t.Errorf("resolved: got %d, want %d", len(resolved), tc.wantResolved)
			}
			for _, c := range unresolved {
				if c.Resolved {
					t.Errorf("unresolved slice contained resolved comment %q", c.ID)
				}
			}
			for _, c := range resolved {
				if !c.Resolved {
					t.Errorf("resolved slice contained unresolved comment %q", c.ID)
				}
			}
		})
	}
}

func TestAggregateStatusPartitions(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	document.EnsureDirs()

	fileA := filepath.Join(dir, "a.go")
	fileB := filepath.Join(dir, "b.go")
	os.WriteFile(fileA, []byte("package a\n"), 0644)
	os.WriteFile(fileB, []byte("package b\n"), 0644)

	stateA := &ReviewState{File: fileA, Comments: []Comment{
		{ID: "a1", Line: 1, Body: "new"},
		{ID: "a2", Line: 2, Body: "old", Resolved: true},
	}}
	stateB := &ReviewState{File: fileB, Comments: []Comment{
		{ID: "b1", Line: 1, Body: "new"},
		{ID: "b2", Line: 2, Body: "new"},
	}}
	if err := Save(stateA); err != nil {
		t.Fatalf("saving a: %v", err)
	}
	if err := Save(stateB); err != nil {
		t.Fatalf("saving b: %v", err)
	}

	if err := SaveSession(&CodeReviewSession{Files: []string{fileA, fileB}}); err != nil {
		t.Fatalf("saving session: %v", err)
	}

	status, err := AggregateStatus()
	if err != nil {
		t.Fatalf("AggregateStatus: %v", err)
	}

	if status.TotalComments != 3 {
		t.Errorf("TotalComments = %d, want 3", status.TotalComments)
	}
	if status.TotalResolvedComments != 1 {
		t.Errorf("TotalResolvedComments = %d, want 1", status.TotalResolvedComments)
	}
	if len(status.LoadErrors) != 0 {
		t.Errorf("unexpected LoadErrors: %v", status.LoadErrors)
	}
	if len(status.Files) != 2 {
		t.Fatalf("Files: got %d, want 2", len(status.Files))
	}

	for _, f := range status.Files {
		switch f.File {
		case fileA:
			if len(f.Comments) != 1 || f.Comments[0].ID != "a1" {
				t.Errorf("a.Comments = %+v", f.Comments)
			}
			if len(f.ResolvedComments) != 1 || f.ResolvedComments[0].ID != "a2" {
				t.Errorf("a.ResolvedComments = %+v", f.ResolvedComments)
			}
		case fileB:
			if len(f.Comments) != 2 {
				t.Errorf("b.Comments = %+v", f.Comments)
			}
			if len(f.ResolvedComments) != 0 {
				t.Errorf("b.ResolvedComments should be empty, got %+v", f.ResolvedComments)
			}
		default:
			t.Errorf("unexpected file in status: %s", f.File)
		}
	}
}

func TestAggregateStatusSurfacesLoadErrors(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	document.EnsureDirs()

	good := filepath.Join(dir, "good.go")
	bad := filepath.Join(dir, "bad.go")
	os.WriteFile(good, []byte("package good\n"), 0644)
	os.WriteFile(bad, []byte("package bad\n"), 0644)

	if err := Save(&ReviewState{File: good, Comments: []Comment{{ID: "g1"}}}); err != nil {
		t.Fatalf("saving good: %v", err)
	}

	// Corrupt the bad file's review YAML so Load fails to parse.
	badYAML := document.ReviewPath(bad)
	if err := os.WriteFile(badYAML, []byte("not: [valid: yaml"), 0644); err != nil {
		t.Fatalf("seeding bad YAML: %v", err)
	}

	if err := SaveSession(&CodeReviewSession{Files: []string{good, bad}}); err != nil {
		t.Fatalf("saving session: %v", err)
	}

	status, err := AggregateStatus()
	if err != nil {
		t.Fatalf("AggregateStatus: %v", err)
	}

	if len(status.Files) != 1 || status.Files[0].File != good {
		t.Errorf("expected only good file in Files, got %+v", status.Files)
	}
	if status.TotalComments != 1 {
		t.Errorf("TotalComments = %d, want 1 (only the good file)", status.TotalComments)
	}
	if len(status.LoadErrors) != 1 {
		t.Fatalf("LoadErrors = %v, want 1 entry", status.LoadErrors)
	}
	if !strings.Contains(status.LoadErrors[0], bad) {
		t.Errorf("LoadErrors[0] = %q, expected to mention %q", status.LoadErrors[0], bad)
	}
}
