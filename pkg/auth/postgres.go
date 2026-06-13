package auth

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq" // side-effect import to register the postgres driver
)

// PostgresAuthenticator manages the database connection pool.
type PostgresAuthenticator struct {
	db *sql.DB
}

// NewPostgresStorage initializes and verifies a new PostgreSQL connection.
func NewPostgresAuthenticator(host string, port int, user, password, dbname, sslmode string) (*PostgresAuthenticator, error) {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	return &PostgresAuthenticator{db: db}, nil
}

// ValidateAPIKey checks if the provided API key exists in the database.
func (s *PostgresAuthenticator) ValidateAPIKey(key string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM api_keys WHERE key = $1)"

	err := s.db.QueryRow(query, key).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to validate api key: %w", err)
	}
	return exists, nil
}

// Close gracefully shuts down the database connection pool.
func (s *PostgresAuthenticator) Close() error {
	return s.db.Close()
}
