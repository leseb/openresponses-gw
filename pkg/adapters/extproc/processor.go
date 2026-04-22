// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package extproc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"

	filterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"

	"github.com/leseb/openresponses-gw/pkg/core/engine"
	"github.com/leseb/openresponses-gw/pkg/core/schema"
	"github.com/leseb/openresponses-gw/pkg/observability/logging"
)

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
	if logger == nil {
		logger = &logging.Logger{Logger: slog.Default()}
	}
	return &Processor{
		engine: eng,
		logger: logger,
	}
}

// Process handles the bidirectional gRPC stream from Envoy.
func (p *Processor) Process(stream extprocv3.ExternalProcessor_ProcessServer) error {
	var (
		path           string
		method         string
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
				return err
			}

		default:
			if err := stream.Send(passthroughResponse()); err != nil {
				return fmt.Errorf("sending passthrough: %w", err)
			}
		}
	}
}

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

func (p *Processor) handleStreaming(ctx context.Context, stream extprocv3.ExternalProcessor_ProcessServer, req *schema.ResponseRequest) error {
	events, err := p.engine.ProcessRequestStream(ctx, req)
	if err != nil {
		return stream.Send(errorResponse(500, "processing_error", err.Error()))
	}

	if err := stream.Send(streamHeadersMsg(200, map[string]string{
		"content-type":  "text/event-stream",
		"cache-control": "no-cache",
		"connection":    "keep-alive",
	})); err != nil {
		return fmt.Errorf("sending stream headers: %w", err)
	}

	for event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			p.logger.Error("Failed to marshal event", "error", err)
			continue
		}

		eventType := schema.ExtractEventType(event)
		sseData := fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)

		if err := stream.Send(streamBodyMsg([]byte(sseData), false)); err != nil {
			return fmt.Errorf("sending SSE event: %w", err)
		}
	}

	if err := stream.Send(streamBodyMsg(nil, true)); err != nil {
		return fmt.Errorf("sending end of stream: %w", err)
	}

	p.logger.Info("ExtProc streaming completed")
	return nil
}

func extractPathAndMethod(headers *extprocv3.HttpHeaders) (string, string) {
	if headers == nil || headers.Headers == nil {
		return "", ""
	}
	var path, method string
	for _, h := range headers.Headers.Headers {
		switch h.Key {
		case ":path":
			path = string(h.RawValue)
			if path == "" {
				path = h.Value
			}
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
