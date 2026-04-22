// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package extproc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	filterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
)

// Processor implements the Envoy ExternalProcessorServer interface.
// It delegates all request handling to an http.Handler, translating
// between the ExtProc gRPC protocol and HTTP semantics.
type Processor struct {
	extprocv3.UnimplementedExternalProcessorServer
	handler http.Handler
}

// NewProcessor creates a new ExtProc processor that delegates to the given handler.
func NewProcessor(handler http.Handler) *Processor {
	return &Processor{handler: handler}
}

// Process handles the bidirectional gRPC stream from Envoy.
func (p *Processor) Process(stream extprocv3.ExternalProcessor_ProcessServer) error {
	var reqHeaders *extprocv3.HttpHeaders

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
			reqHeaders = v.RequestHeaders

			if v.RequestHeaders.EndOfStream {
				return p.handle(stream, reqHeaders, nil)
			}

			if err := stream.Send(requestBodyBuffered()); err != nil {
				return fmt.Errorf("requesting body: %w", err)
			}

		case *extprocv3.ProcessingRequest_RequestBody:
			return p.handle(stream, reqHeaders, v.RequestBody.GetBody())

		default:
			continue
		}
	}
}

// handle builds an http.Request from ExtProc headers and body, passes it
// to the HTTP handler, and translates the response back to ExtProc messages.
func (p *Processor) handle(stream extprocv3.ExternalProcessor_ProcessServer, headers *extprocv3.HttpHeaders, body []byte) error {
	httpReq, err := buildHTTPRequest(stream.Context(), headers, body)
	if err != nil {
		return stream.Send(errorResponse(400, "invalid_request", err.Error()))
	}

	w := newResponseWriter(stream)
	p.handler.ServeHTTP(w, httpReq)
	return w.finish()
}

// buildHTTPRequest reconstructs an http.Request from ExtProc headers and body.
func buildHTTPRequest(ctx context.Context, headers *extprocv3.HttpHeaders, body []byte) (*http.Request, error) {
	var method, path, authority string
	httpHeaders := make(http.Header)

	if headers != nil && headers.Headers != nil {
		for _, h := range headers.Headers.Headers {
			val := string(h.RawValue)
			if val == "" {
				val = h.Value
			}
			switch h.Key {
			case ":method":
				method = val
			case ":path":
				path = val
			case ":authority":
				authority = val
			case ":scheme":
				// skip pseudo-header
			default:
				httpHeaders.Set(h.Key, val)
			}
		}
	}

	if method == "" {
		method = "GET"
	}
	if path == "" {
		path = "/"
	}

	u, err := url.ParseRequestURI(path)
	if err != nil {
		return nil, fmt.Errorf("invalid path %q: %w", path, err)
	}

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header = httpHeaders
	req.Host = authority
	if len(body) > 0 {
		req.ContentLength = int64(len(body))
	}

	return req, nil
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

// responseWriter adapts http.ResponseWriter to ExtProc responses.
// For SSE streaming (Content-Type: text/event-stream), it sends headers via
// StreamedImmediateResponse and pipes each Flush() as a body chunk.
// For all other responses, it buffers and sends an ImmediateResponse.
type responseWriter struct {
	stream      extprocv3.ExternalProcessor_ProcessServer
	header      http.Header
	status      int
	isSSE       bool
	wroteHeader bool
	buf         bytes.Buffer
	sendErr     error
}

func newResponseWriter(stream extprocv3.ExternalProcessor_ProcessServer) *responseWriter {
	return &responseWriter{
		stream: stream,
		header: make(http.Header),
		status: http.StatusOK,
	}
}

func (w *responseWriter) Header() http.Header {
	return w.header
}

func (w *responseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = statusCode
	w.isSSE = w.header.Get("Content-Type") == "text/event-stream"

	if w.isSSE {
		hdrs := w.headerMap()
		if err := w.stream.Send(streamHeadersMsg(statusCode, hdrs)); err != nil {
			w.sendErr = err
		}
	}
}

func (w *responseWriter) Write(data []byte) (int, error) {
	if w.sendErr != nil {
		return 0, w.sendErr
	}
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.buf.Write(data)
}

// Flush sends buffered SSE data as a StreamedImmediateResponse body chunk.
func (w *responseWriter) Flush() {
	if w.sendErr != nil || !w.isSSE || w.buf.Len() == 0 {
		return
	}
	if err := w.stream.Send(streamBodyMsg(w.buf.Bytes(), false)); err != nil {
		w.sendErr = err
	}
	w.buf.Reset()
}

// finish completes the ExtProc response. For SSE, sends end_of_stream.
// For non-SSE, sends the buffered body as an ImmediateResponse.
func (w *responseWriter) finish() error {
	if w.sendErr != nil {
		return w.sendErr
	}

	if w.isSSE {
		if w.buf.Len() > 0 {
			if err := w.stream.Send(streamBodyMsg(w.buf.Bytes(), false)); err != nil {
				return err
			}
		}
		return w.stream.Send(streamBodyMsg(nil, true))
	}

	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.stream.Send(immediateResponseMsg(w.status, w.headerMap(), w.buf.Bytes()))
}

func (w *responseWriter) headerMap() map[string]string {
	hdrs := make(map[string]string, len(w.header))
	for k := range w.header {
		hdrs[strings.ToLower(k)] = w.header.Get(k)
	}
	return hdrs
}
