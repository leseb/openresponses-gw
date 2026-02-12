// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/leseb/openresponses-gw/pkg/core/schema"
	"github.com/leseb/openresponses-gw/pkg/core/state"
)

// handleCreateConversation handles POST /v1/conversations
//
//	@Summary	Create conversation
//	@Tags		Conversations
//	@Accept		json
//	@Produce	json
//	@Param		request	body		schema.CreateConversationRequest	true	"Create conversation request"
//	@Success	200		{object}	schema.Conversation
//	@Failure	400		{object}	map[string]interface{}
//	@Failure	500		{object}	map[string]interface{}
//	@Router		/v1/conversations [post]
func (h *Handler) handleCreateConversation(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var req schema.CreateConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to parse conversation request", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Failed to parse request body")
		return
	}

	// Create conversation
	convID := generateID("conv_")
	now := time.Now()

	stateConv := &state.Conversation{
		ID:        convID,
		SessionID: "", // Not associated with a session for now
		Messages:  []state.Message{},
		Metadata:  convertMetadata(req.Metadata),
		CreatedAt: now,
		UpdatedAt: now,
	}

	err := h.engine.Store().CreateConversation(r.Context(), stateConv)
	if err != nil {
		h.logger.Error("Failed to create conversation", "error", err)
		h.writeError(w, http.StatusInternalServerError, "creation_error", err.Error())
		return
	}

	h.logger.Info("Conversation created", "conversation_id", convID)

	// Return conversation
	conv := schema.Conversation{
		ID:        convID,
		Object:    "conversation",
		CreatedAt: now.Unix(),
		Metadata:  req.Metadata,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(conv)
}

// handleListConversations handles GET /v1/conversations
//
//	@Summary	List conversations
//	@Tags		Conversations
//	@Produce	json
//	@Param		after	query		string	false	"Cursor for pagination"
//	@Param		before	query		string	false	"Cursor for pagination (backwards)"
//	@Param		limit	query		int		false	"Number of items (1-100, default 50)"
//	@Param		order	query		string	false	"Sort order: asc or desc (default desc)"
//	@Success	200		{object}	schema.ListConversationsResponse
//	@Failure	500		{object}	map[string]interface{}
//	@Router		/v1/conversations [get]
func (h *Handler) handleListConversations(w http.ResponseWriter, r *http.Request) {
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

	h.logger.Info("Listing conversations", "after", after, "limit", limit, "order", order)

	// Get conversations from storage
	stateConvs, hasMore, err := h.engine.Store().ListConversationsPaginated(
		r.Context(), after, before, limit, order,
	)
	if err != nil {
		h.logger.Error("Failed to list conversations", "error", err)
		h.writeError(w, http.StatusInternalServerError, "list_error", err.Error())
		return
	}

	// Convert to schema
	conversations := make([]schema.Conversation, 0, len(stateConvs))
	for _, stateConv := range stateConvs {
		conv := schema.Conversation{
			ID:        stateConv.ID,
			Object:    "conversation",
			CreatedAt: stateConv.CreatedAt.Unix(),
			Metadata:  convertMetadataToInterface(stateConv.Metadata),
		}
		conversations = append(conversations, conv)
	}

	// Build response
	listResp := schema.ListConversationsResponse{
		Object:  "list",
		Data:    conversations,
		HasMore: hasMore,
	}

	if len(conversations) > 0 {
		listResp.FirstID = conversations[0].ID
		listResp.LastID = conversations[len(conversations)-1].ID
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(listResp)
}

// handleGetConversation handles GET /v1/conversations/{id}
//
//	@Summary	Get conversation
//	@Tags		Conversations
//	@Produce	json
//	@Param		id	path		string	true	"Conversation ID"
//	@Success	200	{object}	schema.Conversation
//	@Failure	400	{object}	map[string]interface{}
//	@Failure	404	{object}	map[string]interface{}
//	@Router		/v1/conversations/{id} [get]
func (h *Handler) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	// Extract conversation ID from path
	conversationID := r.PathValue("id")
	if conversationID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Conversation ID is required")
		return
	}

	h.logger.Info("Getting conversation", "conversation_id", conversationID)

	// Get conversation from storage
	stateConv, err := h.engine.Store().GetConversation(r.Context(), conversationID)
	if err != nil {
		h.logger.Error("Failed to get conversation", "error", err, "conversation_id", conversationID)
		h.writeError(w, http.StatusNotFound, "conversation_not_found", err.Error())
		return
	}

	// Convert to schema
	conv := schema.Conversation{
		ID:        stateConv.ID,
		Object:    "conversation",
		CreatedAt: stateConv.CreatedAt.Unix(),
		Metadata:  convertMetadataToInterface(stateConv.Metadata),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(conv)
}

// handleDeleteConversation handles DELETE /v1/conversations/{id}
//
//	@Summary	Delete conversation
//	@Tags		Conversations
//	@Produce	json
//	@Param		id	path		string	true	"Conversation ID"
//	@Success	200	{object}	schema.DeleteConversationResponse
//	@Failure	400	{object}	map[string]interface{}
//	@Failure	404	{object}	map[string]interface{}
//	@Router		/v1/conversations/{id} [delete]
func (h *Handler) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	// Extract conversation ID from path
	conversationID := r.PathValue("id")
	if conversationID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Conversation ID is required")
		return
	}

	h.logger.Info("Deleting conversation", "conversation_id", conversationID)

	// Delete conversation from storage
	err := h.engine.Store().DeleteConversation(r.Context(), conversationID)
	if err != nil {
		h.logger.Error("Failed to delete conversation", "error", err, "conversation_id", conversationID)
		h.writeError(w, http.StatusNotFound, "conversation_not_found", err.Error())
		return
	}

	// Return deletion confirmation
	deleteResp := schema.DeleteConversationResponse{
		ID:      conversationID,
		Object:  "conversation.deleted",
		Deleted: true,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(deleteResp)
}

