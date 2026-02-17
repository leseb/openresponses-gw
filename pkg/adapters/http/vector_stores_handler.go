// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/leseb/openresponses-gw/pkg/core/schema"
	"github.com/leseb/openresponses-gw/pkg/storage/memory"
	"github.com/leseb/openresponses-gw/pkg/vectorstore"
)

// handleCreateVectorStore handles POST /v1/vector_stores
//
//	@Summary	Create vector store
//	@Tags		Vector Stores
//	@Accept		json
//	@Produce	json
//	@Param		request	body		schema.CreateVectorStoreRequest	true	"Create vector store request"
//	@Success	200		{object}	schema.VectorStore
//	@Failure	400		{object}	map[string]interface{}
//	@Failure	500		{object}	map[string]interface{}
//	@Router		/v1/vector_stores [post]
func (h *Handler) handleCreateVectorStore(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var req schema.CreateVectorStoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to parse vector store request", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Failed to parse request body")
		return
	}

	// Create vector store
	vsID := generateID("vs_")
	now := time.Now()

	var expiresAfter *memory.VectorStoreExpiration
	if req.ExpiresAfter != nil {
		expiresAfter = &memory.VectorStoreExpiration{
			Anchor: req.ExpiresAfter.Anchor,
			Days:   req.ExpiresAfter.Days,
		}
	}

	vs := &memory.VectorStore{
		ID:           vsID,
		Name:         req.Name,
		Status:       "completed", // Simplified: mark as completed immediately
		UsageBytes:   0,
		FileCounts:   memory.VectorStoreFileCounts{},
		CreatedAt:    now,
		ExpiresAfter: expiresAfter,
		Metadata:     convertMetadata(req.Metadata),
		FileIDs:      []string{},
	}

	err := h.vectorStoresStore.CreateVectorStore(r.Context(), vs)
	if err != nil {
		h.logger.Error("Failed to create vector store", "error", err)
		h.writeError(w, http.StatusInternalServerError, "creation_error", err.Error())
		return
	}

	// Provision backend storage (e.g. Milvus collection)
	if h.vectorStoreService != nil {
		if err := h.vectorStoreService.CreateStore(r.Context(), vsID, 0); err != nil {
			h.logger.Error("Failed to provision vector store backend", "error", err, "vector_store_id", vsID)
			// Continue — metadata is created; backend can be retried
		}
	}

	h.logger.Info("Vector store created", "vector_store_id", vsID)

	// Add files if provided
	if len(req.FileIDs) > 0 {
		for _, fileID := range req.FileIDs {
			vsFile := &memory.VectorStoreFile{
				ID:            generateID("vsf_"),
				VectorStoreID: vsID,
				FileID:        fileID,
				Status:        "in_progress",
				CreatedAt:     now,
			}
			if addErr := h.vectorStoresStore.AddVectorStoreFile(r.Context(), vsFile); addErr != nil {
				h.logger.Error("Failed to add file to vector store", "error", addErr)
				continue
			}
			h.startFileIngestion(vsID, fileID, nil)
		}
	}

	// Return vector store
	schemaVS := convertToSchemaVectorStore(vs)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(schemaVS)
}

