package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/kevindutra/crit/internal/review"
)

var statusCode bool

var statusCmd = &cobra.Command{
	Use:   "status [file]",
	Short: "Show review status as JSON",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if statusCode {
			return runCodeStatus()
		}

		if len(args) == 0 {
			return fmt.Errorf("file argument required (use --code for aggregate code review status)")
		}

		filePath := args[0]

		state, err := review.Load(filePath)
		if err != nil {
			return fmt.Errorf("loading review state: %w", err)
		}

		unresolved, resolved := review.Partition(state)
		out := review.CodeFileStatus{
			File:             state.File,
			Comments:         unresolved,
			ResolvedComments: resolved,
		}

		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(out); err != nil {
			return fmt.Errorf("encoding JSON: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().BoolVar(&statusCode, "code", false, "show aggregate status for code review session")
}

func runCodeStatus() error {
	status, err := review.AggregateStatus()
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(status); err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}

	return nil
}
