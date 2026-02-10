// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package schema

// File represents an uploaded file
type File struct {
	ID        string `json:"id"`         // Format: "file_{uuid}"
	Object    string `json:"object"`     // Always "file"
	Bytes     int64  `json:"bytes"`      // File size in bytes
	CreatedAt int64  `json:"created_at"` // Unix timestamp
	Filename  string `json:"filename"`   // Original filename
	Purpose   string `json:"purpose"`    // Purpose: "assistants", "vision", "batch", "fine-tune"
	Status    string `json:"status"`     // Status: "uploaded", "processed", "error"
	MimeType  string `json:"mime_type"`  // MIME type
}

// UploadFileRequest represents a multipart file upload request
type UploadFileRequest struct {
	File     []byte `json:"-"`       // File content
	Purpose  string `json:"purpose"` // Required: purpose of the file
	Filename string `json:"-"`       // Original filename
	MimeType string `json:"-"`       // MIME type
}

// ListFilesRequest represents a request to list files
type ListFilesRequest struct {
	After   string `json:"after,omitempty"`
	Before  string `json:"before,omitempty"`
	Limit   int    `json:"limit,omitempty"`   // 1-100, default 50
	Order   string `json:"order,omitempty"`   // "asc" or "desc", default "desc"
	Purpose string `json:"purpose,omitempty"` // Filter by purpose
}

// ListFilesResponse represents a list of files
type ListFilesResponse struct {
	Object  string `json:"object"`             // Always "list"
	Data    []File `json:"data"`               // Array of files
	FirstID string `json:"first_id,omitempty"` // ID of first item
	LastID  string `json:"last_id,omitempty"`  // ID of last item
	HasMore bool   `json:"has_more"`           // Whether there are more results
}

// DeleteFileResponse represents the response from deleting a file
type DeleteFileResponse struct {
	ID      string `json:"id"`      // File ID
	Object  string `json:"object"`  // Always "file.deleted"
	Deleted bool   `json:"deleted"` // Always true
}