// handleListVectorStores handles GET /v1/vector_stores
//
//	@Summary	List vector stores
//	@Tags		Vector Stores
//	@Produce	json
//	@Param		after	query		string	false	"Cursor for pagination"
//	@Param		before	query		string	false	"Cursor for pagination (backwards)"
//	@Param		limit	query		int		false	"Number of items (1-100, default 20)"
//	@Param		order	query		string	false	"Sort order: asc or desc (default desc)"
//	@Success	200		{object}	schema.ListVectorStoresResponse
//	@Failure	500		{object}	map[string]interface{}
//	@Router		/v1/vector_stores [get]
func (h *Handler) handleListVectorStores(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()
	after := query.Get("after")
	before := query.Get("before")
	order := query.Get("order")
	if order == "" {
		order = "desc"
	}

	limit := 20
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	h.logger.Info("Listing vector stores", "after", after, "limit", limit, "order", order)

	// Get vector stores from storage
	vectorStores, hasMore, err := h.vectorStoresStore.ListVectorStoresPaginated(
		r.Context(), after, before, limit, order,
	)
	if err != nil {
		h.logger.Error("Failed to list vector stores", "error", err)
		h.writeError(w, http.StatusInternalServerError, "list_error", err.Error())
		return
	}

	// Convert to schema
	schemaVS := make([]schema.VectorStore, 0, len(vectorStores))
	for _, vs := range vectorStores {
		schemaVS = append(schemaVS, convertToSchemaVectorStore(vs))
	}

	// Build response
	listResp := schema.ListVectorStoresResponse{
		Object:  "list",
		Data:    schemaVS,
		HasMore: hasMore,
	}

	if len(schemaVS) > 0 {
		listResp.FirstID = schemaVS[0].ID
		listResp.LastID = schemaVS[len(schemaVS)-1].ID
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(listResp)
}

// handleGetVectorStore handles GET /v1/vector_stores/{id}
//
//	@Summary	Get vector store
//	@Tags		Vector Stores
//	@Produce	json
//	@Param		id	path		string	true	"Vector store ID"
//	@Success	200	{object}	schema.VectorStore
//	@Failure	400	{object}	map[string]interface{}
//	@Failure	404	{object}	map[string]interface{}
//	@Router		/v1/vector_stores/{id} [get]
func (h *Handler) handleGetVectorStore(w http.ResponseWriter, r *http.Request) {
	// Extract vector store ID from path
	vsID := r.PathValue("id")
	if vsID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Vector store ID is required")
		return
	}

	h.logger.Info("Getting vector store", "vector_store_id", vsID)

	// Get vector store from storage
	vs, err := h.vectorStoresStore.GetVectorStore(r.Context(), vsID)
	if err != nil {
		h.logger.Error("Failed to get vector store", "error", err, "vector_store_id", vsID)
		h.writeError(w, http.StatusNotFound, "vector_store_not_found", err.Error())
		return
	}

	// Convert to schema
	schemaVS := convertToSchemaVectorStore(vs)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(schemaVS)
}

// handleUpdateVectorStore handles PUT /v1/vector_stores/{id}
//
//	@Summary	Update vector store
//	@Tags		Vector Stores
//	@Accept		json
//	@Produce	json
//	@Param		id		path		string								true	"Vector store ID"
//	@Param		request	body		schema.UpdateVectorStoreRequest		true	"Update vector store request"
//	@Success	200		{object}	schema.VectorStore
//	@Failure	400		{object}	map[string]interface{}
//	@Failure	404		{object}	map[string]interface{}
//	@Failure	500		{object}	map[string]interface{}
//	@Router		/v1/vector_stores/{id} [put]
func (h *Handler) handleUpdateVectorStore(w http.ResponseWriter, r *http.Request) {
	// Extract vector store ID from path
	vsID := r.PathValue("id")
	if vsID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Vector store ID is required")
		return
	}

	// Parse request body
	var req schema.UpdateVectorStoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to parse update request", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Failed to parse request body")
		return
	}

	h.logger.Info("Updating vector store", "vector_store_id", vsID)

	// Get existing vector store
	vs, err := h.vectorStoresStore.GetVectorStore(r.Context(), vsID)
	if err != nil {
		h.logger.Error("Failed to get vector store", "error", err, "vector_store_id", vsID)
		h.writeError(w, http.StatusNotFound, "vector_store_not_found", err.Error())
		return
	}

	// Update fields if provided
	if req.Name != nil {
		vs.Name = *req.Name
	}
	if req.ExpiresAfter != nil {
		vs.ExpiresAfter = &memory.VectorStoreExpiration{
			Anchor: req.ExpiresAfter.Anchor,
			Days:   req.ExpiresAfter.Days,
		}
	}
	if req.Metadata != nil {
		vs.Metadata = convertMetadata(req.Metadata)
	}

	// Update in storage
	err = h.vectorStoresStore.UpdateVectorStore(r.Context(), vs)
	if err != nil {
		h.logger.Error("Failed to update vector store", "error", err, "vector_store_id", vsID)
		h.writeError(w, http.StatusInternalServerError, "update_error", err.Error())
		return
	}

	// Convert to schema
	schemaVS := convertToSchemaVectorStore(vs)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(schemaVS)
}

