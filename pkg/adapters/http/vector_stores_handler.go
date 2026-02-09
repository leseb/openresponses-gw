// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/leseb/openai-responses-gateway/pkg/core/schema"
	"github.com/leseb/openai-responses-gateway/pkg/storage/memory"
)

// handleCreateVectorStore handles POST /v1/vector_stores
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

	h.logger.Info("Vector store created", "vector_store_id", vsID)

	// Add files if provided
	if len(req.FileIDs) > 0 {
		for _, fileID := range req.FileIDs {
			vsFile := &memory.VectorStoreFile{
				ID:            generateID("vsf_"),
				VectorStoreID: vsID,
				FileID:        fileID,
				Status:        "completed", // Simplified
				CreatedAt:     now,
			}
			h.vectorStoresStore.AddVectorStoreFile(r.Context(), vsFile)
		}
	}

	// Return vector store
	schemaVS := convertToSchemaVectorStore(vs)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(schemaVS)
}

// handleListVectorStores handles GET /v1/vector_stores
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

	vsFile := &memory.VectorStoreFile{
		ID:               generateID("vsf_"),
		VectorStoreID:    vsID,
		FileID:           req.FileID,
		Status:           "completed", // Simplified
		CreatedAt:        now,
		ChunkingStrategy: chunkingStrategy,
	}

	err := h.vectorStoresStore.AddVectorStoreFile(r.Context(), vsFile)
	if err != nil {
		h.logger.Error("Failed to add file to vector store", "error", err, "vector_store_id", vsID, "file_id", req.FileID)
		h.writeError(w, http.StatusInternalServerError, "add_file_error", err.Error())
		return
	}

	// Convert to schema
	schemaVSFile := convertToSchemaVectorStoreFile(vsFile)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(schemaVSFile)
}

// handleListVectorStoreFiles handles GET /v1/vector_stores/{id}/files
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
		ID:     vs.ID,
		Object: "vector_store",
		Name:   vs.Name,
		Status: vs.Status,
		UsageBytes: vs.UsageBytes,
		FileCounts: schema.VectorStoreFileCounts{
			InProgress: vs.FileCounts.InProgress,
			Completed:  vs.FileCounts.Completed,
			Failed:     vs.FileCounts.Failed,
			Cancelled:  vs.FileCounts.Cancelled,
			Total:      vs.FileCounts.Total,
		},
		CreatedAt:     vs.CreatedAt.Unix(),
		ExpiresAt:     expiresAt,
		ExpiresAfter:  expiresAfter,
		LastActiveAt:  lastActiveAt,
		Metadata:      convertMetadataToInterface(vs.Metadata),
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
