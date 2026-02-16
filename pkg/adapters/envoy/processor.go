package envoy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"

	extproc "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// Processor implements the ExternalProcessorServer interface
type Processor struct {
	extproc.UnimplementedExternalProcessorServer
	handler http.Handler
	logger  *slog.Logger
}

// requestContext holds per-stream state captured during RequestHeaders,
// used later in RequestBody to construct the full http.Request.
type requestContext struct {
	method  string
	path    string
	headers http.Header
}

// NewProcessor creates a new ExtProc processor
func NewProcessor(handler http.Handler, logger *slog.Logger) *Processor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Processor{
		handler: handler,
		logger:  logger,
	}
}

// Process handles the ExtProc stream
func (p *Processor) Process(stream extproc.ExternalProcessor_ProcessServer) error {
	p.logger.Debug("new extproc stream started")

	// Per-stream request context: populated in RequestHeaders, consumed in RequestBody.
	var reqCtx *requestContext

	for {
		req, err := stream.Recv()
		if err != nil {
			// EOF and context cancellation are normal — Envoy closes the stream
			// after receiving an ImmediateResponse.
			if err == io.EOF || err == context.Canceled {
				p.logger.Debug("stream closed")
				return nil
			}
			if s, ok := grpcstatus.FromError(err); ok && s.Code() == codes.Canceled {
				p.logger.Debug("stream canceled by envoy")
				return nil
			}
			p.logger.Error("error receiving request", "error", err)
			return err
		}

		var resp *extproc.ProcessingResponse

		switch v := req.Request.(type) {
		case *extproc.ProcessingRequest_RequestHeaders:
			p.logger.Debug("processing request headers")
			resp, reqCtx = p.processRequestHeaders(v)

		case *extproc.ProcessingRequest_RequestBody:
			p.logger.Debug("processing request body")
			resp = p.processRequestBody(reqCtx, v)
			reqCtx = nil // consumed

		case *extproc.ProcessingRequest_ResponseHeaders:
			p.logger.Debug("processing response headers (skip)")
			resp = &extproc.ProcessingResponse{
				Response: &extproc.ProcessingResponse_ResponseHeaders{
					ResponseHeaders: &extproc.HeadersResponse{},
				},
			}

		case *extproc.ProcessingRequest_ResponseBody:
			p.logger.Debug("processing response body (skip)")
			resp = &extproc.ProcessingResponse{
				Response: &extproc.ProcessingResponse_ResponseBody{
					ResponseBody: &extproc.BodyResponse{},
				},
			}

		default:
			p.logger.Warn("unknown request type", "type", fmt.Sprintf("%T", v))
			resp = CreateContinueResponse()
		}

		if err := stream.Send(resp); err != nil {
			p.logger.Error("error sending response", "error", err)
			return err
		}
	}
}

// processRequestHeaders extracts method, path, and headers from the RequestHeaders phase.
// For bodyless methods (GET, DELETE, HEAD, OPTIONS) it delegates immediately.
// For methods with a body (POST, PUT, PATCH) it stores context and returns Continue.
func (p *Processor) processRequestHeaders(v *extproc.ProcessingRequest_RequestHeaders) (*extproc.ProcessingResponse, *requestContext) {
	hdrs := v.RequestHeaders.GetHeaders()

	method := "GET"
	path := "/"
	httpHeaders := make(http.Header)

	for _, h := range hdrs.GetHeaders() {
		key := h.GetKey()
		value := h.GetValue()
		if len(value) == 0 {
			value = string(h.GetRawValue())
		}

		switch key {
		case ":method":
			method = value
		case ":path":
			path = value
		case ":authority", ":scheme":
			// pseudo-headers not needed for handler delegation
		default:
			httpHeaders.Add(key, value)
		}
	}

	p.logger.Debug("extracted request info", "method", method, "path", path)

	// Bodyless methods: delegate immediately
	upper := strings.ToUpper(method)
	if upper == "GET" || upper == "DELETE" || upper == "HEAD" || upper == "OPTIONS" {
		return p.delegateToHandler(method, path, httpHeaders, nil), nil
	}

	// Methods with body: store context, continue to RequestBody phase
	ctx := &requestContext{
		method:  method,
		path:    path,
		headers: httpHeaders,
	}

	// Return continue with request_body_mode override to ensure we receive the body
	return CreateContinueResponse(), ctx
}

// processRequestBody combines stored request context with the body and delegates to the HTTP handler.
func (p *Processor) processRequestBody(reqCtx *requestContext, v *extproc.ProcessingRequest_RequestBody) *extproc.ProcessingResponse {
	body := v.RequestBody.GetBody()

	if reqCtx == nil {
		// No context from headers phase — fall back to POST /v1/responses for backwards compat
		p.logger.Warn("no request context from headers phase, using defaults")
		return p.delegateToHandler("POST", "/v1/responses", make(http.Header), body)
	}

	return p.delegateToHandler(reqCtx.method, reqCtx.path, reqCtx.headers, body)
}

// delegateToHandler constructs an http.Request from the ExtProc data, dispatches
// it through the existing HTTP handler, and converts the result to an ImmediateResponse.
func (p *Processor) delegateToHandler(method, rawPath string, headers http.Header, body []byte) *extproc.ProcessingResponse {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, rawPath, bodyReader)
	if err != nil {
		p.logger.Error("failed to create http request", "error", err)
		return CreateInternalError("failed to construct request")
	}
	req.Header = headers

	recorder := httptest.NewRecorder()
	p.handler.ServeHTTP(recorder, req)

	p.logger.Info("request delegated to handler",
		"method", method,
		"path", rawPath,
		"status", recorder.Code,
	)

	return CreateImmediateResponseFromRecorder(recorder)
}
