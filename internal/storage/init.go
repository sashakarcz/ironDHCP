package storage

import (
	"context"
	"embed"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// EnsureDatabase ensures the database exists, creating it if necessary
func EnsureDatabase(ctx context.Context, connectionString string) error {
	// Try to connect to the database first
	conn, err := pgx.Connect(ctx, connectionString)
	if err != nil {
		// Check if error is "database does not exist"
		if !strings.Contains(err.Error(), "does not exist") {
			return fmt.Errorf("failed to connect to database: %w", err)
		}

		// Extract database name from connection string
		dbName, err := extractDatabaseName(connectionString)
		if err != nil {
			return fmt.Errorf("failed to extract database name: %w", err)
		}

		// Create the database
		if err := createDatabase(ctx, connectionString, dbName); err != nil {
			return fmt.Errorf("failed to create database: %w", err)
		}

		// Verify we can now connect
		conn, err = pgx.Connect(ctx, connectionString)
		if err != nil {
			return fmt.Errorf("failed to connect to newly created database: %w", err)
		}
	}
	defer conn.Close(ctx)

	// Run migrations
	if err := runMigrations(ctx, conn); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// extractDatabaseName extracts the database name from a PostgreSQL connection string
func extractDatabaseName(connStr string) (string, error) {
	// Parse connection string to extract database name
	// Format: postgres://user:pass@host:port/dbname?params
	// or: host=localhost user=user password=pass dbname=dbname

	// Try postgres:// format first
	if strings.HasPrefix(connStr, "postgres://") || strings.HasPrefix(connStr, "postgresql://") {
		parts := strings.Split(connStr, "/")
		if len(parts) < 4 {
			return "", fmt.Errorf("invalid connection string format")
		}
		dbPart := parts[3]
		// Remove query parameters
		dbName := strings.Split(dbPart, "?")[0]
		if dbName == "" {
			return "", fmt.Errorf("no database name in connection string")
		}
		return dbName, nil
	}

	// Try key=value format
	pairs := strings.Split(connStr, " ")
	for _, pair := range pairs {
		if strings.HasPrefix(pair, "dbname=") || strings.HasPrefix(pair, "database=") {
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) == 2 {
				return parts[1], nil
			}
		}
	}

	return "", fmt.Errorf("could not find database name in connection string")
}

// buildPostgresConnectionString replaces the database name with 'postgres'
func buildPostgresConnectionString(connStr, dbName string) string {
	// Replace the database name with 'postgres'
	if strings.HasPrefix(connStr, "postgres://") || strings.HasPrefix(connStr, "postgresql://") {
		return strings.Replace(connStr, "/"+dbName, "/postgres", 1)
	}

	// Key=value format
	pairs := strings.Split(connStr, " ")
	for i, pair := range pairs {
		if strings.HasPrefix(pair, "dbname=") {
			pairs[i] = "dbname=postgres"
			break
		}
		if strings.HasPrefix(pair, "database=") {
			pairs[i] = "database=postgres"
			break
		}
	}
	return strings.Join(pairs, " ")
}

// createDatabase creates a new PostgreSQL database
func createDatabase(ctx context.Context, originalConnStr, dbName string) error {
	// Connect to 'postgres' database to create the new database
	postgresConnStr := buildPostgresConnectionString(originalConnStr, dbName)

	conn, err := pgx.Connect(ctx, postgresConnStr)
	if err != nil {
		return fmt.Errorf("failed to connect to postgres database: %w", err)
	}
	defer conn.Close(ctx)

	// Create database
	// Note: Cannot use parameterized query for CREATE DATABASE
	createSQL := fmt.Sprintf("CREATE DATABASE %s", pgx.Identifier{dbName}.Sanitize())
	_, err = conn.Exec(ctx, createSQL)
	if err != nil {
		return fmt.Errorf("failed to execute CREATE DATABASE: %w", err)
	}

	return nil
}

// runMigrations runs database migrations
func runMigrations(ctx context.Context, conn *pgx.Conn) error {
	// List of migration files to run in order
	migrations := []string{
		"migrations/001_initial_schema.sql",
		"migrations/002_add_boot_options.sql",
		"migrations/003_git_sync_audit.sql",
	}

	for _, migrationFile := range migrations {
		// Read migration file from embedded FS
		migrationSQL, err := migrationsFS.ReadFile(migrationFile)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", migrationFile, err)
		}

		// Execute migration
		_, err = conn.Exec(ctx, string(migrationSQL))
		if err != nil {
			return fmt.Errorf("failed to execute migration %s: %w", migrationFile, err)
		}
	}

	return nil
}
