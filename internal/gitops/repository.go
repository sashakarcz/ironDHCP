package gitops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/yourusername/irondhcp/internal/logger"
)

// RepositoryConfig holds configuration for Git repository operations
type RepositoryConfig struct {
	URL              string
	Branch           string
	LocalPath        string
	Username         string
	Password         string
	PrivateKeyPath   string
	ConfigFilePath   string // Path to config file within repo (e.g., "config.yaml")
	PollInterval     time.Duration
	AutoSync         bool
}

// Repository manages Git repository operations
type Repository struct {
	config *RepositoryConfig
	repo   *git.Repository
}

// CommitInfo contains information about a Git commit
type CommitInfo struct {
	Hash      string
	Message   string
	Author    string
	Timestamp time.Time
}

// NewRepository creates a new Git repository manager
func NewRepository(config *RepositoryConfig) *Repository {
	return &Repository{
		config: config,
	}
}

// Initialize initializes the Git repository (clone or open existing)
func (r *Repository) Initialize(ctx context.Context) error {
	// Check if repository already exists locally
	if _, err := os.Stat(filepath.Join(r.config.LocalPath, ".git")); err == nil {
		// Repository exists, open it
		repo, err := git.PlainOpen(r.config.LocalPath)
		if err != nil {
			return fmt.Errorf("failed to open existing repository: %w", err)
		}
		r.repo = repo

		logger.Info().
			Str("path", r.config.LocalPath).
			Msg("Opened existing Git repository")

		return nil
	}

	// Repository doesn't exist, clone it
	if err := r.clone(ctx); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	return nil
}

// clone clones the Git repository
func (r *Repository) clone(ctx context.Context) error {
	logger.Info().
		Str("url", r.config.URL).
		Str("branch", r.config.Branch).
		Str("path", r.config.LocalPath).
		Msg("Cloning Git repository")

	// Ensure parent directory exists
	if err := os.MkdirAll(r.config.LocalPath, 0755); err != nil {
		return fmt.Errorf("failed to create local path: %w", err)
	}

	// Build clone options
	cloneOptions := &git.CloneOptions{
		URL:           r.config.URL,
		ReferenceName: plumbing.NewBranchReferenceName(r.config.Branch),
		SingleBranch:  true,
		Depth:         1, // Shallow clone
		Progress:      nil,
	}

	// Add authentication if configured
	if r.config.Username != "" && r.config.Password != "" {
		cloneOptions.Auth = &http.BasicAuth{
			Username: r.config.Username,
			Password: r.config.Password,
		}
	}

	// Clone the repository
	repo, err := git.PlainCloneContext(ctx, r.config.LocalPath, false, cloneOptions)
	if err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	r.repo = repo

	logger.Info().
		Str("path", r.config.LocalPath).
		Msg("Successfully cloned Git repository")

	return nil
}

