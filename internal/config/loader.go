package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func Load(path string) (*Config, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	contents, err := os.ReadFile(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var config Config
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	config.BaseDir = filepath.Dir(absolutePath)
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &config, nil
}
