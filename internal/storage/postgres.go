package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store provides database operations for the DHCP server
type Store struct {
	pool *pgxpool.Pool
}

// Config holds database configuration
type Config struct {
	ConnectionString string
	MaxConnections   int32
	MinConnections   int32
	ConnectTimeout   time.Duration
}

// New creates a new Store with the given configuration
func New(ctx context.Context, cfg Config) (*Store, error) {
	// Set default timeout if not specified
	if cfg.ConnectTimeout == 0 {
		cfg.ConnectTimeout = 10 * time.Second
	}

	// Parse connection string and create pool config
	poolConfig, err := pgxpool.ParseConfig(cfg.ConnectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	// Set pool limits
	poolConfig.MaxConns = cfg.MaxConnections
	poolConfig.MinConns = cfg.MinConnections

	// Set connection timeout
	poolConfig.ConnConfig.ConnectTimeout = cfg.ConnectTimeout

	// Create connection pool
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Store{pool: pool}, nil
}

// Close closes the database connection pool
func (s *Store) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

// Health checks if the database connection is healthy
func (s *Store) Health(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// Stats returns connection pool statistics
func (s *Store) Stats() *pgxpool.Stat {
	return s.pool.Stat()
}

// AcquireAdvisoryLock acquires a PostgreSQL advisory lock for the given key
// This is used to prevent concurrent IP allocations
func (s *Store) AcquireAdvisoryLock(ctx context.Context, key int64) error {
	_, err := s.pool.Exec(ctx, "SELECT pg_advisory_lock($1)", key)
	return err
}

// ReleaseAdvisoryLock releases a PostgreSQL advisory lock
func (s *Store) ReleaseAdvisoryLock(ctx context.Context, key int64) error {
	_, err := s.pool.Exec(ctx, "SELECT pg_advisory_unlock($1)", key)
	return err
}

// WithAdvisoryLock executes a function while holding an advisory lock
func (s *Store) WithAdvisoryLock(ctx context.Context, key int64, fn func(context.Context) error) error {
	if err := s.AcquireAdvisoryLock(ctx, key); err != nil {
		return fmt.Errorf("failed to acquire advisory lock: %w", err)
	}
	defer s.ReleaseAdvisoryLock(ctx, key)

	return fn(ctx)
}
