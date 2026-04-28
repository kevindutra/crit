package review

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/kevindutra/crit/internal/document"
	"gopkg.in/yaml.v3"
)

func Load(docPath string) (*ReviewState, error) {
	reviewPath := document.ReviewPath(docPath) // .yaml
	data, err := os.ReadFile(reviewPath)
	if err != nil && os.IsNotExist(err) {
		// Try legacy JSON path
		jsonPath := strings.TrimSuffix(reviewPath, ".yaml") + ".json"
		jsonData, jsonErr := os.ReadFile(jsonPath)
		if jsonErr != nil {
			if os.IsNotExist(jsonErr) {
				return &ReviewState{
					File:     docPath,
					Comments: []Comment{},
				}, nil
			}
			return nil, fmt.Errorf("reading review: %w", jsonErr)
		}
		// Migrate: parse JSON, save as YAML, remove JSON
		var state ReviewState
		if err := json.Unmarshal(jsonData, &state); err != nil {
			return nil, fmt.Errorf("parsing legacy JSON review: %w", err)
		}
		if saveErr := Save(&state); saveErr == nil {
			os.Remove(jsonPath)
		}
		return &state, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading review: %w", err)
	}

	var state ReviewState
	if err := yaml.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing review YAML: %w", err)
	}
	return &state, nil
}

func Save(state *ReviewState) error {
	if err := document.EnsureDirs(); err != nil {
		return err
	}

	reviewPath := document.ReviewPath(state.File)
	lockPath := reviewPath + ".lock"

	fileLock := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	locked, err := fileLock.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("could not acquire lock on %s — another process may be writing. Try again", lockPath)
	}
	defer fileLock.Unlock()

	data, err := yaml.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshaling review: %w", err)
	}

	tmpPath := reviewPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, reviewPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}

	_ = filepath.Dir(lockPath)
	_ = os.Remove(lockPath)

	return nil
}

func (s *ReviewState) AddComment(c Comment) {
	s.Comments = append(s.Comments, c)
}

// MarkAllResolved flips Resolved=true on every unresolved comment and reports
// whether anything changed, so callers can skip a Save when there's no work.
func (s *ReviewState) MarkAllResolved() bool {
	changed := false
	for i := range s.Comments {
		if !s.Comments[i].Resolved {
			s.Comments[i].Resolved = true
			changed = true
		}
	}
	return changed
}

// Counts returns the unresolved and resolved comment totals without
// allocating, for callers that only need counts (vs Partition, which copies).
func (s *ReviewState) Counts() (newCount, resolvedCount int) {
	for _, c := range s.Comments {
		if c.Resolved {
			resolvedCount++
		} else {
			newCount++
		}
	}
	return
}

func (s *ReviewState) DeleteComment(id string) {
	for i, c := range s.Comments {
		if c.ID == id {
			s.Comments = append(s.Comments[:i], s.Comments[i+1:]...)
			return
		}
	}
}
