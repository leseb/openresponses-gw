// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package filestore

import (
	"context"
	"errors"
	"time"
)

// ErrFileNotFound is returned when a file does not exist.
var ErrFileNotFound = errors.New("file not found")

// File represents a stored file with metadata and content.
type File struct {
	ID        string
	Filename  string
	Purpose   string
	MimeType  string
	Bytes     int64
	Content   []byte // populated for CreateFile input; nil for GetFile output
	Status    string
	CreatedAt time.Time
}

// FileStore defines the interface for pluggable file storage backends.
type FileStore interface {
	CreateFile(ctx context.Context, file *File) error
	GetFile(ctx context.Context, fileID string) (*File, error)
	GetFileContent(ctx context.Context, fileID string) ([]byte, error)
	DeleteFile(ctx context.Context, fileID string) error
	ListFilesPaginated(ctx context.Context, after, before string, limit int, order, purpose string) ([]*File, bool, error)
	Close(ctx context.Context) error
}