// handleAddConversationItems handles POST /v1/conversations/{id}/items
//
//	@Summary	Add conversation items
//	@Tags		Conversations
//	@Accept		json
//	@Produce	json
//	@Param		id		path		string									true	"Conversation ID"
//	@Param		request	body		schema.AddConversationItemsRequest		true	"Items to add (max 20)"
//	@Success	200		{object}	schema.AddConversationItemsResponse
//	@Failure	400		{object}	map[string]interface{}
//	@Failure	404		{object}	map[string]interface{}
//	@Router		/v1/conversations/{id}/items [post]
func (h *Handler) handleAddConversationItems(w http.ResponseWriter, r *http.Request) {
	// Extract conversation ID from path
	conversationID := r.PathValue("id")
	if conversationID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Conversation ID is required")
		return
	}

	// Parse request body
	var req schema.AddConversationItemsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to parse items request", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Failed to parse request body")
		return
	}

	// Validate max 20 items
	if len(req.Items) > 20 {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Maximum 20 items per request")
		return
	}

	h.logger.Info("Adding conversation items", "conversation_id", conversationID, "count", len(req.Items))

	// Convert schema items to state messages
	now := time.Now()
	messages := make([]state.Message, 0, len(req.Items))
	for i, item := range req.Items {
		msgID := item.ID
		if msgID == "" {
			msgID = fmt.Sprintf("msg_%s_%d", conversationID, i)
		}

		msg := state.Message{
			ID:        msgID,
			Role:      item.Role,
			Content:   item.Content,
			Metadata:  convertMetadata(item.Metadata),
			CreatedAt: now,
		}
		messages = append(messages, msg)
	}

	// Add items to conversation
	err := h.engine.Store().AddConversationItems(r.Context(), conversationID, messages)
	if err != nil {
		h.logger.Error("Failed to add items", "error", err, "conversation_id", conversationID)
		h.writeError(w, http.StatusNotFound, "conversation_not_found", err.Error())
		return
	}

	// Convert back to schema for response
	items := make([]schema.ConversationItem, 0, len(messages))
	for _, msg := range messages {
		item := schema.ConversationItem{
			ID:        msg.ID,
			Object:    "conversation.item",
			Type:      "message",
			CreatedAt: msg.CreatedAt.Unix(),
			Role:      msg.Role,
			Content:   msg.Content,
			Metadata:  convertMetadataToInterface(msg.Metadata),
		}
		items = append(items, item)
	}

	// Return added items
	addResp := schema.AddConversationItemsResponse{
		Object: "list",
		Data:   items,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(addResp)
}

// handleListConversationItems handles GET /v1/conversations/{id}/items
//
//	@Summary	List conversation items
//	@Tags		Conversations
//	@Produce	json
//	@Param		id		path		string	true	"Conversation ID"
//	@Param		after	query		string	false	"Cursor for pagination"
//	@Param		before	query		string	false	"Cursor for pagination (backwards)"
//	@Param		limit	query		int		false	"Number of items (1-100, default 50)"
//	@Param		order	query		string	false	"Sort order: asc or desc (default desc)"
//	@Success	200		{object}	schema.ListConversationItemsResponse
//	@Failure	400		{object}	map[string]interface{}
//	@Failure	404		{object}	map[string]interface{}
//	@Router		/v1/conversations/{id}/items [get]
func (h *Handler) handleListConversationItems(w http.ResponseWriter, r *http.Request) {
	// Extract conversation ID from path
	conversationID := r.PathValue("id")
	if conversationID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Conversation ID is required")
		return
	}

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

	h.logger.Info("Listing conversation items", "conversation_id", conversationID, "limit", limit)

	// Get items from storage
	messages, hasMore, err := h.engine.Store().ListConversationItems(
		r.Context(), conversationID, after, before, limit, order,
	)
	if err != nil {
		h.logger.Error("Failed to list items", "error", err, "conversation_id", conversationID)
		h.writeError(w, http.StatusNotFound, "conversation_not_found", err.Error())
		return
	}

	// Convert to schema
	items := make([]schema.ConversationItem, 0, len(messages))
	for _, msg := range messages {
		item := schema.ConversationItem{
			ID:        msg.ID,
			Object:    "conversation.item",
			Type:      "message",
			CreatedAt: msg.CreatedAt.Unix(),
			Role:      msg.Role,
			Content:   msg.Content,
			Metadata:  convertMetadataToInterface(msg.Metadata),
		}
		items = append(items, item)
	}

	// Build response
	listResp := schema.ListConversationItemsResponse{
		Object:  "list",
		Data:    items,
		HasMore: hasMore,
	}

	if len(items) > 0 {
		listResp.FirstID = items[0].ID
		listResp.LastID = items[len(items)-1].ID
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(listResp)
}

// Helper functions

func convertMetadata(m map[string]interface{}) map[string]string {
	result := make(map[string]string)
	if m == nil {
		return result
	}
	for k, v := range m {
		if str, ok := v.(string); ok {
			result[k] = str
		} else {
			result[k] = fmt.Sprintf("%v", v)
		}
	}
	return result
}

func convertMetadataToInterface(m map[string]string) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		result[k] = v
	}
	return result
}
