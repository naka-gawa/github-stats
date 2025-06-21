// Package domain contains the core data structures and domain logic for the application.
package domain

// RepoStats holds the activity counts for a single repository.
// It is the core domain entity of this application.
type RepoStats struct {
	Name        string `json:"name"`
	Commits     int    `json:"commits"`
	CreatedPRs  int    `json:"created_prs"`
	ReviewedPRs int    `json:"reviewed_prs"`
}
