// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package vectorstore

import "context"

// MemoryBackend is a no-op Backend implementation used when no real vector
// store is configured. All methods return nil (success) without doing anything.
type MemoryBackend struct{}

// NewMemoryBackend creates a new no-op memory backend.
func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{}
}

func (m *MemoryBackend) CreateStore(ctx context.Context, vectorStoreID string, dimensions int) error {
	return nil
}

func (m *MemoryBackend) DeleteStore(ctx context.Context, vectorStoreID string) error {
	return nil
}

func (m *MemoryBackend) InsertChunks(ctx context.Context, chunks []Chunk) error {
	return nil
}

func (m *MemoryBackend) DeleteFileChunks(ctx context.Context, vectorStoreID, fileID string) error {
	return nil
}

func (m *MemoryBackend) Search(ctx context.Context, vectorStoreID string, queryVector []float32, topK int) ([]SearchResult, error) {
	return nil, nil
}

func (m *MemoryBackend) Close(ctx context.Context) error {
	return nil
}