// handleDeleteVectorStore handles DELETE /v1/vector_stores/{id}
//
//	@Summary	Delete vector store
//	@Tags		Vector Stores
//	@Produce	json
//	@Param		id	path		string	true	"Vector store ID"
//	@Success	200	{object}	schema.DeleteVectorStoreResponse
//	@Failure	400	{object}	map[string]interface{}
//	@Failure	404	{object}	map[string]interface{}
//	@Router		/v1/vector_stores/{id} [delete]
func (h *Handler) handleDeleteVectorStore(w http.ResponseWriter, r *http.Request) {
	// Extract vector store ID from path
	vsID := r.PathValue("id")
	if vsID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Vector store ID is required")
		return
	}

	h.logger.Info("Deleting vector store", "vector_store_id", vsID)

	// Delete vector store from storage
	err := h.vectorStoresStore.DeleteVectorStore(r.Context(), vsID)
	if err != nil {
		h.logger.Error("Failed to delete vector store", "error", err, "vector_store_id", vsID)
		h.writeError(w, http.StatusNotFound, "vector_store_not_found", err.Error())
		return
	}

	// Delete backend storage (e.g. Milvus collection)
	if h.vectorStoreService != nil {
		if delErr := h.vectorStoreService.DeleteStore(r.Context(), vsID); delErr != nil {
			h.logger.Error("Failed to delete vector store backend", "error", delErr, "vector_store_id", vsID)
		}
	}

	// Return deletion confirmation
	deleteResp := schema.DeleteVectorStoreResponse{
		ID:      vsID,
		Object:  "vector_store.deleted",
		Deleted: true,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(deleteResp)
}

// handleAddVectorStoreFile handles POST /v1/vector_stores/{id}/files
//
//	@Summary	Add file to vector store
//	@Tags		Vector Stores
//	@Accept		json
//	@Produce	json
//	@Param		id		path		string								true	"Vector store ID"
//	@Param		request	body		schema.AddVectorStoreFileRequest	true	"Add file request"
//	@Success	200		{object}	schema.VectorStoreFile
//	@Failure	400		{object}	map[string]interface{}
//	@Failure	500		{object}	map[string]interface{}
//	@Router		/v1/vector_stores/{id}/files [post]
func (h *Handler) handleAddVectorStoreFile(w http.ResponseWriter, r *http.Request) {
	// Extract vector store ID from path
	vsID := r.PathValue("id")
	if vsID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Vector store ID is required")
		return
	}

	// Parse request body
	var req schema.AddVectorStoreFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to parse add file request", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Failed to parse request body")
		return
	}

	if req.FileID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "File ID is required")
		return
	}

	h.logger.Info("Adding file to vector store", "vector_store_id", vsID, "file_id", req.FileID)

	// Create vector store file
	now := time.Now()

	var chunkingStrategy *memory.ChunkingStrategy
	if req.ChunkingStrategy != nil {
		chunkingStrategy = &memory.ChunkingStrategy{
			Type: req.ChunkingStrategy.Type,
		}
		if req.ChunkingStrategy.Static != nil {
			chunkingStrategy.Static = &memory.StaticChunkingStrategy{
				MaxChunkSizeTokens: req.ChunkingStrategy.Static.MaxChunkSizeTokens,
				ChunkOverlapTokens: req.ChunkingStrategy.Static.ChunkOverlapTokens,
			}
		}
	}

	// Set initial status based on whether ingestion is possible
	initialStatus := "completed"
	if h.vectorStoreService != nil {
		initialStatus = "in_progress"
	}

	vsFile := &memory.VectorStoreFile{
		ID:               generateID("vsf_"),
		VectorStoreID:    vsID,
		FileID:           req.FileID,
		Status:           initialStatus,
		CreatedAt:        now,
		ChunkingStrategy: chunkingStrategy,
	}

	err := h.vectorStoresStore.AddVectorStoreFile(r.Context(), vsFile)
	if err != nil {
		h.logger.Error("Failed to add file to vector store", "error", err, "vector_store_id", vsID, "file_id", req.FileID)
		h.writeError(w, http.StatusInternalServerError, "add_file_error", err.Error())
		return
	}

	// Trigger async ingestion
	h.startFileIngestion(vsID, req.FileID, chunkingStrategy)

	// Convert to schema
	schemaVSFile := convertToSchemaVectorStoreFile(vsFile)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(schemaVSFile)
}

