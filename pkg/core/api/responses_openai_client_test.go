// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCreateResponse_Success(t *testing.T) {
	want := ResponsesAPIResponse{
		ID:        "resp_abc123",
		Object:    "response",
		Status:    "completed",
		Model:     "test-model",
		CreatedAt: 1234567890,
		Output: []OutputItem{
			{
				Type: "message",
				ID:   "msg_001",
				Role: "assistant",
				Content: []ContentItem{
					{Type: "output_text", Text: "Hello!"},
				},
			},
		},
		Usage: &UsageInfo{InputTokens: 5, OutputTokens: 3, TotalTokens: 8},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/responses" {
			t.Errorf("expected /v1/responses, got %s", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("expected Authorization Bearer test-key, got %s", auth)
		}

		// Verify request body
		var req ResponsesAPIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Model != "test-model" {
			t.Errorf("expected model test-model, got %s", req.Model)
		}
		if req.Stream {
			t.Error("expected stream=false for non-streaming request")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	client := NewOpenAIResponsesClient(srv.URL+"/v1", "test-key")
	got, err := client.CreateResponse(context.Background(), &ResponsesAPIRequest{
		Model: "test-model",
		Input: "Hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.Status != want.Status {
		t.Errorf("Status = %q, want %q", got.Status, want.Status)
	}
	if len(got.Output) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(got.Output))
	}
	if got.Output[0].Content[0].Text != "Hello!" {
		t.Errorf("output text = %q, want %q", got.Output[0].Content[0].Text, "Hello!")
	}
	if got.Usage.TotalTokens != 8 {
		t.Errorf("total_tokens = %d, want 8", got.Usage.TotalTokens)
	}
}

