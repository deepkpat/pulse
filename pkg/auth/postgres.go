package auth

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	_ "github.com/lib/pq" // side-effect import to register the postgres driver
)

type cacheEntry struct {
	valid     bool
	expiresAt time.Time
}

// PostgresAuthenticator manages the database connection pool and an internal auth cache.
type PostgresAuthenticator struct {
	db    *sql.DB
	cache sync.Map
}

// NewPostgresAuthenticator initializes and verifies a new PostgreSQL connection with a tuned pool.
func NewPostgresAuthenticator(host string, port int, user, password, dbname, sslmode string) (*PostgresAuthenticator, error) {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	// explicit connection pool tuning for production stability
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(16)
	db.SetConnMaxLifetime(4 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	return &PostgresAuthenticator{db: db}, nil
}

// ValidateAPIKey checks if the provided API key exists in the database.
// It uses SHA-256 hashing for security and an internal TTL cache for performance.
func (s *PostgresAuthenticator) ValidateAPIKey(key string) (bool, error) {
	// fast path — check in-memory cache
	if val, ok := s.cache.Load(key); ok {
		entry := val.(cacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.valid, nil
		}
	}

	// hash incoming key using SHA-256 to compare against hashed storage
	hash := sha256.Sum256([]byte(key))
	hashedKey := hex.EncodeToString(hash[:])

	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM api_keys WHERE key = $1)"

	err := s.db.QueryRow(query, hashedKey).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to validate api key: %w", err)
	}

	// cache the result for 60 seconds (positive) or 10 seconds (negative)
	ttl := 60 * time.Second
	if !exists {
		ttl = 10 * time.Second
	}
	s.cache.Store(key, cacheEntry{
		valid:     exists,
		expiresAt: time.Now().Add(ttl),
	})

	return exists, nil
}

// Close gracefully shuts down the database connection pool.
func (s *PostgresAuthenticator) Close() error {
	return s.db.Close()
}
