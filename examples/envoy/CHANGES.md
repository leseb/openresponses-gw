# ExtProc Implementation Changes

This document describes the improvements made to the ExtProc integration based on analysis of the Envoy AI Gateway project at `/Users/leseb/go/src/github.com/envoyproxy/ai-gateway`.

## Critical Architectural Fix

### Problem: Incorrect Response Phase Processing

**Before:**
```go
case *extproc.ProcessingRequest_RequestBody:
    // Call engine.ProcessRequest()
    // Try to return response via BodyMutation
    return &ProcessingResponse_ResponseBody{...}
```

**Issue:** We were trying to mutate the response body during request processing, before the backend had even responded. This would not work because:
1. Response body phase happens AFTER the backend responds
2. Body mutations only work for responses that flow through Envoy
3. Our engine needs to call the backend itself

**After:**
```go
case *extproc.ProcessingRequest_RequestBody:
    // Call engine.ProcessRequest()
    // Return ImmediateResponse to bypass backend entirely
    return &ProcessingResponse_ImmediateResponse{
        Status: &HttpStatus{Code: StatusCode_OK},
        Body: responseBody,
        Headers: &HeaderMutation{...},
    }
```

**Solution:** Use `ImmediateResponse` to return responses directly from ExtProc, bypassing Envoy's backend routing entirely. This correctly models our architecture where the ExtProc service IS the backend implementation.

## Improvements Adopted from AI Gateway

### 1. Three-Tiered Error Handling ✅

**ai-gateway pattern:**
- 400 Bad Request - Malformed JSON
- 422 Unprocessable Entity - Semantic validation errors
- 500 Internal Server Error - Backend/processing failures

**Implementation:**
```go
// translator.go
func CreateBadRequestError(message string) *ProcessingResponse
func CreateUnprocessableEntityError(message string) *ProcessingResponse
func CreateInternalError(message string) *ProcessingResponse

// Error response format matches OpenAI spec:
{
  "error": {
    "message": "model field is required",
    "type": "invalid_request_error",
    "code": 422
  }
}
```

### 2. Structured Logging with slog ✅

**ai-gateway pattern:**
- Uses `log/slog` for structured logging
- JSON output format
- Context-aware logging with request IDs
- Different log levels for different severity

**Implementation:**
```go
// cmd/envoy-extproc/main.go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: level,
}))

// processor.go
p.logger.Info("processing request",
    "request_id", requestID,
    "model", respReq.Model,
    "streaming", respReq.Stream,
)
```

### 3. Content Decompression ✅

**ai-gateway pattern:**
- Automatic gzip/brotli decompression
- Handles content-encoding header
- Removes encoding header after decompression

**Implementation:**
```go
// translator.go
func DecodeContent(body []byte, contentEncoding string) ([]byte, error) {
    switch contentEncoding {
    case "gzip":
        // Decompress gzip
    case "br":
        // Decompress brotli
    case "", "identity":
        // No decompression needed
    default:
        return error
    }
}
```

### 4. Proper Header Management ✅

**ai-gateway pattern:**
- Separate headers for streaming vs non-streaming
- Content-Type header management
- Additional headers for SSE streams

**Implementation:**
```go
// translator.go - CreateSuccessResponse
if isStreaming {
    headers = append(headers,
        &HeaderValueOption{Header: &HeaderValue{
            Key: "content-type", Value: "text/event-stream",
        }},
        &HeaderValueOption{Header: &HeaderValue{
            Key: "cache-control", Value: "no-cache",
        }},
        &HeaderValueOption{Header: &HeaderValue{
            Key: "connection", Value: "keep-alive",
        }},
    )
}
```

## Architectural Differences

### Our Approach vs AI Gateway

**AI Gateway:**
- Dual-processor architecture (router + upstream)
- Translates between different LLM provider formats
- Uses request/response body mutations
- Forwards requests to actual backend clusters

**Our Implementation:**
- Single-processor architecture
- Implements the OpenAI Responses API
- Uses ImmediateResponse pattern
- ExtProc service IS the backend

**Why the difference?**
- AI Gateway acts as a translation layer between formats
- Our gateway implements session management and conversation state
- We need to call the backend ourselves to manage state
- ImmediateResponse is the correct pattern for our use case

## Features Not Adopted (Yet)

### 1. Factory Pattern with Generics ⏭️

**ai-gateway pattern:**
```go
type ProcessorFactory[T EndpointSpec] interface {
    NewProcessor(config *RuntimeConfig, spec T) Processor
}
```

**Reason to defer:** Single endpoint type for now (Responses API). Will be useful when we add chat/completions, embeddings, etc.

### 2. Dynamic Metadata ⏭️

**ai-gateway pattern:**
- Tracks token usage in Envoy metadata
- Cost calculation via CEL expressions
- OpenTelemetry integration

