// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package schema

// ListResponsesRequest represents a request to list responses
type ListResponsesRequest struct {
	After  string `json:"after,omitempty"`  // Cursor for pagination
	Before string `json:"before,omitempty"` // Cursor for pagination (backwards)
	Limit  int    `json:"limit,omitempty"`  // Number of items (1-100, default 50)
	Order  string `json:"order,omitempty"`  // Sort order: "asc" or "desc" (default "desc")
	Model  string `json:"model,omitempty"`  // Filter by model
}

// ListResponsesResponse represents a paginated list of responses
type ListResponsesResponse struct {
	Object  string     `json:"object"`            // Always "list"
	Data    []Response `json:"data"`              // Array of responses
	FirstID string     `json:"first_id,omitempty"` // ID of first item
	LastID  string     `json:"last_id,omitempty"`  // ID of last item
	HasMore bool       `json:"has_more"`          // Whether there are more results
}

// DeleteResponseResponse represents the response from deleting a response
type DeleteResponseResponse struct {
	ID      string `json:"id"`      // Response ID
	Object  string `json:"object"`  // Always "response.deleted"
	Deleted bool   `json:"deleted"` // Always true
}

// ListInputItemsRequest represents a request to list input items for a response
type ListInputItemsRequest struct {
	After   string   `json:"after,omitempty"`   // Cursor for pagination
	Before  string   `json:"before,omitempty"`  // Cursor for pagination
	Limit   int      `json:"limit,omitempty"`   // Number of items (default 50)
	Order   string   `json:"order,omitempty"`   // Sort order
	Include []string `json:"include,omitempty"` // Fields to include
}

// ListInputItemsResponse represents a list of input items
type ListInputItemsResponse struct {
	Object  string                 `json:"object"`             // Always "list"
	Data    []interface{}          `json:"data"`               // Input items (messages/files/etc)
	FirstID string                 `json:"first_id,omitempty"` // ID of first item
	LastID  string                 `json:"last_id,omitempty"`  // ID of last item
	HasMore bool                   `json:"has_more"`           // Whether there are more results
}
