// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package vectorstore

import (
	"context"

	"github.com/leseb/openresponses-gw/pkg/provider"
)

// Providers is the registry of vector store backend implementations.
// Import implementation packages with blank imports to register them:
//
//	import _ "github.com/leseb/openresponses-gw/pkg/vectorstore/milvus"
var Providers = provider.NewRegistry[Backend]("vector_store")

// Chunk represents a piece of text with its embedding, ready for insertion.
type Chunk struct {
	ChunkID       string
	FileID        string
	VectorStoreID string
	Content       string
	Vector        []float32
}

// SearchResult represents a single result from a vector similarity search.
type SearchResult struct {
	FileID  string
	ChunkID string
	Content string
	Score   float64
}

// Backend is the interface for vector store storage backends.
type Backend interface {
	// CreateStore provisions a new vector store (e.g. a Milvus collection).
	CreateStore(ctx context.Context, vectorStoreID string, dimensions int) error

	// DeleteStore removes a vector store and all its data.
	DeleteStore(ctx context.Context, vectorStoreID string) error

	// InsertChunks inserts embedded chunks into a vector store.
	InsertChunks(ctx context.Context, chunks []Chunk) error

	// DeleteFileChunks removes all chunks for a given file from a vector store.
	DeleteFileChunks(ctx context.Context, vectorStoreID, fileID string) error

	// Search performs a vector similarity search and returns the top-K results.
	Search(ctx context.Context, vectorStoreID string, queryVector []float32, topK int) ([]SearchResult, error)

	// Close releases any resources held by the backend.
	Close(ctx context.Context) error
}
