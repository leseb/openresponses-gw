// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

// Package filestoretest provides a shared conformance test suite for
// filestore.FileStore implementations. Each backend should call
// RunConformanceTests from its own _test.go file.
package filestoretest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/leseb/openresponses-gw/pkg/filestore"
)

// RunConformanceTests exercises a FileStore implementation against the shared
// contract. The newStore function is called once per sub-test to provide an
// isolated store instance.
func RunConformanceTests(t *testing.T, newStore func(t *testing.T) filestore.FileStore) {
	t.Helper()

	t.Run("CreateAndGet", func(t *testing.T) {
		store := newStore(t)
		defer store.Close(context.Background())
		ctx := context.Background()

		f := &filestore.File{
			ID:        "file_abc123",
			Filename:  "hello.txt",
			Purpose:   "assistants",
			MimeType:  "text/plain",
			Bytes:     5,
			Content:   []byte("hello"),
			Status:    "uploaded",
			CreatedAt: time.Now().Truncate(time.Millisecond),
		}

		if err := store.CreateFile(ctx, f); err != nil {
			t.Fatalf("CreateFile: %v", err)
		}

		got, err := store.GetFile(ctx, f.ID)
		if err != nil {
			t.Fatalf("GetFile: %v", err)
		}

		if got.ID != f.ID || got.Filename != f.Filename || got.Purpose != f.Purpose ||
			got.MimeType != f.MimeType || got.Bytes != f.Bytes || got.Status != f.Status {
			t.Errorf("GetFile returned unexpected metadata: %+v", got)
		}

		// Content should be nil from GetFile (metadata-only)
		if got.Content != nil {
			t.Errorf("expected Content to be nil from GetFile, got %d bytes", len(got.Content))
		}
	})

	t.Run("GetContent", func(t *testing.T) {
		store := newStore(t)
		defer store.Close(context.Background())
		ctx := context.Background()

		content := []byte("file content here")
		f := &filestore.File{
			ID:        "file_content1",
			Filename:  "data.bin",
			Purpose:   "assistants",
			MimeType:  "application/octet-stream",
			Bytes:     int64(len(content)),
			Content:   content,
			Status:    "uploaded",
			CreatedAt: time.Now().Truncate(time.Millisecond),
		}

		if err := store.CreateFile(ctx, f); err != nil {
			t.Fatalf("CreateFile: %v", err)
		}

		got, err := store.GetFileContent(ctx, f.ID)
		if err != nil {
			t.Fatalf("GetFileContent: %v", err)
		}

		if string(got) != string(content) {
			t.Errorf("content mismatch: got %q, want %q", got, content)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		store := newStore(t)
		defer store.Close(context.Background())
		ctx := context.Background()

		f := &filestore.File{
			ID:        "file_del1",
			Filename:  "del.txt",
			Purpose:   "assistants",
			MimeType:  "text/plain",
			Bytes:     3,
			Content:   []byte("del"),
			Status:    "uploaded",
			CreatedAt: time.Now().Truncate(time.Millisecond),
		}

		if err := store.CreateFile(ctx, f); err != nil {
			t.Fatalf("CreateFile: %v", err)
		}

		if err := store.DeleteFile(ctx, f.ID); err != nil {
			t.Fatalf("DeleteFile: %v", err)
		}

		_, err := store.GetFile(ctx, f.ID)
		if !errors.Is(err, filestore.ErrFileNotFound) {
			t.Errorf("expected ErrFileNotFound after delete, got: %v", err)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		store := newStore(t)
		defer store.Close(context.Background())
		ctx := context.Background()

		_, err := store.GetFile(ctx, "file_nonexistent")
		if !errors.Is(err, filestore.ErrFileNotFound) {
			t.Errorf("GetFile expected ErrFileNotFound, got: %v", err)
		}

		_, err = store.GetFileContent(ctx, "file_nonexistent")
		if !errors.Is(err, filestore.ErrFileNotFound) {
			t.Errorf("GetFileContent expected ErrFileNotFound, got: %v", err)
		}

		err = store.DeleteFile(ctx, "file_nonexistent")
		if !errors.Is(err, filestore.ErrFileNotFound) {
			t.Errorf("DeleteFile expected ErrFileNotFound, got: %v", err)
		}
	})

	t.Run("ListPaginated", func(t *testing.T) {
		store := newStore(t)
		defer store.Close(context.Background())
		ctx := context.Background()

		// Create files with distinct timestamps
		baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		for i := 0; i < 5; i++ {
			f := &filestore.File{
				ID:        "file_list" + string(rune('a'+i)),
				Filename:  "f" + string(rune('a'+i)) + ".txt",
				Purpose:   "assistants",
				MimeType:  "text/plain",
				Bytes:     1,
				Content:   []byte("x"),
				Status:    "uploaded",
				CreatedAt: baseTime.Add(time.Duration(i) * time.Second),
			}
			if err := store.CreateFile(ctx, f); err != nil {
				t.Fatalf("CreateFile[%d]: %v", i, err)
			}
		}

		// List all ascending
		files, hasMore, err := store.ListFilesPaginated(ctx, "", "", 10, "asc", "")
		if err != nil {
			t.Fatalf("ListFilesPaginated: %v", err)
		}
		if len(files) != 5 {
			t.Errorf("expected 5 files, got %d", len(files))
		}
		if hasMore {
			t.Errorf("expected hasMore=false")
		}

		// Verify ordering (ascending)
		for i := 1; i < len(files); i++ {
			if files[i].CreatedAt.Before(files[i-1].CreatedAt) {
				t.Errorf("files not in ascending order at index %d", i)
			}
		}

		// List with limit
		files, hasMore, err = store.ListFilesPaginated(ctx, "", "", 3, "asc", "")
		if err != nil {
			t.Fatalf("ListFilesPaginated: %v", err)
		}
		if len(files) != 3 {
			t.Errorf("expected 3 files, got %d", len(files))
		}
		if !hasMore {
			t.Errorf("expected hasMore=true with limit=3 and 5 files")
		}
	})

	t.Run("ListFilterByPurpose", func(t *testing.T) {
		store := newStore(t)
		defer store.Close(context.Background())
		ctx := context.Background()

		baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		purposes := []string{"assistants", "vision", "assistants"}
		for i, p := range purposes {
			f := &filestore.File{
				ID:        "file_purpose" + string(rune('a'+i)),
				Filename:  "f.txt",
				Purpose:   p,
				MimeType:  "text/plain",
				Bytes:     1,
				Content:   []byte("x"),
				Status:    "uploaded",
				CreatedAt: baseTime.Add(time.Duration(i) * time.Second),
			}
			if err := store.CreateFile(ctx, f); err != nil {
				t.Fatalf("CreateFile[%d]: %v", i, err)
			}
		}

		files, _, err := store.ListFilesPaginated(ctx, "", "", 10, "asc", "assistants")
		if err != nil {
			t.Fatalf("ListFilesPaginated: %v", err)
		}
		if len(files) != 2 {
			t.Errorf("expected 2 assistants files, got %d", len(files))
		}
		for _, f := range files {
			if f.Purpose != "assistants" {
				t.Errorf("expected purpose=assistants, got %s", f.Purpose)
			}
		}
	})

	t.Run("DuplicateCreate", func(t *testing.T) {
		store := newStore(t)
		defer store.Close(context.Background())
		ctx := context.Background()

		f := &filestore.File{
			ID:        "file_dup1",
			Filename:  "dup.txt",
			Purpose:   "assistants",
			MimeType:  "text/plain",
			Bytes:     3,
			Content:   []byte("dup"),
			Status:    "uploaded",
			CreatedAt: time.Now().Truncate(time.Millisecond),
		}

		if err := store.CreateFile(ctx, f); err != nil {
			t.Fatalf("first CreateFile: %v", err)
		}

		// Memory backend rejects duplicates; filesystem/S3 overwrite is acceptable.
		// We just ensure no panic.
		_ = store.CreateFile(ctx, f)
	})
}
