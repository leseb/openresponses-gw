package envoy

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	extproc "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/leseb/openai-responses-gateway/pkg/core/engine"
)

// Processor implements the ExternalProcessorServer interface
type Processor struct {
	extproc.UnimplementedExternalProcessorServer
	engine *engine.Engine
	logger *slog.Logger
}

// NewProcessor creates a new ExtProc processor
func NewProcessor(eng *engine.Engine, logger *slog.Logger) *Processor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Processor{
		engine: eng,
		logger: logger,
	}
}

// Process handles the ExtProc stream
func (p *Processor) Process(stream extproc.ExternalProcessor_ProcessServer) error {
	ctx := stream.Context()
	requestID := "unknown"

	p.logger.Debug("new extproc stream started")

	for {
		// Receive next processing request
		req, err := stream.Recv()
		if err == io.EOF {
			p.logger.Debug("stream closed by client", "request_id", requestID)
			return nil
		}
		if err != nil {
			p.logger.Error("error receiving request", "error", err, "request_id", requestID)
			return err
		}

		// Process based on request type
		var resp *extproc.ProcessingResponse

		switch v := req.Request.(type) {
		case *extproc.ProcessingRequest_RequestHeaders:
			// Skip request headers - we don't need to inspect them
			p.logger.Debug("processing request headers (skip)", "request_id", requestID)
			resp = CreateContinueResponse()

		case *extproc.ProcessingRequest_RequestBody:
			// This is where we process the request and generate the response
			p.logger.Debug("processing request body", "request_id", requestID)
			resp, requestID = p.processRequestBody(ctx, req)

		case *extproc.ProcessingRequest_ResponseHeaders:
			// Skip response headers - we're using ImmediateResponse
			p.logger.Debug("processing response headers (skip)", "request_id", requestID)
			resp = &extproc.ProcessingResponse{
				Response: &extproc.ProcessingResponse_ResponseHeaders{
					ResponseHeaders: &extproc.HeadersResponse{},
				},
			}

		case *extproc.ProcessingRequest_ResponseBody:
			// Skip response body - we already sent ImmediateResponse
			p.logger.Debug("processing response body (skip)", "request_id", requestID)
			resp = &extproc.ProcessingResponse{
				Response: &extproc.ProcessingResponse_ResponseBody{
					ResponseBody: &extproc.BodyResponse{},
				},
			}

		default:
			p.logger.Warn("unknown request type", "type", fmt.Sprintf("%T", v), "request_id", requestID)
			resp = CreateContinueResponse()
		}

		// Send response
		if err := stream.Send(resp); err != nil {
			p.logger.Error("error sending response", "error", err, "request_id", requestID)
			return err
		}
	}
}

// processRequestBody processes the request body and returns an immediate response
func (p *Processor) processRequestBody(ctx context.Context, req *extproc.ProcessingRequest) (*extproc.ProcessingResponse, string) {
	// Extract ResponseRequest from the processing request
	respReq, err := ExtractResponseRequest(req)
	if err != nil {
		p.logger.Warn("failed to extract request", "error", err)
		// Check if it's a validation error (400) or parse error (400)
		if err.Error() == "model field is required" || err.Error() == "input field is required" {
			return CreateUnprocessableEntityError(err.Error()), "unknown"
		}
		return CreateBadRequestError(fmt.Sprintf("Invalid request body: %s", err.Error())), "unknown"
	}

	requestID := generateRequestID()
	p.logger.Info("processing request",
		"request_id", requestID,
		"model", respReq.Model,
		"streaming", respReq.Stream,
	)

	// Process the request using the core engine
	// This is where we call the backend (via the engine) and get the response
	engineResp, err := p.engine.ProcessRequest(ctx, respReq)
	if err != nil {
		p.logger.Error("engine processing failed",
			"request_id", requestID,
			"error", err,
		)
		return CreateInternalError(fmt.Sprintf("Failed to process request: %s", err.Error())), requestID
	}

	p.logger.Info("request processed successfully",
		"request_id", requestID,
		"response_id", engineResp.ID,
		"status", engineResp.Status,
	)

	// Create immediate response with the result
	// This bypasses the backend and returns directly to the client
	procResp, err := CreateSuccessResponse(engineResp, respReq.Stream)
	if err != nil {
		p.logger.Error("failed to create success response",
			"request_id", requestID,
			"error", err,
		)
		return CreateInternalError("Failed to create response"), requestID
	}

	return procResp, requestID
}

// generateRequestID generates a simple request ID
// In production, this should use a more sophisticated ID generation
func generateRequestID() string {
	// For now, use a simple counter or UUID
	// This is a placeholder - in production use proper UUID generation
	return fmt.Sprintf("req_%d", 12345) // TODO: Use proper UUID generation
}
