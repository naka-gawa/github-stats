// Package usecase contains the business logic of the application.
package usecase

import (
	"context"
	"log"
	"sort"

	"github.com/naka-gawa/github-stats/internal/domain"
	"github.com/naka-gawa/github-stats/internal/gateway"
	"golang.org/x/sync/errgroup"
)

// Aggregator is the use case for aggregating GitHub stats.
// It orchestrates the fetching and combining of data.
type Aggregator struct {
	fetcher gateway.Fetcher
	logger  *log.Logger
}

// NewAggregator creates a new Aggregator instance.
func NewAggregator(fetcher gateway.Fetcher, logger *log.Logger) *Aggregator {
	return &Aggregator{
		fetcher: fetcher,
		logger:  logger,
	}
}

// Aggregate performs the main business logic.
// It fetches all required data concurrently from the gateway and aggregates it.
// The `calculateLeadTime` flag controls whether the expensive lead time query is executed.
func (a *Aggregator) Aggregate(ctx context.Context, org, user, commitDateRange, prDateRange string, calculateLeadTime bool) ([]*domain.RepoStats, error) {
	a.logger.Println("Usecase: Starting data aggregation...")

	var commitCounts, createdPRCounts, reviewedPRCounts map[string]int
	var leadTimesByRepo map[string][]gateway.PRLeadTimeData

	// Use an errgroup to fetch all data concurrently.
	eg, egCtx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		var err error
		commitCounts, err = a.fetcher.FetchCommits(egCtx, org, user, commitDateRange)
		return err
	})

	eg.Go(func() error {
		var err error
		createdPRCounts, err = a.fetcher.FetchCreatedPRs(egCtx, org, user, prDateRange)
		return err
	})

	eg.Go(func() error {
		var err error
		reviewedPRCounts, err = a.fetcher.FetchReviewedPRs(egCtx, org, user, prDateRange)
		return err
	})

	// Only fetch lead time data if requested.
	if calculateLeadTime {
		eg.Go(func() error {
			var err error
			leadTimesByRepo, err = a.fetcher.FetchPRLeadTimes(egCtx, org, user, prDateRange)
			return err
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	a.logger.Println("Usecase: All data fetched successfully.")

	// Merge all results into a single map.
	statsMap := make(map[string]*domain.RepoStats)

	// Helper function to ensure a map entry exists.
	ensureRepoStat := func(repoName string) {
		if _, ok := statsMap[repoName]; !ok {
			statsMap[repoName] = &domain.RepoStats{Name: repoName}
		}
	}

	for repoName, count := range commitCounts {
		ensureRepoStat(repoName)
		statsMap[repoName].Commits = count
	}
	for repoName, count := range createdPRCounts {
		ensureRepoStat(repoName)
		statsMap[repoName].CreatedPRs = count
	}
	for repoName, count := range reviewedPRCounts {
		ensureRepoStat(repoName)
		statsMap[repoName].ReviewedPRs = count
	}

	// Calculate and add lead times if the data was fetched.
	if calculateLeadTime {
		for repoName, leadTimeDataList := range leadTimesByRepo {
			ensureRepoStat(repoName)
			for _, data := range leadTimeDataList {
				// Calculate the duration from creation to the last review.
				duration := data.LastReviewedAt.Sub(data.CreatedAt)
				statsMap[repoName].LeadTimeToLastReviewSeconds = append(statsMap[repoName].LeadTimeToLastReviewSeconds, duration.Seconds())
			}
		}
	}

	// Convert the map to a slice and sort it by repository name for consistent output.
	sortedStats := make([]*domain.RepoStats, 0, len(statsMap))
	for _, repoStat := range statsMap {
		sortedStats = append(sortedStats, repoStat)
	}
	sort.Slice(sortedStats, func(i, j int) bool {
		return sortedStats[i].Name < sortedStats[j].Name
	})

	a.logger.Println("Usecase: Aggregation complete.")
	return sortedStats, nil
}