**Reason to defer:** Phase 2 observability feature. Current logging is sufficient for initial deployment.

### 3. Request Header Attributes ⏭️

**ai-gateway pattern:**
- Maps request headers to OpenTelemetry attributes
- Supports attribute extraction for metrics/tracing
- Configurable via YAML

**Reason to defer:** Requires full observability stack. Will add with Prometheus/OTEL integration.

### 4. Configuration Hot-Reload ⏭️

**ai-gateway pattern:**
- File watcher for config changes
- Runtime config compilation
- No downtime config updates

**Reason to defer:** Kubernetes deployments use rolling updates. Hot-reload adds complexity without major benefit initially.

### 5. Header/Body Mutation Framework ⏭️

**ai-gateway pattern:**
```go
type HeaderMutator struct {
    originalHeaders map[string]string
    headerMutations *HTTPHeaderMutation
}

type BodyMutator struct {
    originalBody []byte
    bodyMutations *HTTPBodyMutation
}
```

**Reason to defer:** Our gateway doesn't need arbitrary mutations since we control the full request/response lifecycle. May add for advanced customization scenarios.

## Configuration Changes

### Envoy Configuration

**Before:**
```yaml
processing_mode:
  request_body_mode: BUFFERED
  response_body_mode: BUFFERED  # Wrong - we don't process responses

clusters:
- name: backend_cluster
  # Pointed to Ollama - but never reached
```

**After:**
```yaml
processing_mode:
  request_header_mode: SKIP
  request_body_mode: BUFFERED
  response_body_mode: BUFFERED_PARTIAL  # Minimal since we use ImmediateResponse
  response_header_mode: SKIP

clusters:
- name: extproc_passthrough
  # Placeholder - never reached due to ImmediateResponse
```

### Command-Line Flags

**Added:**
- `-log-level` - Configure logging verbosity (debug, info, warn, error)

**Existing:**
- `-config` - Path to configuration file
- `-port` - gRPC port for ExtProc

## Testing Improvements

### New Test Cases

1. **Error Handling - Missing Required Fields**
   - Tests 422 Unprocessable Entity response
   - Validates error response format

2. **Error Handling - Malformed JSON**
   - Tests 400 Bad Request response
   - Validates error type

3. **Success Response Validation**
   - Already existed, now enhanced with more checks

### Test Script Enhancements

```bash
# examples/envoy/test-via-envoy.sh
- Added error handling test cases
- Validates error response structure
- Checks error types match expected values
```

## Performance Considerations

### ImmediateResponse Benefits

1. **Reduced Latency:** No backend round-trip through Envoy
2. **Simplified Flow:** Single-phase processing
3. **Better Error Handling:** Can return errors immediately
4. **Resource Efficiency:** No buffer accumulation for responses

### Trade-offs

1. **No Backend Fallback:** Can't fall back to direct backend if ExtProc fails
2. **ExtProc is Critical Path:** Must be highly available
3. **Memory Usage:** Full request buffered in ExtProc

**Mitigation:**
- Health checks ensure ExtProc availability
- Kubernetes handles restarts and scaling
- `failure_mode_allow: false` provides clear failures vs silent issues

## Dependencies Added

```
github.com/andybalholm/brotli  - Brotli compression support
log/slog                        - Structured logging (stdlib)
```

## Code Statistics

**Before:**
- translator.go: ~100 lines (basic translation)
- processor.go: ~80 lines (incorrect architecture)
- Total: ~180 lines

**After:**
- translator.go: ~238 lines (comprehensive error handling, decompression)
- processor.go: ~153 lines (structured logging, proper phases)
- Total: ~391 lines

**Quality Improvements:**
- Proper error types and status codes
- Structured logging throughout
- Better documentation
- Correct ExtProc protocol usage

## Migration Guide

### For Existing Deployments

1. **Update Envoy Configuration:**
   - Change processing mode to skip response headers
   - Update cluster configuration (backend no longer used)

2. **Update ExtProc Command:**
   - Add `-log-level` flag for production deployments
   - Recommended: `-log-level info` for production

3. **Update Monitoring:**
   - Check ExtProc health via gRPC health protocol
   - Monitor ExtProc-specific Envoy stats
   - Parse JSON logs for alerting

4. **Test Error Handling:**
   - Verify 400/422/500 responses work correctly
   - Ensure error format matches OpenAI spec
   - Test with malformed requests

## References

- AI Gateway ExtProc Implementation: `/Users/leseb/go/src/github.com/envoyproxy/ai-gateway/internal/extproc/`
- Envoy ExtProc Protocol: https://www.envoyproxy.io/docs/envoy/latest/api-v3/service/ext_proc/v3/external_processor.proto
- OpenAI Error Format: https://platform.openai.com/docs/guides/error-codes
