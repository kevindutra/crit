package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var setupClaudeCmd = &cobra.Command{
	Use:    "setup-claude",
	Short:  "Install Claude Code skills (deprecated: use 'crit setup --agent claude')",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(os.Stderr, "crit: setup-claude is deprecated — use 'crit setup --agent claude' instead")
		setupAgentFlag = "claude"
		return setupCmd.RunE(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(setupClaudeCmd)
	// Reuse the same --project and --force vars from setup.go
	setupClaudeCmd.Flags().BoolVar(&setupProject, "project", false, "install to .claude/skills/ in the current directory instead of globally")
	setupClaudeCmd.Flags().BoolVar(&setupForce, "force", false, "overwrite existing files")
}
