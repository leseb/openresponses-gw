package envoy

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"

	"github.com/andybalholm/brotli"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extproc "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/leseb/openai-responses-gateway/pkg/core/schema"
)

// ErrorType represents different error categories
type ErrorType string

const (
	ErrorTypeBadRequest          ErrorType = "bad_request"
	ErrorTypeInvalidRequest      ErrorType = "invalid_request_error"
	ErrorTypeAuthenticationError ErrorType = "authentication_error"
	ErrorTypePermissionError     ErrorType = "permission_error"
	ErrorTypeNotFound            ErrorType = "not_found_error"
	ErrorTypeRateLimitError      ErrorType = "rate_limit_error"
	ErrorTypeInternalError       ErrorType = "internal_server_error"
	ErrorTypeServiceUnavailable  ErrorType = "service_unavailable"
)

// ExtractResponseRequest extracts the ResponseRequest from an ExtProc ProcessingRequest
func ExtractResponseRequest(req *extproc.ProcessingRequest) (*schema.ResponseRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("processing request is nil")
	}

	// Extract body from request_body phase
	requestBody := req.GetRequestBody()
	if requestBody == nil {
		return nil, fmt.Errorf("request body is nil")
	}

	body := requestBody.GetBody()
	if len(body) == 0 {
		return nil, fmt.Errorf("request body is empty")
	}

	// Unmarshal JSON to ResponseRequest
	var respReq schema.ResponseRequest
	if err := json.Unmarshal(body, &respReq); err != nil {
		return nil, fmt.Errorf("failed to parse request body: %w", err)
	}

	// Basic validation
	if respReq.Model == nil || *respReq.Model == "" {
		return nil, fmt.Errorf("model field is required")
	}

	if respReq.Input == nil {
		return nil, fmt.Errorf("input field is required")
	}

	return &respReq, nil
}

// CreateSuccessResponse creates an ImmediateResponse with the successful result
func CreateSuccessResponse(resp *schema.Response, isStreaming bool) (*extproc.ProcessingResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("response is nil")
	}

	// Marshal response to JSON
	body, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	headers := []*corev3.HeaderValueOption{
		{
			Header: &corev3.HeaderValue{
				Key:   "content-type",
				Value: "application/json",
			},
		},
	}

	// Add streaming headers if needed
	if isStreaming {
		headers = append(headers,
			&corev3.HeaderValueOption{
				Header: &corev3.HeaderValue{
					Key:   "content-type",
					Value: "text/event-stream",
				},
			},
			&corev3.HeaderValueOption{
				Header: &corev3.HeaderValue{
					Key:   "cache-control",
					Value: "no-cache",
				},
			},
			&corev3.HeaderValueOption{
				Header: &corev3.HeaderValue{
					Key:   "connection",
					Value: "keep-alive",
				},
			},
		)
	}

	return &extproc.ProcessingResponse{
		Response: &extproc.ProcessingResponse_ImmediateResponse{
			ImmediateResponse: &extproc.ImmediateResponse{
				Status: &typev3.HttpStatus{
					Code: typev3.StatusCode_OK,
				},
				Body:    body,
				Headers: &extproc.HeaderMutation{SetHeaders: headers},
			},
		},
	}, nil
}

// CreateErrorResponse creates an ImmediateResponse with error details
func CreateErrorResponse(statusCode typev3.StatusCode, errorType ErrorType, message string) *extproc.ProcessingResponse {
	// Create error response matching OpenAI format
	errorResp := map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    string(errorType),
			"code":    statusCodeToHTTP(statusCode),
		},
	}

	body, _ := json.Marshal(errorResp)

	return &extproc.ProcessingResponse{
		Response: &extproc.ProcessingResponse_ImmediateResponse{
			ImmediateResponse: &extproc.ImmediateResponse{
				Status: &typev3.HttpStatus{
					Code: statusCode,
				},
				Body: body,
				Headers: &extproc.HeaderMutation{
					SetHeaders: []*corev3.HeaderValueOption{
						{
							Header: &corev3.HeaderValue{
								Key:   "content-type",
								Value: "application/json",
							},
						},
					},
				},
			},
		},
	}
}

// CreateBadRequestError creates a 400 error response
func CreateBadRequestError(message string) *extproc.ProcessingResponse {
	return CreateErrorResponse(typev3.StatusCode_BadRequest, ErrorTypeBadRequest, message)
}

// CreateInternalError creates a 500 error response
func CreateInternalError(message string) *extproc.ProcessingResponse {
	return CreateErrorResponse(typev3.StatusCode_InternalServerError, ErrorTypeInternalError, message)
}

// CreateUnprocessableEntityError creates a 422 error response
func CreateUnprocessableEntityError(message string) *extproc.ProcessingResponse {
	return CreateErrorResponse(typev3.StatusCode_UnprocessableEntity, ErrorTypeInvalidRequest, message)
}

// CreateContinueResponse creates a continue response (for phases we don't process)
func CreateContinueResponse() *extproc.ProcessingResponse {
	return &extproc.ProcessingResponse{
		Response: &extproc.ProcessingResponse_RequestHeaders{
			RequestHeaders: &extproc.HeadersResponse{},
		},
	}
}

// DecodeContent decompresses body content if needed
func DecodeContent(body []byte, contentEncoding string) ([]byte, error) {
	switch contentEncoding {
	case "gzip":
		reader, err := gzip.NewReader(io.NopCloser(io.Reader(nil)))
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer reader.Close()

		decoded, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress gzip: %w", err)
		}
		return decoded, nil

	case "br":
		reader := brotli.NewReader(io.NopCloser(io.Reader(nil)))
		decoded, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress brotli: %w", err)
		}
		return decoded, nil

	case "", "identity":
		return body, nil

	default:
		return nil, fmt.Errorf("unsupported content encoding: %s", contentEncoding)
	}
}

// statusCodeToHTTP converts Envoy StatusCode to HTTP status code number
func statusCodeToHTTP(code typev3.StatusCode) int {
	switch code {
	case typev3.StatusCode_OK:
		return 200
	case typev3.StatusCode_BadRequest:
		return 400
	case typev3.StatusCode_Unauthorized:
		return 401
	case typev3.StatusCode_Forbidden:
		return 403
	case typev3.StatusCode_NotFound:
		return 404
	case typev3.StatusCode_UnprocessableEntity:
		return 422
	case typev3.StatusCode_TooManyRequests:
		return 429
	case typev3.StatusCode_InternalServerError:
		return 500
	case typev3.StatusCode_ServiceUnavailable:
		return 503
	default:
		return 500
	}
}
