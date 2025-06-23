package usecase

import (
	"context"
	"errors"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/naka-gawa/github-stats/internal/domain"
	"github.com/naka-gawa/github-stats/internal/gateway"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockFetcher is a mock implementation of the gateway.Fetcher interface.
type mockFetcher struct {
	mock.Mock
	mu sync.Mutex
}

// FetchCommits is our mock's implementation of the FetchCommits method.
func (m *mockFetcher) FetchCommits(ctx context.Context, org, user, dateRange string) (map[string]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	args := m.Called(ctx, org, user, dateRange)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int), args.Error(1)
}

// FetchCreatedPRs is the mock's implementation for created PRs.
func (m *mockFetcher) FetchCreatedPRs(ctx context.Context, org, user, dateRange string) (map[string]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	args := m.Called(ctx, org, user, dateRange)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int), args.Error(1)
}

// FetchReviewedPRs is the mock's implementation for reviewed PRs.
func (m *mockFetcher) FetchReviewedPRs(ctx context.Context, org, user, dateRange string) (map[string]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	args := m.Called(ctx, org, user, dateRange)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int), args.Error(1)
}

// FetchPRLeadTimes is the mock's implementation for fetching lead time data.
func (m *mockFetcher) FetchPRLeadTimes(ctx context.Context, org, user, dateRange string) (map[string][]gateway.PRLeadTimeData, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	args := m.Called(ctx, org, user, dateRange)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string][]gateway.PRLeadTimeData), args.Error(1)
}

func TestAggregator_Aggregate(t *testing.T) {
	baseTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	// Define the structure for our test cases
	testCases := []struct {
		name              string
		calculateLeadTime bool
		mockCommits       map[string]int
		mockCreatedPRs    map[string]int
		mockReviewedPRs   map[string]int
		mockLeadTimeData  map[string][]gateway.PRLeadTimeData
		mockErr           error
		expectedResult    []*domain.RepoStats
		expectError       bool
	}{
		{
			name:              "happy path - without lead time",
			calculateLeadTime: false,
			mockCommits:       map[string]int{"repo-a": 10, "repo-b": 5},
			mockCreatedPRs:    map[string]int{"repo-a": 2, "repo-c": 1},
			mockReviewedPRs:   map[string]int{"repo-b": 3, "repo-c": 4},
			expectedResult: []*domain.RepoStats{
				{Name: "repo-a", Commits: 10, CreatedPRs: 2, ReviewedPRs: 0},
				{Name: "repo-b", Commits: 5, CreatedPRs: 0, ReviewedPRs: 3},
				{Name: "repo-c", Commits: 0, CreatedPRs: 1, ReviewedPRs: 4},
			},
			expectError: false,
		},
		{
			name:              "happy path - with lead time calculation",
			calculateLeadTime: true,
			mockCommits:       map[string]int{"repo-a": 1},
			mockCreatedPRs:    map[string]int{"repo-a": 1},
			mockReviewedPRs:   map[string]int{},
			mockLeadTimeData: map[string][]gateway.PRLeadTimeData{
				"repo-a": {
					{
						CreatedAt:      baseTime.Add(-2 * time.Hour), // 2 hours ago
						LastReviewedAt: baseTime.Add(-1 * time.Hour), // 1 hour ago
					},
				},
			},
			expectedResult: []*domain.RepoStats{
				{
					Name: "repo-a", Commits: 1, CreatedPRs: 1, ReviewedPRs: 0,
					LeadTimeToLastReviewSeconds: []float64{3600},
				},
			},
			expectError: false,
		},
		{
			name:              "error case - fetch commits fails",
			calculateLeadTime: false,
			mockErr:           errors.New("github api error"),
			expectedResult:    nil,
			expectError:       true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			logger := log.New(io.Discard, "", 0)
			fetcher := new(mockFetcher)

			// Set up mock expectations based on the test case data
			if tc.mockErr != nil {
				fetcher.On("FetchCommits", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, tc.mockErr)
				fetcher.On("FetchCreatedPRs", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, tc.mockErr)
				fetcher.On("FetchReviewedPRs", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, tc.mockErr)
			} else {
				fetcher.On("FetchCommits", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(tc.mockCommits, nil)
				fetcher.On("FetchCreatedPRs", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(tc.mockCreatedPRs, nil)
				fetcher.On("FetchReviewedPRs", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(tc.mockReviewedPRs, nil)
			}

			// Only set expectation for lead time if it's being calculated
			if tc.calculateLeadTime {
				fetcher.On("FetchPRLeadTimes", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(tc.mockLeadTimeData, nil)
			}

			aggregator := NewAggregator(fetcher, logger)
			results, err := aggregator.Aggregate(ctx, "any-org", "any-user", "any-commit-range", "any-pr-range", tc.calculateLeadTime)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, results)
			} else {
				assert.NoError(t, err)
				// For the lead time test, we need to be careful with comparing float values.
				// A simple assert.Equal is fine here as we control the input.
				assert.Equal(t, tc.expectedResult, results)
			}

			fetcher.AssertExpectations(t)
		})
	}
}
