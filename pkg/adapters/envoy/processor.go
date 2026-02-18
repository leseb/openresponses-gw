// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extproc "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/leseb/openresponses-gw/pkg/core/api"
	"github.com/leseb/openresponses-gw/pkg/core/engine"
)

// Processor implements the ExternalProcessorServer interface.
// It uses filter chain mode: injects state into requests and processes responses.
type Processor struct {
	extproc.UnimplementedExternalProcessorServer
	engine   *engine.Engine
	logger   *slog.Logger
	injector engine.StateInjector
}

// NewProcessor creates a new ExtProc processor.
func NewProcessor(eng *engine.Engine, logger *slog.Logger) *Processor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Processor{
		engine:   eng,
		logger:   logger,
		injector: engine.NewHeaderStateInjector(),
	}
}

// requestContext holds state for a single request across phases.
type requestContext struct {
	requestID     string
	preparedReq   *engine.PreparedRequest
	originalBody  []byte
	stateInjected bool
}

// Process handles the ExtProc stream.
// It processes all four phases: request headers, request body, response headers, response body.
func (p *Processor) Process(stream extproc.ExternalProcessor_ProcessServer) error {
	p.logger.Debug("new extproc stream started")

	ctx := stream.Context()
	reqCtx := &requestContext{
		requestID: "unknown",
	}

	for {
		req, err := stream.Recv()
		if err != nil {
			// EOF and context cancellation are normal — Envoy closes the stream
			// after the response is fully processed.
			if err == io.EOF || err == context.Canceled {
				p.logger.Debug("stream closed", "request_id", reqCtx.requestID)
				return nil
			}
			if s, ok := grpcstatus.FromError(err); ok && s.Code() == codes.Canceled {
				p.logger.Debug("stream canceled by envoy", "request_id", reqCtx.requestID)
				return nil
			}
			p.logger.Error("error receiving request", "error", err, "request_id", reqCtx.requestID)
			return err
		}

		var resp *extproc.ProcessingResponse

		switch v := req.Request.(type) {
		case *extproc.ProcessingRequest_RequestHeaders:
			p.logger.Debug("processing request headers", "request_id", reqCtx.requestID)
			resp = p.processRequestHeaders(reqCtx)

		case *extproc.ProcessingRequest_RequestBody:
			p.logger.Debug("processing request body", "request_id", reqCtx.requestID)
			resp = p.processRequestBody(ctx, req, reqCtx)

		case *extproc.ProcessingRequest_ResponseHeaders:
			p.logger.Debug("processing response headers", "request_id", reqCtx.requestID)
			resp = p.processResponseHeaders(reqCtx)

		case *extproc.ProcessingRequest_ResponseBody:
			p.logger.Debug("processing response body", "request_id", reqCtx.requestID)
			resp = p.processResponseBody(ctx, v.ResponseBody, reqCtx)

		default:
			p.logger.Warn("unknown request type", "type", fmt.Sprintf("%T", v), "request_id", reqCtx.requestID)
			resp = CreateContinueResponse()
		}

		if err := stream.Send(resp); err != nil {
			p.logger.Error("error sending response", "error", err, "request_id", reqCtx.requestID)
			return err
		}
	}
}

// processRequestHeaders handles the request headers phase.
// We continue without modification - the body phase will handle preparation.
func (p *Processor) processRequestHeaders(_ *requestContext) *extproc.ProcessingResponse {
	return &extproc.ProcessingResponse{
		Response: &extproc.ProcessingResponse_RequestHeaders{
			RequestHeaders: &extproc.HeadersResponse{
				Response: &extproc.CommonResponse{},
			},
		},
	}
}

