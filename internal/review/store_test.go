package review

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevindutra/crit/internal/document"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	docPath := filepath.Join(dir, "plan.md")
	os.WriteFile(docPath, []byte("# Test\n"), 0644)

	document.EnsureDirs()

	// Load returns empty state for new file
	state, err := Load(docPath)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if len(state.Comments) != 0 {
		t.Error("expected no comments")
	}

	// Add comment and save
	state.AddComment(Comment{ID: "c1", Line: 1, Body: "test comment", })
	if err := Save(state); err != nil {
		t.Fatalf("saving: %v", err)
	}

	// Verify file is YAML
	reviewPath := document.ReviewPath(docPath)
	if !strings.HasSuffix(reviewPath, ".yaml") {
		t.Errorf("expected .yaml extension, got %s", reviewPath)
	}
	data, err := os.ReadFile(reviewPath)
	if err != nil {
		t.Fatalf("reading review file: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "{") {
		t.Error("review file looks like JSON, expected YAML")
	}

	// Reload and verify
	loaded, err := Load(docPath)
	if err != nil {
		t.Fatalf("reloading: %v", err)
	}
	if len(loaded.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(loaded.Comments))
	}
	if loaded.Comments[0].Body != "test comment" {
		t.Errorf("comment body mismatch: %s", loaded.Comments[0].Body)
	}
}

func TestMultilineBody(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	docPath := filepath.Join(dir, "plan.md")
	os.WriteFile(docPath, []byte("# Test\n"), 0644)

	document.EnsureDirs()

	state, _ := Load(docPath)
	multiline := "First line.\nSecond line.\nThird line."
	state.AddComment(Comment{ID: "c1", Line: 1, Body: multiline, })
	if err := Save(state); err != nil {
		t.Fatalf("saving: %v", err)
	}

	// Verify YAML uses block scalar (no \n escapes)
	data, _ := os.ReadFile(document.ReviewPath(docPath))
	content := string(data)
	if strings.Contains(content, `\n`) {
		t.Error("YAML should not contain escaped newlines")
	}

	// Round-trip preserves multiline
	loaded, err := Load(docPath)
	if err != nil {
		t.Fatalf("reloading: %v", err)
	}
	if loaded.Comments[0].Body != multiline {
		t.Errorf("multiline body mismatch:\ngot:  %q\nwant: %q", loaded.Comments[0].Body, multiline)
	}
}

func TestMigrationFromJSON(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	docPath := filepath.Join(dir, "plan.md")
	os.WriteFile(docPath, []byte("# Test\n"), 0644)

	document.EnsureDirs()

	// Write a legacy JSON review file
	state := &ReviewState{
		File: docPath,
		Comments: []Comment{
			{ID: "legacy1", Line: 1, Body: "old comment", },
		},
	}
	yamlPath := document.ReviewPath(docPath)
	jsonPath := strings.TrimSuffix(yamlPath, ".yaml") + ".json"
	jsonData, _ := json.MarshalIndent(state, "", "  ")
	os.WriteFile(jsonPath, jsonData, 0644)

	// Load should find JSON, migrate to YAML, delete JSON
	loaded, err := Load(docPath)
	if err != nil {
		t.Fatalf("loading with migration: %v", err)
	}
	if len(loaded.Comments) != 1 {
		t.Fatalf("expected 1 comment after migration, got %d", len(loaded.Comments))
	}
	if loaded.Comments[0].Body != "old comment" {
		t.Errorf("comment body mismatch after migration: %s", loaded.Comments[0].Body)
	}

	// JSON file should be deleted
	if _, err := os.Stat(jsonPath); !os.IsNotExist(err) {
		t.Error("legacy JSON file should have been deleted after migration")
	}

	// YAML file should exist
	if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
		t.Error("YAML file should exist after migration")
	}

	if loaded.Comments[0].Resolved {
		t.Error("legacy JSON comment should default to Resolved=false, got true")
	}
}

func TestResolvedRoundTrip(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	docPath := filepath.Join(dir, "plan.md")
	os.WriteFile(docPath, []byte("# Test\n"), 0644)

	document.EnsureDirs()

	state, _ := Load(docPath)
	state.AddComment(Comment{ID: "r1", Line: 1, Body: "resolved", Resolved: true})
	state.AddComment(Comment{ID: "r2", Line: 2, Body: "open"})
	if err := Save(state); err != nil {
		t.Fatalf("saving: %v", err)
	}

	loaded, err := Load(docPath)
	if err != nil {
		t.Fatalf("reloading: %v", err)
	}
	if len(loaded.Comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(loaded.Comments))
	}
	byID := map[string]Comment{}
	for _, c := range loaded.Comments {
		byID[c.ID] = c
	}
	if !byID["r1"].Resolved {
		t.Error("r1 should be Resolved=true after round-trip")
	}
	if byID["r2"].Resolved {
		t.Error("r2 should be Resolved=false after round-trip")
	}
}

func TestLegacyYAMLDefaultsResolvedFalse(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	docPath := filepath.Join(dir, "plan.md")
	os.WriteFile(docPath, []byte("# Test\n"), 0644)

	document.EnsureDirs()

	// Hand-write a YAML file with no `resolved:` field.
	yamlPath := document.ReviewPath(docPath)
	legacyYAML := `file: ` + docPath + `
comments:
  - id: legacy
    line: 1
    content_snippet: ""
    body: old
    created_at: 0001-01-01T00:00:00Z
`
	if err := os.WriteFile(yamlPath, []byte(legacyYAML), 0644); err != nil {
		t.Fatalf("seeding legacy YAML: %v", err)
	}

	loaded, err := Load(docPath)
	if err != nil {
		t.Fatalf("loading legacy YAML: %v", err)
	}
	if len(loaded.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(loaded.Comments))
	}
	if loaded.Comments[0].Resolved {
		t.Error("legacy YAML comment with no `resolved` field should default to false")
	}
}
