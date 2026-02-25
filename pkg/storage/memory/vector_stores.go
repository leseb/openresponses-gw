// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// VectorStore represents a stored vector store
type VectorStore struct {
	ID           string
	Name         string
	Status       string
	UsageBytes   int64
	FileCounts   VectorStoreFileCounts
	CreatedAt    time.Time
	ExpiresAt    *time.Time
	ExpiresAfter *VectorStoreExpiration
	LastActiveAt *time.Time
	Metadata     map[string]string
	FileIDs      []string // Track associated files
}

// VectorStoreFileCounts represents file count statistics
type VectorStoreFileCounts struct {
	InProgress int
	Completed  int
	Failed     int
	Cancelled  int
	Total      int
}

// VectorStoreExpiration represents expiration policy
type VectorStoreExpiration struct {
	Anchor string
	Days   int
}

// VectorStoreFile represents a file associated with a vector store
type VectorStoreFile struct {
	ID               string
	VectorStoreID    string
	FileID           string
	Status           string
	UsageBytes       int64
	CreatedAt        time.Time
	LastError        *VectorStoreFileError
	ChunkingStrategy *ChunkingStrategy
	Attributes       map[string]interface{} // File attributes for filtering
}

// VectorStoreFileError represents an error processing a file
type VectorStoreFileError struct {
	Code    string
	Message string
}

// ChunkingStrategy represents the chunking strategy
type ChunkingStrategy struct {
	Type   string
	Static *StaticChunkingStrategy
}

// StaticChunkingStrategy represents static chunking parameters
type StaticChunkingStrategy struct {
	MaxChunkSizeTokens int
	ChunkOverlapTokens int
}

// VectorStoresStore is an in-memory vector stores store
type VectorStoresStore struct {
	mu           sync.RWMutex
	vectorStores map[string]*VectorStore
	vsFiles      map[string]*VectorStoreFile      // Key: vector_store_id:file_id
	vsBatches    map[string]*VectorStoreFileBatch // Key: batch_id
}

// NewVectorStoresStore creates a new vector stores store
func NewVectorStoresStore() *VectorStoresStore {
	return &VectorStoresStore{
		vectorStores: make(map[string]*VectorStore),
		vsFiles:      make(map[string]*VectorStoreFile),
		vsBatches:    make(map[string]*VectorStoreFileBatch),
	}
}

// CreateVectorStore creates a new vector store
func (s *VectorStoresStore) CreateVectorStore(ctx context.Context, vs *VectorStore) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.vectorStores[vs.ID]; exists {
		return fmt.Errorf("vector store %s already exists", vs.ID)
	}

	s.vectorStores[vs.ID] = vs
	return nil
}

// GetVectorStore retrieves a vector store by ID
func (s *VectorStoresStore) GetVectorStore(ctx context.Context, vsID string) (*VectorStore, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	vs, exists := s.vectorStores[vsID]
	if !exists {
		return nil, fmt.Errorf("vector store %s not found", vsID)
	}

	return vs, nil
}

// UpdateVectorStore updates an existing vector store
func (s *VectorStoresStore) UpdateVectorStore(ctx context.Context, vs *VectorStore) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.vectorStores[vs.ID]; !exists {
		return fmt.Errorf("vector store %s not found", vs.ID)
	}

	s.vectorStores[vs.ID] = vs
	return nil
}

// DeleteVectorStore deletes a vector store
func (s *VectorStoresStore) DeleteVectorStore(ctx context.Context, vsID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.vectorStores[vsID]; !exists {
		return fmt.Errorf("vector store %s not found", vsID)
	}

	// Delete all associated files
	for key := range s.vsFiles {
		if s.vsFiles[key].VectorStoreID == vsID {
			delete(s.vsFiles, key)
		}
	}

	delete(s.vectorStores, vsID)
	return nil
}

