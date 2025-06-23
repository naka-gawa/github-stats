// Package gateway provides a gateway to the GitHub API,
// abstracting away the underlying REST and GraphQL clients.
package gateway

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/go-github/v62/github"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"

	"github.com/gofri/go-github-ratelimit/github_ratelimit"
)

// PRLeadTimeData holds the necessary timestamps for calculating lead time for a single PR.
type PRLeadTimeData struct {
	CreatedAt      time.Time
	LastReviewedAt time.Time
}

// Fetcher defines the behavior of a gateway for fetching information from GitHub.
type Fetcher interface {
	FetchCommits(ctx context.Context, org, user, dateRange string) (map[string]int, error)
	FetchCreatedPRs(ctx context.Context, org, user, dateRange string) (map[string]int, error)
	FetchReviewedPRs(ctx context.Context, org, user, dateRange string) (map[string]int, error)
	// New method to fetch lead time data for pull requests.
	FetchPRLeadTimes(ctx context.Context, org, user, dateRange string) (map[string][]PRLeadTimeData, error)
}

// GitHubGateway is the concrete implementation of the Fetcher interface.
type GitHubGateway struct {
	restClient    *github.Client
	graphqlClient *githubv4.Client
	logger        *log.Logger
}

// searchIssuesQuery is for the simple PR count queries.
type searchIssuesQuery struct {
	Search struct {
		PageInfo struct {
			HasNextPage bool
			EndCursor   githubv4.String
		}
		Edges []struct {
			Node struct {
				Typename    string `graphql:"__typename"`
				PullRequest struct {
					Repository struct {
						NameWithOwner string
					}
				} `graphql:"... on PullRequest"`
			}
		}
	} `graphql:"search(query: $query, type: ISSUE, first: 100, after: $cursor)"`
}

// prLeadTimeQuery defines the structure for the more complex GraphQL query to fetch lead times.
type prLeadTimeQuery struct {
	Search struct {
		PageInfo struct {
			HasNextPage bool
			EndCursor   githubv4.String
		}
		Edges []struct {
			Node struct {
				Typename    string `graphql:"__typename"`
				PullRequest struct {
					Repository struct {
						NameWithOwner string
					}
					CreatedAt githubv4.DateTime
					Reviews   struct {
						Nodes []struct {
							SubmittedAt githubv4.DateTime
						}
					} `graphql:"reviews(first: 100, states: [COMMENTED, APPROVED, CHANGES_REQUESTED])"`
				} `graphql:"... on PullRequest"`
			}
		}
	} `graphql:"search(query: $query, type: ISSUE, first: 20, after: $cursor)"` // Use a smaller page size for this complex query
}

