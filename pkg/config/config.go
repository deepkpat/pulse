package config

import (
	"fmt"
	"os"

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
