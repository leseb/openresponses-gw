// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	extproc "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"

	"github.com/leseb/openresponses-gw/pkg/core/api"
	"github.com/leseb/openresponses-gw/pkg/core/config"
	"github.com/leseb/openresponses-gw/pkg/core/engine"
	"github.com/leseb/openresponses-gw/pkg/core/schema"
	"github.com/leseb/openresponses-gw/pkg/storage/sqlite"
)

func TestNewProcessor(t *testing.T) {
	// Test with nil engine and logger (should not panic)
	proc := NewProcessor(nil, nil)
	if proc == nil {
		t.Fatal("expected non-nil processor")
	}
	if proc.logger == nil {
		t.Error("expected non-nil logger")
	}
	if proc.injector == nil {
		t.Error("expected non-nil injector")
	}
}

func TestGenerateRequestID(t *testing.T) {
	// Test format
	id := generateRequestID()
	if len(id) < 5 {
		t.Errorf("expected ID with prefix, got %q", id)
	}
	if id[:4] != "req_" {
		t.Errorf("expected prefix req_, got %q", id[:4])
	}

	// Test uniqueness
	id2 := generateRequestID()
	if id == id2 {
		t.Error("expected unique IDs")
	}
}

// --- Test helpers ---

func newTestProcessor(t *testing.T) *Processor {
	t.Helper()
	store, err := sqlite.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	cfg := &config.EngineConfig{
		ModelEndpoint: "http://localhost:8080/v1",
		BackendAPI:    "chat_completions",
	}
	eng, err := engine.New(cfg, store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return NewProcessor(eng, slog.Default())
}

func makeProcessingRequest(body []byte) *extproc.ProcessingRequest {
	return &extproc.ProcessingRequest{
		Request: &extproc.ProcessingRequest_RequestBody{
			RequestBody: &extproc.HttpBody{
				Body: body,
			},
		},
	}
}

// --- processRequestBody tests ---

func TestProcessRequestBody_ValidRequest(t *testing.T) {
	proc := newTestProcessor(t)
	ctx := context.Background()
	reqCtx := &requestContext{requestID: "unknown"}

	body, _ := json.Marshal(schema.ResponseRequest{
		Model: stringPtr("test-model"),
		Input: "hello",
	})
	req := makeProcessingRequest(body)

	resp := proc.processRequestBody(ctx, req, reqCtx)

	// Should return a request body response with body mutation and header mutations
	rb := resp.GetRequestBody()
	if rb == nil {
		t.Fatal("expected RequestBody response, got nil")
	}
	if rb.Response == nil {
		t.Fatal("expected non-nil CommonResponse")
	}
	if rb.Response.BodyMutation == nil {
		t.Fatal("expected non-nil BodyMutation")
	}
	mutatedBody := rb.Response.GetBodyMutation().GetBody()
	if len(mutatedBody) == 0 {
		t.Error("expected non-empty mutated body")
	}

	// Verify state was injected
	if !reqCtx.stateInjected {
		t.Error("expected stateInjected=true")
	}
	if reqCtx.preparedReq == nil {
		t.Error("expected non-nil preparedReq")
	}
	if reqCtx.requestID == "unknown" {
		t.Error("expected requestID to be updated")
	}
}

func TestProcessRequestBody_InvalidJSON(t *testing.T) {
	proc := newTestProcessor(t)
	ctx := context.Background()
	reqCtx := &requestContext{requestID: "unknown"}

	req := makeProcessingRequest([]byte("not valid json"))

	resp := proc.processRequestBody(ctx, req, reqCtx)

	// Should return an immediate error response (BadRequest)
	imm := resp.GetImmediateResponse()
	if imm == nil {
		t.Fatal("expected ImmediateResponse for invalid JSON")
	}
	if imm.Status == nil || imm.Status.Code != 400 {
		t.Errorf("expected status code 400, got %v", imm.Status)
	}

	// Verify body contains error message
	var errResp map[string]interface{}
	if err := json.Unmarshal(imm.Body, &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	errObj, ok := errResp["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected error object in response")
	}
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "Invalid request body") {
		t.Errorf("expected 'Invalid request body' in error message, got %q", msg)
	}
}

