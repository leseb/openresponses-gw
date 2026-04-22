# ExtProc StreamedImmediateResponse Adapter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an Envoy ExtProc adapter that uses StreamedImmediateResponse (Envoy v1.37.0) to serve the Responses API through Envoy's filter chain, supporting both streaming (SSE) and non-streaming responses.

**Architecture:** The adapter implements Envoy's `ExternalProcessor` gRPC service. When it receives `request_headers` for a Responses API path, it reads the buffered request body, parses the JSON into a `schema.ResponseRequest`, and delegates to the existing `engine.Engine`. For streaming requests, it uses `StreamedImmediateResponse` to send headers first, then body chunks for each SSE event, and finally signals end-of-stream. For non-streaming requests, it uses `ImmediateResponse` to send the complete JSON response. All other paths are passed through to the upstream unchanged.

**Tech Stack:** `github.com/envoyproxy/go-control-plane/envoy v1.37.0` (ExtProc proto types), `google.golang.org/grpc` (gRPC server)

---

## File Structure

| File | Responsibility |
|------|----------------|
| `pkg/adapters/extproc/server.go` | gRPC server lifecycle: create, start, stop, register with grpc.Server |
| `pkg/adapters/extproc/processor.go` | `ExternalProcessorServer` implementation: Process() method, request header/body dispatch, routing logic |
| `pkg/adapters/extproc/response.go` | Response helpers: build ImmediateResponse, build StreamedImmediateResponse headers/body/end, header construction |
| `pkg/adapters/extproc/processor_test.go` | Unit tests for the processor using a mock gRPC stream |
| `pkg/core/config/config.go` | Add `ExtProc` config struct |
| `cmd/server/main.go` | Start gRPC server alongside HTTP when ExtProc is enabled |

---

### Task 1: Add go-control-plane dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Add the dependency**

```bash
cd /Users/leseb/conductor/workspaces/openresponses-gw/curitiba
go get github.com/envoyproxy/go-control-plane/envoy@v1.37.0
go mod tidy
```

- [ ] **Step 2: Verify the dependency is present**

```bash
grep "go-control-plane/envoy" go.mod
```

Expected: `github.com/envoyproxy/go-control-plane/envoy v1.37.0`

- [ ] **Step 3: Verify build still works**

```bash
make build
```

Expected: successful build

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "feat: add envoy go-control-plane v1.37.0 dependency for ExtProc adapter"
```

---

### Task 2: Add ExtProc configuration

**Files:**
- Modify: `pkg/core/config/config.go`

- [ ] **Step 1: Add ExtProcConfig struct and wire it into Config**

Add this to `pkg/core/config/config.go`:

```go
// ExtProcConfig contains ExtProc gRPC server configuration
type ExtProcConfig struct {
	Enabled bool   `yaml:"enabled"`
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
}
```

Add to the `Config` struct:

```go
ExtProc ExtProcConfig `yaml:"extproc"`
```

Add env var overrides in `Load()`:

```go
// ExtProc env overrides
if v := os.Getenv("EXTPROC_ENABLED"); v == "true" {
	cfg.ExtProc.Enabled = true
}
if v := os.Getenv("EXTPROC_PORT"); v != "" {
	if p, err := strconv.Atoi(v); err == nil {
		cfg.ExtProc.Port = p
	}
}
```

Add defaults function:

```go
func applyExtProcDefaults(cfg *ExtProcConfig) {
	if cfg.Port == 0 {
		cfg.Port = 50051
	}
	if cfg.Host == "" {
		cfg.Host = "0.0.0.0"
	}
}
```

Call `applyExtProcDefaults(&cfg.ExtProc)` in `Load()` and set defaults in `Default()`.

Add `"strconv"` to imports in config.go.

- [ ] **Step 2: Verify build**

```bash
make build
```

Expected: successful build

- [ ] **Step 3: Commit**

```bash
git add pkg/core/config/config.go
git commit -m "feat: add ExtProc configuration with env var overrides"
```

---

### Task 3: Implement response helpers

**Files:**
- Create: `pkg/adapters/extproc/response.go`

These helper functions build the Envoy ExtProc proto messages. They are stateless and easy to test in isolation.

- [ ] **Step 1: Create response.go with helper functions**

```go
// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package extproc