// handleListVectorStoreFiles handles GET /v1/vector_stores/{id}/files
//
//	@Summary	List vector store files
//	@Tags		Vector Stores
//	@Produce	json
//	@Param		id		path		string	true	"Vector store ID"
//	@Param		after	query		string	false	"Cursor for pagination"
//	@Param		before	query		string	false	"Cursor for pagination (backwards)"
//	@Param		limit	query		int		false	"Number of items (1-100, default 20)"
//	@Param		order	query		string	false	"Sort order: asc or desc (default desc)"
//	@Param		filter	query		string	false	"Filter by status: in_progress, completed, failed, cancelled"
//	@Success	200		{object}	schema.ListVectorStoreFilesResponse
//	@Failure	400		{object}	map[string]interface{}
//	@Failure	500		{object}	map[string]interface{}
//	@Router		/v1/vector_stores/{id}/files [get]
func (h *Handler) handleListVectorStoreFiles(w http.ResponseWriter, r *http.Request) {
	// Extract vector store ID from path
	vsID := r.PathValue("id")
	if vsID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Vector store ID is required")
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	after := query.Get("after")
	before := query.Get("before")
	filter := query.Get("filter")
	order := query.Get("order")
	if order == "" {
		order = "desc"
	}

	limit := 20
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	h.logger.Info("Listing vector store files", "vector_store_id", vsID, "limit", limit, "filter", filter)

	// Get files from storage
	files, hasMore, err := h.vectorStoresStore.ListVectorStoreFilesPaginated(
		r.Context(), vsID, after, before, limit, order, filter,
	)
	if err != nil {
		h.logger.Error("Failed to list vector store files", "error", err, "vector_store_id", vsID)
		h.writeError(w, http.StatusInternalServerError, "list_error", err.Error())
		return
	}

	// Convert to schema
	schemaFiles := make([]schema.VectorStoreFile, 0, len(files))
	for _, vsFile := range files {
		schemaFiles = append(schemaFiles, convertToSchemaVectorStoreFile(vsFile))
	}

	// Build response
	listResp := schema.ListVectorStoreFilesResponse{
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

// Helper functions

func convertToSchemaVectorStore(vs *memory.VectorStore) schema.VectorStore {
	var expiresAt *int64
	if vs.ExpiresAt != nil {
		ts := vs.ExpiresAt.Unix()
		expiresAt = &ts
	}

	var lastActiveAt *int64
	if vs.LastActiveAt != nil {
		ts := vs.LastActiveAt.Unix()
		lastActiveAt = &ts
	}

	var expiresAfter *schema.VectorStoreExpiration
	if vs.ExpiresAfter != nil {
		expiresAfter = &schema.VectorStoreExpiration{
			Anchor: vs.ExpiresAfter.Anchor,
			Days:   vs.ExpiresAfter.Days,
		}
	}

	return schema.VectorStore{
		ID:         vs.ID,
		Object:     "vector_store",
		Name:       vs.Name,
		Status:     vs.Status,
		UsageBytes: vs.UsageBytes,
		FileCounts: schema.VectorStoreFileCounts{
			InProgress: vs.FileCounts.InProgress,
			Completed:  vs.FileCounts.Completed,
			Failed:     vs.FileCounts.Failed,
			Cancelled:  vs.FileCounts.Cancelled,
			Total:      vs.FileCounts.Total,
		},
		CreatedAt:    vs.CreatedAt.Unix(),
		ExpiresAt:    expiresAt,
		ExpiresAfter: expiresAfter,
		LastActiveAt: lastActiveAt,
		Metadata:     convertMetadataToInterface(vs.Metadata),
	}
}

func convertToSchemaVectorStoreFile(vsFile *memory.VectorStoreFile) schema.VectorStoreFile {
	var lastError *schema.VectorStoreFileError
	if vsFile.LastError != nil {
		lastError = &schema.VectorStoreFileError{
			Code:    vsFile.LastError.Code,
			Message: vsFile.LastError.Message,
		}
	}

	var chunkingStrategy *schema.ChunkingStrategy
	if vsFile.ChunkingStrategy != nil {
		chunkingStrategy = &schema.ChunkingStrategy{
			Type: vsFile.ChunkingStrategy.Type,
		}
		if vsFile.ChunkingStrategy.Static != nil {
			chunkingStrategy.Static = &schema.StaticChunkingStrategy{
				MaxChunkSizeTokens: vsFile.ChunkingStrategy.Static.MaxChunkSizeTokens,
				ChunkOverlapTokens: vsFile.ChunkingStrategy.Static.ChunkOverlapTokens,
			}
		}
	}

	return schema.VectorStoreFile{
		ID:               vsFile.FileID,
		Object:           "vector_store.file",
		VectorStoreID:    vsFile.VectorStoreID,
		Status:           vsFile.Status,
		UsageBytes:       vsFile.UsageBytes,
		CreatedAt:        vsFile.CreatedAt.Unix(),
		LastError:        lastError,
		ChunkingStrategy: chunkingStrategy,
	}
}

// handleGetVectorStoreFile handles GET /v1/vector_stores/{id}/files/{file_id}
//
//	@Summary	Get vector store file
//	@Tags		Vector Stores
//	@Produce	json
//	@Param		id		path		string	true	"Vector store ID"
//	@Param		file_id	path		string	true	"File ID"
//	@Success	200		{object}	schema.VectorStoreFile
//	@Failure	400		{object}	map[string]interface{}
//	@Failure	404		{object}	map[string]interface{}
//	@Router		/v1/vector_stores/{id}/files/{file_id} [get]
func (h *Handler) handleGetVectorStoreFile(w http.ResponseWriter, r *http.Request) {
	vsID := r.PathValue("id")
	fileID := r.PathValue("file_id")

	if vsID == "" || fileID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Vector store ID and file ID are required")
		return
	}

	h.logger.Info("Getting vector store file", "vector_store_id", vsID, "file_id", fileID)

	vsFile, err := h.vectorStoresStore.GetVectorStoreFile(r.Context(), vsID, fileID)
	if err != nil {
		h.logger.Error("Failed to get vector store file", "error", err)
		h.writeError(w, http.StatusNotFound, "file_not_found", err.Error())
		return
	}

	schemaVSFile := convertToSchemaVectorStoreFile(vsFile)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(schemaVSFile)
}

// handleDeleteVectorStoreFile handles DELETE /v1/vector_stores/{id}/files/{file_id}
//
//	@Summary	Delete vector store file
//	@Tags		Vector Stores
//	@Produce	json
//	@Param		id		path		string	true	"Vector store ID"
//	@Param		file_id	path		string	true	"File ID"
//	@Success	200		{object}	schema.DeleteVectorStoreFileResponse
//	@Failure	400		{object}	map[string]interface{}
//	@Failure	404		{object}	map[string]interface{}
//	@Router		/v1/vector_stores/{id}/files/{file_id} [delete]
func (h *Handler) handleDeleteVectorStoreFile(w http.ResponseWriter, r *http.Request) {
	vsID := r.PathValue("id")
	fileID := r.PathValue("file_id")

	if vsID == "" || fileID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Vector store ID and file ID are required")
		return
	}

	h.logger.Info("Deleting vector store file", "vector_store_id", vsID, "file_id", fileID)

	err := h.vectorStoresStore.DeleteVectorStoreFile(r.Context(), vsID, fileID)
	if err != nil {
		h.logger.Error("Failed to delete vector store file", "error", err)
		h.writeError(w, http.StatusNotFound, "file_not_found", err.Error())
		return
	}

	// Remove chunks from backend
	if h.vectorStoreService != nil {
		if rmErr := h.vectorStoreService.RemoveFile(r.Context(), vsID, fileID); rmErr != nil {
			h.logger.Error("Failed to remove file chunks from backend", "error", rmErr, "vector_store_id", vsID, "file_id", fileID)
		}
	}

	deleteResp := schema.DeleteVectorStoreFileResponse{
		ID:      fileID,
		Object:  "vector_store.file.deleted",
		Deleted: true,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(deleteResp)
}

// handleGetVectorStoreFileContent handles GET /v1/vector_stores/{id}/files/{file_id}/content
//
//	@Summary	Get vector store file content
//	@Tags		Vector Stores
//	@Produce	octet-stream
//	@Param		id		path		string	true	"Vector store ID"
//	@Param		file_id	path		string	true	"File ID"
//	@Success	200		{file}		binary
//	@Failure	400		{object}	map[string]interface{}
//	@Failure	404		{object}	map[string]interface{}
//	@Failure	500		{object}	map[string]interface{}
//	@Router		/v1/vector_stores/{id}/files/{file_id}/content [get]
func (h *Handler) handleGetVectorStoreFileContent(w http.ResponseWriter, r *http.Request) {
	vsID := r.PathValue("id")
	fileID := r.PathValue("file_id")

	if vsID == "" || fileID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Vector store ID and file ID are required")
		return
	}

	h.logger.Info("Getting vector store file content", "vector_store_id", vsID, "file_id", fileID)

	// Get file metadata from files store (not vector store)
	file, err := h.filesStore.GetFile(r.Context(), fileID)
	if err != nil {
		h.logger.Error("Failed to get file", "error", err)
		h.writeError(w, http.StatusNotFound, "file_not_found", err.Error())
		return
	}

	// Get file content
	content, err := h.filesStore.GetFileContent(r.Context(), fileID)
	if err != nil {
		h.logger.Error("Failed to get file content", "error", err)
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

// handleSearchVectorStore handles POST /v1/vector_stores/{id}/search
//
//	@Summary	Search vector store
//	@Tags		Vector Stores
//	@Accept		json
//	@Produce	json
//	@Param		id		path		string									true	"Vector store ID"
//	@Param		request	body		schema.SearchVectorStoreRequest			true	"Search request"
//	@Success	200		{object}	schema.SearchVectorStoreResponse
//	@Failure	400		{object}	map[string]interface{}
//	@Failure	500		{object}	map[string]interface{}
//	@Router		/v1/vector_stores/{id}/search [post]
func (h *Handler) handleSearchVectorStore(w http.ResponseWriter, r *http.Request) {
	vsID := r.PathValue("id")

	if vsID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Vector store ID is required")
		return
	}

	// Parse request body
	var req schema.SearchVectorStoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to parse search request", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Failed to parse request body")
		return
	}

	// Extract the query string (supports both string and array forms)
	var queryStr string
	switch q := req.Query.(type) {
	case string:
		queryStr = q
	case []interface{}:
		if len(q) > 0 {
			if s, ok := q[0].(string); ok {
				queryStr = s
			}
		}
	}

	h.logger.Info("Searching vector store", "vector_store_id", vsID, "query", queryStr)

	topK := 10
	if req.MaxNumResults != nil && *req.MaxNumResults > 0 {
		topK = *req.MaxNumResults
	} else if req.TopK > 0 {
		topK = req.TopK
	}

	var results []vectorstore.SearchResult
	if h.vectorStoreService != nil {
		var searchErr error
		results, searchErr = h.vectorStoreService.Search(r.Context(), vsID, queryStr, topK)
		if searchErr != nil {
			h.logger.Error("Vector store search failed", "error", searchErr, "vector_store_id", vsID)
			h.writeError(w, http.StatusInternalServerError, "search_error", searchErr.Error())
			return
		}
	}

	// Convert to schema results
	data := make([]schema.VectorStoreSearchResult, 0, len(results))
	for _, r := range results {
		data = append(data, schema.VectorStoreSearchResult{
			FileID:   r.FileID,
			Filename: "",
			Score:    r.Score,
			Content: []schema.VectorStoreSearchResultContent{
				{Type: "text", Text: r.Content},
			},
		})
	}

	searchResp := schema.SearchVectorStoreResponse{
		Object:      "vector_store.search_results.page",
		SearchQuery: []string{queryStr},
		Data:        data,
		HasMore:     false,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(searchResp)
}

