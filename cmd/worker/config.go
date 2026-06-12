package main

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
		Env:         "development",
		Concurrency: 1,
		Redis: RedisConfig{
			Addr:       "localhost:6379",
			StreamName: "pulse_stream",
			GroupName:  "pulse_worker_group",
		},
		ClickHouse: ClickHouseConfig{
			Addr:     "localhost:9000",
			User:     "pulse_admin",
			Password: "pulse_super_secret_password",
			Database: "pulse",
		},
	}
}