import (
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
)

// makeHeader creates a HeaderValueOption for use in HeaderMutation.SetHeaders.
func makeHeader(key, value string) *corev3.HeaderValueOption {
	return &corev3.HeaderValueOption{
		Header: &corev3.HeaderValue{
			Key:      key,
			RawValue: []byte(value),
		},
	}
}

// immediateResponseMsg builds a ProcessingResponse containing an ImmediateResponse.
// Used for non-streaming responses and errors.
func immediateResponseMsg(statusCode int, headers map[string]string, body []byte) *extprocv3.ProcessingResponse {
	hdrs := make([]*corev3.HeaderValueOption, 0, len(headers))
	for k, v := range headers {
		hdrs = append(hdrs, makeHeader(k, v))
	}
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_ImmediateResponse{
			ImmediateResponse: &extprocv3.ImmediateResponse{
				Status: &typev3.HttpStatus{
					Code: typev3.StatusCode(statusCode),
				},
				Headers: &extprocv3.HeaderMutation{
					SetHeaders: hdrs,
				},
				Body: body,
			},
		},
	}
}

// streamHeadersMsg builds the first StreamedImmediateResponse message
// containing the response headers. This initiates the streamed response.
func streamHeadersMsg(statusCode int, headers map[string]string) *extprocv3.ProcessingResponse {
	hdrs := make([]*corev3.HeaderValueOption, 0, len(headers)+1)
	hdrs = append(hdrs, makeHeader(":status", fmt.Sprintf("%d", statusCode)))
	for k, v := range headers {
		hdrs = append(hdrs, makeHeader(k, v))
	}
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_StreamedImmediateResponse{
			StreamedImmediateResponse: &extprocv3.StreamedImmediateResponse{
				Response: &extprocv3.StreamedImmediateResponse_HeadersResponse{
					HeadersResponse: &extprocv3.HttpHeaders{
						Headers: &corev3.HeaderMap{
							Headers: toHeaderValues(hdrs),
						},
					},
				},
			},
		},
	}
}

// streamBodyMsg builds a StreamedImmediateResponse containing a body chunk.
func streamBodyMsg(data []byte, endOfStream bool) *extprocv3.ProcessingResponse {
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_StreamedImmediateResponse{
			StreamedImmediateResponse: &extprocv3.StreamedImmediateResponse{
				Response: &extprocv3.StreamedImmediateResponse_BodyResponse{
					BodyResponse: &extprocv3.StreamedBodyResponse{
						Body:        data,
						EndOfStream: endOfStream,
					},
				},
			},
		},
	}
}

// toHeaderValues extracts HeaderValue slice from HeaderValueOption slice,
// used to populate HeaderMap.Headers.
func toHeaderValues(opts []*corev3.HeaderValueOption) []*corev3.HeaderValue {
	vals := make([]*corev3.HeaderValue, len(opts))
	for i, o := range opts {
		vals[i] = o.Header
	}
	return vals
}

// errorResponse builds an ImmediateResponse with a JSON error body.
func errorResponse(statusCode int, errType, message string) *extprocv3.ProcessingResponse {
	body, _ := json.Marshal(map[string]interface{}{
		"error": map[string]string{
			"type":    errType,
			"message": message,
		},
	})
	return immediateResponseMsg(statusCode, map[string]string{
		"content-type": "application/json",
	}, body)
}

