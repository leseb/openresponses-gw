// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package schema

// Model represents an available model
type Model struct {
	ID      string `json:"id"`       // Model identifier (e.g., "gpt-4", "llama3.2:3b")
	Object  string `json:"object"`   // Always "model"
	Created int64  `json:"created"`  // Unix timestamp
	OwnedBy string `json:"owned_by"` // Organization/provider

	// Optional metadata
	Description string                 `json:"description,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ListModelsResponse represents a list of models
type ListModelsResponse struct {
	Object string  `json:"object"` // Always "list"
	Data   []Model `json:"data"`   // Array of models
}