// ListVectorStoresPaginated lists vector stores with pagination
func (s *VectorStoresStore) ListVectorStoresPaginated(ctx context.Context, after, before string, limit int, order string) ([]*VectorStore, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Default limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Collect all vector stores
	var allStores []*VectorStore
	for _, vs := range s.vectorStores {
		allStores = append(allStores, vs)
	}

	// Apply cursor-based pagination
	var filtered []*VectorStore
	foundAfter := after == ""

	for _, vs := range allStores {
		// Handle after cursor
		if after != "" && !foundAfter {
			if vs.ID == after {
				foundAfter = true
			}
			continue
		}

		// Handle before cursor
		if before != "" && vs.ID == before {
			break
		}

		filtered = append(filtered, vs)

		// Limit results
		if len(filtered) >= limit {
			break
		}
	}

	// Check if there are more results
	hasMore := len(allStores) > len(filtered) && len(filtered) == limit

	return filtered, hasMore, nil
}

// AddVectorStoreFile adds a file to a vector store
func (s *VectorStoresStore) AddVectorStoreFile(ctx context.Context, vsFile *VectorStoreFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if vector store exists
	vs, exists := s.vectorStores[vsFile.VectorStoreID]
	if !exists {
		return fmt.Errorf("vector store %s not found", vsFile.VectorStoreID)
	}

	// Create composite key
	key := vsFile.VectorStoreID + ":" + vsFile.FileID

	if _, exists := s.vsFiles[key]; exists {
		return fmt.Errorf("file %s already in vector store %s", vsFile.FileID, vsFile.VectorStoreID)
	}

	s.vsFiles[key] = vsFile

	// Update vector store file counts
	vs.FileIDs = append(vs.FileIDs, vsFile.FileID)
	vs.FileCounts.Total++
	switch vsFile.Status {
	case "in_progress":
		vs.FileCounts.InProgress++
	case "completed":
		vs.FileCounts.Completed++
	case "failed":
		vs.FileCounts.Failed++
	case "cancelled":
		vs.FileCounts.Cancelled++
	}

	return nil
}

// GetVectorStoreFile retrieves a file from a vector store
func (s *VectorStoresStore) GetVectorStoreFile(ctx context.Context, vsID, fileID string) (*VectorStoreFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := vsID + ":" + fileID
	vsFile, exists := s.vsFiles[key]
	if !exists {
		return nil, fmt.Errorf("file %s not found in vector store %s", fileID, vsID)
	}

	return vsFile, nil
}

// UpdateVectorStoreFile updates a file's metadata in a vector store
func (s *VectorStoresStore) UpdateVectorStoreFile(ctx context.Context, vsFile *VectorStoreFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := vsFile.VectorStoreID + ":" + vsFile.FileID
	old, exists := s.vsFiles[key]
	if !exists {
		return fmt.Errorf("file %s not found in vector store %s", vsFile.FileID, vsFile.VectorStoreID)
	}

	// Update file counts if status changed
	if old.Status != vsFile.Status {
		vs, vsExists := s.vectorStores[vsFile.VectorStoreID]
		if vsExists {
			decrementFileCount(&vs.FileCounts, old.Status)
			incrementFileCount(&vs.FileCounts, vsFile.Status)
		}
	}

	s.vsFiles[key] = vsFile
	return nil
}

// DeleteVectorStoreFile removes a file from a vector store
func (s *VectorStoresStore) DeleteVectorStoreFile(ctx context.Context, vsID, fileID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := vsID + ":" + fileID
	vsFile, exists := s.vsFiles[key]
	if !exists {
		return fmt.Errorf("file %s not found in vector store %s", fileID, vsID)
	}

	// Update vector store file counts
	vs, exists := s.vectorStores[vsID]
	if exists {
		vs.FileCounts.Total--
		switch vsFile.Status {
		case "in_progress":
			vs.FileCounts.InProgress--
		case "completed":
			vs.FileCounts.Completed--
		case "failed":
			vs.FileCounts.Failed--
		case "cancelled":
			vs.FileCounts.Cancelled--
		}

		// Remove from file IDs
		for i, fid := range vs.FileIDs {
			if fid == fileID {
				vs.FileIDs = append(vs.FileIDs[:i], vs.FileIDs[i+1:]...)
				break
			}
		}
	}

	delete(s.vsFiles, key)
	return nil
}