// Pull pulls the latest changes from the remote repository
func (r *Repository) Pull(ctx context.Context) (*CommitInfo, bool, error) {
	if r.repo == nil {
		return nil, false, fmt.Errorf("repository not initialized")
	}

	// Get current HEAD before pull
	headBefore, err := r.repo.Head()
	if err != nil {
		return nil, false, fmt.Errorf("failed to get HEAD before pull: %w", err)
	}

	// Get worktree
	worktree, err := r.repo.Worktree()
	if err != nil {
		return nil, false, fmt.Errorf("failed to get worktree: %w", err)
	}

	// Build pull options
	pullOptions := &git.PullOptions{
		RemoteName:    "origin",
		ReferenceName: plumbing.NewBranchReferenceName(r.config.Branch),
		SingleBranch:  true,
		Force:         false,
	}

	// Add authentication if configured
	if r.config.Username != "" && r.config.Password != "" {
		pullOptions.Auth = &http.BasicAuth{
			Username: r.config.Username,
			Password: r.config.Password,
		}
	}

	// Pull changes
	err = worktree.PullContext(ctx, pullOptions)
	if err != nil {
		if err == git.NoErrAlreadyUpToDate {
			// No changes, return current commit info
			commitInfo, err := r.GetCurrentCommit()
			if err != nil {
				return nil, false, fmt.Errorf("failed to get current commit: %w", err)
			}
			return commitInfo, false, nil
		}
		return nil, false, fmt.Errorf("failed to pull: %w", err)
	}

	// Get new HEAD after pull
	headAfter, err := r.repo.Head()
	if err != nil {
		return nil, false, fmt.Errorf("failed to get HEAD after pull: %w", err)
	}

	// Check if there were changes
	hasChanges := headBefore.Hash() != headAfter.Hash()

	// Get commit info
	commitInfo, err := r.GetCurrentCommit()
	if err != nil {
		return nil, false, fmt.Errorf("failed to get commit info: %w", err)
	}

	if hasChanges {
		logger.Info().
			Str("commit", commitInfo.Hash).
			Str("message", commitInfo.Message).
			Str("author", commitInfo.Author).
			Msg("Pulled new changes from Git repository")
	} else {
		logger.Debug().Msg("No new changes in Git repository")
	}

	return commitInfo, hasChanges, nil
}

// GetCurrentCommit returns information about the current HEAD commit
func (r *Repository) GetCurrentCommit() (*CommitInfo, error) {
	if r.repo == nil {
		return nil, fmt.Errorf("repository not initialized")
	}

	// Get HEAD reference
	head, err := r.repo.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD: %w", err)
	}

	// Get commit object
	commit, err := r.repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("failed to get commit object: %w", err)
	}

	return &CommitInfo{
		Hash:      commit.Hash.String(),
		Message:   commit.Message,
		Author:    commit.Author.Name,
		Timestamp: commit.Author.When,
	}, nil
}

// GetConfigFilePath returns the full path to the config file within the repository
func (r *Repository) GetConfigFilePath() string {
	return filepath.Join(r.config.LocalPath, r.config.ConfigFilePath)
}

// Cleanup removes the local repository directory
func (r *Repository) Cleanup() error {
	if r.config.LocalPath == "" {
		return nil
	}

	logger.Info().
		Str("path", r.config.LocalPath).
		Msg("Cleaning up Git repository")

	return os.RemoveAll(r.config.LocalPath)
}

// GetCommitByHash retrieves commit information by hash
func (r *Repository) GetCommitByHash(hash string) (*CommitInfo, error) {
	if r.repo == nil {
		return nil, fmt.Errorf("repository not initialized")
	}

	commitHash := plumbing.NewHash(hash)
	commit, err := r.repo.CommitObject(commitHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit %s: %w", hash, err)
	}

	return &CommitInfo{
		Hash:      commit.Hash.String(),
		Message:   commit.Message,
		Author:    commit.Author.Name,
		Timestamp: commit.Author.When,
	}, nil
}

// GetCommitLog retrieves the commit log with a limit
func (r *Repository) GetCommitLog(limit int) ([]*CommitInfo, error) {
	if r.repo == nil {
		return nil, fmt.Errorf("repository not initialized")
	}

	// Get HEAD reference
	head, err := r.repo.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD: %w", err)
	}

	// Get commit iterator
	iter, err := r.repo.Log(&git.LogOptions{
		From: head.Hash(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get commit log: %w", err)
	}
	defer iter.Close()

	var commits []*CommitInfo
	count := 0

	err = iter.ForEach(func(c *object.Commit) error {
		if count >= limit {
			return fmt.Errorf("limit reached")
		}

		commits = append(commits, &CommitInfo{
			Hash:      c.Hash.String(),
			Message:   c.Message,
			Author:    c.Author.Name,
			Timestamp: c.Author.When,
		})

		count++
		return nil
	})

	// Ignore "limit reached" error as it's expected
	if err != nil && err.Error() != "limit reached" {
		return nil, fmt.Errorf("failed to iterate commits: %w", err)
	}

	return commits, nil
}
