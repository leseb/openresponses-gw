package envoy

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/andybalholm/brotli"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extproc "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/leseb/openresponses-gw/pkg/core/schema"
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

// CreateImmediateResponseFromRecorder converts an httptest.ResponseRecorder into
// an ExtProc ImmediateResponse, mapping HTTP status codes and copying headers/body.
func CreateImmediateResponseFromRecorder(recorder *httptest.ResponseRecorder) *extproc.ProcessingResponse {
	result := recorder.Result()
	defer result.Body.Close()

	body, _ := io.ReadAll(result.Body)

	// Build response headers from the recorder
	var headers []*corev3.HeaderValueOption
	for key, values := range result.Header {
		for _, val := range values {
			headers = append(headers, &corev3.HeaderValueOption{
				Header: &corev3.HeaderValue{
					Key:      http.CanonicalHeaderKey(key),
					RawValue: []byte(val),
				},
			})
		}
	}

	return &extproc.ProcessingResponse{
		Response: &extproc.ProcessingResponse_ImmediateResponse{
			ImmediateResponse: &extproc.ImmediateResponse{
				Status: &typev3.HttpStatus{
					Code: httpStatusToEnvoyCode(result.StatusCode),
				},
				Body:    body,
				Headers: &extproc.HeaderMutation{SetHeaders: headers},
			},
		},
	}
}

// extractEventTypeFromJSON extracts the SSE event type from a streaming event.
// For *schema.RawStreamingEvent, returns EventType directly (since MarshalJSON
// returns raw data without a type field). For all other event types, unmarshals
// just the "type" field from the already-marshaled JSON bytes.
func extractEventTypeFromJSON(event interface{}, data []byte) string {
	if raw, ok := event.(*schema.RawStreamingEvent); ok {
		return raw.EventType
	}
	var typed struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typed); err == nil && typed.Type != "" {
		return typed.Type
	}
	return "message"
}

// formatSSEEvents marshals each event to JSON, extracts the event type,
// and writes SSE format (event: <type>\ndata: <json>\n\n).
// Events that fail to marshal are skipped.
func formatSSEEvents(events []interface{}) []byte {
	var buf bytes.Buffer
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}
		eventType := extractEventTypeFromJSON(event, data)
		buf.WriteString("event: ")
		buf.WriteString(eventType)
		buf.WriteByte('\n')
		buf.WriteString("data: ")
		buf.Write(data)
		buf.WriteByte('\n')
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

// CreateSSEImmediateResponse creates an ImmediateResponse with status 200,
// SSE body, and appropriate streaming headers.
func CreateSSEImmediateResponse(sseBody []byte) *extproc.ProcessingResponse {
	return &extproc.ProcessingResponse{
		Response: &extproc.ProcessingResponse_ImmediateResponse{
			ImmediateResponse: &extproc.ImmediateResponse{
				Status: &typev3.HttpStatus{
					Code: typev3.StatusCode_OK,
				},
				Body: sseBody,
				Headers: &extproc.HeaderMutation{
					SetHeaders: []*corev3.HeaderValueOption{
						{
							Header: &corev3.HeaderValue{
								Key:      "content-type",
								RawValue: []byte("text/event-stream"),
							},
						},
						{
							Header: &corev3.HeaderValue{
								Key:      "cache-control",
								RawValue: []byte("no-cache"),
							},
						},
						{
							Header: &corev3.HeaderValue{
								Key:      "connection",
								RawValue: []byte("keep-alive"),
							},
						},
					},
				},
			},
		},
	}
}

// httpStatusToEnvoyCode converts an HTTP status code integer to the Envoy StatusCode enum.
func httpStatusToEnvoyCode(code int) typev3.StatusCode {
	// Envoy StatusCode enum values match HTTP status codes numerically
	envoyCode := typev3.StatusCode(code)
	// Validate that this is a known enum value; fall back to 500 if unknown
	if _, ok := typev3.StatusCode_name[int32(envoyCode)]; ok {
		return envoyCode
	}
	return typev3.StatusCode_InternalServerError
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