// processRequestBody handles the request body phase.
// This is where we prepare the request, inject state, and modify the body/headers.
func (p *Processor) processRequestBody(ctx context.Context, req *extproc.ProcessingRequest, reqCtx *requestContext) *extproc.ProcessingResponse {
	// Extract ResponseRequest from the processing request
	respReq, err := ExtractResponseRequest(req)
	if err != nil {
		p.logger.Warn("failed to extract request", "error", err)
		if err.Error() == "model field is required" || err.Error() == "input field is required" {
			return CreateUnprocessableEntityError(err.Error())
		}
		return CreateBadRequestError(fmt.Sprintf("Invalid request body: %s", err.Error()))
	}

	reqCtx.requestID = generateRequestID()
	p.logger.Info("preparing request",
		"request_id", reqCtx.requestID,
		"model", respReq.Model,
	)

	// Store original body for potential modification
	reqBody := req.GetRequestBody()
	if reqBody != nil {
		reqCtx.originalBody = reqBody.GetBody()
	}

	// Prepare the request (resolve conversation, expand tools, build state)
	prepared, err := p.engine.PrepareRequest(ctx, respReq)
	if err != nil {
		p.logger.Error("failed to prepare request",
			"request_id", reqCtx.requestID,
			"error", err,
		)
		return CreateInternalError(fmt.Sprintf("Failed to prepare request: %s", err.Error()))
	}

	reqCtx.preparedReq = prepared
	reqCtx.stateInjected = true

	p.logger.Info("request prepared successfully",
		"request_id", reqCtx.requestID,
		"response_id", prepared.State.ResponseID,
		"conversation_id", prepared.State.ConversationID,
	)

	// Inject state into the request
	modifiedBody, err := p.injector.InjectIntoBody(reqCtx.originalBody, prepared.State)
	if err != nil {
		p.logger.Error("failed to inject state into body",
			"request_id", reqCtx.requestID,
			"error", err,
		)
		return CreateInternalError("Failed to inject state")
	}

	// Get headers to add
	stateHeaders := p.injector.InjectIntoHeaders(prepared.State)

	// Build header mutations
	var headerMutations []*corev3.HeaderValueOption
	for key, value := range stateHeaders {
		headerMutations = append(headerMutations, &corev3.HeaderValueOption{
			Header: &corev3.HeaderValue{
				Key:   key,
				Value: value,
			},
		})
	}

	// Return response that modifies body and adds headers
	return &extproc.ProcessingResponse{
		Response: &extproc.ProcessingResponse_RequestBody{
			RequestBody: &extproc.BodyResponse{
				Response: &extproc.CommonResponse{
					HeaderMutation: &extproc.HeaderMutation{
						SetHeaders: headerMutations,
					},
					BodyMutation: &extproc.BodyMutation{
						Mutation: &extproc.BodyMutation_Body{
							Body: modifiedBody,
						},
					},
				},
			},
		},
	}
}

// processResponseHeaders handles the response headers phase.
// We continue without modification.
func (p *Processor) processResponseHeaders(_ *requestContext) *extproc.ProcessingResponse {
	return &extproc.ProcessingResponse{
		Response: &extproc.ProcessingResponse_ResponseHeaders{
			ResponseHeaders: &extproc.HeadersResponse{
				Response: &extproc.CommonResponse{},
			},
		},
	}
}

// processResponseBody handles the response body phase.
// This is where we process the backend response and save state.
func (p *Processor) processResponseBody(ctx context.Context, body *extproc.HttpBody, reqCtx *requestContext) *extproc.ProcessingResponse {
	// If we didn't inject state (no prepared request), just continue
	if !reqCtx.stateInjected || reqCtx.preparedReq == nil {
		p.logger.Debug("no state injection, passing through response", "request_id", reqCtx.requestID)
		return &extproc.ProcessingResponse{
			Response: &extproc.ProcessingResponse_ResponseBody{
				ResponseBody: &extproc.BodyResponse{
					Response: &extproc.CommonResponse{},
				},
			},
		}
	}

	// Parse the backend response
	var backendResp api.ResponsesAPIResponse
	if err := json.Unmarshal(body.Body, &backendResp); err != nil {
		p.logger.Error("failed to parse backend response",
			"request_id", reqCtx.requestID,
			"error", err,
		)
		// Continue with the original response - don't block the request
		return &extproc.ProcessingResponse{
			Response: &extproc.ProcessingResponse_ResponseBody{
				ResponseBody: &extproc.BodyResponse{
					Response: &extproc.CommonResponse{},
				},
			},
		}
	}

	// Process the response through the engine to save state
	finalResp, err := p.engine.ProcessResponse(ctx, reqCtx.preparedReq.State, &backendResp)
	if err != nil {
		p.logger.Error("failed to process response",
			"request_id", reqCtx.requestID,
			"error", err,
		)
		// Continue with the original response - don't block the request
		return &extproc.ProcessingResponse{
			Response: &extproc.ProcessingResponse_ResponseBody{
				ResponseBody: &extproc.BodyResponse{
					Response: &extproc.CommonResponse{},
				},
			},
		}
	}

	p.logger.Info("response processed successfully",
		"request_id", reqCtx.requestID,
		"response_id", finalResp.ID,
		"status", finalResp.Status,
	)

	// Marshal the final response with gateway's response ID
	modifiedBody, err := json.Marshal(finalResp)
	if err != nil {
		p.logger.Error("failed to marshal final response",
			"request_id", reqCtx.requestID,
			"error", err,
		)
		return &extproc.ProcessingResponse{
			Response: &extproc.ProcessingResponse_ResponseBody{
				ResponseBody: &extproc.BodyResponse{
					Response: &extproc.CommonResponse{},
				},
			},
		}
	}

	// Return modified response body
	return &extproc.ProcessingResponse{
		Response: &extproc.ProcessingResponse_ResponseBody{
			ResponseBody: &extproc.BodyResponse{
				Response: &extproc.CommonResponse{
					BodyMutation: &extproc.BodyMutation{
						Mutation: &extproc.BodyMutation_Body{
							Body: modifiedBody,
						},
					},
				},
			},
		},
	}
}

// generateRequestID generates a unique request ID.
func generateRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "req_unknown"
	}
	return "req_" + hex.EncodeToString(b)
}
