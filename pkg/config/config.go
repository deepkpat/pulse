package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Load reads a YAML configuration file from the given path and unmarshals it into the provided dst.
func Load(path string, dst any) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open config file %s: %w", path, err)
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("failed to decode config file %s: %w", path, err)
	}

	return nil
}

// GetEnv returns the value of the environment variable named by the key or the fallback value if it is not set.
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// GetEnvInt returns the value of the environment variable named by the key as an int or the fallback value if it is not set or invalid.
func GetEnvInt(key string, fallback int) int {
	valStr := os.Getenv(key)
	if valStr == "" {
		return fallback
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return fallback
	}
	return val
}

// GetEnvDuration returns the value of the environment variable named by the key as a time.Duration or the fallback value if it is not set or invalid.
func GetEnvDuration(key string, fallback time.Duration) time.Duration {
	valStr := os.Getenv(key)
	if valStr == "" {
		return fallback
	}
	val, err := time.ParseDuration(valStr)
	if err != nil {
		return fallback
	}
	return val
}
