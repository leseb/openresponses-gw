// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package http

import (
	"encoding/json"
	"net/http"
)

// handleListModels handles GET /v1/models
func (h *Handler) handleListModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.modelsService.ListModels(r.Context())
	if err != nil {
		h.logger.Error("Failed to list models", "error", err)
		h.writeError(w, http.StatusInternalServerError, "models_error", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(models)
}

// handleGetModel handles GET /v1/models/{id}
func (h *Handler) handleGetModel(w http.ResponseWriter, r *http.Request) {
	// Extract model ID from path
	modelID := r.PathValue("id")
	if modelID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Model ID is required")
		return
	}

	model, err := h.modelsService.GetModel(r.Context(), modelID)
	if err != nil {
		h.logger.Error("Failed to get model", "error", err, "model_id", modelID)
		h.writeError(w, http.StatusNotFound, "model_not_found", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(model)
}
