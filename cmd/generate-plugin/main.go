// Command generate-plugin renders skill templates into static plugin files.
//
// Usage:
//
//	go run ./cmd/generate-plugin
//
// This writes the three skill files to plugin/crit/commands/ using the
// PluginConfig from the agent package, ensuring the plugin marketplace
// files stay in sync with the shared templates.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kevindutra/crit/internal/agent"
)

func main() {
	review, codeReview, planReview := agent.RenderSkills(agent.PluginConfig)

	files := []struct {
		name    string
		content []byte
	}{
		{"review.md", review},
		{"code-review.md", codeReview},
		{"plan-review.md", planReview},
	}

	dir := filepath.Join("plugin", "crit", "commands")
	for _, f := range files {
		path := filepath.Join(dir, f.name)
		if err := os.WriteFile(path, f.content, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("Generated %s\n", path)
	}
}
