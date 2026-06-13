package main

import (
	"time"

	"github.com/deepkpat/pulse/pkg/config"
)

type Config struct {
	Env    string       `yaml:"env"`
	Redis  RedisConfig  `yaml:"redis"`
	Server ServerConfig `yaml:"server"`
}

type RedisConfig struct {
	Addr       string `yaml:"addr"`
	StreamName string `yaml:"stream_name"`
	GroupName  string `yaml:"group_name"`
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
		Server: ServerConfig{
			Addr:         config.GetEnv("PULSE_SERVER_ADDR", ":8000"),
			ReadTimeout:  config.GetEnvDuration("PULSE_SERVER_READ_TIMEOUT", 4*time.Second),
			WriteTimeout: config.GetEnvDuration("PULSE_SERVER_WRITE_TIMEOUT", 8*time.Second),
			IdleTimeout:  config.GetEnvDuration("PULSE_SERVER_IDLE_TIMEOUT", 128*time.Second),
		},
	}
}
