package main

import (
	"time"

	"github.com/deepkpat/pulse/pkg/config"
)

type Config struct {
	Env      string         `yaml:"env"`
	Redis    RedisConfig    `yaml:"redis"`
	Postgres PostgresConfig `yaml:"postgres"`
	Server   ServerConfig   `yaml:"server"`
}

type RedisConfig struct {
	Addr       string        `yaml:"addr"`
	StreamName string        `yaml:"stream_name"`
	GroupName  string        `yaml:"group_name"`
	DedupTTL   time.Duration `yaml:"dedup_ttl"`
}

type PostgresConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	SSLMode  string `yaml:"sslmode"`
}

type ServerConfig struct {
	Addr         string        `yaml:"addr"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
}

// DefaultConfig returns sane hardcoded defaults for local development.
func DefaultConfig() *Config {
	return &Config{
		Env: "development",
		Redis: RedisConfig{
			Addr:       "localhost:6379",
			StreamName: "pulse_stream",
			GroupName:  "pulse_worker_group",
			DedupTTL:   16 * time.Minute,
		},
		Postgres: PostgresConfig{
			Host:     "localhost",
			Port:     5432,
			User:     "pulse_pg",
			Password: "",
			DBName:   "pulse",
			SSLMode:  "disable",
		},
		Server: ServerConfig{
			Addr:         ":8000",
			ReadTimeout:  4 * time.Second,
			WriteTimeout: 8 * time.Second,
			IdleTimeout:  128 * time.Second,
		},
	}
}

// ApplyEnvOverrides updates the config with environment variables.
func (c *Config) ApplyEnvOverrides() {
	c.Env = config.GetEnv("PULSE_ENV", c.Env)

	// redis overrides
	c.Redis.Addr = config.GetEnv("PULSE_REDIS_ADDR", c.Redis.Addr)
	c.Redis.StreamName = config.GetEnv("PULSE_REDIS_STREAM_NAME", c.Redis.StreamName)
	c.Redis.GroupName = config.GetEnv("PULSE_REDIS_GROUP_NAME", c.Redis.GroupName)
	c.Redis.DedupTTL = config.GetEnvDuration("PULSE_REDIS_DEDUP_TTL", c.Redis.DedupTTL)

	// postgres overrides
	c.Postgres.Host = config.GetEnv("PULSE_PG_HOST", c.Postgres.Host)
	c.Postgres.Port = config.GetEnvInt("PULSE_PG_PORT", c.Postgres.Port)
	c.Postgres.User = config.GetEnv("PULSE_PG_USER", c.Postgres.User)
	c.Postgres.Password = config.GetEnv("PULSE_PG_PASSWORD", c.Postgres.Password)
	c.Postgres.DBName = config.GetEnv("PULSE_PG_DBNAME", c.Postgres.DBName)
	c.Postgres.SSLMode = config.GetEnv("PULSE_PG_SSLMODE", c.Postgres.SSLMode)

	// server overrides
	c.Server.Addr = config.GetEnv("PULSE_SERVER_ADDR", c.Server.Addr)
	c.Server.ReadTimeout = config.GetEnvDuration("PULSE_SERVER_READ_TIMEOUT", c.Server.ReadTimeout)
	c.Server.WriteTimeout = config.GetEnvDuration("PULSE_SERVER_WRITE_TIMEOUT", c.Server.WriteTimeout)
	c.Server.IdleTimeout = config.GetEnvDuration("PULSE_SERVER_IDLE_TIMEOUT", c.Server.IdleTimeout)
}
