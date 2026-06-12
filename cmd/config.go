package main

import (
	"time"
)

type Config struct {
	Env    string      `yaml:"env"`
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
		Env: "development",
		Redis: RedisConfig{
			Addr:       "localhost:6379",
			StreamName: "pulse_stream",
			GroupName:  "pulse_worker_group",
		},
		Server: ServerConfig{
			Addr:         ":8000",
			ReadTimeout:  4 * time.Second,
			WriteTimeout: 8 * time.Second,
			IdleTimeout:  128 * time.Second,
		},
	}
}