// handleCreateVectorStoreFileBatch handles POST /v1/vector_stores/{id}/file_batches
//
//	@Summary	Create vector store file batch
//	@Tags		Vector Stores
//	@Accept		json
//	@Produce	json
//	@Param		id		path		string										true	"Vector store ID"
//	@Param		request	body		schema.CreateVectorStoreFileBatchRequest		true	"File batch request"
//	@Success	200		{object}	schema.VectorStoreFileBatch
//	@Failure	400		{object}	map[string]interface{}
//	@Failure	500		{object}	map[string]interface{}
//	@Router		/v1/vector_stores/{id}/file_batches [post]
func (h *Handler) handleCreateVectorStoreFileBatch(w http.ResponseWriter, r *http.Request) {
	vsID := r.PathValue("id")

	if vsID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Vector store ID is required")
		return
	}

	// Parse request body
	var req schema.CreateVectorStoreFileBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to parse batch request", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Failed to parse request body")
		return
	}

	h.logger.Info("Creating file batch", "vector_store_id", vsID, "file_count", len(req.FileIDs))

	// Create batch
	batchID := generateID("vsfb_")
	now := time.Now()

	batch := &memory.VectorStoreFileBatch{
		ID:            batchID,
		VectorStoreID: vsID,
		Status:        "completed", // Simplified: mark as completed immediately
		FileCounts: memory.VectorStoreFileCounts{
			Completed: len(req.FileIDs),
			Total:     len(req.FileIDs),
		},
		CreatedAt: now,
	}

	err := h.vectorStoresStore.CreateVectorStoreFileBatch(r.Context(), batch)
	if err != nil {
		h.logger.Error("Failed to create batch", "error", err)
		h.writeError(w, http.StatusInternalServerError, "creation_error", err.Error())
		return
	}

	// Add files to batch
	for _, fileID := range req.FileIDs {
		vsFile := &memory.VectorStoreFile{
			ID:            generateID("vsf_"),
			VectorStoreID: vsID,
			FileID:        fileID,
			Status:        "completed",
			CreatedAt:     now,
		}
		h.vectorStoresStore.AddVectorStoreFile(r.Context(), vsFile)
	}

	// Return batch
	schemaBatch := convertToSchemaFileBatch(batch)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(schemaBatch)
}