// passthroughResponse tells Envoy to continue processing the request
// through its normal filter chain (forward to upstream).
func passthroughResponse() *extprocv3.ProcessingResponse {
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_RequestHeaders{
			RequestHeaders: &extprocv3.HeadersResponse{},
		},
	}
}
```

Add `"encoding/json"` and `"fmt"` to the imports.

- [ ] **Step 2: Verify build**

```bash
cd /Users/leseb/conductor/workspaces/openresponses-gw/curitiba && go build ./pkg/adapters/extproc/...
```

Expected: successful build

- [ ] **Step 3: Commit**

```bash
git add pkg/adapters/extproc/response.go
git commit -m "feat: add ExtProc response helper functions"
```

---

### Task 4: Implement the ExtProc processor

**Files:**
- Create: `pkg/adapters/extproc/processor.go`

This is the core file implementing the `ExternalProcessorServer` interface. The `Process()` method handles the bidirectional gRPC stream.

**Flow:**
1. Receive `request_headers` from Envoy
2. Extract `:path`, `:method`, `content-type` from headers
3. If not a POST to a responses path, send passthrough response
4. Request body mode override to BUFFERED so we get the full body
5. Receive `request_body` from Envoy
6. Parse JSON into `schema.ResponseRequest`
7. If `req.Stream`: use engine.ProcessRequestStream() and send events via StreamedImmediateResponse
8. If non-streaming: use engine.ProcessRequest() and send via ImmediateResponse

- [ ] **Step 1: Create processor.go**

```go
// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package extproc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	filterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"

	"github.com/leseb/openresponses-gw/pkg/core/engine"
	"github.com/leseb/openresponses-gw/pkg/core/schema"
	"github.com/leseb/openresponses-gw/pkg/observability/logging"
)

// responsePaths lists the URL paths this ExtProc handles.
var responsePaths = map[string]bool{
	"/responses":    true,
	"/v1/responses": true,
}

// Processor implements the Envoy ExternalProcessorServer interface.
type Processor struct {
	extprocv3.UnimplementedExternalProcessorServer
	engine *engine.Engine
	logger *logging.Logger
}

// NewProcessor creates a new ExtProc processor.
func NewProcessor(eng *engine.Engine, logger *logging.Logger) *Processor {
	return &Processor{
		engine: eng,
		logger: logger,
	}
}

// Process handles the bidirectional gRPC stream from Envoy.
func (p *Processor) Process(stream extprocv3.ExternalProcessor_ProcessServer) error {
	// State for this request
	var (
		path          string
		method        string
		isResponsesAPI bool
	)

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		switch v := req.Request.(type) {
		case *extprocv3.ProcessingRequest_RequestHeaders:
			path, method = extractPathAndMethod(v.RequestHeaders)
			isResponsesAPI = method == "POST" && responsePaths[path]

			if !isResponsesAPI {
				if err := stream.Send(passthroughResponse()); err != nil {
					return fmt.Errorf("sending passthrough: %w", err)
				}
				continue
			}

			// Request the full body so we can parse the JSON
			if err := stream.Send(requestBodyBuffered()); err != nil {
				return fmt.Errorf("requesting body: %w", err)
			}

		case *extprocv3.ProcessingRequest_RequestBody:
			if !isResponsesAPI {
				if err := stream.Send(passthroughResponse()); err != nil {
					return fmt.Errorf("sending passthrough: %w", err)
				}
				continue
			}

			body := v.RequestBody.GetBody()
			if err := p.handleResponsesRequest(stream, body); err != nil {
				p.logger.Error("Failed to handle responses request", "error", err)
				if sendErr := stream.Send(errorResponse(500, "processing_error", err.Error())); sendErr != nil {
					return fmt.Errorf("sending error response: %w", sendErr)
				}
			}

		default:
			// For any other message type (response_headers, response_body, trailers),
			// send a passthrough response to continue normal processing.
			if err := stream.Send(passthroughResponse()); err != nil {
				return fmt.Errorf("sending passthrough: %w", err)
			}
		}
	}
}

// handleResponsesRequest parses the request body and delegates to the engine.
func (p *Processor) handleResponsesRequest(stream extprocv3.ExternalProcessor_ProcessServer, body []byte) error {
	var req schema.ResponseRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return stream.Send(errorResponse(400, "invalid_request", "Failed to parse request body"))
	}

	if err := req.Validate(); err != nil {
		return stream.Send(errorResponse(400, "invalid_request", err.Error()))
	}

	p.logger.Info("ExtProc processing response request",
		"model", req.Model,
		"stream", req.Stream)

	ctx := stream.Context()

	if req.Stream {
		return p.handleStreaming(ctx, stream, &req)
	}
	return p.handleNonStreaming(ctx, stream, &req)
}

