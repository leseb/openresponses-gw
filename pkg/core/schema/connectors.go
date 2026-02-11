// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package schema

// Connector represents a registered MCP connector
type Connector struct {
	ConnectorID   string                 `json:"connector_id"`
	Object        string                 `json:"object"`                 // Always "connector"
	ConnectorType string                 `json:"connector_type"`         // Always "mcp" for now
	URL           string                 `json:"url"`                    // MCP server URL
	ServerLabel   string                 `json:"server_label,omitempty"` // Display label
	CreatedAt     int64                  `json:"created_at"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// RegisterConnectorRequest represents a request to register a connector
type RegisterConnectorRequest struct {
	ConnectorID   string                 `json:"connector_id"`           // Required
	ConnectorType string                 `json:"connector_type"`         // Required, must be "mcp"
	URL           string                 `json:"url"`                    // Required
	ServerLabel   string                 `json:"server_label,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// ListConnectorsResponse represents a list of connectors
type ListConnectorsResponse struct {
	Object  string      `json:"object"`             // Always "list"
	Data    []Connector `json:"data"`               // Array of connectors
	FirstID string      `json:"first_id,omitempty"` // ID of first item
	LastID  string      `json:"last_id,omitempty"`  // ID of last item
	HasMore bool        `json:"has_more"`           // Whether there are more results
}

// DeleteConnectorResponse represents the response from deleting a connector
type DeleteConnectorResponse struct {
	ConnectorID string `json:"connector_id"`
	Object      string `json:"object"`  // Always "connector.deleted"
	Deleted     bool   `json:"deleted"` // Always true
}
