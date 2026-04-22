// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package extproc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc/metadata"
)

type mockStream struct {
	ctx       context.Context
	requests  []*extprocv3.ProcessingRequest
	responses []*extprocv3.ProcessingResponse
	recvIdx   int
}

func newMockStream(ctx context.Context, requests ...*extprocv3.ProcessingRequest) *mockStream {
	return &mockStream{
		ctx:      ctx,
		requests: requests,
	}
}

func (m *mockStream) Send(resp *extprocv3.ProcessingResponse) error {
	m.responses = append(m.responses, resp)
	return nil
}

func (m *mockStream) Recv() (*extprocv3.ProcessingRequest, error) {
	if m.recvIdx >= len(m.requests) {
		return nil, io.EOF
	}
	req := m.requests[m.recvIdx]
	m.recvIdx++
	return req, nil
}

func (m *mockStream) Context() context.Context     { return m.ctx }
func (m *mockStream) SetHeader(metadata.MD) error  { return nil }
func (m *mockStream) SendHeader(metadata.MD) error { return nil }
func (m *mockStream) SetTrailer(metadata.MD)       {}
func (m *mockStream) SendMsg(any) error            { return nil }
func (m *mockStream) RecvMsg(any) error            { return nil }

func makeHeaders(path, method string, endOfStream bool) *extprocv3.ProcessingRequest {
	return &extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: &extprocv3.HttpHeaders{
				Headers: &corev3.HeaderMap{
					Headers: []*corev3.HeaderValue{
						{Key: ":path", RawValue: []byte(path)},
						{Key: ":method", RawValue: []byte(method)},
						{Key: "content-type", RawValue: []byte("application/json")},
					},
				},
				EndOfStream: endOfStream,
			},
		},
	}
}

func makeBody(body string) *extprocv3.ProcessingRequest {
	return &extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_RequestBody{
			RequestBody: &extprocv3.HttpBody{
				Body:        []byte(body),
				EndOfStream: true,
			},
		},
	}
}

func testHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})
	mux.HandleFunc("POST /v1/responses", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)

		if body["stream"] == true {
			flusher := w.(http.Flusher)
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)

			fmt.Fprintf(w, "event: response.created\ndata: {\"type\":\"response.created\"}\n\n")
			flusher.Flush()
			fmt.Fprintf(w, "event: response.completed\ndata: {\"type\":\"response.completed\"}\n\n")
			flusher.Flush()
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "completed"})
	})
	return mux
}

func TestProcess_GET_ImmediateResponse(t *testing.T) {
	p := NewProcessor(testHandler())
	stream := newMockStream(context.Background(),
		makeHeaders("/health", "GET", true),
	)

	if err := p.Process(stream); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(stream.responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(stream.responses))
	}

	imm := stream.responses[0].GetImmediateResponse()
	if imm == nil {
		t.Fatal("expected ImmediateResponse")
	}
	if imm.Status.Code != 200 {
		t.Fatalf("expected status 200, got %d", imm.Status.Code)
	}
	if !strings.Contains(string(imm.Body), "healthy") {
		t.Fatalf("expected body to contain 'healthy', got %s", string(imm.Body))
	}
}

func TestProcess_POST_NonStreaming_ImmediateResponse(t *testing.T) {
	p := NewProcessor(testHandler())
	stream := newMockStream(context.Background(),
		makeHeaders("/v1/responses", "POST", false),
		makeBody(`{"model":"test","input":"hi"}`),
	)

	if err := p.Process(stream); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First response: body buffered mode override
	if len(stream.responses) < 2 {
		t.Fatalf("expected at least 2 responses, got %d", len(stream.responses))
	}
	if stream.responses[0].ModeOverride == nil {
		t.Fatal("expected first response to have ModeOverride for body buffering")
	}

	// Second response: ImmediateResponse with JSON
	imm := stream.responses[1].GetImmediateResponse()
	if imm == nil {
		t.Fatal("expected ImmediateResponse")
	}
	if imm.Status.Code != 200 {
		t.Fatalf("expected status 200, got %d", imm.Status.Code)
	}
	if !strings.Contains(string(imm.Body), "completed") {
		t.Fatalf("expected body to contain 'completed', got %s", string(imm.Body))
	}
}

