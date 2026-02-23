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
	Server       ServerConfig       `yaml:"server"`
	Engine       EngineConfig       `yaml:"engine"`
	Embedding    EmbeddingConfig    `yaml:"embedding"`
	VectorStore  VectorStoreConfig  `yaml:"vector_store"`
	FileStore    FileStoreConfig    `yaml:"file_store"`
	SessionStore SessionStoreConfig `yaml:"session_store"`
}

// SessionStoreConfig contains session store backend configuration
type SessionStoreConfig struct {
	Type string `yaml:"type"` // "sqlite" (default) or "postgres"
	DSN  string `yaml:"dsn"`  // SQLite: ":memory:" (default) or file path; PostgreSQL: "postgres://user:pass@host:5432/dbname?sslmode=disable"
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
	BackendAPI    string        `yaml:"backend_api"` // "chat_completions" (default) or "responses"
	MaxTokens     int           `yaml:"max_tokens"`
	Timeout       time.Duration `yaml:"timeout"`
}

// EmbeddingConfig contains embedding service configuration
type EmbeddingConfig struct {
	Endpoint   string `yaml:"endpoint"` // e.g. "https://api.openai.com/v1"
	APIKey     string `yaml:"api_key"`
	Model      string `yaml:"model"`      // e.g. "text-embedding-3-small"
	Dimensions int    `yaml:"dimensions"` // default 1536
}

// VectorStoreConfig contains vector store backend configuration
type VectorStoreConfig struct {
	Type          string `yaml:"type"`           // "memory" (default) or "milvus"
	MilvusAddress string `yaml:"milvus_address"` // e.g. "localhost:19530"
}

// FileStoreConfig contains file storage backend configuration
type FileStoreConfig struct {
	Type       string `yaml:"type"`     // "memory" (default), "filesystem", "s3"
	BaseDir    string `yaml:"base_dir"` // filesystem only
	S3Bucket   string `yaml:"s3_bucket"`
	S3Region   string `yaml:"s3_region"`
	S3Prefix   string `yaml:"s3_prefix"`
	S3Endpoint string `yaml:"s3_endpoint"` // for MinIO compatibility
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
	if v := os.Getenv("BACKEND_API"); v != "" {
		cfg.Engine.BackendAPI = v
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

	// File store env overrides
	if v := os.Getenv("FILE_STORE_TYPE"); v != "" {
		cfg.FileStore.Type = v
	}
	if v := os.Getenv("FILE_STORE_BASE_DIR"); v != "" {
		cfg.FileStore.BaseDir = v
		if cfg.FileStore.Type == "" {
			cfg.FileStore.Type = "filesystem"
		}
	}
	if v := os.Getenv("FILE_STORE_S3_BUCKET"); v != "" {
		cfg.FileStore.S3Bucket = v
		if cfg.FileStore.Type == "" {
			cfg.FileStore.Type = "s3"
		}
	}
	if v := os.Getenv("FILE_STORE_S3_REGION"); v != "" {
		cfg.FileStore.S3Region = v
	}
	if v := os.Getenv("FILE_STORE_S3_PREFIX"); v != "" {
		cfg.FileStore.S3Prefix = v
	}
	if v := os.Getenv("FILE_STORE_S3_ENDPOINT"); v != "" {
		cfg.FileStore.S3Endpoint = v
	}

	// Session store env overrides
	if v := os.Getenv("SESSION_STORE_TYPE"); v != "" {
		cfg.SessionStore.Type = v
	}
	if v := os.Getenv("SESSION_STORE_DSN"); v != "" {
		cfg.SessionStore.DSN = v
	}

	// Apply defaults
	applyEngineDefaults(&cfg.Engine)
	applyEmbeddingDefaults(&cfg.Embedding)
	applyVectorStoreDefaults(&cfg.VectorStore)
	applyFileStoreDefaults(&cfg.FileStore)
	applySessionStoreDefaults(&cfg.SessionStore)

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

	fsCfg := FileStoreConfig{
		Type:       os.Getenv("FILE_STORE_TYPE"),
		BaseDir:    os.Getenv("FILE_STORE_BASE_DIR"),
		S3Bucket:   os.Getenv("FILE_STORE_S3_BUCKET"),
		S3Region:   os.Getenv("FILE_STORE_S3_REGION"),
		S3Prefix:   os.Getenv("FILE_STORE_S3_PREFIX"),
		S3Endpoint: os.Getenv("FILE_STORE_S3_ENDPOINT"),
	}
	if fsCfg.Type == "" && fsCfg.BaseDir != "" {
		fsCfg.Type = "filesystem"
	}
	if fsCfg.Type == "" && fsCfg.S3Bucket != "" {
		fsCfg.Type = "s3"
	}
	applyFileStoreDefaults(&fsCfg)

	ssCfg := SessionStoreConfig{
		Type: os.Getenv("SESSION_STORE_TYPE"),
		DSN:  os.Getenv("SESSION_STORE_DSN"),
	}
	applySessionStoreDefaults(&ssCfg)

	engCfg := EngineConfig{
		ModelEndpoint: os.Getenv("OPENAI_API_ENDPOINT"),
		APIKey:        os.Getenv("OPENAI_API_KEY"),
		BackendAPI:    os.Getenv("BACKEND_API"),
		MaxTokens:     4096,
		Timeout:       60 * time.Second,
	}
	applyEngineDefaults(&engCfg)

	return &Config{
		Server: ServerConfig{
			Host:    "0.0.0.0",
			Port:    8080,
			Timeout: 60 * time.Second,
		},
		Engine:       engCfg,
		Embedding:    embCfg,
		VectorStore:  vsCfg,
		FileStore:    fsCfg,
		SessionStore: ssCfg,
	}
}

func applyEngineDefaults(cfg *EngineConfig) {
	if cfg.BackendAPI == "" {
		cfg.BackendAPI = "chat_completions"
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

func applyFileStoreDefaults(cfg *FileStoreConfig) {
	if cfg.Type == "" {
		cfg.Type = "memory"
	}
}

func applySessionStoreDefaults(cfg *SessionStoreConfig) {
	if cfg.Type == "" {
		cfg.Type = "sqlite"
	}
	if cfg.DSN == "" {
		cfg.DSN = ":memory:"
	}
}
