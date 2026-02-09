// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// File represents a stored file with metadata and content
type File struct {
	ID        string
	Filename  string
	Purpose   string
	MimeType  string
	Bytes     int64
	Content   []byte
	Status    string
	CreatedAt time.Time
}

// FilesStore is an in-memory files store
type FilesStore struct {
	mu    sync.RWMutex
	files map[string]*File
}

// NewFilesStore creates a new files store
func NewFilesStore() *FilesStore {
	return &FilesStore{
		files: make(map[string]*File),
	}
}

// CreateFile creates a new file
func (s *FilesStore) CreateFile(ctx context.Context, file *File) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.files[file.ID]; exists {
		return fmt.Errorf("file %s already exists", file.ID)
	}

	s.files[file.ID] = file
	return nil
}

// GetFile retrieves a file by ID
func (s *FilesStore) GetFile(ctx context.Context, fileID string) (*File, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	file, exists := s.files[fileID]
	if !exists {
		return nil, fmt.Errorf("file %s not found", fileID)
	}

	return file, nil
}

// GetFileContent retrieves file content by ID
func (s *FilesStore) GetFileContent(ctx context.Context, fileID string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	file, exists := s.files[fileID]
	if !exists {
		return nil, fmt.Errorf("file %s not found", fileID)
	}

	return file.Content, nil
}

// DeleteFile deletes a file
func (s *FilesStore) DeleteFile(ctx context.Context, fileID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.files[fileID]; !exists {
		return fmt.Errorf("file %s not found", fileID)
	}

	delete(s.files, fileID)
	return nil
}

// ListFilesPaginated lists files with pagination
func (s *FilesStore) ListFilesPaginated(ctx context.Context, after, before string, limit int, order, purpose string) ([]*File, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Default limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// Collect all files
	var allFiles []*File
	for _, file := range s.files {
		// Filter by purpose if specified
		if purpose != "" && file.Purpose != purpose {
			continue
		}
		allFiles = append(allFiles, file)
	}

	// Apply cursor-based pagination
	var filtered []*File
	foundAfter := after == ""

	for _, file := range allFiles {
		// Handle after cursor
		if after != "" && !foundAfter {
			if file.ID == after {
				foundAfter = true
			}
			continue
		}

		// Handle before cursor
		if before != "" && file.ID == before {
			break
		}

		filtered = append(filtered, file)

		// Limit results
		if len(filtered) >= limit {
			break
		}
	}

	// Check if there are more results
	hasMore := len(allFiles) > len(filtered) && len(filtered) == limit

	return filtered, hasMore, nil
}