func TestCreateResponse_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"type":"invalid_request","message":"bad model"}}`)
	}))
	defer srv.Close()

	client := NewOpenAIResponsesClient(srv.URL+"/v1", "test-key")
	_, err := client.CreateResponse(context.Background(), &ResponsesAPIRequest{
		Model: "bad-model",
		Input: "Hello",
	})
	if err == nil {
		t.Fatal("expected error for 400 status")
	}
	if want := "backend returned status 400"; !contains(err.Error(), want) {
		t.Errorf("error = %q, want to contain %q", err.Error(), want)
	}
}

func TestCreateResponse_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `internal error`)
	}))
	defer srv.Close()

	client := NewOpenAIResponsesClient(srv.URL+"/v1", "test-key")
	_, err := client.CreateResponse(context.Background(), &ResponsesAPIRequest{
		Model: "model",
		Input: "Hello",
	})
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
	if want := "backend returned status 500"; !contains(err.Error(), want) {
		t.Errorf("error = %q, want to contain %q", err.Error(), want)
	}
}

func TestCreateResponse_NoAuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("expected no Authorization header, got %s", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ResponsesAPIResponse{ID: "resp_1", Status: "completed"})
	}))
	defer srv.Close()

	client := NewOpenAIResponsesClient(srv.URL+"/v1", "")
	_, err := client.CreateResponse(context.Background(), &ResponsesAPIRequest{
		Model: "model",
		Input: "Hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateResponseStream_Success(t *testing.T) {
	sseBody := "event: response.created\n" +
		`data: {"type":"response.created","response":{"id":"resp_1"}}` + "\n\n" +
		"event: response.output_text.delta\n" +
		`data: {"type":"response.output_text.delta","delta":"Hello"}` + "\n\n" +
		"event: response.output_text.delta\n" +
		`data: {"type":"response.output_text.delta","delta":" world"}` + "\n\n" +
		"event: response.completed\n" +
		`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed"}}` + "\n\n" +
		"data: [DONE]\n\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if accept := r.Header.Get("Accept"); accept != "text/event-stream" {
			t.Errorf("expected Accept text/event-stream, got %s", accept)
		}

		// Verify store=false is set
		var req ResponsesAPIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Store == nil || *req.Store != false {
			t.Error("expected store=false in streaming request")
		}
		if !req.Stream {
			t.Error("expected stream=true")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	client := NewOpenAIResponsesClient(srv.URL+"/v1", "test-key")
	events, err := client.CreateResponseStream(context.Background(), &ResponsesAPIRequest{
		Model: "test-model",
		Input: "Hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var collected []ResponsesStreamEvent
	for evt := range events {
		collected = append(collected, evt)
	}

	if len(collected) != 4 {
		t.Fatalf("expected 4 events, got %d", len(collected))
	}

	expectedTypes := []string{
		"response.created",
		"response.output_text.delta",
		"response.output_text.delta",
		"response.completed",
	}
	for i, wantType := range expectedTypes {
		if collected[i].Type != wantType {
			t.Errorf("event[%d].Type = %q, want %q", i, collected[i].Type, wantType)
		}
	}

	// Verify raw JSON data is preserved
	var delta struct {
		Delta string `json:"delta"`
	}
	if err := json.Unmarshal(collected[1].Data, &delta); err != nil {
		t.Fatalf("failed to parse delta event data: %v", err)
	}
	if delta.Delta != "Hello" {
		t.Errorf("delta = %q, want %q", delta.Delta, "Hello")
	}
}

func TestCreateResponseStream_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"bad request"}`)
	}))
	defer srv.Close()

	client := NewOpenAIResponsesClient(srv.URL+"/v1", "test-key")
	_, err := client.CreateResponseStream(context.Background(), &ResponsesAPIRequest{
		Model: "model",
		Input: "Hello",
	})
	if err == nil {
		t.Fatal("expected error for 400 status")
	}
}

func TestCreateResponseStream_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)

		// Send first event
		fmt.Fprint(w, "event: response.created\ndata: {\"type\":\"response.created\"}\n\n")
		flusher.Flush()

		// Wait for context cancellation (simulates slow stream)
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	client := NewOpenAIResponsesClient(srv.URL+"/v1", "test-key")
	events, err := client.CreateResponseStream(ctx, &ResponsesAPIRequest{
		Model: "model",
		Input: "Hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read first event
	select {
	case evt := <-events:
		if evt.Type != "response.created" {
			t.Errorf("Type = %q, want response.created", evt.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first event")
	}

	// Cancel context
	cancel()

	// Channel should close
	select {
	case _, ok := <-events:
		if ok {
			// Might get one more event through before close; drain
			for range events {
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for channel close after cancellation")
	}
}

func TestCreateResponse_TrailingSlashTrimmed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("expected /v1/responses, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ResponsesAPIResponse{ID: "resp_1", Status: "completed"})
	}))
	defer srv.Close()

	// baseURL with trailing slash
	client := NewOpenAIResponsesClient(srv.URL+"/v1/", "key")
	_, err := client.CreateResponse(context.Background(), &ResponsesAPIRequest{
		Model: "model",
		Input: "Hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateResponse_FunctionCallOutput(t *testing.T) {
	want := ResponsesAPIResponse{
		ID:     "resp_fc",
		Object: "response",
		Status: "completed",
		Model:  "test-model",
		Output: []OutputItem{
			{
				Type:      "function_call",
				ID:        "fc_1",
				Name:      "get_weather",
				Arguments: `{"location":"NYC"}`,
				CallID:    "call_abc",
				Status:    "completed",
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	client := NewOpenAIResponsesClient(srv.URL+"/v1", "key")
	got, err := client.CreateResponse(context.Background(), &ResponsesAPIRequest{
		Model: "test-model",
		Input: "What's the weather?",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got.Output) != 1 {
		t.Fatalf("expected 1 output, got %d", len(got.Output))
	}
	out := got.Output[0]
	if out.Type != "function_call" {
		t.Errorf("Type = %q, want function_call", out.Type)
	}
	if out.Name != "get_weather" {
		t.Errorf("Name = %q, want get_weather", out.Name)
	}
	if out.CallID != "call_abc" {
		t.Errorf("CallID = %q, want call_abc", out.CallID)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
