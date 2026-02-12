// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package milvus

import (
	"context"
	"fmt"
	"strings"

	"github.com/leseb/openresponses-gw/pkg/vectorstore"
	milvusclient "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

const (
	fieldChunkID   = "chunk_id"
	fieldFileID    = "file_id"
	fieldContent   = "content"
	fieldEmbedding = "embedding"

	maxContentLength = 65535
	maxChunkIDLength = 256
	maxFileIDLength  = 256
)

// Backend implements vectorstore.Backend using Milvus.
// One Milvus collection is created per vector store.
type Backend struct {
	client milvusclient.Client
}

// NewBackend connects to Milvus and returns a Backend.
func NewBackend(ctx context.Context, address string) (*Backend, error) {
	c, err := milvusclient.NewClient(ctx, milvusclient.Config{
		Address: address,
	})
	if err != nil {
		return nil, fmt.Errorf("milvus connect %s: %w", address, err)
	}
	return &Backend{client: c}, nil
}

// collectionName derives a Milvus collection name from a vector store ID.
// Milvus collection names must start with a letter or underscore, so we
// keep the "vs_" prefix which satisfies that constraint.
func collectionName(vectorStoreID string) string {
	return vectorStoreID
}

// CreateStore creates a Milvus collection, an HNSW index, and loads it.
func (b *Backend) CreateStore(ctx context.Context, vectorStoreID string, dimensions int) error {
	coll := collectionName(vectorStoreID)

	schema := entity.NewSchema().
		WithName(coll).
		WithField(entity.NewField().
			WithName(fieldChunkID).
			WithDataType(entity.FieldTypeVarChar).
			WithMaxLength(int64(maxChunkIDLength)).
			WithIsPrimaryKey(true)).
		WithField(entity.NewField().
			WithName(fieldFileID).
			WithDataType(entity.FieldTypeVarChar).
			WithMaxLength(int64(maxFileIDLength))).
		WithField(entity.NewField().
			WithName(fieldContent).
			WithDataType(entity.FieldTypeVarChar).
			WithMaxLength(int64(maxContentLength))).
		WithField(entity.NewField().
			WithName(fieldEmbedding).
			WithDataType(entity.FieldTypeFloatVector).
			WithDim(int64(dimensions)))

	if err := b.client.CreateCollection(ctx, schema, 1); err != nil {
		return fmt.Errorf("create collection %s: %w", coll, err)
	}

	idx, err := entity.NewIndexHNSW(entity.COSINE, 16, 200)
	if err != nil {
		return fmt.Errorf("create HNSW index params: %w", err)
	}

	if err := b.client.CreateIndex(ctx, coll, fieldEmbedding, idx, false); err != nil {
		return fmt.Errorf("create index on %s: %w", coll, err)
	}

	if err := b.client.LoadCollection(ctx, coll, false); err != nil {
		return fmt.Errorf("load collection %s: %w", coll, err)
	}

	return nil
}

// DeleteStore drops the Milvus collection for the given vector store.
func (b *Backend) DeleteStore(ctx context.Context, vectorStoreID string) error {
	coll := collectionName(vectorStoreID)

	exists, err := b.client.HasCollection(ctx, coll)
	if err != nil {
		return fmt.Errorf("check collection %s: %w", coll, err)
	}
	if !exists {
		return nil
	}

	if err := b.client.DropCollection(ctx, coll); err != nil {
		return fmt.Errorf("drop collection %s: %w", coll, err)
	}
	return nil
}

// InsertChunks inserts embedded chunks into the appropriate Milvus collection.
// All chunks must belong to the same vector store.
func (b *Backend) InsertChunks(ctx context.Context, chunks []vectorstore.Chunk) error {
	if len(chunks) == 0 {
		return nil
	}

	coll := collectionName(chunks[0].VectorStoreID)

	chunkIDs := make([]string, len(chunks))
	fileIDs := make([]string, len(chunks))
	contents := make([]string, len(chunks))
	vectors := make([][]float32, len(chunks))

	for i, c := range chunks {
		chunkIDs[i] = c.ChunkID
		fileIDs[i] = c.FileID
		content := c.Content
		if len(content) > maxContentLength {
			content = content[:maxContentLength]
		}
		contents[i] = content
		vectors[i] = c.Vector
	}

	dim := len(vectors[0])
	_, err := b.client.Insert(ctx, coll, "",
		entity.NewColumnVarChar(fieldChunkID, chunkIDs),
		entity.NewColumnVarChar(fieldFileID, fileIDs),
		entity.NewColumnVarChar(fieldContent, contents),
		entity.NewColumnFloatVector(fieldEmbedding, dim, vectors),
	)
	if err != nil {
		return fmt.Errorf("insert into %s: %w", coll, err)
	}

	if err := b.client.Flush(ctx, coll, false); err != nil {
		return fmt.Errorf("flush %s: %w", coll, err)
	}

	return nil
}

// DeleteFileChunks removes all chunks for a given file from the vector store.
func (b *Backend) DeleteFileChunks(ctx context.Context, vectorStoreID, fileID string) error {
	coll := collectionName(vectorStoreID)

	exists, err := b.client.HasCollection(ctx, coll)
	if err != nil {
		return fmt.Errorf("check collection %s: %w", coll, err)
	}
	if !exists {
		return nil
	}

	expr := fmt.Sprintf(`%s == "%s"`, fieldFileID, escapeExpr(fileID))
	if err := b.client.Delete(ctx, coll, "", expr); err != nil {
		return fmt.Errorf("delete file chunks from %s: %w", coll, err)
	}
	return nil
}

// Search performs a vector similarity search in the given vector store.
func (b *Backend) Search(ctx context.Context, vectorStoreID string, queryVector []float32, topK int) ([]vectorstore.SearchResult, error) {
	coll := collectionName(vectorStoreID)

	exists, err := b.client.HasCollection(ctx, coll)
	if err != nil {
		return nil, fmt.Errorf("check collection %s: %w", coll, err)
	}
	if !exists {
		return nil, nil
	}

	if topK <= 0 {
		topK = 10
	}

	sp, err := entity.NewIndexHNSWSearchParam(64)
	if err != nil {
		return nil, fmt.Errorf("create search params: %w", err)
	}

	results, err := b.client.Search(
		ctx,
		coll,
		nil,
		"",
		[]string{fieldChunkID, fieldFileID, fieldContent},
		[]entity.Vector{entity.FloatVector(queryVector)},
		fieldEmbedding,
		entity.COSINE,
		topK,
		sp,
	)
	if err != nil {
		return nil, fmt.Errorf("search %s: %w", coll, err)
	}

	if len(results) == 0 {
		return nil, nil
	}

	sr := results[0]
	if sr.Err != nil {
		return nil, fmt.Errorf("search result error: %w", sr.Err)
	}

	chunkIDCol := sr.Fields.GetColumn(fieldChunkID)
	fileIDCol := sr.Fields.GetColumn(fieldFileID)
	contentCol := sr.Fields.GetColumn(fieldContent)

	var out []vectorstore.SearchResult
	for i := 0; i < sr.ResultCount; i++ {
		chunkID, _ := chunkIDCol.GetAsString(i)
		fileID, _ := fileIDCol.GetAsString(i)
		content, _ := contentCol.GetAsString(i)

		out = append(out, vectorstore.SearchResult{
			FileID:  fileID,
			ChunkID: chunkID,
			Content: content,
			Score:   float64(sr.Scores[i]),
		})
	}

	return out, nil
}

// Close releases the Milvus client connection.
func (b *Backend) Close(ctx context.Context) error {
	return b.client.Close()
}

// escapeExpr escapes double quotes in a string for Milvus filter expressions.
func escapeExpr(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}
