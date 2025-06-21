package gateway

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-github/v62/github"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestGateway creates a GitHubGateway that communicates with a mock HTTP server.
func setupTestGateway(t *testing.T, handler http.Handler) (*GitHubGateway, *httptest.Server) {
	server := httptest.NewServer(handler)

	// Setup REST client to point to the mock server.
	restClient := github.NewClient(server.Client())
	baseURL, err := url.Parse(server.URL + "/")
	require.NoError(t, err)
	restClient.BaseURL = baseURL

	// Use NewEnterpriseClient to point the GraphQL client to our mock server's URL.
	graphqlClient := githubv4.NewEnterpriseClient(server.URL, server.Client())
	logger := log.New(io.Discard, "", 0)

	gateway := &GitHubGateway{
		restClient:    restClient,
		graphqlClient: graphqlClient,
		logger:        logger,
	}

	return gateway, server
}

func TestGitHubGateway_FetchCommits(t *testing.T) {
	// This test remains the same as before.
	testCases := []struct {
		name           string
		handlerFunc    func(w http.ResponseWriter, r *http.Request)
		expectedMap    map[string]int
		expectError    bool
		expectedErrMsg string
	}{
		{
			name: "happy path - successfully fetches commits",
			handlerFunc: func(w http.ResponseWriter, r *http.Request) {
				assert.Contains(t, r.URL.String(), "/search/commits")
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, `{"total_count": 2, "items": [{"repository": {"full_name": "org/repo-a"}}, {"repository": {"full_name": "org/repo-b"}}]}`)
			},
			expectedMap: map[string]int{"org/repo-a": 1, "org/repo-b": 1},
			expectError: false,
		},
		{
			name: "error case - GitHub API returns an error",
			handlerFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, `{"message": "Internal Server Error"}`)
			},
			expectError:    true,
			expectedErrMsg: "failed to search commits with REST API",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gateway, server := setupTestGateway(t, http.HandlerFunc(tc.handlerFunc))
			defer server.Close()
			resultMap, err := gateway.FetchCommits(context.Background(), "any-org", "any-user", "")
			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedMap, resultMap)
			}
		})
	}
}

// TestGitHubGateway_GraphQLFetches consolidates the GraphQL tests into a single table-driven test.
func TestGitHubGateway_GraphQLFetches(t *testing.T) {
	testCases := []struct {
		name           string
		methodToTest   func(gateway *GitHubGateway) (map[string]int, error)
		queryContains  string
		responseBody   string
		expectedMap    map[string]int
		expectError    bool
		expectedErrMsg string
	}{
		{
			name: "FetchCreatedPRs - happy path",
			methodToTest: func(gateway *GitHubGateway) (map[string]int, error) {
				return gateway.FetchCreatedPRs(context.Background(), "any-org", "any-user", "")
			},
			queryContains: "author:any-user",
			// THE FIX IS HERE: The mock JSON is now "flattened" as the library expects.
			responseBody: `{"data":{"search":{"edges":[{"node":{"__typename":"PullRequest","repository":{"nameWithOwner":"org/repo-created"}}}]}}}`,
			expectedMap:  map[string]int{"org/repo-created": 1},
			expectError:  false,
		},
		{
			name: "FetchReviewedPRs - happy path",
			methodToTest: func(gateway *GitHubGateway) (map[string]int, error) {
				return gateway.FetchReviewedPRs(context.Background(), "any-org", "any-user", "")
			},
			queryContains: "reviewed-by:any-user",
			responseBody:  `{"data":{"search":{"edges":[{"node":{"__typename":"PullRequest","repository":{"nameWithOwner":"org/repo-reviewed"}}}]}}}`,
			expectedMap:   map[string]int{"org/repo-reviewed": 1},
			expectError:   false,
		},
		{
			name: "FetchCreatedPRs - error case",
			methodToTest: func(gateway *GitHubGateway) (map[string]int, error) {
				return gateway.FetchCreatedPRs(context.Background(), "any-org", "any-user", "")
			},
			queryContains:  "author:any-user",
			responseBody:   `{"errors":[{"message":"Something went wrong"}]}`,
			expectError:    true,
			expectedErrMsg: "failed to execute GraphQL query",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: set up a handler that checks the query and returns the specified response.
			handler := func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				// THE FIX IS HERE: We inspect the raw body string, which is simpler and more robust for this test.
				assert.Contains(t, string(body), tc.queryContains)

				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, tc.responseBody)
			}
			gateway, server := setupTestGateway(t, http.HandlerFunc(handler))
			defer server.Close()

			// Act: call the method under test.
			resultMap, err := tc.methodToTest(gateway)

			// Assert: check the results.
			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedMap, resultMap)
			}
		})
	}
}