func TestProcessRequestBody_MissingModel(t *testing.T) {
	proc := newTestProcessor(t)
	ctx := context.Background()
	reqCtx := &requestContext{requestID: "unknown"}

	body, _ := json.Marshal(map[string]interface{}{
		"input": "hello",
	})
	req := makeProcessingRequest(body)

	resp := proc.processRequestBody(ctx, req, reqCtx)

	// Should return UnprocessableEntity (422)
	imm := resp.GetImmediateResponse()
	if imm == nil {
		t.Fatal("expected ImmediateResponse for missing model")
	}
	if imm.Status == nil || imm.Status.Code != 422 {
		t.Errorf("expected status code 422, got %v", imm.Status)
	}
}

func TestProcessRequestBody_MissingInput(t *testing.T) {
	proc := newTestProcessor(t)
	ctx := context.Background()
	reqCtx := &requestContext{requestID: "unknown"}

	body, _ := json.Marshal(map[string]interface{}{
		"model": "test-model",
	})
	req := makeProcessingRequest(body)

	resp := proc.processRequestBody(ctx, req, reqCtx)

	// Should return UnprocessableEntity (422)
	imm := resp.GetImmediateResponse()
	if imm == nil {
		t.Fatal("expected ImmediateResponse for missing input")
	}
	if imm.Status == nil || imm.Status.Code != 422 {
		t.Errorf("expected status code 422, got %v", imm.Status)
	}
}

func TestProcessRequestBody_SetsRequestContext(t *testing.T) {
	proc := newTestProcessor(t)
	ctx := context.Background()
	reqCtx := &requestContext{requestID: "unknown"}

	body, _ := json.Marshal(schema.ResponseRequest{
		Model: stringPtr("test-model"),
		Input: "hello",
	})
	req := makeProcessingRequest(body)

	proc.processRequestBody(ctx, req, reqCtx)

	if !reqCtx.stateInjected {
		t.Error("expected stateInjected=true")
	}
	if reqCtx.preparedReq == nil {
		t.Fatal("expected non-nil preparedReq")
	}
	if reqCtx.preparedReq.State == nil {
		t.Fatal("expected non-nil State in preparedReq")
	}
	if reqCtx.preparedReq.State.ResponseID == "" {
		t.Error("expected non-empty ResponseID in state")
	}
	if !strings.HasPrefix(reqCtx.requestID, "req_") {
		t.Errorf("expected requestID with req_ prefix, got %q", reqCtx.requestID)
	}
}

// --- processResponseBody tests ---

func TestProcessResponseBody_NoStateInjected(t *testing.T) {
	proc := newTestProcessor(t)
	ctx := context.Background()
	reqCtx := &requestContext{
		requestID:     "req-1",
		stateInjected: false,
		preparedReq:   nil,
	}

	body := &extproc.HttpBody{
		Body: []byte(`{"id":"resp-1","output":[]}`),
	}

	resp := proc.processResponseBody(ctx, body, reqCtx)

	// Should pass through with empty CommonResponse
	rb := resp.GetResponseBody()
	if rb == nil {
		t.Fatal("expected ResponseBody response")
	}
	if rb.Response.BodyMutation != nil {
		t.Error("expected no body mutation for pass-through")
	}
}

