// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/leseb/openresponses-gw/pkg/core/schema"
	"github.com/leseb/openresponses-gw/pkg/storage/memory"
)

// handleRegisterConnector handles POST /v1/connectors
func (h *Handler) handleRegisterConnector(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var req schema.RegisterConnectorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to parse connector request", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Failed to parse request body")
		return
	}

	// Validate required fields
	if req.ConnectorID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "connector_id is required")
		return
	}
	if req.ConnectorType == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "connector_type is required")
		return
	}
	if req.ConnectorType != "mcp" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "connector_type must be \"mcp\"")
		return
	}
	if req.URL == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "url is required")
		return
	}

	now := time.Now()

	connector := &memory.Connector{
		ConnectorID:   req.ConnectorID,
		ConnectorType: req.ConnectorType,
		URL:           req.URL,
		ServerLabel:   req.ServerLabel,
		CreatedAt:     now,
		Metadata:      convertMetadata(req.Metadata),
	}

	err := h.connectorsStore.CreateConnector(r.Context(), connector)
	if err != nil {
		h.logger.Error("Failed to register connector", "error", err)
		h.writeError(w, http.StatusInternalServerError, "creation_error", err.Error())
		return
	}

	h.logger.Info("Connector registered", "connector_id", req.ConnectorID)

	// Return connector
	schemaConnector := schema.Connector{
		ConnectorID:   connector.ConnectorID,
		Object:        "connector",
		ConnectorType: connector.ConnectorType,
		URL:           connector.URL,
		ServerLabel:   connector.ServerLabel,
		CreatedAt:     connector.CreatedAt.Unix(),
		Metadata:      convertMetadataToInterface(connector.Metadata),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(schemaConnector)
}

// handleListConnectors handles GET /v1/connectors
func (h *Handler) handleListConnectors(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()
	after := query.Get("after")
	before := query.Get("before")
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

	h.logger.Info("Listing connectors", "after", after, "limit", limit, "order", order)

	// Get connectors from storage
	connectors, hasMore, err := h.connectorsStore.ListConnectorsPaginated(
		r.Context(), after, before, limit, order,
	)
	if err != nil {
		h.logger.Error("Failed to list connectors", "error", err)
		h.writeError(w, http.StatusInternalServerError, "list_error", err.Error())
		return
	}

	// Convert to schema
	schemaConnectors := make([]schema.Connector, 0, len(connectors))
	for _, connector := range connectors {
		c := schema.Connector{
			ConnectorID:   connector.ConnectorID,
			Object:        "connector",
			ConnectorType: connector.ConnectorType,
			URL:           connector.URL,
			ServerLabel:   connector.ServerLabel,
			CreatedAt:     connector.CreatedAt.Unix(),
			Metadata:      convertMetadataToInterface(connector.Metadata),
		}
		schemaConnectors = append(schemaConnectors, c)
	}

	// Build response
	listResp := schema.ListConnectorsResponse{
		Object:  "list",
		Data:    schemaConnectors,
		HasMore: hasMore,
	}

	if len(schemaConnectors) > 0 {
		listResp.FirstID = schemaConnectors[0].ConnectorID
		listResp.LastID = schemaConnectors[len(schemaConnectors)-1].ConnectorID
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(listResp)
}

// handleGetConnector handles GET /v1/connectors/{connector_id}
func (h *Handler) handleGetConnector(w http.ResponseWriter, r *http.Request) {
	// Extract connector ID from path
	connectorID := r.PathValue("connector_id")
	if connectorID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Connector ID is required")
		return
	}

	h.logger.Info("Getting connector", "connector_id", connectorID)

	// Get connector from storage
	connector, err := h.connectorsStore.GetConnector(r.Context(), connectorID)
	if err != nil {
		h.logger.Error("Failed to get connector", "error", err, "connector_id", connectorID)
		h.writeError(w, http.StatusNotFound, "connector_not_found", err.Error())
		return
	}

	// Convert to schema
	schemaConnector := schema.Connector{
		ConnectorID:   connector.ConnectorID,
		Object:        "connector",
		ConnectorType: connector.ConnectorType,
		URL:           connector.URL,
		ServerLabel:   connector.ServerLabel,
		CreatedAt:     connector.CreatedAt.Unix(),
		Metadata:      convertMetadataToInterface(connector.Metadata),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(schemaConnector)
}

// handleDeleteConnector handles DELETE /v1/connectors/{connector_id}
func (h *Handler) handleDeleteConnector(w http.ResponseWriter, r *http.Request) {
	// Extract connector ID from path
	connectorID := r.PathValue("connector_id")
	if connectorID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Connector ID is required")
		return
	}

	h.logger.Info("Deleting connector", "connector_id", connectorID)

	// Delete connector from storage
	err := h.connectorsStore.DeleteConnector(r.Context(), connectorID)
	if err != nil {
		h.logger.Error("Failed to delete connector", "error", err, "connector_id", connectorID)
		h.writeError(w, http.StatusNotFound, "connector_not_found", err.Error())
		return
	}

	// Return deletion confirmation
	deleteResp := schema.DeleteConnectorResponse{
		ConnectorID: connectorID,
		Object:      "connector.deleted",
		Deleted:     true,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(deleteResp)
}
