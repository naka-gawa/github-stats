// Package cmd contains all the CLI commands for the application,
// built using the Cobra library.
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "github-stats",
	Short: "A CLI tool to aggregate GitHub user contributions.",
	Long: `github-stats is a CLI tool that aggregates a user's contributions
(commits, PRs created/reviewed) per repository within a GitHub organization.
You can specify a date range to filter the results.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Add a persistent flag for verbose output, available to all commands.
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose/debug logging")
}
