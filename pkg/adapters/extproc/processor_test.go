// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package extproc

import (
	"context"
	"io"
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

func (m *mockStream) Context() context.Context    { return m.ctx }
func (m *mockStream) SetHeader(metadata.MD) error  { return nil }
func (m *mockStream) SendHeader(metadata.MD) error { return nil }
func (m *mockStream) SetTrailer(metadata.MD)       {}
func (m *mockStream) SendMsg(any) error            { return nil }
func (m *mockStream) RecvMsg(any) error            { return nil }

func makeRequestHeaders(path, method string) *extprocv3.ProcessingRequest {
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
				EndOfStream: false,
			},
		},
	}
}

func makeRequestBody(body string) *extprocv3.ProcessingRequest {
	return &extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_RequestBody{
			RequestBody: &extprocv3.HttpBody{
				Body:        []byte(body),
				EndOfStream: true,
			},
		},
	}
}

func TestProcess_NonResponsesPath_Passthrough(t *testing.T) {
	p := NewProcessor(nil, nil)
	stream := newMockStream(context.Background(),
		makeRequestHeaders("/v1/models", "GET"),
	)

	if err := p.Process(stream); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(stream.responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(stream.responses))
	}
	if stream.responses[0].GetRequestHeaders() == nil {
		t.Fatal("expected passthrough (RequestHeaders) response")
	}
}

func TestProcess_NonPOST_Passthrough(t *testing.T) {
	p := NewProcessor(nil, nil)
	stream := newMockStream(context.Background(),
		makeRequestHeaders("/v1/responses", "GET"),
	)

	if err := p.Process(stream); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(stream.responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(stream.responses))
	}
	if stream.responses[0].GetRequestHeaders() == nil {
		t.Fatal("expected passthrough (RequestHeaders) response")
	}
}

func TestProcess_InvalidJSON_Returns400(t *testing.T) {
	p := NewProcessor(nil, nil)
	stream := newMockStream(context.Background(),
		makeRequestHeaders("/v1/responses", "POST"),
		makeRequestBody("not json"),
	)

	if err := p.Process(stream); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(stream.responses) < 2 {
		t.Fatalf("expected at least 2 responses, got %d", len(stream.responses))
	}

	// First response: requestBodyBuffered mode override
	if stream.responses[0].GetRequestHeaders() == nil {
		t.Fatal("expected first response to be RequestHeaders (body buffered)")
	}

	// Second response: error
	imm := stream.responses[1].GetImmediateResponse()
	if imm == nil {
		t.Fatal("expected ImmediateResponse for invalid JSON")
	}
	if imm.Status.Code != 400 {
		t.Fatalf("expected status 400, got %d", imm.Status.Code)
	}
}

func TestProcess_MissingModel_Returns400(t *testing.T) {
	p := NewProcessor(nil, nil)
	stream := newMockStream(context.Background(),
		makeRequestHeaders("/v1/responses", "POST"),
		makeRequestBody(`{"input": "hello"}`),
	)

	if err := p.Process(stream); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(stream.responses) < 2 {
		t.Fatalf("expected at least 2 responses, got %d", len(stream.responses))
	}

	imm := stream.responses[1].GetImmediateResponse()
	if imm == nil {
		t.Fatal("expected ImmediateResponse for validation error")
	}
	if imm.Status.Code != 400 {
		t.Fatalf("expected status 400, got %d", imm.Status.Code)
	}
}

func TestProcess_QueryStringStripped(t *testing.T) {
	p := NewProcessor(nil, nil)
	stream := newMockStream(context.Background(),
		makeRequestHeaders("/v1/responses?foo=bar", "POST"),
		makeRequestBody(`{"input": "hello"}`),
	)

	if err := p.Process(stream); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have matched /v1/responses (query stripped) and requested body
	if len(stream.responses) < 1 {
		t.Fatal("expected at least 1 response")
	}
	// First response should be body buffered (not passthrough)
	if stream.responses[0].ModeOverride == nil {
		t.Fatal("expected ModeOverride for body buffering on /v1/responses?foo=bar")
	}
}
