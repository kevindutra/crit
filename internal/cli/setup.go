package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kevindutra/crit/internal/agent"
)

var setupAgentFlag string
var setupProject bool
var setupForce bool

// agentAliases maps common alternative names to adapter slugs.
var agentAliases = map[string]string{
	"claude-code": "claude",
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Install crit skills for your coding agent",
	Long:  "Installs crit instruction files for the specified agent. Auto-detects the agent if --agent is not provided.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		var adapter agent.Adapter

		if setupAgentFlag != "" {
			name := resolveAgentAlias(setupAgentFlag)
			adapter = agent.ByName(name)
			if adapter == nil {
				return fmt.Errorf("unknown agent: %s (available: %s)", setupAgentFlag, agent.Names())
			}
		} else {
			adapter = agent.Detect()
			if adapter == nil {
				return fmt.Errorf("could not detect agent; specify with --agent (available: %s)", agent.Names())
			}
			fmt.Fprintf(os.Stderr, "Detected agent: %s\n", adapter.Name())
		}

		scope := agent.ScopeGlobal
		if setupProject {
			scope = agent.ScopeProject
		}

		basePath := adapter.SetupBasePath(scope)

		for _, f := range adapter.SetupFiles(scope) {
			targetPath := filepath.Join(basePath, f.RelPath)

			if f.Mode == agent.ModeAppendSection {
				if err := writeSection(targetPath, f.Content); err != nil {
					return fmt.Errorf("writing %s: %w", f.RelPath, err)
				}
				fmt.Printf("Updated %s with crit section\n", targetPath)
				continue
			}

			if !setupForce {
				if _, err := os.Stat(targetPath); err == nil {
					fmt.Printf("Skipping %s (already exists, use --force to overwrite)\n", f.RelPath)
					continue
				}
			}

			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("creating directory for %s: %w", f.RelPath, err)
			}

			if err := os.WriteFile(targetPath, f.Content, 0644); err != nil {
				return fmt.Errorf("writing %s: %w", f.RelPath, err)
			}

			scopeLabel := "globally"
			if setupProject {
				scopeLabel = "for this project"
			}
			fmt.Printf("Installed %s %s to %s\n", f.RelPath, scopeLabel, targetPath)
		}

		return nil
	},
}

// sectionPattern matches the crit delimited section in AGENTS.md.
var sectionPattern = regexp.MustCompile(`(?s)<!-- crit:start -->.*?<!-- crit:end -->\n?`)

// writeSection appends or replaces the crit section in an existing file.
// If the file doesn't exist, it creates it with just the section content.
func writeSection(path string, content []byte) error {
	existing, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(path, content, 0644)
		}
		return err
	}

	if sectionPattern.Match(existing) {
		updated := sectionPattern.ReplaceAll(existing, content)
		return os.WriteFile(path, updated, 0644)
	}

	// Append with a blank line separator
	separator := "\n"
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		separator = "\n\n"
	}

	updated := string(existing) + separator + string(content)
	return os.WriteFile(path, []byte(updated), 0644)
}

// resolveAgentAlias resolves common aliases before looking up the adapter.
func resolveAgentAlias(name string) string {
	lower := strings.ToLower(name)
	if resolved, ok := agentAliases[lower]; ok {
		return resolved
	}
	return name
}

func init() {
	rootCmd.AddCommand(setupCmd)
	setupCmd.Flags().StringVar(&setupAgentFlag, "agent", "", fmt.Sprintf("agent to configure (available: %s)", agent.Names()))
	setupCmd.Flags().BoolVar(&setupProject, "project", false, "install to the current project directory instead of globally")
	setupCmd.Flags().BoolVar(&setupForce, "force", false, "overwrite existing files")
}