// handleNonStreaming processes a non-streaming request and sends an ImmediateResponse.
func (p *Processor) handleNonStreaming(ctx context.Context, stream extprocv3.ExternalProcessor_ProcessServer, req *schema.ResponseRequest) error {
	resp, err := p.engine.ProcessRequest(ctx, req)
	if err != nil {
		return stream.Send(errorResponse(500, "processing_error", err.Error()))
	}

	respJSON, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshaling response: %w", err)
	}

	return stream.Send(immediateResponseMsg(200, map[string]string{
		"content-type": "application/json",
	}, respJSON))
}

// handleStreaming processes a streaming request using StreamedImmediateResponse.
func (p *Processor) handleStreaming(ctx context.Context, stream extprocv3.ExternalProcessor_ProcessServer, req *schema.ResponseRequest) error {
	events, err := p.engine.ProcessRequestStream(ctx, req)
	if err != nil {
		return stream.Send(errorResponse(500, "processing_error", err.Error()))
	}

	// Send headers first
	if err := stream.Send(streamHeadersMsg(200, map[string]string{
		"content-type":  "text/event-stream",
		"cache-control": "no-cache",
		"connection":    "keep-alive",
	})); err != nil {
		return fmt.Errorf("sending stream headers: %w", err)
	}

	// Stream each SSE event as a body chunk
	for event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			p.logger.Error("Failed to marshal event", "error", err)
			continue
		}

		eventType := extractEventType(event)
		sseData := fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)

		if err := stream.Send(streamBodyMsg([]byte(sseData), false)); err != nil {
			return fmt.Errorf("sending SSE event: %w", err)
		}
	}

	// Signal end of stream
	if err := stream.Send(streamBodyMsg(nil, true)); err != nil {
		return fmt.Errorf("sending end of stream: %w", err)
	}

	p.logger.Info("ExtProc streaming completed")
	return nil
}

// extractPathAndMethod extracts :path and :method pseudo-headers from request headers.
func extractPathAndMethod(headers *extprocv3.HttpHeaders) (path, method string) {
	if headers == nil || headers.Headers == nil {
		return "", ""
	}
	for _, h := range headers.Headers.Headers {
		switch h.Key {
		case ":path":
			path = string(h.RawValue)
			if path == "" {
				path = h.Value
			}
			// Strip query string
			if idx := strings.IndexByte(path, '?'); idx >= 0 {
				path = path[:idx]
			}
		case ":method":
			method = string(h.RawValue)
			if method == "" {
				method = h.Value
			}
		}
	}
	return path, method
}

// requestBodyBuffered tells Envoy to buffer the full request body and send it.
func requestBodyBuffered() *extprocv3.ProcessingResponse {
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_RequestHeaders{
			RequestHeaders: &extprocv3.HeadersResponse{},
		},
		ModeOverride: &filterv3.ProcessingMode{
			RequestBodyMode: filterv3.ProcessingMode_BUFFERED,
		},
	}
}
```

Note: The `extractEventType` function is reused from the HTTP adapter. We import it or duplicate it — since the HTTP adapter's version is in package `http`, we duplicate it in the extproc package to avoid a cross-adapter dependency. Copy the same type switch from `pkg/adapters/http/handler.go:extractEventType`.

- [ ] **Step 2: Copy extractEventType into processor.go**

Add the `extractEventType` function (identical to the one in `pkg/adapters/http/handler.go` lines 431-498) at the bottom of `processor.go`. It uses the same type switch over all `schema.*StreamingEvent` types.

- [ ] **Step 3: Verify build**

```bash
go build ./pkg/adapters/extproc/...
```

Expected: successful build

- [ ] **Step 4: Commit**

```bash
git add pkg/adapters/extproc/processor.go
git commit -m "feat: implement ExtProc processor with StreamedImmediateResponse"
```

---

### Task 5: Implement the gRPC server

**Files:**
- Create: `pkg/adapters/extproc/server.go`

- [ ] **Step 1: Create server.go**

```go
// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package extproc

