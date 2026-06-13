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
	Addr       string `yaml:"addr"`
	StreamName string `yaml:"stream_name"`
	GroupName  string `yaml:"group_name"`
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

func DefaultConfig() *Config {
	return &Config{
		Env: config.GetEnv("PULSE_ENV", "development"),
		Redis: RedisConfig{
			Addr:       config.GetEnv("PULSE_REDIS_ADDR", "localhost:6379"),
			StreamName: config.GetEnv("PULSE_REDIS_STREAM_NAME", "pulse_stream"),
			GroupName:  config.GetEnv("PULSE_REDIS_GROUP_NAME", "pulse_worker_group"),
		},
		Postgres: PostgresConfig{
			Host:     config.GetEnv("PULSE_PG_HOST", "localhost"),
			Port:     config.GetEnvInt("PULSE_PG_PORT", 5432),
			User:     config.GetEnv("PULSE_PG_USER", "pulse_pg"),
			Password: config.GetEnv("PULSE_PG_PASSWORD", "pulse_pg_super_secret_password"),
			DBName:   config.GetEnv("PULSE_PG_DBNAME", "pulse"),
			SSLMode:  config.GetEnv("PULSE_PG_SSLMODE", "disable"),
		},
		Server: ServerConfig{
			Addr:         config.GetEnv("PULSE_SERVER_ADDR", ":8000"),
			ReadTimeout:  config.GetEnvDuration("PULSE_SERVER_READ_TIMEOUT", 4*time.Second),
			WriteTimeout: config.GetEnvDuration("PULSE_SERVER_WRITE_TIMEOUT", 8*time.Second),
			IdleTimeout:  config.GetEnvDuration("PULSE_SERVER_IDLE_TIMEOUT", 128*time.Second),
		},
	}
}
