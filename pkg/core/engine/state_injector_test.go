// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/leseb/openresponses-gw/pkg/core/api"
	"github.com/leseb/openresponses-gw/pkg/core/schema"
)

func TestConversationState_Model(t *testing.T) {
	tests := []struct {
		name  string
		state *ConversationState
		want  string
	}{
		{
			name:  "nil state",
			state: nil,
			want:  "",
		},
		{
			name: "nil original request",
			state: &ConversationState{
				OriginalRequest: nil,
			},
			want: "",
		},
		{
			name: "nil model in request",
			state: &ConversationState{
				OriginalRequest: &schema.ResponseRequest{},
			},
			want: "",
		},
		{
			name: "model present",
			state: &ConversationState{
				OriginalRequest: &schema.ResponseRequest{
					Model: stringPtr("gpt-4"),
				},
			},
			want: "gpt-4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			if tt.state != nil {
				got = tt.state.Model()
			}
			if got != tt.want {
				t.Errorf("ConversationState.Model() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- HeaderStateInjector tests ---

func TestHeaderStateInjector_InjectIntoBody(t *testing.T) {
	injector := NewHeaderStateInjector()
	originalBody := []byte(`{"model": "test", "input": "hello"}`)
	state := &ConversationState{
		ConversationID: "conv_123",
		ResponseID:     "resp_456",
	}

	result, err := injector.InjectIntoBody(originalBody, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Body should be unchanged (state goes in headers)
	if string(result) != string(originalBody) {
		t.Errorf("body should be unchanged, got %s", string(result))
	}
}

func TestHeaderStateInjector_InjectIntoHeaders(t *testing.T) {
	injector := NewHeaderStateInjector()

	tests := []struct {
		name      string
		state     *ConversationState
		wantNil   bool
		wantKey   string
		checkFunc func(t *testing.T, value string)
	}{
		{
			name:    "nil state",
			state:   nil,
			wantNil: true,
		},
		{
			name: "valid state",
			state: &ConversationState{
				ConversationID: "conv_123",
				ResponseID:     "resp_456",
				Messages: []api.Message{
					{Role: "user", Content: "hello"},
				},
			},
			wantNil: false,
			wantKey: "x-openresponses-state",
			checkFunc: func(t *testing.T, value string) {
				// Decode and verify
				decoded, err := base64.StdEncoding.DecodeString(value)
				if err != nil {
					t.Fatalf("failed to decode base64: %v", err)
				}
				var state ConversationState
				if err := json.Unmarshal(decoded, &state); err != nil {
					t.Fatalf("failed to unmarshal state: %v", err)
				}
				if state.ConversationID != "conv_123" {
					t.Errorf("expected ConversationID=conv_123, got %q", state.ConversationID)
				}
				if state.ResponseID != "resp_456" {
					t.Errorf("expected ResponseID=resp_456, got %q", state.ResponseID)
				}
				if len(state.Messages) != 1 {
					t.Errorf("expected 1 message, got %d", len(state.Messages))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := injector.InjectIntoHeaders(tt.state)
			if tt.wantNil {
				if headers != nil {
					t.Errorf("expected nil headers, got %v", headers)
				}
				return
			}
			if headers == nil {
				t.Fatal("expected non-nil headers")
			}
			value, ok := headers[tt.wantKey]
			if !ok {
				t.Fatalf("expected key %q in headers", tt.wantKey)
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, value)
			}
		})
	}
}

func TestHeaderStateInjector_ExtractFromRequest(t *testing.T) {
	injector := NewHeaderStateInjector()

	// Create a valid state and encode it
	originalState := &ConversationState{
		ConversationID: "conv_abc",
		ResponseID:     "resp_xyz",
		Messages: []api.Message{
			{Role: "user", Content: "test"},
			{Role: "assistant", Content: "response"},
		},
		MCPToolNames: map[string]string{
			"tool1": "http://server1",
		},
	}
	stateJSON, _ := json.Marshal(originalState)
	encoded := base64.StdEncoding.EncodeToString(stateJSON)

	tests := []struct {
		name    string
		headers map[string]string
		wantErr bool
		check   func(t *testing.T, state *ConversationState)
	}{
		{
			name:    "missing header",
			headers: map[string]string{},
			wantErr: true,
		},
		{
			name: "invalid base64",
			headers: map[string]string{
				"x-openresponses-state": "not-valid-base64!!!",
			},
			wantErr: true,
		},
		{
			name: "invalid JSON",
			headers: map[string]string{
				"x-openresponses-state": base64.StdEncoding.EncodeToString([]byte("not json")),
			},
			wantErr: true,
		},
		{
			name: "valid state",
			headers: map[string]string{
				"x-openresponses-state": encoded,
			},
			wantErr: false,
			check: func(t *testing.T, state *ConversationState) {
				if state.ConversationID != "conv_abc" {
					t.Errorf("expected ConversationID=conv_abc, got %q", state.ConversationID)
				}
				if state.ResponseID != "resp_xyz" {
					t.Errorf("expected ResponseID=resp_xyz, got %q", state.ResponseID)
				}
				if len(state.Messages) != 2 {
					t.Errorf("expected 2 messages, got %d", len(state.Messages))
				}
				if state.MCPToolNames["tool1"] != "http://server1" {
					t.Errorf("expected MCPToolNames[tool1]=http://server1, got %q", state.MCPToolNames["tool1"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, err := injector.ExtractFromRequest(tt.headers, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractFromRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, state)
			}
		})
	}
}

// --- BodyStateInjector tests ---

func TestBodyStateInjector_InjectIntoBody(t *testing.T) {
	injector := NewBodyStateInjector()

	tests := []struct {
		name         string
		originalBody []byte
		state        *ConversationState
		wantErr      bool
		checkFunc    func(t *testing.T, result []byte)
	}{
		{
			name:         "nil state",
			originalBody: []byte(`{"model":"test"}`),
			state:        nil,
			wantErr:      false,
			checkFunc: func(t *testing.T, result []byte) {
				if string(result) != `{"model":"test"}` {
					t.Errorf("expected unchanged body, got %s", string(result))
				}
			},
		},
		{
			name:         "invalid JSON body",
			originalBody: []byte(`not json`),
			state:        &ConversationState{ConversationID: "conv_1"},
			wantErr:      true,
		},
		{
			name:         "valid injection",
			originalBody: []byte(`{"model":"test","input":"hello"}`),
			state: &ConversationState{
				ConversationID: "conv_inject",
				ResponseID:     "resp_inject",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result []byte) {
				var body map[string]interface{}
				if err := json.Unmarshal(result, &body); err != nil {
					t.Fatalf("failed to unmarshal result: %v", err)
				}
				// Original fields preserved
				if body["model"] != "test" {
					t.Errorf("expected model=test, got %v", body["model"])
				}
				if body["input"] != "hello" {
					t.Errorf("expected input=hello, got %v", body["input"])
				}
				// State field added
				stateField, ok := body["_openresponses_state"].(string)
				if !ok {
					t.Fatal("expected _openresponses_state field")
				}
				// Decode and verify
				decoded, err := base64.StdEncoding.DecodeString(stateField)
				if err != nil {
					t.Fatalf("failed to decode state: %v", err)
				}
				var state ConversationState
				if err := json.Unmarshal(decoded, &state); err != nil {
					t.Fatalf("failed to unmarshal state: %v", err)
				}
				if state.ConversationID != "conv_inject" {
					t.Errorf("expected ConversationID=conv_inject, got %q", state.ConversationID)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := injector.InjectIntoBody(tt.originalBody, tt.state)
			if (err != nil) != tt.wantErr {
				t.Errorf("InjectIntoBody() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checkFunc != nil {
				tt.checkFunc(t, result)
			}
		})
	}
}

func TestBodyStateInjector_InjectIntoHeaders(t *testing.T) {
	injector := NewBodyStateInjector()
	headers := injector.InjectIntoHeaders(&ConversationState{ConversationID: "test"})
	if headers != nil {
		t.Error("expected nil headers for BodyStateInjector")
	}
}

func TestBodyStateInjector_ExtractFromRequest(t *testing.T) {
	injector := NewBodyStateInjector()

	// Create valid state and embed in body
	state := &ConversationState{
		ConversationID: "conv_body",
		ResponseID:     "resp_body",
	}
	stateJSON, _ := json.Marshal(state)
	encoded := base64.StdEncoding.EncodeToString(stateJSON)
	validBody := []byte(fmt.Sprintf(`{"model":"test","_openresponses_state":"%s"}`, encoded))

	tests := []struct {
		name    string
		body    []byte
		wantErr bool
		check   func(t *testing.T, state *ConversationState)
	}{
		{
			name:    "invalid JSON",
			body:    []byte(`not json`),
			wantErr: true,
		},
		{
			name:    "missing state field",
			body:    []byte(`{"model":"test"}`),
			wantErr: true,
		},
		{
			name:    "invalid state encoding",
			body:    []byte(`{"_openresponses_state":"not-valid"}`),
			wantErr: true,
		},
		{
			name:    "valid extraction",
			body:    validBody,
			wantErr: false,
			check: func(t *testing.T, state *ConversationState) {
				if state.ConversationID != "conv_body" {
					t.Errorf("expected ConversationID=conv_body, got %q", state.ConversationID)
				}
				if state.ResponseID != "resp_body" {
					t.Errorf("expected ResponseID=resp_body, got %q", state.ResponseID)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, err := injector.ExtractFromRequest(nil, tt.body)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractFromRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, state)
			}
		})
	}
}

// --- FileSearchConfig tests ---

func TestFileSearchConfig_Serialization(t *testing.T) {
	config := FileSearchConfig{
		VectorStoreIDs: []string{"vs-1", "vs-2"},
		MaxNumResults:  15,
	}

	// Marshal
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal
	var parsed FileSearchConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(parsed.VectorStoreIDs) != 2 {
		t.Errorf("expected 2 vector store IDs, got %d", len(parsed.VectorStoreIDs))
	}
	if parsed.MaxNumResults != 15 {
		t.Errorf("expected MaxNumResults=15, got %d", parsed.MaxNumResults)
	}
}

// --- PreparedRequest tests ---

func TestPreparedRequest_Fields(t *testing.T) {
	prepared := &PreparedRequest{
		State: &ConversationState{
			ConversationID: "conv_prep",
			ResponseID:     "resp_prep",
		},
		BackendRequest: &api.ResponsesAPIRequest{
			Model: "test-model",
		},
		Model: "test-model",
	}

	if prepared.State == nil {
		t.Fatal("expected non-nil State")
	}
	if prepared.State.ConversationID != "conv_prep" {
		t.Errorf("expected ConversationID=conv_prep, got %q", prepared.State.ConversationID)
	}
	if prepared.Model != "test-model" {
		t.Errorf("expected Model=test-model, got %q", prepared.Model)
	}
}