import (
	"fmt"
	"net"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/leseb/openresponses-gw/pkg/core/engine"
	"github.com/leseb/openresponses-gw/pkg/observability/logging"
)

// Server wraps the gRPC server for the ExtProc service.
type Server struct {
	grpcServer *grpc.Server
	listener   net.Listener
	logger     *logging.Logger
}

// NewServer creates a new ExtProc gRPC server.
func NewServer(eng *engine.Engine, logger *logging.Logger) *Server {
	gs := grpc.NewServer()
	processor := NewProcessor(eng, logger)
	extprocv3.RegisterExternalProcessorServer(gs, processor)

	// Register health service for Envoy health checking
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(gs, healthSrv)
	healthSrv.SetServingStatus("envoy.service.ext_proc.v3.ExternalProcessor", healthpb.HealthCheckResponse_SERVING)

	return &Server{
		grpcServer: gs,
		logger:     logger,
	}
}

// Start begins listening on the given address. Blocks until stopped.
func (s *Server) Start(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	s.listener = lis
	s.logger.Info("ExtProc gRPC server listening", "address", addr)
	return s.grpcServer.Serve(lis)
}

// Stop gracefully stops the gRPC server.
func (s *Server) Stop() {
	s.logger.Info("Stopping ExtProc gRPC server")
	s.grpcServer.GracefulStop()
}
```

- [ ] **Step 2: Verify build**

```bash
go build ./pkg/adapters/extproc/...
```

Expected: successful build

- [ ] **Step 3: Commit**

```bash
git add pkg/adapters/extproc/server.go
git commit -m "feat: add ExtProc gRPC server with health checking"
```

---

### Task 6: Wire ExtProc server into main

**Files:**
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Add ExtProc server startup to main.go**

Add the import:
```go
extprocAdapter "github.com/leseb/openresponses-gw/pkg/adapters/extproc"
```

After the HTTP adapter initialization (after line ~209), add:

```go
// Initialize ExtProc adapter (optional)
var extprocServer *extprocAdapter.Server
if cfg.ExtProc.Enabled {
	extprocServer = extprocAdapter.NewServer(eng, logger)
	logger.Info("Initialized ExtProc adapter")
}
```

After the HTTP server goroutine (after line ~233), add:

```go
// Start ExtProc gRPC server (if enabled)
if extprocServer != nil {
	grpcAddr := fmt.Sprintf("%s:%d", cfg.ExtProc.Host, cfg.ExtProc.Port)
	go func() {
		if err := extprocServer.Start(grpcAddr); err != nil {
			logger.Error("ExtProc gRPC server error", "error", err)
			os.Exit(1)
		}
	}()
}
```

In the shutdown section (before `logger.Info("Server stopped gracefully")`), add:

```go
if extprocServer != nil {
	extprocServer.Stop()
}
```

- [ ] **Step 2: Verify build**

```bash
make build
```

Expected: successful build

- [ ] **Step 3: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat: wire ExtProc gRPC server into main startup"
```

---

### Task 7: Unit tests for the processor

**Files:**
- Create: `pkg/adapters/extproc/processor_test.go`

Tests use a mock `ExternalProcessor_ProcessServer` to verify the processor handles requests correctly without a real gRPC connection or Envoy.

- [ ] **Step 1: Create processor_test.go**

Test cases:
1. **Non-responses path passes through**: Send request_headers with `:path: /v1/models`, verify passthrough response
2. **Non-POST method passes through**: Send request_headers with `:method: GET, :path: /v1/responses`, verify passthrough
3. **Non-streaming request**: Send request_headers + request_body with valid JSON `{stream: false}`, verify ImmediateResponse with JSON body
4. **Streaming request**: Send request_headers + request_body with `{stream: true}`, verify StreamedImmediateResponse sequence: headers → body chunks → end_of_stream
5. **Invalid JSON returns 400**: Send request_body with invalid JSON, verify error ImmediateResponse
6. **Validation error returns 400**: Send request_body with missing `model` field