func TestProcess_POST_Streaming_StreamedImmediateResponse(t *testing.T) {
	p := NewProcessor(testHandler())
	stream := newMockStream(context.Background(),
		makeHeaders("/v1/responses", "POST", false),
		makeBody(`{"model":"test","input":"hi","stream":true}`),
	)

	if err := p.Process(stream); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect: body buffered + streamed headers + body chunks + end_of_stream
	if len(stream.responses) < 4 {
		t.Fatalf("expected at least 4 responses, got %d", len(stream.responses))
	}

	// Response 0: body buffered
	if stream.responses[0].ModeOverride == nil {
		t.Fatal("expected ModeOverride for body buffering")
	}

	// Response 1: StreamedImmediateResponse with headers
	sir := stream.responses[1].GetStreamedImmediateResponse()
	if sir == nil {
		t.Fatal("expected StreamedImmediateResponse for headers")
	}
	if sir.GetHeadersResponse() == nil {
		t.Fatal("expected headers_response in first StreamedImmediateResponse")
	}

	// Middle responses: body chunks with SSE data
	hasSSEData := false
	for i := 2; i < len(stream.responses); i++ {
		sir := stream.responses[i].GetStreamedImmediateResponse()
		if sir == nil {
			continue
		}
		br := sir.GetBodyResponse()
		if br != nil && len(br.Body) > 0 {
			hasSSEData = true
			if !strings.Contains(string(br.Body), "event:") {
				t.Fatalf("expected SSE event data, got %s", string(br.Body))
			}
		}
	}
	if !hasSSEData {
		t.Fatal("expected at least one SSE body chunk")
	}

	// Last response: end_of_stream
	last := stream.responses[len(stream.responses)-1].GetStreamedImmediateResponse()
	if last == nil {
		t.Fatal("expected final StreamedImmediateResponse")
	}
	lastBody := last.GetBodyResponse()
	if lastBody == nil || !lastBody.EndOfStream {
		t.Fatal("expected end_of_stream in final response")
	}
}

func TestProcess_404_ImmediateResponse(t *testing.T) {
	p := NewProcessor(testHandler())
	stream := newMockStream(context.Background(),
		makeHeaders("/nonexistent", "GET", true),
	)

	if err := p.Process(stream); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(stream.responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(stream.responses))
	}

	imm := stream.responses[0].GetImmediateResponse()
	if imm == nil {
		t.Fatal("expected ImmediateResponse")
	}
	if imm.Status.Code != 404 {
		t.Fatalf("expected status 404, got %d", imm.Status.Code)
	}
}

func TestProcess_HeadersPreserved(t *testing.T) {
	p := NewProcessor(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	stream := newMockStream(context.Background(),
		&extprocv3.ProcessingRequest{
			Request: &extprocv3.ProcessingRequest_RequestHeaders{
				RequestHeaders: &extprocv3.HttpHeaders{
					Headers: &corev3.HeaderMap{
						Headers: []*corev3.HeaderValue{
							{Key: ":path", RawValue: []byte("/test")},
							{Key: ":method", RawValue: []byte("GET")},
							{Key: "authorization", RawValue: []byte("Bearer tok123")},
						},
					},
					EndOfStream: true,
				},
			},
		},
	)

	if err := p.Process(stream); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	imm := stream.responses[0].GetImmediateResponse()
	if imm == nil {
		t.Fatal("expected ImmediateResponse")
	}
	if imm.Status.Code != 200 {
		t.Fatalf("expected 200 (auth passed), got %d", imm.Status.Code)
	}
}