// ListVectorStoreFilesPaginated lists files in a vector store with pagination
func (s *VectorStoresStore) ListVectorStoreFilesPaginated(ctx context.Context, vsID, after, before string, limit int, order, filter string) ([]*VectorStoreFile, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check if vector store exists
	if _, exists := s.vectorStores[vsID]; !exists {
		return nil, false, fmt.Errorf("vector store %s not found", vsID)
	}

	// Default limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Collect all files for this vector store
	var allFiles []*VectorStoreFile
	for _, vsFile := range s.vsFiles {
		if vsFile.VectorStoreID == vsID {
			// Apply filter if specified
			if filter != "" && vsFile.Status != filter {
				continue
			}
			allFiles = append(allFiles, vsFile)
		}
	}

	// Apply cursor-based pagination
	var filtered []*VectorStoreFile
	foundAfter := after == ""

	for _, vsFile := range allFiles {
		// Handle after cursor
		if after != "" && !foundAfter {
			if vsFile.FileID == after {
				foundAfter = true
			}
			continue
		}

		// Handle before cursor
		if before != "" && vsFile.FileID == before {
			break
		}

		filtered = append(filtered, vsFile)

		// Limit results
		if len(filtered) >= limit {
			break
		}
	}

	// Check if there are more results
	hasMore := len(allFiles) > len(filtered) && len(filtered) == limit

	return filtered, hasMore, nil
}

// VectorStoreFileBatch represents a batch of files being processed
type VectorStoreFileBatch struct {
	ID            string
	VectorStoreID string
	Status        string // "in_progress", "completed", "cancelled", "failed"
	FileCounts    VectorStoreFileCounts
	CreatedAt     time.Time
}

// CreateVectorStoreFileBatch creates a new file batch
func (s *VectorStoresStore) CreateVectorStoreFileBatch(ctx context.Context, batch *VectorStoreFileBatch) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.vsBatches == nil {
		s.vsBatches = make(map[string]*VectorStoreFileBatch)
	}

	s.vsBatches[batch.ID] = batch
	return nil
}

// GetVectorStoreFileBatch retrieves a file batch by ID
func (s *VectorStoresStore) GetVectorStoreFileBatch(ctx context.Context, vsID, batchID string) (*VectorStoreFileBatch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	batch, exists := s.vsBatches[batchID]
	if !exists || batch.VectorStoreID != vsID {
		return nil, fmt.Errorf("batch %s not found in vector store %s", batchID, vsID)
	}

	return batch, nil
}

func incrementFileCount(fc *VectorStoreFileCounts, status string) {
	switch status {
	case "in_progress":
		fc.InProgress++
	case "completed":
		fc.Completed++
	case "failed":
		fc.Failed++
	case "cancelled":
		fc.Cancelled++
	}
}

func decrementFileCount(fc *VectorStoreFileCounts, status string) {
	switch status {
	case "in_progress":
		if fc.InProgress > 0 {
			fc.InProgress--
		}
	case "completed":
		if fc.Completed > 0 {
			fc.Completed--
		}
	case "failed":
		if fc.Failed > 0 {
			fc.Failed--
		}
	case "cancelled":
		if fc.Cancelled > 0 {
			fc.Cancelled--
		}
	}
}

// UpdateVectorStoreFileBatch updates a file batch
func (s *VectorStoresStore) UpdateVectorStoreFileBatch(ctx context.Context, batch *VectorStoreFileBatch) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.vsBatches[batch.ID]; !exists {
		return fmt.Errorf("batch %s not found", batch.ID)
	}

	s.vsBatches[batch.ID] = batch
	return nil
}
