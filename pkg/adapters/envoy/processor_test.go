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
	if proc.stateInjector == nil {
		t.Error("expected non-nil stateInjector")
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
	return newTestProcessorWithBackend(t, "chat_completions")
}

func newTestProcessorWithBackend(t *testing.T, backendAPI string) *Processor {
	t.Helper()
	store, err := sqlite.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	cfg := &config.EngineConfig{
		ModelEndpoint: "http://localhost:8080/v1",
		BackendAPI:    backendAPI,
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

// --- processRequestHeaders tests ---

func TestProcessRequestHeaders_ChatCompletionsPath(t *testing.T) {
	proc := newTestProcessor(t) // default backend_api=chat_completions
	reqCtx := &requestContext{requestID: "unknown"}

	resp := proc.processRequestHeaders(reqCtx)

	rh := resp.GetRequestHeaders()
	if rh == nil {
		t.Fatal("expected RequestHeaders response")
	}
	if rh.Response.HeaderMutation == nil {
		t.Fatal("expected header mutations for chat_completions backend")
	}

	// content-length should be removed (body phase will replace the body)
	foundRemoveContentLength := false
	for _, h := range rh.Response.HeaderMutation.RemoveHeaders {
		if h == "content-length" {
			foundRemoveContentLength = true
		}
	}
	if !foundRemoveContentLength {
		t.Error("expected content-length to be removed")
	}

	// :path should be set to /v1/chat/completions
	foundPath := false
	for _, h := range rh.Response.HeaderMutation.SetHeaders {
		if h.Header.Key == ":path" {
			foundPath = true
			if string(h.Header.RawValue) != "/v1/chat/completions" {
				t.Errorf("expected :path=/v1/chat/completions, got %q", string(h.Header.RawValue))
			}
		}
	}
	if !foundPath {
		t.Error("expected :path header mutation for chat_completions backend")
	}
}

func TestProcessRequestHeaders_ResponsesNoPathMutation(t *testing.T) {
	proc := newTestProcessorWithBackend(t, "responses")
	reqCtx := &requestContext{requestID: "unknown"}

	resp := proc.processRequestHeaders(reqCtx)

	rh := resp.GetRequestHeaders()
	if rh == nil {
		t.Fatal("expected RequestHeaders response")
	}
	if rh.Response.HeaderMutation == nil {
		t.Fatal("expected header mutations (content-length removal)")
	}

	// content-length should still be removed
	foundRemoveContentLength := false
	for _, h := range rh.Response.HeaderMutation.RemoveHeaders {
		if h == "content-length" {
			foundRemoveContentLength = true
		}
	}
	if !foundRemoveContentLength {
		t.Error("expected content-length to be removed")
	}

	// Verify :path header is NOT mutated for responses backend
	for _, h := range rh.Response.HeaderMutation.SetHeaders {
		if h.Header.Key == ":path" {
			t.Error("unexpected :path header mutation for responses backend")
		}
	}
}

func TestProcessRequestHeaders_NilEngine(t *testing.T) {
	proc := NewProcessor(nil, nil)
	reqCtx := &requestContext{requestID: "unknown"}

	// Should not panic with nil engine
	resp := proc.processRequestHeaders(reqCtx)

	rh := resp.GetRequestHeaders()
	if rh == nil {
		t.Fatal("expected RequestHeaders response")
	}
	// content-length should still be removed
	if rh.Response.HeaderMutation == nil {
		t.Fatal("expected header mutations (content-length removal)")
	}
	foundRemoveContentLength := false
	for _, h := range rh.Response.HeaderMutation.RemoveHeaders {
		if h == "content-length" {
			foundRemoveContentLength = true
		}
	}
	if !foundRemoveContentLength {
		t.Error("expected content-length to be removed even with nil engine")
	}
	// No path mutation expected when engine is nil
	if len(rh.Response.HeaderMutation.SetHeaders) > 0 {
		t.Error("expected no set header mutations with nil engine")
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

	// Body should be a chat completions request (default backend_api)
	var chatReq api.ChatCompletionRequest
	if err := json.Unmarshal(mutatedBody, &chatReq); err != nil {
		t.Fatalf("expected body to be a ChatCompletionRequest: %v", err)
	}
	if chatReq.Model != "test-model" {
		t.Errorf("expected model=test-model, got %q", chatReq.Model)
	}
	if len(chatReq.Messages) == 0 {
		t.Error("expected non-empty messages in chat request")
	}

	// Verify state headers are injected (but NOT :path — that's done in headers phase)
	if rb.Response.HeaderMutation != nil {
		for _, h := range rb.Response.HeaderMutation.SetHeaders {
			if h.Header.Key == ":path" {
				t.Error(":path mutation should be in headers phase, not body phase")
			}
		}
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

func TestProcessRequestBody_ResponsesBackend(t *testing.T) {
	proc := newTestProcessorWithBackend(t, "responses")
	ctx := context.Background()
	reqCtx := &requestContext{requestID: "unknown"}

	body, _ := json.Marshal(schema.ResponseRequest{
		Model: stringPtr("test-model"),
		Input: "hello",
	})
	req := makeProcessingRequest(body)

	resp := proc.processRequestBody(ctx, req, reqCtx)

	rb := resp.GetRequestBody()
	if rb == nil {
		t.Fatal("expected RequestBody response")
	}

	mutatedBody := rb.Response.GetBodyMutation().GetBody()

	// Body should be a Responses API request
	var apiReq api.ResponsesAPIRequest
	if err := json.Unmarshal(mutatedBody, &apiReq); err != nil {
		t.Fatalf("expected body to be a ResponsesAPIRequest: %v", err)
	}
	if apiReq.Model != "test-model" {
		t.Errorf("expected model=test-model, got %q", apiReq.Model)
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

func TestProcessRequestBody_ValidationError(t *testing.T) {
	proc := newTestProcessor(t)
	ctx := context.Background()
	reqCtx := &requestContext{requestID: "unknown"}

	// Send a request that passes extraction but fails engine.PrepareRequest validation.
	// The Validate() method inside PrepareRequest wraps errors as "invalid request: ...".
	// Missing input triggers a validation error from the engine.
	body, _ := json.Marshal(map[string]interface{}{
		"model": "test-model",
		"input": nil,
	})
	req := makeProcessingRequest(body)

	resp := proc.processRequestBody(ctx, req, reqCtx)

	// Should return an error (either 400 or 422 depending on where the error is caught)
	imm := resp.GetImmediateResponse()
	if imm == nil {
		// If not an immediate response, it should at least not be a successful mutation
		rb := resp.GetRequestBody()
		if rb != nil && reqCtx.stateInjected {
			t.Error("expected validation error, not successful state injection")
		}
		return
	}
	// Validation errors from PrepareRequest should return 400
	if imm.Status.Code != 400 && imm.Status.Code != 422 {
		t.Errorf("expected status code 400 or 422, got %v", imm.Status.Code)
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

func TestProcessResponseBody_ChatCompletionsResponse(t *testing.T) {
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

	// Now process a chat completions backend response (since backend_api=chat_completions)
	content := "Hello there!"
	chatResp := api.ChatCompletionResponse{
		ID:      "chatcmpl-1",
		Object:  "chat.completion",
		Model:   "test-model",
		Created: 1234567890,
		Choices: []api.ChatCompletionChoice{
			{
				Index: 0,
				Message: api.ChatCompletionChoiceMsg{
					Role:    "assistant",
					Content: &content,
				},
				FinishReason: "stop",
			},
		},
		Usage: &api.ChatCompletionUsage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}
	respBody, _ := json.Marshal(chatResp)

	resp := proc.processResponseBody(ctx, &extproc.HttpBody{Body: respBody}, reqCtx)

	rb := resp.GetResponseBody()
	if rb == nil {
		t.Fatal("expected ResponseBody response")
	}
	if rb.Response.BodyMutation == nil {
		t.Fatal("expected body mutation")
	}

	// Parse the modified body — should be a Responses API response
	modifiedBody := rb.Response.GetBodyMutation().GetBody()
	var finalResp schema.Response
	if err := json.Unmarshal(modifiedBody, &finalResp); err != nil {
		t.Fatalf("failed to parse modified response body: %v", err)
	}

	// Verify gateway response ID is used (not the chat completion ID)
	if finalResp.ID != reqCtx.preparedReq.State.ResponseID {
		t.Errorf("expected response ID=%q, got %q", reqCtx.preparedReq.State.ResponseID, finalResp.ID)
	}
	if finalResp.Status != "completed" {
		t.Errorf("expected status=completed, got %q", finalResp.Status)
	}
	// Verify the text content was converted
	if len(finalResp.Output) == 0 {
		t.Fatal("expected non-empty output")
	}
}

func TestProcessResponseBody_ResponsesBackend(t *testing.T) {
	proc := newTestProcessorWithBackend(t, "responses")
	ctx := context.Background()

	// First prepare a request
	reqCtx := &requestContext{requestID: "unknown"}
	reqBody, _ := json.Marshal(schema.ResponseRequest{
		Model: stringPtr("test-model"),
		Input: "hello",
	})
	proc.processRequestBody(ctx, makeProcessingRequest(reqBody), reqCtx)
	if !reqCtx.stateInjected {
		t.Fatal("setup: state not injected")
	}

	// Process a Responses API backend response
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

	modifiedBody := rb.Response.GetBodyMutation().GetBody()
	var finalResp schema.Response
	if err := json.Unmarshal(modifiedBody, &finalResp); err != nil {
		t.Fatalf("failed to parse modified response body: %v", err)
	}

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

	// Backend chat completion response with a different ID
	content := "hi"
	chatResp := api.ChatCompletionResponse{
		ID:      "chatcmpl-should-be-replaced",
		Object:  "chat.completion",
		Model:   "test-model",
		Created: 1234567890,
		Choices: []api.ChatCompletionChoice{
			{
				Index: 0,
				Message: api.ChatCompletionChoiceMsg{
					Role:    "assistant",
					Content: &content,
				},
				FinishReason: "stop",
			},
		},
	}
	respBody, _ := json.Marshal(chatResp)

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
	if finalResp.ID == "chatcmpl-should-be-replaced" {
		t.Error("response ID was not rewritten from backend ID")
	}
	if finalResp.ID != gatewayResponseID {
		t.Errorf("expected gateway response ID=%q, got %q", gatewayResponseID, finalResp.ID)
	}
}
