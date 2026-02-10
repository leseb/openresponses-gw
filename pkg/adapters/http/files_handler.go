// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/leseb/openresponses-gw/pkg/core/schema"
	"github.com/leseb/openresponses-gw/pkg/storage/memory"
)

const (
	maxFileSize = 512 * 1024 * 1024 // 512 MB
)

// handleUploadFile handles POST /v1/files
func (h *Handler) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form
	err := r.ParseMultipartForm(maxFileSize)
	if err != nil {
		h.logger.Error("Failed to parse multipart form", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Failed to parse multipart form")
		return
	}

	// Get file from form
	file, header, err := r.FormFile("file")
	if err != nil {
		h.logger.Error("Failed to get file from form", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid_request", "File is required")
		return
	}
	defer file.Close()

	// Get purpose from form
	purpose := r.FormValue("purpose")
	if purpose == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Purpose is required")
		return
	}

	// Validate purpose
	validPurposes := map[string]bool{
		"assistants": true,
		"vision":     true,
		"batch":      true,
		"fine-tune":  true,
	}
	if !validPurposes[purpose] {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid purpose")
		return
	}

	// Read file content
	content, err := io.ReadAll(file)
	if err != nil {
		h.logger.Error("Failed to read file content", "error", err)
		h.writeError(w, http.StatusInternalServerError, "read_error", "Failed to read file content")
		return
	}

	// Create file
	fileID := generateID("file_")
	now := time.Now()

	storeFile := &memory.File{
		ID:        fileID,
		Filename:  header.Filename,
		Purpose:   purpose,
		MimeType:  header.Header.Get("Content-Type"),
		Bytes:     int64(len(content)),
		Content:   content,
		Status:    "uploaded",
		CreatedAt: now,
	}

	err = h.filesStore.CreateFile(r.Context(), storeFile)
	if err != nil {
		h.logger.Error("Failed to create file", "error", err)
		h.writeError(w, http.StatusInternalServerError, "creation_error", err.Error())
		return
	}

	h.logger.Info("File uploaded", "file_id", fileID, "filename", header.Filename, "bytes", len(content))

	// Return file
	schemaFile := schema.File{
		ID:        storeFile.ID,
		Object:    "file",
		Bytes:     storeFile.Bytes,
		CreatedAt: storeFile.CreatedAt.Unix(),
		Filename:  storeFile.Filename,
		Purpose:   storeFile.Purpose,
		Status:    storeFile.Status,
		MimeType:  storeFile.MimeType,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(schemaFile)
}

// handleListFiles handles GET /v1/files
func (h *Handler) handleListFiles(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()
	after := query.Get("after")
	before := query.Get("before")
	purpose := query.Get("purpose")
	order := query.Get("order")
	if order == "" {
		order = "desc"
	}

	limit := 50
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	h.logger.Info("Listing files", "after", after, "limit", limit, "order", order, "purpose", purpose)

	// Get files from storage
	files, hasMore, err := h.filesStore.ListFilesPaginated(
		r.Context(), after, before, limit, order, purpose,
	)
	if err != nil {
		h.logger.Error("Failed to list files", "error", err)
		h.writeError(w, http.StatusInternalServerError, "list_error", err.Error())
		return
	}

	// Convert to schema
	schemaFiles := make([]schema.File, 0, len(files))
	for _, file := range files {
		f := schema.File{
			ID:        file.ID,
			Object:    "file",
			Bytes:     file.Bytes,
			CreatedAt: file.CreatedAt.Unix(),
			Filename:  file.Filename,
			Purpose:   file.Purpose,
			Status:    file.Status,
			MimeType:  file.MimeType,
		}
		schemaFiles = append(schemaFiles, f)
	}

	// Build response
	listResp := schema.ListFilesResponse{
		Object:  "list",
		Data:    schemaFiles,
		HasMore: hasMore,
	}

	if len(schemaFiles) > 0 {
		listResp.FirstID = schemaFiles[0].ID
		listResp.LastID = schemaFiles[len(schemaFiles)-1].ID
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(listResp)
}

// handleGetFile handles GET /v1/files/{id}
func (h *Handler) handleGetFile(w http.ResponseWriter, r *http.Request) {
	// Extract file ID from path
	fileID := r.PathValue("id")
	if fileID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "File ID is required")
		return
	}

	h.logger.Info("Getting file", "file_id", fileID)

	// Get file from storage
	file, err := h.filesStore.GetFile(r.Context(), fileID)
	if err != nil {
		h.logger.Error("Failed to get file", "error", err, "file_id", fileID)
		h.writeError(w, http.StatusNotFound, "file_not_found", err.Error())
		return
	}

	// Convert to schema
	schemaFile := schema.File{
		ID:        file.ID,
		Object:    "file",
		Bytes:     file.Bytes,
		CreatedAt: file.CreatedAt.Unix(),
		Filename:  file.Filename,
		Purpose:   file.Purpose,
		Status:    file.Status,
		MimeType:  file.MimeType,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(schemaFile)
}

// handleGetFileContent handles GET /v1/files/{id}/content
func (h *Handler) handleGetFileContent(w http.ResponseWriter, r *http.Request) {
	// Extract file ID from path
	fileID := r.PathValue("id")
	if fileID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "File ID is required")
		return
	}

	h.logger.Info("Getting file content", "file_id", fileID)

	// Get file metadata
	file, err := h.filesStore.GetFile(r.Context(), fileID)
	if err != nil {
		h.logger.Error("Failed to get file", "error", err, "file_id", fileID)
		h.writeError(w, http.StatusNotFound, "file_not_found", err.Error())
		return
	}

	// Get file content
	content, err := h.filesStore.GetFileContent(r.Context(), fileID)
	if err != nil {
		h.logger.Error("Failed to get file content", "error", err, "file_id", fileID)
		h.writeError(w, http.StatusInternalServerError, "read_error", err.Error())
		return
	}

	// Set content headers
	w.Header().Set("Content-Type", file.MimeType)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+file.Filename+"\"")
	w.Header().Set("Content-Length", strconv.FormatInt(file.Bytes, 10))

	// Write content
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

// handleDeleteFile handles DELETE /v1/files/{id}
func (h *Handler) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	// Extract file ID from path
	fileID := r.PathValue("id")
	if fileID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "File ID is required")
		return
	}

	h.logger.Info("Deleting file", "file_id", fileID)

	// Delete file from storage
	err := h.filesStore.DeleteFile(r.Context(), fileID)
	if err != nil {
		h.logger.Error("Failed to delete file", "error", err, "file_id", fileID)
		h.writeError(w, http.StatusNotFound, "file_not_found", err.Error())
		return
	}

	// Return deletion confirmation
	deleteResp := schema.DeleteFileResponse{
		ID:      fileID,
		Object:  "file.deleted",
		Deleted: true,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(deleteResp)
}
