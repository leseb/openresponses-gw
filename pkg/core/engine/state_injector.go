// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/leseb/openresponses-gw/pkg/core/api"
	"github.com/leseb/openresponses-gw/pkg/core/schema"
)

// ConversationState holds the state that needs to be passed through the filter chain.
// This is serialized and injected into the request for filter chain mode.
type ConversationState struct {
	// ConversationID is the conversation this request belongs to
	ConversationID string `json:"conversation_id"`
	// ResponseID is the gateway-generated response ID
	ResponseID string `json:"response_id"`
	// Messages contains the reconstructed conversation history
	Messages []api.Message `json:"messages"`
	// ExpandedTools contains tools after MCP/file_search expansion
	ExpandedTools []schema.ResponsesToolParam `json:"expanded_tools,omitempty"`
	// MCPToolNames maps tool names to their MCP server labels for server-side execution
	MCPToolNames map[string]string `json:"mcp_tool_names,omitempty"`
	// FileSearchConfigs maps tool names to file_search configurations
	FileSearchConfigs map[string]FileSearchConfig `json:"file_search_configs,omitempty"`
	// OriginalRequest stores the original request for response processing
	OriginalRequest *schema.ResponseRequest `json:"original_request,omitempty"`
}

// FileSearchConfig holds configuration for file_search tools (exported for serialization)
type FileSearchConfig struct {
	VectorStoreIDs []string `json:"vector_store_ids"`
	MaxNumResults  int      `json:"max_num_results"`
}

// Model returns the model name from the original request.
func (s *ConversationState) Model() string {
	if s.OriginalRequest != nil && s.OriginalRequest.Model != nil {
		return *s.OriginalRequest.Model
	}
	return ""
}

// PreparedRequest contains all the information needed for filter chain mode
// after the preparation phase and before sending to the backend.
type PreparedRequest struct {
	// State is the conversation state to inject
	State *ConversationState
	// BackendRequest is the request to send to the backend (without state injection)
	BackendRequest *api.ResponsesAPIRequest
	// Model is the model to use for the request
	Model string
}

// StateInjector handles injecting and extracting conversation state
// for filter chain mode requests.
type StateInjector interface {
	// InjectIntoBody modifies the request body to include conversation state
	InjectIntoBody(originalBody []byte, state *ConversationState) ([]byte, error)
	// InjectIntoHeaders returns headers to add with state information
	InjectIntoHeaders(state *ConversationState) map[string]string
	// ExtractFromRequest extracts state from headers and/or body
	ExtractFromRequest(headers map[string]string, body []byte) (*ConversationState, error)
}

// HeaderStateInjector implements StateInjector by encoding state in headers.
// This is the recommended approach as it doesn't modify the request body.
type HeaderStateInjector struct{}

// NewHeaderStateInjector creates a new HeaderStateInjector.
func NewHeaderStateInjector() *HeaderStateInjector {
	return &HeaderStateInjector{}
}

// stateHeaderName is the header used to pass state through the filter chain
const stateHeaderName = "x-openresponses-state"

// InjectIntoBody returns the original body unchanged (state is in headers).
func (h *HeaderStateInjector) InjectIntoBody(originalBody []byte, _ *ConversationState) ([]byte, error) {
	return originalBody, nil
}

// InjectIntoHeaders returns a map with the state encoded in a header.
func (h *HeaderStateInjector) InjectIntoHeaders(state *ConversationState) map[string]string {
	if state == nil {
		return nil
	}

	// Serialize state to JSON and base64 encode
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return nil
	}

	return map[string]string{
		stateHeaderName: base64.StdEncoding.EncodeToString(stateJSON),
	}
}

// ExtractFromRequest extracts state from the header.
func (h *HeaderStateInjector) ExtractFromRequest(headers map[string]string, _ []byte) (*ConversationState, error) {
	encoded, ok := headers[stateHeaderName]
	if !ok {
		return nil, fmt.Errorf("state header %s not found", stateHeaderName)
	}

	// Decode base64
	stateJSON, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode state header: %w", err)
	}

	// Unmarshal JSON
	var state ConversationState
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	return &state, nil
}

// BodyStateInjector implements StateInjector by embedding state in the request body.
// This modifies the body by adding a gateway-specific field.
type BodyStateInjector struct{}

// NewBodyStateInjector creates a new BodyStateInjector.
func NewBodyStateInjector() *BodyStateInjector {
	return &BodyStateInjector{}
}

// stateBodyField is the field name used to embed state in the body
const stateBodyField = "_openresponses_state"

// InjectIntoBody adds the state as a field in the JSON body.
func (b *BodyStateInjector) InjectIntoBody(originalBody []byte, state *ConversationState) ([]byte, error) {
	if state == nil {
		return originalBody, nil
	}

	// Parse the original body
	var bodyMap map[string]interface{}
	if err := json.Unmarshal(originalBody, &bodyMap); err != nil {
		return nil, fmt.Errorf("failed to parse body: %w", err)
	}

	// Serialize state and add to body
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}

	// Add base64-encoded state to avoid nested JSON issues
	bodyMap[stateBodyField] = base64.StdEncoding.EncodeToString(stateJSON)

	// Re-marshal the body
	return json.Marshal(bodyMap)
}

// InjectIntoHeaders returns nil (state is in body).
func (b *BodyStateInjector) InjectIntoHeaders(_ *ConversationState) map[string]string {
	return nil
}

// ExtractFromRequest extracts state from the body field.
func (b *BodyStateInjector) ExtractFromRequest(_ map[string]string, body []byte) (*ConversationState, error) {
	var bodyMap map[string]interface{}
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		return nil, fmt.Errorf("failed to parse body: %w", err)
	}

	encoded, ok := bodyMap[stateBodyField].(string)
	if !ok {
		return nil, fmt.Errorf("state field %s not found in body", stateBodyField)
	}

	// Decode base64
	stateJSON, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode state from body: %w", err)
	}

	// Unmarshal JSON
	var state ConversationState
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	return &state, nil
}
