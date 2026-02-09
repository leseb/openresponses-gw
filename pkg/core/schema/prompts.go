// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package schema

// Prompt represents a prompt template
type Prompt struct {
	ID          string                 `json:"id"`     // Format: "prompt_{uuid}"
	Object      string                 `json:"object"` // Always "prompt"
	Name        string                 `json:"name"`   // Human-readable name
	Description string                 `json:"description,omitempty"`
	Template    string                 `json:"template"`             // Template with {{variables}}
	Variables   []string               `json:"variables,omitempty"`  // List of variable names
	CreatedAt   int64                  `json:"created_at"`           // Unix timestamp
	UpdatedAt   int64                  `json:"updated_at,omitempty"` // Unix timestamp
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// CreatePromptRequest represents a request to create a prompt
type CreatePromptRequest struct {
	Name        string                 `json:"name"` // Required
	Description string                 `json:"description,omitempty"`
	Template    string                 `json:"template"` // Required
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// UpdatePromptRequest represents a request to update a prompt
type UpdatePromptRequest struct {
	Name        *string                `json:"name,omitempty"`
	Description *string                `json:"description,omitempty"`
	Template    *string                `json:"template,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ListPromptsRequest represents a request to list prompts
type ListPromptsRequest struct {
	After  string `json:"after,omitempty"`
	Before string `json:"before,omitempty"`
	Limit  int    `json:"limit,omitempty"` // 1-100, default 50
	Order  string `json:"order,omitempty"` // "asc" or "desc", default "desc"
}

// ListPromptsResponse represents a list of prompts
type ListPromptsResponse struct {
	Object  string   `json:"object"`             // Always "list"
	Data    []Prompt `json:"data"`               // Array of prompts
	FirstID string   `json:"first_id,omitempty"` // ID of first item
	LastID  string   `json:"last_id,omitempty"`  // ID of last item
	HasMore bool     `json:"has_more"`           // Whether there are more results
}

// DeletePromptResponse represents the response from deleting a prompt
type DeletePromptResponse struct {
	ID      string `json:"id"`      // Prompt ID
	Object  string `json:"object"`  // Always "prompt.deleted"
	Deleted bool   `json:"deleted"` // Always true
}
