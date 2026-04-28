package review

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kevindutra/crit/internal/document"
	"gopkg.in/yaml.v3"
)

const sessionFile = ".crit/code-review.yaml"

// CodeReviewSession tracks which files belong to the current code review.
type CodeReviewSession struct {
	Files     []string  `yaml:"files"`
	DiffBase  string    `yaml:"diff_base"`
	CreatedAt time.Time `yaml:"created_at"`
}

// SaveSession writes the session manifest.
func SaveSession(session *CodeReviewSession) error {
	if err := document.EnsureDirs(); err != nil {
		return err
	}

	data, err := yaml.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshaling session: %w", err)
	}

	dir := filepath.Dir(sessionFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating session dir: %w", err)
	}

	if err := os.WriteFile(sessionFile, data, 0644); err != nil {
		return fmt.Errorf("writing session file: %w", err)
	}

	return nil
}

// LoadSession reads the current code review session.
func LoadSession() (*CodeReviewSession, error) {
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no active code review session (run `crit review --code` first)")
		}
		return nil, fmt.Errorf("reading session: %w", err)
	}

	var session CodeReviewSession
	if err := yaml.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("parsing session: %w", err)
	}

	return &session, nil
}

// CodeFileStatus represents a single file's review status in aggregate output.
type CodeFileStatus struct {
	File             string    `json:"file"`
	Comments         []Comment `json:"comments"`
	ResolvedComments []Comment `json:"resolved_comments,omitempty"`
}

// CodeReviewStatus is the aggregate status for all files in a code review.
type CodeReviewStatus struct {
	Files                 []CodeFileStatus `json:"files"`
	TotalComments         int              `json:"total_comments"`
	TotalResolvedComments int              `json:"total_resolved,omitempty"`
	LoadErrors            []string         `json:"load_errors,omitempty"`
}

// Partition splits a ReviewState's comments into unresolved and resolved
// slices based on the Resolved flag.
func Partition(state *ReviewState) (unresolved, resolved []Comment) {
	for _, c := range state.Comments {
		if c.Resolved {
			resolved = append(resolved, c)
		} else {
			unresolved = append(unresolved, c)
		}
	}
	return unresolved, resolved
}

// AggregateStatus loads ReviewState for all files in the current session and
// partitions each into unresolved/resolved comments.
func AggregateStatus() (*CodeReviewStatus, error) {
	session, err := LoadSession()
	if err != nil {
		return nil, err
	}

	result := &CodeReviewStatus{}
	for _, file := range session.Files {
		state, err := Load(file)
		if err != nil {
			result.LoadErrors = append(result.LoadErrors, fmt.Sprintf("%s: %v", file, err))
			continue
		}
		unresolved, resolved := Partition(state)
		result.Files = append(result.Files, CodeFileStatus{
			File:             file,
			Comments:         unresolved,
			ResolvedComments: resolved,
		})
		result.TotalComments += len(unresolved)
		result.TotalResolvedComments += len(resolved)
	}

	return result, nil
}
