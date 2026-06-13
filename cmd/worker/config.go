package main

import (
	"github.com/deepkpat/pulse/pkg/config"
)

type Config struct {
	Env         string           `yaml:"env"`
	Concurrency int              `yaml:"concurrency"`
	Redis       RedisConfig      `yaml:"redis"`
	ClickHouse  ClickHouseConfig `yaml:"clickhouse"`
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

func DefaultConfig() *Config {
	return &Config{
		Env:         config.GetEnv("PULSE_ENV", "development"),
		Concurrency: config.GetEnvInt("PULSE_CONCURRENCY", 2),
		Redis: RedisConfig{
			Addr:       config.GetEnv("PULSE_REDIS_ADDR", "localhost:6379"),
			StreamName: config.GetEnv("PULSE_REDIS_STREAM_NAME", "pulse_stream"),
			GroupName:  config.GetEnv("PULSE_REDIS_GROUP_NAME", "pulse_worker_group"),
		},
		ClickHouse: ClickHouseConfig{
			Addr:     config.GetEnv("PULSE_CLICKHOUSE_ADDR", "localhost:9000"),
			User:     config.GetEnv("PULSE_CLICKHOUSE_USER", "pulse_ch"),
			Password: config.GetEnv("PULSE_CLICKHOUSE_PASSWORD", "pulse_ch_super_secret_password"),
			Database: config.GetEnv("PULSE_CLICKHOUSE_DATABASE", "pulse"),
		},
	}
}
