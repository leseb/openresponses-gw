// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"fmt"

	"github.com/leseb/openresponses-gw/pkg/core/api"
	"github.com/leseb/openresponses-gw/pkg/filestore"
	"github.com/leseb/openresponses-gw/pkg/vectorstore"
)

// VectorStoreService coordinates file ingestion, search, and lifecycle
// across the FilesStore, EmbeddingClient, and vectorstore.Backend.
//
// Nil-safe: NewVectorStoreService returns nil if embedder or backend is nil.
// All methods are nil-receiver safe and return nil on a nil receiver.
type VectorStoreService struct {
	files    filestore.FileStore
	embedder api.EmbeddingClient
	backend  vectorstore.Backend
}

// NewVectorStoreService creates a VectorStoreService.
// Returns nil if either embedder or backend is nil (feature disabled).
func NewVectorStoreService(files filestore.FileStore, embedder api.EmbeddingClient, backend vectorstore.Backend) *VectorStoreService {
	if embedder == nil || backend == nil {
		return nil
	}
	return &VectorStoreService{
		files:    files,
		embedder: embedder,
		backend:  backend,
	}
}

// CreateStore provisions the backend storage for a vector store.
func (s *VectorStoreService) CreateStore(ctx context.Context, vectorStoreID string, dimensions int) error {
	if s == nil {
		return nil
	}
	return s.backend.CreateStore(ctx, vectorStoreID, dimensions)
}

// DeleteStore removes the backend storage for a vector store.
func (s *VectorStoreService) DeleteStore(ctx context.Context, vectorStoreID string) error {
	if s == nil {
		return nil
	}
	return s.backend.DeleteStore(ctx, vectorStoreID)
}

// IngestFile reads a file's content, chunks it, embeds the chunks, and
// inserts them into the vector store backend.
func (s *VectorStoreService) IngestFile(ctx context.Context, vectorStoreID, fileID string, chunkSize, overlap int) error {
	if s == nil {
		return nil
	}

	// Read file content
	content, err := s.files.GetFileContent(ctx, fileID)
	if err != nil {
		return fmt.Errorf("read file %s: %w", fileID, err)
	}

	text := string(content)
	if text == "" {
		return nil
	}

	// Chunk the text
	chunks := vectorstore.ChunkText(text, chunkSize, overlap)
	if len(chunks) == 0 {
		return nil
	}

	// Embed all chunks in a single batch
	vectors, err := s.embedder.Embed(ctx, chunks)
	if err != nil {
		return fmt.Errorf("embed chunks for file %s: %w", fileID, err)
	}

	if len(vectors) != len(chunks) {
		return fmt.Errorf("embedding count mismatch: got %d, expected %d", len(vectors), len(chunks))
	}

	// Build chunk objects
	vsChunks := make([]vectorstore.Chunk, len(chunks))
	for i, text := range chunks {
		vsChunks[i] = vectorstore.Chunk{
			ChunkID:       fmt.Sprintf("%s_chunk_%d", fileID, i),
			FileID:        fileID,
			VectorStoreID: vectorStoreID,
			Content:       text,
			Vector:        vectors[i],
		}
	}

	// Insert into backend
	if err := s.backend.InsertChunks(ctx, vsChunks); err != nil {
		return fmt.Errorf("insert chunks for file %s: %w", fileID, err)
	}

	return nil
}

// RemoveFile removes all chunks for a file from the vector store backend.
func (s *VectorStoreService) RemoveFile(ctx context.Context, vectorStoreID, fileID string) error {
	if s == nil {
		return nil
	}
	return s.backend.DeleteFileChunks(ctx, vectorStoreID, fileID)
}

// Search embeds the query and performs vector similarity search.
func (s *VectorStoreService) Search(ctx context.Context, vectorStoreID, query string, topK int) ([]vectorstore.SearchResult, error) {
	if s == nil {
		return nil, nil
	}

	if topK <= 0 {
		topK = 10
	}

	// Embed the query
	vectors, err := s.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vectors) == 0 {
		return nil, nil
	}

	// Search
	return s.backend.Search(ctx, vectorStoreID, vectors[0], topK)
}
