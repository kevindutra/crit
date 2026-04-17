package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kevindutra/crit/internal/review"
)

var clearCode bool

var clearCmd = &cobra.Command{
	Use:   "clear [file]",
	Short: "Clear all comments for a document or code review session",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if clearCode {
			return runCodeClear()
		}

		if len(args) == 0 {
			return fmt.Errorf("file argument required (use --code for code review session)")
		}

		filePath := args[0]

		state, err := review.Load(filePath)
		if err != nil {
			return fmt.Errorf("loading review state: %w", err)
		}

		count := len(state.Comments)
		state.Comments = []review.Comment{}

		if err := review.Save(state); err != nil {
			return fmt.Errorf("saving review: %w", err)
		}

		fmt.Printf("Cleared %d comment(s) for %s\n", count, filePath)
		return nil
	},
}

func runCodeClear() error {
	session, err := review.LoadSession()
	if err != nil {
		return err
	}

	total := 0
	for _, file := range session.Files {
		state, err := review.Load(file)
		if err != nil {
			continue
		}
		count := len(state.Comments)
		if count == 0 {
			continue
		}
		state.Comments = []review.Comment{}
		if err := review.Save(state); err != nil {
			return fmt.Errorf("saving review for %s: %w", file, err)
		}
		total += count
	}

	fmt.Printf("Cleared %d comment(s) across %d file(s)\n", total, len(session.Files))
	return nil
}

func init() {
	rootCmd.AddCommand(clearCmd)
	clearCmd.Flags().BoolVar(&clearCode, "code", false, "clear comments for all files in the code review session")
}
