package cli

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

//go:embed agent/gemini/crit.md
var geminiAgentContent embed.FS

var setupGeminiProject bool
var setupGeminiForce bool

var setupGeminiCmd = &cobra.Command{
	Use:   "setup-gemini",
	Short: "Install Gemini CLI agents for crit review workflow",
	Long:  "Installs the crit agent to ~/.gemini/agents/crit.md (or .gemini/ in current directory with --project).",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		var baseDir string

		if setupGeminiProject {
			baseDir = ".gemini"
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("could not determine home directory: %w", err)
			}
			baseDir = filepath.Join(home, ".gemini")
		}

		scope := "globally"
		if setupGeminiProject {
			scope = "for this project"
		}

		// 1. Install the Agent
		agentTargetDir := filepath.Join(baseDir, "agents")
		agentTargetPath := filepath.Join(agentTargetDir, "crit.md")

		if err := installFile(agentTargetDir, agentTargetPath, "agent/gemini/crit.md", "crit agent", scope, setupGeminiForce); err != nil {
			return err
		}

		fmt.Println("\nSuccess! You can now use crit with Gemini CLI:")
		fmt.Println("  - Use @crit to start a review")
		return nil
	},
}

func installFile(targetDir, targetPath, embedPath, displayName, scope string, force bool) error {
	if !force {
		if _, err := os.Stat(targetPath); err == nil {
			fmt.Printf("Skipping %s (already exists, use --force to overwrite)\n", displayName)
			return nil
		}
	}

	content, err := geminiAgentContent.ReadFile(embedPath)
	if err != nil {
		return fmt.Errorf("reading embedded %s: %w", displayName, err)
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", targetDir, err)
	}

	if err := os.WriteFile(targetPath, content, 0644); err != nil {
		return fmt.Errorf("writing %s file: %w", displayName, err)
	}

	fmt.Printf("Installed %s %s to %s\n", displayName, scope, targetPath)
	return nil
}

func init() {
	rootCmd.AddCommand(setupGeminiCmd)
	setupGeminiCmd.Flags().BoolVar(&setupGeminiProject, "project", false, "install to .gemini/ in the current directory instead of globally")
	setupGeminiCmd.Flags().BoolVar(&setupGeminiForce, "force", false, "overwrite existing files")
}
