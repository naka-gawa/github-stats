package usecase

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"

	"github.com/naka-gawa/github-stats/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockFetcher is a mock implementation of the gateway.Fetcher interface.
// It allows us to simulate the behavior of the GitHub gateway without making real API calls.
type mockFetcher struct {
	mock.Mock
}

// FetchCommits is our mock's implementation of the FetchCommits method.
func (m *mockFetcher) FetchCommits(ctx context.Context, org, user, dateRange string) (map[string]int, error) {
	args := m.Called(ctx, org, user, dateRange)
	// We need to handle the case where the returned map is nil (e.g., when an error occurs).
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int), args.Error(1)
}

// FetchCreatedPRs is the mock's implementation for created PRs.
func (m *mockFetcher) FetchCreatedPRs(ctx context.Context, org, user, dateRange string) (map[string]int, error) {
	args := m.Called(ctx, org, user, dateRange)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int), args.Error(1)
}

// FetchReviewedPRs is the mock's implementation for reviewed PRs.
func (m *mockFetcher) FetchReviewedPRs(ctx context.Context, org, user, dateRange string) (map[string]int, error) {
	args := m.Called(ctx, org, user, dateRange)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int), args.Error(1)
}

// TestAggregator_Aggregate uses a table-driven approach to test the aggregator.
func TestAggregator_Aggregate(t *testing.T) {
	// Define the structure for our test cases
	testCases := []struct {
		name              string
		mockCommits       map[string]int
		mockCreatedPRs    map[string]int
		mockReviewedPRs   map[string]int
		mockCommitErr     error
		mockCreatedPRErr  error
		mockReviewedPRErr error
		expectedResult    []*domain.RepoStats
		expectError       bool
	}{
		{
			name:            "happy path - successfully aggregates data from multiple sources",
			mockCommits:     map[string]int{"repo-a": 10, "repo-b": 5},
			mockCreatedPRs:  map[string]int{"repo-a": 2, "repo-c": 1},
			mockReviewedPRs: map[string]int{"repo-b": 3, "repo-c": 4},
			expectedResult: []*domain.RepoStats{
				{Name: "repo-a", Commits: 10, CreatedPRs: 2, ReviewedPRs: 0},
				{Name: "repo-b", Commits: 5, CreatedPRs: 0, ReviewedPRs: 3},
				{Name: "repo-c", Commits: 0, CreatedPRs: 1, ReviewedPRs: 4},
			},
			expectError: false,
		},
		{
			name:           "error case - fetch commits fails",
			mockCommitErr:  errors.New("github api error"),
			expectedResult: nil,
			expectError:    true,
		},
		{
			name:            "empty case - all fetchers return empty maps",
			mockCommits:     map[string]int{},
			mockCreatedPRs:  map[string]int{},
			mockReviewedPRs: map[string]int{},
			expectedResult:  []*domain.RepoStats{}, // Expect an empty slice, not nil
			expectError:     false,
		},
		{
			name:            "partial data case - only commits have data",
			mockCommits:     map[string]int{"repo-a": 7},
			mockCreatedPRs:  map[string]int{},
			mockReviewedPRs: map[string]int{},
			expectedResult: []*domain.RepoStats{
				{Name: "repo-a", Commits: 7, CreatedPRs: 0, ReviewedPRs: 0},
			},
			expectError: false,
		},
	}

	// Iterate over the test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// --- Arrange: Set up the test for this specific case ---
			ctx := context.Background()
			logger := log.New(io.Discard, "", 0)
			fetcher := new(mockFetcher)

			// THE FIX IS HERE: We now provide 4 arguments to the mock's On() method,
			// matching the actual method signature (ctx, org, user, dateRange).
			fetcher.On("FetchCommits", mock.Anything, "any-org", "any-user", "any-commit-range").Return(tc.mockCommits, tc.mockCommitErr)
			fetcher.On("FetchCreatedPRs", mock.Anything, "any-org", "any-user", "any-pr-range").Return(tc.mockCreatedPRs, tc.mockCreatedPRErr)
			fetcher.On("FetchReviewedPRs", mock.Anything, "any-org", "any-user", "any-pr-range").Return(tc.mockReviewedPRs, tc.mockReviewedPRErr)

			aggregator := NewAggregator(fetcher, logger)

			// --- Act: Execute the method we want to test ---
			results, err := aggregator.Aggregate(ctx, "any-org", "any-user", "any-commit-range", "any-pr-range")

			// --- Assert: Check the results ---
			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, results)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedResult, results)
			}

			// Verify that the mock methods were called as expected
			fetcher.AssertExpectations(t)
		})
	}
}