// handleGetVectorStoreFileBatch handles GET /v1/vector_stores/{id}/file_batches/{batch_id}
//
//	@Summary	Get vector store file batch
//	@Tags		Vector Stores
//	@Produce	json
//	@Param		id			path		string	true	"Vector store ID"
//	@Param		batch_id	path		string	true	"Batch ID"
//	@Success	200			{object}	schema.VectorStoreFileBatch
//	@Failure	400			{object}	map[string]interface{}
//	@Failure	404			{object}	map[string]interface{}
//	@Router		/v1/vector_stores/{id}/file_batches/{batch_id} [get]
func (h *Handler) handleGetVectorStoreFileBatch(w http.ResponseWriter, r *http.Request) {
	vsID := r.PathValue("id")
	batchID := r.PathValue("batch_id")

	if vsID == "" || batchID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Vector store ID and batch ID are required")
		return
	}

	h.logger.Info("Getting file batch", "vector_store_id", vsID, "batch_id", batchID)

	batch, err := h.vectorStoresStore.GetVectorStoreFileBatch(r.Context(), vsID, batchID)
	if err != nil {
		h.logger.Error("Failed to get batch", "error", err)
		h.writeError(w, http.StatusNotFound, "batch_not_found", err.Error())
		return
	}

	schemaBatch := convertToSchemaFileBatch(batch)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(schemaBatch)
}

