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

// Fetcher defines the behavior of a gateway for fetching information from GitHub.
// By defining an interface, we can easily mock this gateway in our tests.
type Fetcher interface {
	FetchCommits(ctx context.Context, org, user, dateRange string) (map[string]int, error)
	FetchCreatedPRs(ctx context.Context, org, user, dateRange string) (map[string]int, error)
	FetchReviewedPRs(ctx context.Context, org, user, dateRange string) (map[string]int, error)
}

// GitHubGateway is the concrete implementation of the Fetcher interface.
type GitHubGateway struct {
	restClient    *github.Client
	graphqlClient *githubv4.Client
	logger        *log.Logger
}

// searchIssuesQuery defines the structure for the GraphQL response.
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

// FetchCommits retrieves commit information using the REST API and returns a map of repository names to their commit counts.
func (g *GitHubGateway) FetchCommits(ctx context.Context, org, user, dateRange string) (map[string]int, error) {
	g.logger.Println("[1/3] Fetching commit data using REST API...")
	query := fmt.Sprintf("org:%s author:%s%s", org, user, dateRange)
	opts := &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
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

// FetchCreatedPRs retrieves created pull request information using the GraphQL API.
func (g *GitHubGateway) FetchCreatedPRs(ctx context.Context, org, user, dateRange string) (map[string]int, error) {
	g.logger.Println("[2/3] Fetching created PR data...")
	query := fmt.Sprintf("org:%s author:%s is:pr%s", org, user, dateRange)
	return g.fetchPRs(ctx, query)
}

// FetchReviewedPRs retrieves reviewed pull request information using the GraphQL API.
func (g *GitHubGateway) FetchReviewedPRs(ctx context.Context, org, user, dateRange string) (map[string]int, error) {
	g.logger.Println("[3/3] Fetching reviewed PR data...")
	query := fmt.Sprintf("org:%s reviewed-by:%s is:pr%s", org, user, dateRange)
	return g.fetchPRs(ctx, query)
}

// fetchPRs is a helper function to avoid duplicating the GraphQL query logic.
func (g *GitHubGateway) fetchPRs(ctx context.Context, query string) (map[string]int, error) {
	variables := map[string]interface{}{
		"query":  githubv4.String(query),
		"cursor": (*githubv4.String)(nil),
	}
	prCounts := make(map[string]int)
	for {
		var q searchIssuesQuery
		if err := g.graphqlClient.Query(ctx, &q, variables); err != nil {
			return nil, fmt.Errorf("failed to execute GraphQL query: %w", err)
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
		g.logger.Println("  Fetching next page of pull requests...")
	}
	g.logger.Printf("Completed fetching pull requests for query: %s\n", query)
	return prCounts, nil
}
