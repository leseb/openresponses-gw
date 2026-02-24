// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package s3

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/leseb/openresponses-gw/pkg/filestore"
)

func init() {
	filestore.Providers.Register("s3", func(ctx context.Context, params map[string]string) (filestore.FileStore, error) {
		return New(ctx, Options{
			Bucket:   params["bucket"],
			Region:   params["region"],
			Prefix:   params["prefix"],
			Endpoint: params["endpoint"],
		})
	})
}

// compile-time check
var _ filestore.FileStore = (*Store)(nil)

// Options configures the S3 backend.
type Options struct {
	Bucket   string // required
	Region   string // e.g. "us-east-1"
	Prefix   string // key prefix, e.g. "files/"
	Endpoint string // custom endpoint for MinIO compatibility
}

// fileMetadata is the JSON sidecar stored alongside each file in S3.
type fileMetadata struct {
	ID        string    `json:"id"`
	Filename  string    `json:"filename"`
	Purpose   string    `json:"purpose"`
	MimeType  string    `json:"mime_type"`
	Bytes     int64     `json:"bytes"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// Store implements filestore.FileStore backed by S3 (or MinIO).
//
// Object layout:
//
//	<prefix><file_id>/content
//	<prefix><file_id>/metadata.json
type Store struct {
	client *s3.Client
	bucket string
	prefix string
}

// New creates an S3-backed Store.
func New(ctx context.Context, opts Options) (*Store, error) {
	if opts.Bucket == "" {
		return nil, fmt.Errorf("s3 filestore: bucket is required")
	}

	optFns := []func(*awsconfig.LoadOptions) error{}
	if opts.Region != "" {
		optFns = append(optFns, awsconfig.WithRegion(opts.Region))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	s3Opts := []func(*s3.Options){}
	if opts.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(opts.Endpoint)
			o.UsePathStyle = true // required for MinIO
		})
	}

	client := s3.NewFromConfig(cfg, s3Opts...)

	return &Store{
		client: client,
		bucket: opts.Bucket,
		prefix: opts.Prefix,
	}, nil
}

func (s *Store) contentKey(fileID string) string {
	return s.prefix + fileID + "/content"
}

func (s *Store) metadataKey(fileID string) string {
	return s.prefix + fileID + "/metadata.json"
}

// CreateFile uploads both content and metadata.json to S3.
func (s *Store) CreateFile(ctx context.Context, file *filestore.File) error {
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

	// Upload content
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(s.contentKey(file.ID)),
		Body:        bytes.NewReader(file.Content),
		ContentType: aws.String(file.MimeType),
	})
	if err != nil {
		return fmt.Errorf("put content: %w", err)
	}

	// Upload metadata
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(s.metadataKey(file.ID)),
		Body:        bytes.NewReader(metaBytes),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return fmt.Errorf("put metadata: %w", err)
	}

	return nil
}

// GetFile returns file metadata (Content is nil).
func (s *Store) GetFile(ctx context.Context, fileID string) (*filestore.File, error) {
	meta, err := s.readMetadata(ctx, fileID)
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

// GetFileContent returns the raw file bytes from S3.
func (s *Store) GetFileContent(ctx context.Context, fileID string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.contentKey(fileID)),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("file %s: %w", fileID, filestore.ErrFileNotFound)
		}
		return nil, fmt.Errorf("get content: %w", err)
	}
	defer out.Body.Close()

	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("read content body: %w", err)
	}
	return data, nil
}

// DeleteFile removes both the content and metadata objects.
func (s *Store) DeleteFile(ctx context.Context, fileID string) error {
	// Check existence first
	_, err := s.readMetadata(ctx, fileID)
	if err != nil {
		return err
	}

	_, err = s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(s.bucket),
		Delete: &s3types.Delete{
			Objects: []s3types.ObjectIdentifier{
				{Key: aws.String(s.contentKey(fileID))},
				{Key: aws.String(s.metadataKey(fileID))},
			},
			Quiet: aws.Bool(true),
		},
	})
	if err != nil {
		return fmt.Errorf("delete objects: %w", err)
	}
	return nil
}

// ListFilesPaginated lists files sorted by CreatedAt with cursor-based pagination.
func (s *Store) ListFilesPaginated(ctx context.Context, after, before string, limit int, order, purpose string) ([]*filestore.File, bool, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// List "directories" under prefix using delimiter
	delimiter := "/"
	var allFileIDs []string

	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket:    aws.String(s.bucket),
		Prefix:    aws.String(s.prefix),
		Delimiter: aws.String(delimiter),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, false, fmt.Errorf("list objects: %w", err)
		}
		for _, cp := range page.CommonPrefixes {
			// Extract file ID from prefix: "<prefix><file_id>/"
			dir := aws.ToString(cp.Prefix)
			dir = strings.TrimPrefix(dir, s.prefix)
			dir = strings.TrimSuffix(dir, "/")
			if dir != "" {
				allFileIDs = append(allFileIDs, dir)
			}
		}
	}

	// Fetch metadata concurrently with a semaphore
	const maxConcurrency = 10
	sem := make(chan struct{}, maxConcurrency)
	var mu sync.Mutex
	var allFiles []*filestore.File
	var fetchErr error

	var wg sync.WaitGroup
	for _, id := range allFileIDs {
		if fetchErr != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(fileID string) {
			defer wg.Done()
			defer func() { <-sem }()

			meta, err := s.readMetadata(ctx, fileID)
			if err != nil {
				mu.Lock()
				if fetchErr == nil {
					fetchErr = err
				}
				mu.Unlock()
				return
			}

			if purpose != "" && meta.Purpose != purpose {
				return
			}

			f := &filestore.File{
				ID:        meta.ID,
				Filename:  meta.Filename,
				Purpose:   meta.Purpose,
				MimeType:  meta.MimeType,
				Bytes:     meta.Bytes,
				Status:    meta.Status,
				CreatedAt: meta.CreatedAt,
			}

			mu.Lock()
			allFiles = append(allFiles, f)
			mu.Unlock()
		}(id)
	}
	wg.Wait()

	if fetchErr != nil {
		return nil, false, fetchErr
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

// Close is a no-op for the S3 store.
func (s *Store) Close(_ context.Context) error {
	return nil
}

// readMetadata fetches and unmarshals metadata.json from S3.
func (s *Store) readMetadata(ctx context.Context, fileID string) (*fileMetadata, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.metadataKey(fileID)),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("file %s: %w", fileID, filestore.ErrFileNotFound)
		}
		return nil, fmt.Errorf("get metadata: %w", err)
	}
	defer out.Body.Close()

	var meta fileMetadata
	if err := json.NewDecoder(out.Body).Decode(&meta); err != nil {
		return nil, fmt.Errorf("decode metadata for %s: %w", fileID, err)
	}
	return &meta, nil
}

// isNotFound checks whether the error indicates a missing S3 object.
func isNotFound(err error) bool {
	var nsk *s3types.NoSuchKey
	if ok := errors.As(err, &nsk); ok {
		return true
	}
	// Some S3-compatible services return a generic "NotFound" status.
	return strings.Contains(err.Error(), "NoSuchKey") || strings.Contains(err.Error(), "NotFound")
}
