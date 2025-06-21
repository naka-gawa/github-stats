// Package cmd contains all the CLI commands for the application,
// built using the Cobra library.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/naka-gawa/github-stats/internal/gateway"
	"github.com/naka-gawa/github-stats/internal/usecase"
	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Aggregates GitHub user activity and outputs as JSON",
	Long:  `Aggregates activity (commits, created/reviewed PRs) for a specified GitHub user and organization, and outputs the result in JSON format.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()

		// Get the verbose flag from the root command to set up the logger.
		// The flag is now correctly defined on rootCmd.
		verbose, _ := cmd.InheritedFlags().GetBool("verbose")
		logger := log.New(io.Discard, "", log.LstdFlags) // Default: discard all logs.
		if verbose {
			logger.SetOutput(os.Stderr) // If verbose, log to standard error.
		}

		// Get other flags.
		org, _ := cmd.Flags().GetString("org")
		user, _ := cmd.Flags().GetString("user")
		fromStr, _ := cmd.Flags().GetString("from")
		toStr, _ := cmd.Flags().GetString("to")
		token := os.Getenv("GITHUB_TOKEN")
		if token == "" {
			fmt.Fprintln(os.Stderr, "Error: GITHUB_TOKEN environment variable is not set.")
			os.Exit(1)
		}

		// Build date range query strings.
		// NOTE: Commit search uses "author-date", while PR search uses "created".
		var commitDateRange, prDateRange string
		if fromStr != "" || toStr != "" {
			const githubDateLayout = "2006-01-02"
			const inputDateLayout = "2006/01/02"
			fromQuery := "*"
			if fromStr != "" {
				fromTime, err := time.Parse(inputDateLayout, fromStr)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Invalid --from date format. Please use YYYY/MM/DD. Error: %v\n", err)
					os.Exit(1)
				}
				fromQuery = fromTime.Format(githubDateLayout)
			}
			toQuery := "*"
			if toStr != "" {
				toTime, err := time.Parse(inputDateLayout, toStr)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Invalid --to date format. Please use YYYY/MM/DD. Error: %v\n", err)
					os.Exit(1)
				}
				toQuery = toTime.Format(githubDateLayout)
			}
			// Note: The leading space is important for concatenation.
			commitDateRange = fmt.Sprintf(" author-date:%s..%s", fromQuery, toQuery)
			prDateRange = fmt.Sprintf(" created:%s..%s", fromQuery, toQuery)
		}

		// Inject dependencies and run the main business logic.
		githubGateway, err := gateway.NewGitHubGateway(token, logger)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create GitHub gateway: %v\n", err)
			os.Exit(1)
		}
		aggregator := usecase.NewAggregator(githubGateway, logger)

		// Pass the correct date ranges to the aggregator
		results, err := aggregator.Aggregate(ctx, org, user, commitDateRange, prDateRange)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to aggregate stats: %v\n", err)
			os.Exit(1)
		}

		// Marshal the results into a pretty-printed JSON string.
		jsonData, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to marshal results to JSON: %v\n", err)
			os.Exit(1)
		}

		// Print the final JSON to standard output.
		fmt.Println(string(jsonData))
	},
}

func init() {
	rootCmd.AddCommand(statsCmd)
	statsCmd.PersistentFlags().StringP("org", "o", "", "Target GitHub organization name (required)")
	statsCmd.PersistentFlags().StringP("user", "u", "", "Target GitHub user name (required)")
	statsCmd.MarkPersistentFlagRequired("org")
	statsCmd.MarkPersistentFlagRequired("user")
	statsCmd.Flags().String("from", "", "Start date for stats (YYYY/MM/DD)")
	statsCmd.Flags().String("to", "", "End date for stats (YYYY/MM/DD)")
}
