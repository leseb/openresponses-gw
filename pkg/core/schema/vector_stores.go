// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package schema

// VectorStore represents a vector store
type VectorStore struct {
	ID           string                 `json:"id"`                       // Format: "vs_{uuid}"
	Object       string                 `json:"object"`                   // Always "vector_store"
	Name         string                 `json:"name"`                     // Human-readable name
	Status       string                 `json:"status"`                   // "in_progress", "completed", "failed"
	UsageBytes   int64                  `json:"usage_bytes"`              // Total bytes used
	FileCounts   VectorStoreFileCounts  `json:"file_counts"`              // File count statistics
	CreatedAt    int64                  `json:"created_at"`               // Unix timestamp
	ExpiresAt    *int64                 `json:"expires_at,omitempty"`     // Unix timestamp
	ExpiresAfter *VectorStoreExpiration `json:"expires_after,omitempty"`  // Expiration policy
	LastActiveAt *int64                 `json:"last_active_at,omitempty"` // Unix timestamp
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// VectorStoreFileCounts represents file count statistics
type VectorStoreFileCounts struct {
	InProgress int `json:"in_progress"` // Files being processed
	Completed  int `json:"completed"`   // Files successfully processed
	Failed     int `json:"failed"`      // Files that failed processing
	Cancelled  int `json:"cancelled"`   // Files that were cancelled
	Total      int `json:"total"`       // Total files
}

// VectorStoreExpiration represents expiration policy
type VectorStoreExpiration struct {
	Anchor string `json:"anchor"` // "last_active_at"
	Days   int    `json:"days"`   // Number of days
}

// CreateVectorStoreRequest represents a request to create a vector store
type CreateVectorStoreRequest struct {
	Name         string                 `json:"name,omitempty"`
	FileIDs      []string               `json:"file_ids,omitempty"` // Up to 500 files
	ExpiresAfter *VectorStoreExpiration `json:"expires_after,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// UpdateVectorStoreRequest represents a request to update a vector store
type UpdateVectorStoreRequest struct {
	Name         *string                `json:"name,omitempty"`
	ExpiresAfter *VectorStoreExpiration `json:"expires_after,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// ListVectorStoresRequest represents a request to list vector stores
type ListVectorStoresRequest struct {
	After  string `json:"after,omitempty"`
	Before string `json:"before,omitempty"`
	Limit  int    `json:"limit,omitempty"` // 1-100, default 20
	Order  string `json:"order,omitempty"` // "asc" or "desc", default "desc"
}

// ListVectorStoresResponse represents a list of vector stores
type ListVectorStoresResponse struct {
	Object  string        `json:"object"`             // Always "list"
	Data    []VectorStore `json:"data"`               // Array of vector stores
	FirstID string        `json:"first_id,omitempty"` // ID of first item
	LastID  string        `json:"last_id,omitempty"`  // ID of last item
	HasMore bool          `json:"has_more"`           // Whether there are more results
}

// DeleteVectorStoreResponse represents the response from deleting a vector store
type DeleteVectorStoreResponse struct {
	ID      string `json:"id"`      // Vector store ID
	Object  string `json:"object"`  // Always "vector_store.deleted"
	Deleted bool   `json:"deleted"` // Always true
}

// VectorStoreFile represents a file in a vector store
type VectorStoreFile struct {
	ID               string                `json:"id"`                    // Format: "file_{uuid}"
	Object           string                `json:"object"`                // Always "vector_store.file"
	VectorStoreID    string                `json:"vector_store_id"`       // Associated vector store
	Status           string                `json:"status"`                // "in_progress", "completed", "failed", "cancelled"
	UsageBytes       int64                 `json:"usage_bytes,omitempty"` // Bytes used
	CreatedAt        int64                 `json:"created_at"`            // Unix timestamp
	LastError        *VectorStoreFileError `json:"last_error,omitempty"`  // Last error if failed
	ChunkingStrategy *ChunkingStrategy     `json:"chunking_strategy,omitempty"`
}

// VectorStoreFileError represents an error processing a file
type VectorStoreFileError struct {
	Code    string `json:"code"`    // Error code
	Message string `json:"message"` // Error message
}

// ChunkingStrategy represents the chunking strategy
type ChunkingStrategy struct {
	Type   string                  `json:"type"` // "auto" or "static"
	Static *StaticChunkingStrategy `json:"static,omitempty"`
}

// StaticChunkingStrategy represents static chunking parameters
type StaticChunkingStrategy struct {
	MaxChunkSizeTokens int `json:"max_chunk_size_tokens"` // Max tokens per chunk
	ChunkOverlapTokens int `json:"chunk_overlap_tokens"`  // Overlap between chunks
}

// AddVectorStoreFileRequest represents a request to add a file to a vector store
type AddVectorStoreFileRequest struct {
	FileID           string            `json:"file_id"` // Required
	ChunkingStrategy *ChunkingStrategy `json:"chunking_strategy,omitempty"`
}

// ListVectorStoreFilesRequest represents a request to list files in a vector store
type ListVectorStoreFilesRequest struct {
	After  string `json:"after,omitempty"`
	Before string `json:"before,omitempty"`
	Limit  int    `json:"limit,omitempty"`  // 1-100, default 20
	Order  string `json:"order,omitempty"`  // "asc" or "desc", default "desc"
	Filter string `json:"filter,omitempty"` // "in_progress", "completed", "failed", "cancelled"
}

// ListVectorStoreFilesResponse represents a list of files in a vector store
type ListVectorStoreFilesResponse struct {
	Object  string            `json:"object"`             // Always "list"
	Data    []VectorStoreFile `json:"data"`               // Array of files
	FirstID string            `json:"first_id,omitempty"` // ID of first item
	LastID  string            `json:"last_id,omitempty"`  // ID of last item
	HasMore bool              `json:"has_more"`           // Whether there are more results
}

// DeleteVectorStoreFileResponse represents the response from removing a file from a vector store
type DeleteVectorStoreFileResponse struct {
	ID      string `json:"id"`      // File ID
	Object  string `json:"object"`  // Always "vector_store.file.deleted"
	Deleted bool   `json:"deleted"` // Always true
}

// SearchVectorStoreRequest represents a request to search a vector store
type SearchVectorStoreRequest struct {
	Query  string                 `json:"query"`            // Search query
	TopK   int                    `json:"top_k,omitempty"`  // Number of results to return (default: 10)
	Filter map[string]interface{} `json:"filter,omitempty"` // Optional filter criteria
}

// SearchVectorStoreResponse represents search results from a vector store
type SearchVectorStoreResponse struct {
	Object string                    `json:"object"` // Always "list"
	Data   []VectorStoreSearchResult `json:"data"`   // Array of search results
}

// VectorStoreSearchResult represents a single search result
type VectorStoreSearchResult struct {
	FileID   string                 `json:"file_id"`            // ID of the file
	Score    float64                `json:"score"`              // Similarity score
	Content  string                 `json:"content,omitempty"`  // Matched content snippet
	Metadata map[string]interface{} `json:"metadata,omitempty"` // Optional metadata
}

// VectorStoreFileBatch represents a batch of files being processed
type VectorStoreFileBatch struct {
	ID            string                `json:"id"`              // Batch ID
	Object        string                `json:"object"`          // Always "vector_store.file_batch"
	VectorStoreID string                `json:"vector_store_id"` // Vector store ID
	Status        string                `json:"status"`          // "in_progress", "completed", "cancelled", "failed"
	FileCounts    VectorStoreFileCounts `json:"file_counts"`     // File count by status
	CreatedAt     int64                 `json:"created_at"`      // Unix timestamp
}

// CreateVectorStoreFileBatchRequest represents a request to create a file batch
type CreateVectorStoreFileBatchRequest struct {
	FileIDs []string `json:"file_ids"` // Array of file IDs (max 500)
}
