// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OpenAIResponsesClient implements ResponsesAPIClient using net/http.
// It calls a backend's /v1/responses endpoint directly.
type OpenAIResponsesClient struct {
	baseURL    string // e.g. "http://localhost:8000/v1"
	apiKey     string
	httpClient *http.Client
}

// NewOpenAIResponsesClient creates a new Responses API client.
// baseURL should include the /v1 prefix (e.g. "http://localhost:8000/v1").
func NewOpenAIResponsesClient(baseURL, apiKey string) *OpenAIResponsesClient {
	return &OpenAIResponsesClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

// CreateResponse sends a non-streaming request to the backend.
func (c *OpenAIResponsesClient) CreateResponse(ctx context.Context, req *ResponsesAPIRequest) (*ResponsesAPIResponse, error) {
	req.Stream = false

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request to backend failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("backend returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result ResponsesAPIResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// CreateResponseStream sends a streaming request to the backend and returns
// a channel of SSE events. The channel is closed when the stream ends.
func (c *OpenAIResponsesClient) CreateResponseStream(ctx context.Context, req *ResponsesAPIRequest) (<-chan ResponsesStreamEvent, error) {
	req.Stream = true
	// Gateway owns storage, not the backend
	storeFalse := false
	req.Store = &storeFalse

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	c.setHeaders(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request to backend failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("backend returned status %d: %s", resp.StatusCode, string(respBody))
	}

	events := make(chan ResponsesStreamEvent, 10)

	go func() {
		defer close(events)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		// Increase max token size for large SSE payloads
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		var eventType string

		for scanner.Scan() {
			line := scanner.Text()

			// Empty line signals end of an event
			if line == "" {
				eventType = ""
				continue
			}

			if strings.HasPrefix(line, "event: ") {
				eventType = strings.TrimPrefix(line, "event: ")
				continue
			}

			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")

				// [DONE] signals end of stream
				if data == "[DONE]" {
					return
				}

				evt := ResponsesStreamEvent{
					Type: eventType,
					Data: json.RawMessage(data),
				}

				select {
				case events <- evt:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return events, nil
}

func (c *OpenAIResponsesClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}