func TestProcessResponseBody_ValidResponse(t *testing.T) {
	proc := newTestProcessor(t)
	ctx := context.Background()

	// First prepare a request to get valid state
	reqCtx := &requestContext{requestID: "unknown"}
	reqBody, _ := json.Marshal(schema.ResponseRequest{
		Model: stringPtr("test-model"),
		Input: "hello",
	})
	proc.processRequestBody(ctx, makeProcessingRequest(reqBody), reqCtx)
	if !reqCtx.stateInjected {
		t.Fatal("setup: state not injected")
	}

	// Now process a backend response
	backendResp := api.ResponsesAPIResponse{
		ID:     "backend-resp-1",
		Status: "completed",
		Output: []api.OutputItem{
			{
				Type: "message",
				ID:   "msg-1",
				Role: "assistant",
				Content: []api.ContentItem{
					{Type: "output_text", Text: "Hello there!"},
				},
			},
		},
		Usage: &api.UsageInfo{
			InputTokens:  10,
			OutputTokens: 5,
			TotalTokens:  15,
		},
	}
	respBody, _ := json.Marshal(backendResp)

	resp := proc.processResponseBody(ctx, &extproc.HttpBody{Body: respBody}, reqCtx)

	rb := resp.GetResponseBody()
	if rb == nil {
		t.Fatal("expected ResponseBody response")
	}
	if rb.Response.BodyMutation == nil {
		t.Fatal("expected body mutation")
	}

	// Parse the modified body
	modifiedBody := rb.Response.GetBodyMutation().GetBody()
	var finalResp schema.Response
	if err := json.Unmarshal(modifiedBody, &finalResp); err != nil {
		t.Fatalf("failed to parse modified response body: %v", err)
	}

	// Verify gateway response ID is used
	if finalResp.ID != reqCtx.preparedReq.State.ResponseID {
		t.Errorf("expected response ID=%q, got %q", reqCtx.preparedReq.State.ResponseID, finalResp.ID)
	}
	if finalResp.Status != "completed" {
		t.Errorf("expected status=completed, got %q", finalResp.Status)
	}
}

func TestProcessResponseBody_InvalidJSON(t *testing.T) {
	proc := newTestProcessor(t)
	ctx := context.Background()

	// Prepare a request to get valid state
	reqCtx := &requestContext{requestID: "unknown"}
	reqBody, _ := json.Marshal(schema.ResponseRequest{
		Model: stringPtr("test-model"),
		Input: "hello",
	})
	proc.processRequestBody(ctx, makeProcessingRequest(reqBody), reqCtx)

	// Send non-JSON backend response
	resp := proc.processResponseBody(ctx, &extproc.HttpBody{Body: []byte("not json")}, reqCtx)

	// Should pass through gracefully (no body mutation)
	rb := resp.GetResponseBody()
	if rb == nil {
		t.Fatal("expected ResponseBody response")
	}
	if rb.Response.BodyMutation != nil {
		t.Error("expected pass-through (no body mutation) for invalid JSON")
	}
}

func TestProcessResponseBody_ResponseIDRewrite(t *testing.T) {
	proc := newTestProcessor(t)
	ctx := context.Background()

	// Prepare a request
	reqCtx := &requestContext{requestID: "unknown"}
	reqBody, _ := json.Marshal(schema.ResponseRequest{
		Model: stringPtr("test-model"),
		Input: "hello",
	})
	proc.processRequestBody(ctx, makeProcessingRequest(reqBody), reqCtx)

	gatewayResponseID := reqCtx.preparedReq.State.ResponseID

	// Backend response with a different ID
	backendResp := api.ResponsesAPIResponse{
		ID:     "backend-id-should-be-replaced",
		Status: "completed",
		Output: []api.OutputItem{
			{
				Type:    "message",
				ID:      "msg-1",
				Role:    "assistant",
				Content: []api.ContentItem{{Type: "output_text", Text: "hi"}},
			},
		},
	}
	respBody, _ := json.Marshal(backendResp)

	resp := proc.processResponseBody(ctx, &extproc.HttpBody{Body: respBody}, reqCtx)

	rb := resp.GetResponseBody()
	if rb == nil || rb.Response.BodyMutation == nil {
		t.Fatal("expected body mutation")
	}

	var finalResp schema.Response
	if err := json.Unmarshal(rb.Response.GetBodyMutation().GetBody(), &finalResp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// The response ID should be the gateway's, not the backend's
	if finalResp.ID == "backend-id-should-be-replaced" {
		t.Error("response ID was not rewritten from backend ID")
	}
	if finalResp.ID != gatewayResponseID {
		t.Errorf("expected gateway response ID=%q, got %q", gatewayResponseID, finalResp.ID)
	}
}