The mock stream needs to implement `ExternalProcessor_ProcessServer` with:
- A queue of `ProcessingRequest` messages to return from `Recv()`
- A slice collecting `ProcessingResponse` messages from `Send()`
- A `context.Context` from `Context()`

```go
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

// mockStream implements ExternalProcessor_ProcessServer for testing.
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

func (m *mockStream) Context() context.Context         { return m.ctx }
func (m *mockStream) SetHeader(metadata.MD) error      { return nil }
func (m *mockStream) SendHeader(metadata.MD) error      { return nil }
func (m *mockStream) SetTrailer(metadata.MD)            {}
func (m *mockStream) SendMsg(interface{}) error         { return nil }
func (m *mockStream) RecvMsg(interface{}) error         { return nil }

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

	err := p.Process(stream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(stream.responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(stream.responses))
	}

	resp := stream.responses[0]
	if resp.GetRequestHeaders() == nil {
		t.Fatal("expected passthrough (RequestHeaders) response")
	}
}

func TestProcess_NonPOST_Passthrough(t *testing.T) {
	p := NewProcessor(nil, nil)
	stream := newMockStream(context.Background(),
		makeRequestHeaders("/v1/responses", "GET"),
	)

	err := p.Process(stream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(stream.responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(stream.responses))
	}

	resp := stream.responses[0]
	if resp.GetRequestHeaders() == nil {
		t.Fatal("expected passthrough (RequestHeaders) response")
	}
}

func TestProcess_InvalidJSON_Returns400(t *testing.T) {
	p := NewProcessor(nil, nil)
	stream := newMockStream(context.Background(),
		makeRequestHeaders("/v1/responses", "POST"),
		makeRequestBody("not json"),
	)

	err := p.Process(stream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First response: requestBodyBuffered mode override
	// Second response: error
	if len(stream.responses) < 2 {
		t.Fatalf("expected at least 2 responses, got %d", len(stream.responses))
	}

	errResp := stream.responses[1]
	imm := errResp.GetImmediateResponse()
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

	err := p.Process(stream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(stream.responses) < 2 {
		t.Fatalf("expected at least 2 responses, got %d", len(stream.responses))
	}

	errResp := stream.responses[1]
	imm := errResp.GetImmediateResponse()
	if imm == nil {
		t.Fatal("expected ImmediateResponse for validation error")
	}
	if imm.Status.Code != 400 {
		t.Fatalf("expected status 400, got %d", imm.Status.Code)
	}
}
```

Note: Tests for non-streaming and streaming with a real engine require setting up an engine with a mock backend, which would be integration-level. The tests above verify routing, parsing, and error handling without engine dependencies.

- [ ] **Step 2: Run tests**

```bash
go test ./pkg/adapters/extproc/... -v -run TestProcess
```

Expected: all tests pass

- [ ] **Step 3: Commit**

```bash
git add pkg/adapters/extproc/processor_test.go
git commit -m "test: add ExtProc processor unit tests"
```

---

### Task 8: Run all checks and verify

**Files:** (none modified)

- [ ] **Step 1: Run full test suite**

```bash
make test
```

Expected: all existing tests pass

- [ ] **Step 2: Run linter**

```bash
make lint
```

Expected: no lint errors

- [ ] **Step 3: Run pre-commit**

```bash
pre-commit run --all-files
```

Expected: all hooks pass

- [ ] **Step 4: Verify the binary starts with ExtProc enabled**

```bash
EXTPROC_ENABLED=true OPENAI_API_ENDPOINT=http://localhost:8000 make run &
sleep 2
# Check gRPC health
grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check 2>/dev/null || echo "grpcurl not installed, skipping health check"
kill %1
```

Expected: both HTTP and gRPC servers start successfully
