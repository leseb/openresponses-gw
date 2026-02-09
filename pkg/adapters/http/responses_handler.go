// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/leseb/openai-responses-gateway/pkg/core/schema"
)

// handleListResponses handles GET /v1/responses
func (h *Handler) handleListResponses(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()
	after := query.Get("after")
	before := query.Get("before")
	model := query.Get("model")
	order := query.Get("order")
	if order == "" {
		order = "desc"
	}

	limit := 50 // default
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	h.logger.Info("Listing responses",
		"after", after,
		"before", before,
		"limit", limit,
		"order", order,
		"model", model)

	// Get paginated responses from storage
	stateResponses, hasMore, err := h.engine.Store().ListResponsesPaginated(
		r.Context(), after, before, limit, order, model,
	)
	if err != nil {
		h.logger.Error("Failed to list responses", "error", err)
		h.writeError(w, http.StatusInternalServerError, "list_error", err.Error())
		return
	}

	// Convert state.Response to schema.Response
	responses := make([]schema.Response, 0, len(stateResponses))
	for _, stateResp := range stateResponses {
		// Convert stored response to schema format
		// This is a simplified conversion - in production, properly deserialize
		resp := schema.Response{
			ID:        stateResp.ID,
			Object:    "response",
			Status:    stateResp.Status,
			CreatedAt: stateResp.CreatedAt.Unix(),
		}
		if stateResp.CompletedAt != nil {
			completedAt := stateResp.CompletedAt.Unix()
			resp.CompletedAt = &completedAt
		}
		responses = append(responses, resp)
	}

	// Build response
	listResp := schema.ListResponsesResponse{
		Object:  "list",
		Data:    responses,
		HasMore: hasMore,
	}

	if len(responses) > 0 {
		listResp.FirstID = responses[0].ID
		listResp.LastID = responses[len(responses)-1].ID
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(listResp)
}

// handleGetResponse handles GET /v1/responses/{id}
func (h *Handler) handleGetResponse(w http.ResponseWriter, r *http.Request) {
	// Extract response ID from path
	responseID := r.PathValue("id")
	if responseID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Response ID is required")
		return
	}

	h.logger.Info("Getting response", "response_id", responseID)

	// Get response from storage
	stateResp, err := h.engine.Store().GetResponse(r.Context(), responseID)
	if err != nil {
		h.logger.Error("Failed to get response", "error", err, "response_id", responseID)
		h.writeError(w, http.StatusNotFound, "response_not_found", err.Error())
		return
	}

	// Convert to schema.Response
	// This is simplified - in production, properly deserialize
	resp := schema.Response{
		ID:        stateResp.ID,
		Object:    "response",
		Status:    stateResp.Status,
		CreatedAt: stateResp.CreatedAt.Unix(),
	}
	if stateResp.CompletedAt != nil {
		completedAt := stateResp.CompletedAt.Unix()
		resp.CompletedAt = &completedAt
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// handleDeleteResponse handles DELETE /v1/responses/{id}
func (h *Handler) handleDeleteResponse(w http.ResponseWriter, r *http.Request) {
	// Extract response ID from path
	responseID := r.PathValue("id")
	if responseID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Response ID is required")
		return
	}

	h.logger.Info("Deleting response", "response_id", responseID)

	// Delete response from storage
	err := h.engine.Store().DeleteResponse(r.Context(), responseID)
	if err != nil {
		h.logger.Error("Failed to delete response", "error", err, "response_id", responseID)
		h.writeError(w, http.StatusNotFound, "response_not_found", err.Error())
		return
	}

	// Return deletion confirmation
	deleteResp := schema.DeleteResponseResponse{
		ID:      responseID,
		Object:  "response.deleted",
		Deleted: true,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(deleteResp)
}

// handleListResponseInputItems handles GET /v1/responses/{id}/input_items
func (h *Handler) handleListResponseInputItems(w http.ResponseWriter, r *http.Request) {
	// Extract response ID from path
	responseID := r.PathValue("id")
	if responseID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Response ID is required")
		return
	}

	h.logger.Info("Listing input items", "response_id", responseID)

	// Get input items from storage
	inputItems, err := h.engine.Store().GetResponseInputItems(r.Context(), responseID)
	if err != nil {
		h.logger.Error("Failed to get input items", "error", err, "response_id", responseID)
		h.writeError(w, http.StatusNotFound, "response_not_found", err.Error())
		return
	}

	// Build response
	// This is simplified - in production, properly format input items
	var items []interface{}
	if inputItems != nil {
		items = []interface{}{inputItems}
	} else {
		items = []interface{}{}
	}

	listResp := schema.ListInputItemsResponse{
		Object:  "list",
		Data:    items,
		HasMore: false,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(listResp)
}
