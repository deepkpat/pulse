package auth

import (
	"context"
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
	db          *sql.DB
	cache       sync.Map
	stopJanitor chan struct{}
}

// NewPostgresAuthenticator initializes the authenticator with an existing database connection.
func NewPostgresAuthenticator(db *sql.DB) *PostgresAuthenticator {
	s := &PostgresAuthenticator{
		db:          db,
		stopJanitor: make(chan struct{}),
	}
	s.startJanitor()
	return s
}

func (s *PostgresAuthenticator) startJanitor() {
	ticker := time.NewTicker(4 * time.Minute)
	go func() {
		for {
			select {
			case <-ticker.C:
				s.cache.Range(func(key, value interface{}) bool {
					entry := value.(cacheEntry)
					if time.Now().After(entry.expiresAt) {
						s.cache.Delete(key)
					}
					return true
				})
			case <-s.stopJanitor:
				ticker.Stop()
				return
			}
		}
	}()
}

// ValidateAPIKey checks if the provided API key exists in the database.
// It uses SHA-256 hashing for security and an internal TTL cache for performance.
func (s *PostgresAuthenticator) ValidateAPIKey(ctx context.Context, key string) (bool, error) {
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

	err := s.db.QueryRowContext(ctx, query, hashedKey).Scan(&exists)
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

// Close gracefully shuts down the janitor and the database connection pool.
func (s *PostgresAuthenticator) Close() error {
	close(s.stopJanitor)
	return s.db.Close()
}
