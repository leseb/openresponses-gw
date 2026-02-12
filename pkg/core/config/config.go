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
	Server      ServerConfig      `yaml:"server"`
	Engine      EngineConfig      `yaml:"engine"`
	Embedding   EmbeddingConfig   `yaml:"embedding"`
	VectorStore VectorStoreConfig `yaml:"vector_store"`
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

// EmbeddingConfig contains embedding service configuration
type EmbeddingConfig struct {
	Endpoint   string `yaml:"endpoint"`   // e.g. "https://api.openai.com/v1"
	APIKey     string `yaml:"api_key"`
	Model      string `yaml:"model"`      // e.g. "text-embedding-3-small"
	Dimensions int    `yaml:"dimensions"` // default 1536
}

// VectorStoreConfig contains vector store backend configuration
type VectorStoreConfig struct {
	Type          string `yaml:"type"`           // "memory" (default) or "milvus"
	MilvusAddress string `yaml:"milvus_address"` // e.g. "localhost:19530"
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

	// Embedding env overrides
	if v := os.Getenv("EMBEDDING_ENDPOINT"); v != "" {
		cfg.Embedding.Endpoint = v
	}
	if v := os.Getenv("EMBEDDING_API_KEY"); v != "" {
		cfg.Embedding.APIKey = v
	}
	if v := os.Getenv("EMBEDDING_MODEL"); v != "" {
		cfg.Embedding.Model = v
	}

	// Vector store env overrides
	if v := os.Getenv("MILVUS_ADDRESS"); v != "" {
		cfg.VectorStore.MilvusAddress = v
		cfg.VectorStore.Type = "milvus"
	}

	// Apply defaults
	applyEmbeddingDefaults(&cfg.Embedding)
	applyVectorStoreDefaults(&cfg.VectorStore)

	return &cfg, nil
}

// Default returns default configuration
func Default() *Config {
	embCfg := EmbeddingConfig{
		Endpoint: os.Getenv("EMBEDDING_ENDPOINT"),
		APIKey:   os.Getenv("EMBEDDING_API_KEY"),
		Model:    os.Getenv("EMBEDDING_MODEL"),
	}
	applyEmbeddingDefaults(&embCfg)

	vsCfg := VectorStoreConfig{}
	if v := os.Getenv("MILVUS_ADDRESS"); v != "" {
		vsCfg.MilvusAddress = v
		vsCfg.Type = "milvus"
	}
	applyVectorStoreDefaults(&vsCfg)

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
		Embedding:   embCfg,
		VectorStore: vsCfg,
	}
}

func applyEmbeddingDefaults(cfg *EmbeddingConfig) {
	if cfg.Model == "" {
		cfg.Model = "text-embedding-3-small"
	}
	if cfg.Dimensions == 0 {
		cfg.Dimensions = 1536
	}
}

func applyVectorStoreDefaults(cfg *VectorStoreConfig) {
	if cfg.Type == "" {
		cfg.Type = "memory"
	}
}
