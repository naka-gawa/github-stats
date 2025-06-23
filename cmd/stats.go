package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/montanaflynn/stats"
	"github.com/naka-gawa/github-stats/internal/gateway"
	"github.com/naka-gawa/github-stats/internal/usecase"
	"github.com/spf13/cobra"
)

// LeadTimePercentiles defines the structure for percentile data.
type LeadTimePercentiles struct {
	P99 float64 `json:"p99_hours"`
	P95 float64 `json:"p95_hours"`
	P90 float64 `json:"p90_hours"`
	P75 float64 `json:"p75_hours"`
	P50 float64 `json:"p50_hours"` // Median
}

// OutputRepoStats defines the structure for the final JSON output.
type OutputRepoStats struct {
	Name                string               `json:"name"`
	Commits             int                  `json:"commits"`
	CreatedPRs          int                  `json:"created_prs"`
	ReviewedPRs         int                  `json:"reviewed_prs"`
	AnalyzedPRCount     int                  `json:"analyzed_pr_count,omitempty"`
	LeadTimePercentiles *LeadTimePercentiles `json:"lead_time_percentiles_hours,omitempty"`
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Aggregates GitHub user activity and outputs as JSON",
	Long:  `Aggregates activity (commits, created/reviewed PRs) for a specified GitHub user and organization, and outputs the result in JSON format.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		verbose, _ := cmd.InheritedFlags().GetBool("verbose")
		logger := log.New(io.Discard, "", log.LstdFlags)
		if verbose {
			logger.SetOutput(os.Stderr)
		}

		org, _ := cmd.Flags().GetString("org")
		user, _ := cmd.Flags().GetString("user")
		fromStr, _ := cmd.Flags().GetString("from")
		toStr, _ := cmd.Flags().GetString("to")
		calculateLeadTime, _ := cmd.Flags().GetBool("lead-time")
		token := os.Getenv("GITHUB_TOKEN")
		if token == "" {
			fmt.Fprintln(os.Stderr, "Error: GITHUB_TOKEN environment variable is not set.")
			os.Exit(1)
		}

		// Build date range query strings.
		var commitDateRange, prDateRange string
		if fromStr != "" || toStr != "" {
			const githubDateLayout = "2006-01-02"
			const inputDateLayout = "2006/01/02"
			fromQuery, toQuery := "*", "*"
			if fromStr != "" {
				t, err := time.Parse(inputDateLayout, fromStr)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Invalid --from date format: %v\n", err)
					os.Exit(1)
				}
				fromQuery = t.Format(githubDateLayout)
			}
			if toStr != "" {
				t, err := time.Parse(inputDateLayout, toStr)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Invalid --to date format: %v\n", err)
					os.Exit(1)
				}
				toQuery = t.Format(githubDateLayout)
			}
			commitDateRange = fmt.Sprintf(" author-date:%s..%s", fromQuery, toQuery)
			prDateRange = fmt.Sprintf(" created:%s..%s", fromQuery, toQuery)
		}

		githubGateway, err := gateway.NewGitHubGateway(token, logger)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize GitHub gateway: %v\n", err)
			os.Exit(1)
		}
		aggregator := usecase.NewAggregator(githubGateway, logger)

		domainResults, err := aggregator.Aggregate(ctx, org, user, commitDateRange, prDateRange, calculateLeadTime)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to aggregate stats: %v\n", err)
			os.Exit(1)
		}

		outputResults := make([]OutputRepoStats, 0, len(domainResults))
		for _, repoStat := range domainResults {
			outputStat := OutputRepoStats{
				Name:        repoStat.Name,
				Commits:     repoStat.Commits,
				CreatedPRs:  repoStat.CreatedPRs,
				ReviewedPRs: repoStat.ReviewedPRs,
			}

			// Calculate percentiles if lead time data is available.
			if calculateLeadTime && len(repoStat.LeadTimeToLastReviewSeconds) > 0 {
				data := stats.Float64Data(repoStat.LeadTimeToLastReviewSeconds)
				outputStat.AnalyzedPRCount = len(data)

				p99, _ := stats.Percentile(data, 99)
				p95, _ := stats.Percentile(data, 95)
				p90, _ := stats.Percentile(data, 90)
				p75, _ := stats.Percentile(data, 75)
				p50, _ := stats.Percentile(data, 50)

				outputStat.LeadTimePercentiles = &LeadTimePercentiles{
					P99: p99 / 3600, // Convert seconds to hours
					P95: p95 / 3600,
					P90: p90 / 3600,
					P75: p75 / 3600,
					P50: p50 / 3600,
				}
			}
			outputResults = append(outputResults, outputStat)
		}

		// Marshal the final results into a pretty-printed JSON string.
		jsonData, err := json.MarshalIndent(outputResults, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to marshal results to JSON: %v\n", err)
			os.Exit(1)
		}

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
	statsCmd.Flags().Bool("lead-time", true, "Calculate and include PR review lead time percentiles (slower)")
}