// handleListVectorStoreFileBatchFiles handles GET /v1/vector_stores/{id}/file_batches/{batch_id}/files
//
//	@Summary	List vector store file batch files
//	@Tags		Vector Stores
//	@Produce	json
//	@Param		id			path		string	true	"Vector store ID"
//	@Param		batch_id	path		string	true	"Batch ID"
//	@Param		after		query		string	false	"Cursor for pagination"
//	@Param		before		query		string	false	"Cursor for pagination (backwards)"
//	@Param		limit		query		int		false	"Number of items (1-100, default 20)"
//	@Param		order		query		string	false	"Sort order: asc or desc (default desc)"
//	@Param		filter		query		string	false	"Filter by status"
//	@Success	200			{object}	schema.ListVectorStoreFilesResponse
//	@Failure	400			{object}	map[string]interface{}
//	@Failure	500			{object}	map[string]interface{}
//	@Router		/v1/vector_stores/{id}/file_batches/{batch_id}/files [get]
func (h *Handler) handleListVectorStoreFileBatchFiles(w http.ResponseWriter, r *http.Request) {
	vsID := r.PathValue("id")
	batchID := r.PathValue("batch_id")

	if vsID == "" || batchID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Vector store ID and batch ID are required")
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	after := query.Get("after")
	before := query.Get("before")
	filter := query.Get("filter")
	order := query.Get("order")
	if order == "" {
		order = "desc"
	}

	limit := 20
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	h.logger.Info("Listing batch files", "vector_store_id", vsID, "batch_id", batchID)

	// For now, return all files in the vector store (since we don't track batch membership)
	files, hasMore, err := h.vectorStoresStore.ListVectorStoreFilesPaginated(
		r.Context(), vsID, after, before, limit, order, filter,
	)
	if err != nil {
		h.logger.Error("Failed to list batch files", "error", err)
		h.writeError(w, http.StatusInternalServerError, "list_error", err.Error())
		return
	}

	// Convert to schema
	schemaFiles := make([]schema.VectorStoreFile, 0, len(files))
	for _, vsFile := range files {
		schemaFiles = append(schemaFiles, convertToSchemaVectorStoreFile(vsFile))
	}

	// Build response
	listResp := schema.ListVectorStoreFilesResponse{
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

// handleCancelVectorStoreFileBatch handles POST /v1/vector_stores/{id}/file_batches/{batch_id}/cancel
//
//	@Summary	Cancel vector store file batch
//	@Tags		Vector Stores
//	@Produce	json
//	@Param		id			path		string	true	"Vector store ID"
//	@Param		batch_id	path		string	true	"Batch ID"
//	@Success	200			{object}	schema.VectorStoreFileBatch
//	@Failure	400			{object}	map[string]interface{}
//	@Failure	404			{object}	map[string]interface{}
//	@Failure	500			{object}	map[string]interface{}
//	@Router		/v1/vector_stores/{id}/file_batches/{batch_id}/cancel [post]
func (h *Handler) handleCancelVectorStoreFileBatch(w http.ResponseWriter, r *http.Request) {
	vsID := r.PathValue("id")
	batchID := r.PathValue("batch_id")

	if vsID == "" || batchID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Vector store ID and batch ID are required")
		return
	}

	h.logger.Info("Canceling file batch", "vector_store_id", vsID, "batch_id", batchID)

	// Get batch
	batch, err := h.vectorStoresStore.GetVectorStoreFileBatch(r.Context(), vsID, batchID)
	if err != nil {
		h.logger.Error("Failed to get batch", "error", err)
		h.writeError(w, http.StatusNotFound, "batch_not_found", err.Error())
		return
	}

	// Update status to cancelled
	batch.Status = "cancelled"
	err = h.vectorStoresStore.UpdateVectorStoreFileBatch(r.Context(), batch)
	if err != nil {
		h.logger.Error("Failed to cancel batch", "error", err)
		h.writeError(w, http.StatusInternalServerError, "cancel_error", err.Error())
		return
	}

	schemaBatch := convertToSchemaFileBatch(batch)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(schemaBatch)
}

// startFileIngestion triggers async file ingestion via the VectorStoreService.
// If the service is nil (feature disabled), this is a no-op.
func (h *Handler) startFileIngestion(vsID, fileID string, cs *memory.ChunkingStrategy) {
	if h.vectorStoreService == nil {
		return
	}

	chunkSize := vectorstore.DefaultChunkSize
	overlap := vectorstore.DefaultChunkOverlap
	if cs != nil && cs.Static != nil {
		if cs.Static.MaxChunkSizeTokens > 0 {
			chunkSize = vectorstore.TokensToChars(cs.Static.MaxChunkSizeTokens)
		}
		if cs.Static.ChunkOverlapTokens > 0 {
			overlap = vectorstore.TokensToChars(cs.Static.ChunkOverlapTokens)
		}
	}

	go func() {
		ctx := context.Background()
		if err := h.vectorStoreService.IngestFile(ctx, vsID, fileID, chunkSize, overlap); err != nil {
			h.logger.Error("File ingestion failed", "error", err, "vector_store_id", vsID, "file_id", fileID)
			// Update file status to failed
			if vsFile, getErr := h.vectorStoresStore.GetVectorStoreFile(ctx, vsID, fileID); getErr == nil {
				vsFile.Status = "failed"
				vsFile.LastError = &memory.VectorStoreFileError{
					Code:    "ingestion_failed",
					Message: err.Error(),
				}
				h.vectorStoresStore.UpdateVectorStoreFile(ctx, vsFile)
			}
			return
		}

		// Update file status to completed
		if vsFile, getErr := h.vectorStoresStore.GetVectorStoreFile(ctx, vsID, fileID); getErr == nil {
			vsFile.Status = "completed"
			h.vectorStoresStore.UpdateVectorStoreFile(ctx, vsFile)
		}
		h.logger.Info("File ingestion completed", "vector_store_id", vsID, "file_id", fileID)
	}()
}

// convertToSchemaFileBatch converts internal batch to schema
func convertToSchemaFileBatch(batch *memory.VectorStoreFileBatch) schema.VectorStoreFileBatch {
	return schema.VectorStoreFileBatch{
		ID:            batch.ID,
		Object:        "vector_store.file_batch",
		VectorStoreID: batch.VectorStoreID,
		Status:        batch.Status,
		FileCounts: schema.VectorStoreFileCounts{
			InProgress: batch.FileCounts.InProgress,
			Completed:  batch.FileCounts.Completed,
			Failed:     batch.FileCounts.Failed,
			Cancelled:  batch.FileCounts.Cancelled,
			Total:      batch.FileCounts.Total,
		},
		CreatedAt: batch.CreatedAt.Unix(),
	}
}
