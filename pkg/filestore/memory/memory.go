// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/leseb/openresponses-gw/pkg/filestore"
)

func init() {
	filestore.Providers.Register("memory", func(_ context.Context, _ map[string]string) (filestore.FileStore, error) {
		return New(), nil
	})
}

// compile-time check
var _ filestore.FileStore = (*Store)(nil)

// Store is an in-memory file store.
type Store struct {
	mu    sync.RWMutex
	files map[string]*filestore.File
}

// New creates a new in-memory file store.
func New() *Store {
	return &Store{
		files: make(map[string]*filestore.File),
	}
}

// CreateFile stores a new file.
func (s *Store) CreateFile(_ context.Context, file *filestore.File) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.files[file.ID]; exists {
		return fmt.Errorf("file %s already exists", file.ID)
	}

	s.files[file.ID] = file
	return nil
}

// GetFile returns file metadata (Content is nil).
func (s *Store) GetFile(_ context.Context, fileID string) (*filestore.File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	file, exists := s.files[fileID]
	if !exists {
		return nil, fmt.Errorf("file %s: %w", fileID, filestore.ErrFileNotFound)
	}

	// Return a copy without content
	cp := *file
	cp.Content = nil
	return &cp, nil
}

// GetFileContent returns the raw file bytes.
func (s *Store) GetFileContent(_ context.Context, fileID string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	file, exists := s.files[fileID]
	if !exists {
		return nil, fmt.Errorf("file %s: %w", fileID, filestore.ErrFileNotFound)
	}

	return file.Content, nil
}

// DeleteFile removes a file.
func (s *Store) DeleteFile(_ context.Context, fileID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.files[fileID]; !exists {
		return fmt.Errorf("file %s: %w", fileID, filestore.ErrFileNotFound)
	}

	delete(s.files, fileID)
	return nil
}

// ListFilesPaginated returns files with cursor-based pagination sorted by CreatedAt.
func (s *Store) ListFilesPaginated(_ context.Context, after, before string, limit int, order, purpose string) ([]*filestore.File, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// Collect and filter by purpose
	allFiles := make([]*filestore.File, 0, len(s.files))
	for _, file := range s.files {
		if purpose != "" && file.Purpose != purpose {
			continue
		}
		allFiles = append(allFiles, file)
	}

	// Sort by CreatedAt for deterministic ordering
	sort.Slice(allFiles, func(i, j int) bool {
		if order == "desc" {
			return allFiles[i].CreatedAt.After(allFiles[j].CreatedAt)
		}
		return allFiles[i].CreatedAt.Before(allFiles[j].CreatedAt)
	})

	// Apply cursor-based pagination
	var filtered []*filestore.File
	foundAfter := after == ""

	for _, file := range allFiles {
		if after != "" && !foundAfter {
			if file.ID == after {
				foundAfter = true
			}
			continue
		}

		if before != "" && file.ID == before {
			break
		}

		filtered = append(filtered, file)

		if len(filtered) >= limit {
			break
		}
	}

	hasMore := len(allFiles) > len(filtered) && len(filtered) == limit

	return filtered, hasMore, nil
}

// Close is a no-op for the in-memory store.
func (s *Store) Close(_ context.Context) error {
	return nil
}
