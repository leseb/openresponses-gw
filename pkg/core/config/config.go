// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the main configuration
type Config struct {
	Server ServerConfig `yaml:"server"`
	Engine EngineConfig `yaml:"engine"`
}

// ServerConfig contains HTTP server configuration
type ServerConfig struct {
	Host    string        `yaml:"host"`
	Port    int           `yaml:"port"`
	Timeout time.Duration `yaml:"timeout"`
}

// EngineConfig contains engine configuration
type EngineConfig struct {
	ModelEndpoint string        `yaml:"model_endpoint"`
	APIKey        string        `yaml:"api_key"`
	MaxTokens     int           `yaml:"max_tokens"`
	Timeout       time.Duration `yaml:"timeout"`
}

// Load loads configuration from a YAML file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Load from environment variables (override file config)
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		cfg.Engine.APIKey = apiKey
	}
	if endpoint := os.Getenv("OPENAI_API_ENDPOINT"); endpoint != "" {
		cfg.Engine.ModelEndpoint = endpoint
	}

	return &cfg, nil
}

// Default returns default configuration
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Host:    "0.0.0.0",
			Port:    8080,
			Timeout: 60 * time.Second,
		},
		Engine: EngineConfig{
			ModelEndpoint: os.Getenv("OPENAI_API_ENDPOINT"),
			APIKey:        os.Getenv("OPENAI_API_KEY"),
			MaxTokens:     4096,
			Timeout:       60 * time.Second,
		},
	}
}
