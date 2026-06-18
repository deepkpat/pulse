package main

import (
	"github.com/deepkpat/pulse/pkg/config"
)

type Config struct {
	Env         string           `yaml:"env"`
	Concurrency int              `yaml:"concurrency"`
	Redis       RedisConfig      `yaml:"redis"`
	ClickHouse  ClickHouseConfig `yaml:"clickhouse"`
	MetricsAddr string           `yaml:"metrics_addr"`
}

type RedisConfig struct {
	Addr       string `yaml:"addr"`
	StreamName string `yaml:"stream_name"`
	GroupName  string `yaml:"group_name"`
}

type ClickHouseConfig struct {
	Addr     string `yaml:"addr"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
}

// DefaultConfig returns sane hardcoded defaults for local development.
func DefaultConfig() *Config {
	return &Config{
		Env:         "development",
		Concurrency: 2,
		Redis: RedisConfig{
			Addr:       "localhost:6379",
			StreamName: "pulse_stream",
			GroupName:  "pulse_worker_group",
		},
		ClickHouse: ClickHouseConfig{
			Addr:     "localhost:9000",
			User:     "pulse_ch",
			Password: "",
			Database: "pulse",
		},
		MetricsAddr: ":9091",
	}
}

// ApplyEnvOverrides updates the config with environment variables.
func (c *Config) ApplyEnvOverrides() {
	c.Env = config.GetEnv("PULSE_ENV", c.Env)
	c.Concurrency = config.GetEnvInt("PULSE_CONCURRENCY", c.Concurrency)

	// redis overrides
	c.Redis.Addr = config.GetEnv("PULSE_REDIS_ADDR", c.Redis.Addr)
	c.Redis.StreamName = config.GetEnv("PULSE_REDIS_STREAM_NAME", c.Redis.StreamName)
	c.Redis.GroupName = config.GetEnv("PULSE_REDIS_GROUP_NAME", c.Redis.GroupName)

	// clickhouse overrides
	c.ClickHouse.Addr = config.GetEnv("PULSE_CLICKHOUSE_ADDR", c.ClickHouse.Addr)
	c.ClickHouse.User = config.GetEnv("PULSE_CLICKHOUSE_USER", c.ClickHouse.User)
	c.ClickHouse.Password = config.GetEnv("PULSE_CLICKHOUSE_PASSWORD", c.ClickHouse.Password)
	c.ClickHouse.Database = config.GetEnv("PULSE_CLICKHOUSE_DATABASE", c.ClickHouse.Database)

	// metrics override
	c.MetricsAddr = config.GetEnv("PULSE_WORKER_METRICS_ADDR", c.MetricsAddr)
}