// NewGitHubGateway is a constructor that creates a new instance of GitHubGateway.
func NewGitHubGateway(token string, logger *log.Logger) (Fetcher, error) {
	rateLimitWaiter, err := github_ratelimit.NewRateLimitWaiter(nil, github_ratelimit.WithSingleSleepLimit(1*time.Hour, nil))
	if err != nil {
		return nil, fmt.Errorf("failed to create rate limit waiter: %w", err)
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := &http.Client{
		Transport: &oauth2.Transport{
			Base:   rateLimitWaiter,
			Source: ts,
		},
	}
	return &GitHubGateway{
		restClient:    github.NewClient(httpClient),
		graphqlClient: githubv4.NewClient(httpClient),
		logger:        logger,
	}, nil
}

func (g *GitHubGateway) FetchCommits(ctx context.Context, org, user, dateRange string) (map[string]int, error) {
	g.logger.Println("[1/4] Fetching commit data using REST API...")
	query := fmt.Sprintf("org:%s author:%s%s", org, user, dateRange)
	opts := &github.SearchOptions{ListOptions: github.ListOptions{PerPage: 100}}
	commitCounts := make(map[string]int)
	for {
		result, resp, err := g.restClient.Search.Commits(ctx, query, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to search commits with REST API: %w", err)
		}
		for _, commit := range result.Commits {
			repoName := commit.GetRepository().GetFullName()
			commitCounts[repoName]++
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
		g.logger.Println("  Fetching next page of commits...")
	}
	g.logger.Println("Completed fetching commit data.")
	return commitCounts, nil
}

func (g *GitHubGateway) FetchCreatedPRs(ctx context.Context, org, user, dateRange string) (map[string]int, error) {
	g.logger.Println("[2/4] Fetching created PR data...")
	query := fmt.Sprintf("org:%s author:%s is:pr%s", org, user, dateRange)
	return g.fetchPRCounts(ctx, query)
}

func (g *GitHubGateway) FetchReviewedPRs(ctx context.Context, org, user, dateRange string) (map[string]int, error) {
	g.logger.Println("[3/4] Fetching reviewed PR data...")
	query := fmt.Sprintf("org:%s reviewed-by:%s is:pr%s", org, user, dateRange)
	return g.fetchPRCounts(ctx, query)
}

func (g *GitHubGateway) fetchPRCounts(ctx context.Context, query string) (map[string]int, error) {
	variables := map[string]interface{}{"query": githubv4.String(query), "cursor": (*githubv4.String)(nil)}
	prCounts := make(map[string]int)
	for {
		var q searchIssuesQuery
		if err := g.graphqlClient.Query(ctx, &q, variables); err != nil {
			return nil, fmt.Errorf("failed to execute GraphQL query for counts: %w", err)
		}
		for _, edge := range q.Search.Edges {
			if repoName := edge.Node.PullRequest.Repository.NameWithOwner; repoName != "" {
				prCounts[repoName]++
			}
		}
		if !q.Search.PageInfo.HasNextPage {
			break
		}
		variables["cursor"] = githubv4.NewString(q.Search.PageInfo.EndCursor)
		g.logger.Println("  Fetching next page of pull requests for counts...")
	}
	g.logger.Printf("Completed fetching pull request counts for query: %s\n", query)
	return prCounts, nil
}

// FetchPRLeadTimes fetches PR creation and last review timestamps.
func (g *GitHubGateway) FetchPRLeadTimes(ctx context.Context, org, user, dateRange string) (map[string][]PRLeadTimeData, error) {
	g.logger.Println("[4/4] Fetching PR lead time data...")
	// We are looking for PRs authored by the user that are now merged or closed.
	query := fmt.Sprintf("org:%s author:%s is:pr is:closed%s", org, user, dateRange)

	variables := map[string]interface{}{
		"query":  githubv4.String(query),
		"cursor": (*githubv4.String)(nil),
	}

	leadTimesByRepo := make(map[string][]PRLeadTimeData)

	for {
		var q prLeadTimeQuery
		if err := g.graphqlClient.Query(ctx, &q, variables); err != nil {
			return nil, fmt.Errorf("failed to execute GraphQL query for lead times: %w", err)
		}

		for _, edge := range q.Search.Edges {
			prNode := edge.Node.PullRequest
			if edge.Node.Typename != "PullRequest" || len(prNode.Reviews.Nodes) == 0 {
				continue // Skip if not a PR or has no reviews.
			}

			// Find the latest review timestamp.
			lastReviewedAt := prNode.Reviews.Nodes[0].SubmittedAt.Time
			for _, review := range prNode.Reviews.Nodes[1:] {
				if review.SubmittedAt.After(lastReviewedAt) {
					lastReviewedAt = review.SubmittedAt.Time
				}
			}

			data := PRLeadTimeData{
				CreatedAt:      prNode.CreatedAt.Time,
				LastReviewedAt: lastReviewedAt,
			}

			repoName := prNode.Repository.NameWithOwner
			leadTimesByRepo[repoName] = append(leadTimesByRepo[repoName], data)
		}

		if !q.Search.PageInfo.HasNextPage {
			break
		}
		variables["cursor"] = q.Search.PageInfo.EndCursor
		g.logger.Println("  Fetching next page of PRs for lead time analysis...")
	}
	g.logger.Println("Completed fetching PR lead time data.")
	return leadTimesByRepo, nil
}
