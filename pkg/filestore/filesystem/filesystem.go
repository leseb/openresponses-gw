// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/leseb/openresponses-gw/pkg/filestore"
)

func init() {
	filestore.Providers.Register("filesystem", func(_ context.Context, params map[string]string) (filestore.FileStore, error) {
		return New(params["base_dir"])
	})
}

// compile-time check
var _ filestore.FileStore = (*Store)(nil)

// fileMetadata is the on-disk representation stored in metadata.json.
type fileMetadata struct {
	ID        string    `json:"id"`
	Filename  string    `json:"filename"`
	Purpose   string    `json:"purpose"`
	MimeType  string    `json:"mime_type"`
	Bytes     int64     `json:"bytes"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// Store implements filestore.FileStore backed by a local filesystem.
//
// Layout:
//
//	<baseDir>/<file_id>/content        — raw file bytes
//	<baseDir>/<file_id>/metadata.json  — JSON metadata sidecar
type Store struct {
	baseDir string
}

// New creates a filesystem-backed Store, creating baseDir if it does not exist.
func New(baseDir string) (*Store, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("create base dir %s: %w", baseDir, err)
	}
	return &Store{baseDir: baseDir}, nil
}

// CreateFile writes the file content and metadata to disk atomically.
func (s *Store) CreateFile(_ context.Context, file *filestore.File) error {
	dir := filepath.Join(s.baseDir, file.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create file dir: %w", err)
	}

	// Write content atomically (temp file + rename)
	contentPath := filepath.Join(dir, "content")
	tmpContent := contentPath + ".tmp"
	if err := os.WriteFile(tmpContent, file.Content, 0o644); err != nil {
		return fmt.Errorf("write content: %w", err)
	}
	if err := os.Rename(tmpContent, contentPath); err != nil {
		return fmt.Errorf("rename content: %w", err)
	}

	// Write metadata
	meta := fileMetadata{
		ID:        file.ID,
		Filename:  file.Filename,
		Purpose:   file.Purpose,
		MimeType:  file.MimeType,
		Bytes:     file.Bytes,
		Status:    file.Status,
		CreatedAt: file.CreatedAt,
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	metaPath := filepath.Join(dir, "metadata.json")
	tmpMeta := metaPath + ".tmp"
	if err := os.WriteFile(tmpMeta, metaBytes, 0o644); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}
	if err := os.Rename(tmpMeta, metaPath); err != nil {
		return fmt.Errorf("rename metadata: %w", err)
	}

	return nil
}

// GetFile returns file metadata (Content is nil).
func (s *Store) GetFile(_ context.Context, fileID string) (*filestore.File, error) {
	meta, err := s.readMetadata(fileID)
	if err != nil {
		return nil, err
	}

	return &filestore.File{
		ID:        meta.ID,
		Filename:  meta.Filename,
		Purpose:   meta.Purpose,
		MimeType:  meta.MimeType,
		Bytes:     meta.Bytes,
		Status:    meta.Status,
		CreatedAt: meta.CreatedAt,
	}, nil
}

// GetFileContent returns the raw file bytes.
func (s *Store) GetFileContent(_ context.Context, fileID string) ([]byte, error) {
	contentPath := filepath.Join(s.baseDir, fileID, "content")
	data, err := os.ReadFile(contentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file %s: %w", fileID, filestore.ErrFileNotFound)
		}
		return nil, fmt.Errorf("read content: %w", err)
	}
	return data, nil
}

// DeleteFile removes the file directory and all its contents.
func (s *Store) DeleteFile(_ context.Context, fileID string) error {
	dir := filepath.Join(s.baseDir, fileID)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file %s: %w", fileID, filestore.ErrFileNotFound)
		}
		return fmt.Errorf("stat file dir: %w", err)
	}
	return os.RemoveAll(dir)
}

// ListFilesPaginated lists files sorted by CreatedAt with cursor-based pagination.
func (s *Store) ListFilesPaginated(_ context.Context, after, before string, limit int, order, purpose string) ([]*filestore.File, bool, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, false, fmt.Errorf("read base dir: %w", err)
	}

	// Read metadata for each entry
	var allFiles []*filestore.File
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, err := s.readMetadata(entry.Name())
		if err != nil {
			continue // skip corrupt entries
		}
		if purpose != "" && meta.Purpose != purpose {
			continue
		}
		allFiles = append(allFiles, &filestore.File{
			ID:        meta.ID,
			Filename:  meta.Filename,
			Purpose:   meta.Purpose,
			MimeType:  meta.MimeType,
			Bytes:     meta.Bytes,
			Status:    meta.Status,
			CreatedAt: meta.CreatedAt,
		})
	}

	// Sort by CreatedAt
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

// Close is a no-op for the filesystem store.
func (s *Store) Close(_ context.Context) error {
	return nil
}

// readMetadata reads and unmarshals the metadata.json for a file ID.
func (s *Store) readMetadata(fileID string) (*fileMetadata, error) {
	metaPath := filepath.Join(s.baseDir, fileID, "metadata.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file %s: %w", fileID, filestore.ErrFileNotFound)
		}
		return nil, fmt.Errorf("read metadata: %w", err)
	}

	var meta fileMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("unmarshal metadata for %s: %w", fileID, err)
	}
	return &meta, nil
}
