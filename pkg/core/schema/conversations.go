// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package schema

// Conversation represents a conversation
type Conversation struct {
	ID        string                 `json:"id"`         // Format: "conv_{uuid}"
	Object    string                 `json:"object"`     // Always "conversation"
	CreatedAt int64                  `json:"created_at"` // Unix timestamp
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// CreateConversationRequest represents a request to create a conversation
type CreateConversationRequest struct {
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ListConversationsRequest represents a request to list conversations
type ListConversationsRequest struct {
	After  string `json:"after,omitempty"`
	Before string `json:"before,omitempty"`
	Limit  int    `json:"limit,omitempty"` // 1-100, default 50
	Order  string `json:"order,omitempty"` // "asc" or "desc", default "desc"
}

// ListConversationsResponse represents a list of conversations
type ListConversationsResponse struct {
	Object  string         `json:"object"`             // Always "list"
	Data    []Conversation `json:"data"`               // Array of conversations
	FirstID string         `json:"first_id,omitempty"` // ID of first item
	LastID  string         `json:"last_id,omitempty"`  // ID of last item
	HasMore bool           `json:"has_more"`           // Whether there are more results
}

// DeleteConversationResponse represents the response from deleting a conversation
type DeleteConversationResponse struct {
	ID      string `json:"id"`      // Conversation ID
	Object  string `json:"object"`  // Always "conversation.deleted"
	Deleted bool   `json:"deleted"` // Always true
}

// ConversationItem represents an item in a conversation
type ConversationItem struct {
	ID        string                 `json:"id"`         // Item ID
	Object    string                 `json:"object"`     // Item type (message, function_call, etc.)
	Type      string                 `json:"type"`       // Item type
	CreatedAt int64                  `json:"created_at"` // Unix timestamp
	Content   interface{}            `json:"content,omitempty"`
	Role      string                 `json:"role,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// AddConversationItemsRequest represents a request to add items to a conversation
type AddConversationItemsRequest struct {
	Items []ConversationItem `json:"items"` // Items to add (max 20 per request)
}

// AddConversationItemsResponse represents the response from adding items
type AddConversationItemsResponse struct {
	Object string             `json:"object"` // Always "list"
	Data   []ConversationItem `json:"data"`   // Added items with IDs
}

// ListConversationItemsRequest represents a request to list conversation items
type ListConversationItemsRequest struct {
	After  string `json:"after,omitempty"`
	Before string `json:"before,omitempty"`
	Limit  int    `json:"limit,omitempty"` // Default 50
	Order  string `json:"order,omitempty"` // Default "desc"
}

// ListConversationItemsResponse represents a list of conversation items
type ListConversationItemsResponse struct {
	Object  string             `json:"object"`             // Always "list"
	Data    []ConversationItem `json:"data"`               // Array of items
	FirstID string             `json:"first_id,omitempty"` // ID of first item
	LastID  string             `json:"last_id,omitempty"`  // ID of last item
	HasMore bool               `json:"has_more"`           // Whether there are more results
}
