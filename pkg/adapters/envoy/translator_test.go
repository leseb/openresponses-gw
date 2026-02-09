package envoy

import (
	"encoding/json"
	"testing"

	extproc "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/leseb/openai-responses-gateway/pkg/core/schema"
)

func TestExtractResponseRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *extproc.ProcessingRequest
		want    *schema.ResponseRequest
		wantErr bool
	}{
		{
			name:    "nil request",
			req:     nil,
			wantErr: true,
		},
		{
			name: "nil request body",
			req: &extproc.ProcessingRequest{
				Request: &extproc.ProcessingRequest_RequestHeaders{},
			},
			wantErr: true,
		},
		{
			name: "empty body",
			req: &extproc.ProcessingRequest{
				Request: &extproc.ProcessingRequest_RequestBody{
					RequestBody: &extproc.HttpBody{
						Body: []byte{},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid JSON",
			req: &extproc.ProcessingRequest{
				Request: &extproc.ProcessingRequest_RequestBody{
					RequestBody: &extproc.HttpBody{
						Body: []byte(`{invalid json`),
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing model field",
			req: &extproc.ProcessingRequest{
				Request: &extproc.ProcessingRequest_RequestBody{
					RequestBody: &extproc.HttpBody{
						Body: []byte(`{"input":"test"}`),
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing input field",
			req: &extproc.ProcessingRequest{
				Request: &extproc.ProcessingRequest_RequestBody{
					RequestBody: &extproc.HttpBody{
						Body: []byte(`{"model":"llama3.2:3b"}`),
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid request",
			req: &extproc.ProcessingRequest{
				Request: &extproc.ProcessingRequest_RequestBody{
					RequestBody: &extproc.HttpBody{
						Body: []byte(`{"model":"llama3.2:3b","input":"Hello"}`),
					},
				},
			},
			want: &schema.ResponseRequest{
				Model: stringPtr("llama3.2:3b"),
				Input: "Hello",
			},
			wantErr: false,
		},
		{
			name: "valid request with array input",
			req: &extproc.ProcessingRequest{
				Request: &extproc.ProcessingRequest_RequestBody{
					RequestBody: &extproc.HttpBody{
						Body: []byte(`{"model":"llama3.2:3b","input":[{"type":"message","role":"user","content":"Hello"}]}`),
					},
				},
			},
			want: &schema.ResponseRequest{
				Model: stringPtr("llama3.2:3b"),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractResponseRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractResponseRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got == nil {
					t.Errorf("ExtractResponseRequest() got nil, want non-nil")
					return
				}
				if got.Model == nil || *got.Model != *tt.want.Model {
					t.Errorf("ExtractResponseRequest() model = %v, want %v", got.Model, tt.want.Model)
				}
			}
		})
	}
}

func TestCreateSuccessResponse(t *testing.T) {
	tests := []struct {
		name        string
		resp        *schema.Response
		isStreaming bool
		wantErr     bool
		checkFunc   func(*testing.T, *extproc.ProcessingResponse)
	}{
		{
			name:    "nil response",
			resp:    nil,
			wantErr: true,
		},
		{
			name: "non-streaming response",
			resp: &schema.Response{
				ID:     "resp_123",
				Object: "response",
				Model:  "llama3.2:3b",
				Status: "completed",
			},
			isStreaming: false,
			wantErr:     false,
			checkFunc: func(t *testing.T, pr *extproc.ProcessingResponse) {
				ir := pr.GetImmediateResponse()
				if ir == nil {
					t.Fatal("expected ImmediateResponse, got nil")
				}
				if ir.Status.Code != typev3.StatusCode_OK {
					t.Errorf("expected status 200, got %v", ir.Status.Code)
				}

				// Check content-type header
				hasContentType := false
				for _, h := range ir.Headers.SetHeaders {
					if h.Header.Key == "content-type" && h.Header.Value == "application/json" {
						hasContentType = true
						break
					}
				}
				if !hasContentType {
					t.Error("missing content-type: application/json header")
				}

				// Verify body is valid JSON
				var respData schema.Response
				if err := json.Unmarshal(ir.Body, &respData); err != nil {
					t.Errorf("invalid JSON in body: %v", err)
				}
				if respData.ID != "resp_123" {
					t.Errorf("expected ID resp_123, got %s", respData.ID)
				}
			},
		},
		{
			name: "streaming response",
			resp: &schema.Response{
				ID:     "resp_456",
				Object: "response",
				Model:  "llama3.2:3b",
				Status: "in_progress",
			},
			isStreaming: true,
			wantErr:     false,
			checkFunc: func(t *testing.T, pr *extproc.ProcessingResponse) {
				ir := pr.GetImmediateResponse()
				if ir == nil {
					t.Fatal("expected ImmediateResponse, got nil")
				}

				// Check for streaming headers
				headers := make(map[string]string)
				for _, h := range ir.Headers.SetHeaders {
					headers[h.Header.Key] = h.Header.Value
				}

				if headers["content-type"] != "text/event-stream" {
					t.Errorf("expected content-type: text/event-stream, got %s", headers["content-type"])
				}
				if headers["cache-control"] != "no-cache" {
					t.Error("missing cache-control: no-cache header")
				}
				if headers["connection"] != "keep-alive" {
					t.Error("missing connection: keep-alive header")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CreateSuccessResponse(tt.resp, tt.isStreaming)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateSuccessResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checkFunc != nil {
				tt.checkFunc(t, got)
			}
		})
	}
}

func TestCreateErrorResponse(t *testing.T) {
	tests := []struct {
		name       string
		statusCode typev3.StatusCode
		errorType  ErrorType
		message    string
		wantStatus int
	}{
		{
			name:       "bad request",
			statusCode: typev3.StatusCode_BadRequest,
			errorType:  ErrorTypeBadRequest,
			message:    "Invalid input",
			wantStatus: 400,
		},
		{
			name:       "internal error",
			statusCode: typev3.StatusCode_InternalServerError,
			errorType:  ErrorTypeInternalError,
			message:    "Something went wrong",
			wantStatus: 500,
		},
		{
			name:       "unprocessable entity",
			statusCode: typev3.StatusCode_UnprocessableEntity,
			errorType:  ErrorTypeInvalidRequest,
			message:    "Validation failed",
			wantStatus: 422,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CreateErrorResponse(tt.statusCode, tt.errorType, tt.message)
			ir := got.GetImmediateResponse()
			if ir == nil {
				t.Fatal("expected ImmediateResponse, got nil")
			}

			if ir.Status.Code != tt.statusCode {
				t.Errorf("expected status code %v, got %v", tt.statusCode, ir.Status.Code)
			}

			// Parse error body
			var errorResp map[string]interface{}
			if err := json.Unmarshal(ir.Body, &errorResp); err != nil {
				t.Fatalf("failed to unmarshal error body: %v", err)
			}

			errorObj, ok := errorResp["error"].(map[string]interface{})
			if !ok {
				t.Fatal("error object not found in response")
			}

			if errorObj["message"] != tt.message {
				t.Errorf("expected message %s, got %s", tt.message, errorObj["message"])
			}

			if errorObj["type"] != string(tt.errorType) {
				t.Errorf("expected type %s, got %s", tt.errorType, errorObj["type"])
			}

			if int(errorObj["code"].(float64)) != tt.wantStatus {
				t.Errorf("expected code %d, got %v", tt.wantStatus, errorObj["code"])
			}
		})
	}
}

func TestCreateContinueResponse(t *testing.T) {
	got := CreateContinueResponse()
	if got.GetRequestHeaders() == nil {
		t.Error("expected RequestHeaders response, got nil")
	}
}

func TestCreateBadRequestError(t *testing.T) {
	msg := "bad request test"
	got := CreateBadRequestError(msg)
	ir := got.GetImmediateResponse()
	if ir.Status.Code != typev3.StatusCode_BadRequest {
		t.Errorf("expected BadRequest status, got %v", ir.Status.Code)
	}
}

func TestCreateInternalError(t *testing.T) {
	msg := "internal error test"
	got := CreateInternalError(msg)
	ir := got.GetImmediateResponse()
	if ir.Status.Code != typev3.StatusCode_InternalServerError {
		t.Errorf("expected InternalServerError status, got %v", ir.Status.Code)
	}
}

func TestCreateUnprocessableEntityError(t *testing.T) {
	msg := "validation failed"
	got := CreateUnprocessableEntityError(msg)
	ir := got.GetImmediateResponse()
	if ir.Status.Code != typev3.StatusCode_UnprocessableEntity {
		t.Errorf("expected UnprocessableEntity status, got %v", ir.Status.Code)
	}
}

// Helper function
func stringPtr(s string) *string {
	return &s
}
