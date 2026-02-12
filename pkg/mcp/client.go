// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
)

// Client is a stateless MCP client that communicates over HTTP using JSON-RPC 2.0.
type Client struct {
	httpClient *http.Client
	serverURL  string
	sessionID  string
	nextID     atomic.Int64
}

// NewClient creates a new MCP client targeting the given server URL.
func NewClient(serverURL string) *Client {
	return &Client{
		httpClient: &http.Client{},
		serverURL:  serverURL,
	}
}

// Initialize performs the MCP initialize handshake and stores the session ID.
func (c *Client) Initialize(ctx context.Context) error {
	params := InitializeParams{
		ProtocolVersion: "2025-03-26",
		ClientInfo: ClientInfo{
			Name:    "openresponses-gw",
			Version: "0.1.0",
		},
		Capabilities: map[string]any{},
	}

	raw, headers, err := c.callWithHeaders(ctx, "initialize", params)
	if err != nil {
		return fmt.Errorf("mcp initialize: %w", err)
	}

	// Store session ID from response header
	if sid := headers.Get("Mcp-Session-Id"); sid != "" {
		c.sessionID = sid
	}

	var result InitializeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("mcp initialize: unmarshal result: %w", err)
	}

	// Send initialized notification (no response expected, but required by spec)
	_ = c.notify(ctx, "notifications/initialized")

	return nil
}

// ListTools returns the tools exposed by the MCP server.
func (c *Client) ListTools(ctx context.Context) ([]ToolInfo, error) {
	raw, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("mcp tools/list: %w", err)
	}

	var result ToolsListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("mcp tools/list: unmarshal result: %w", err)
	}
	return result.Tools, nil
}

// CallTool invokes a tool on the MCP server.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (*ToolCallResult, error) {
	params := ToolCallParams{
		Name:      name,
		Arguments: args,
	}

	raw, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("mcp tools/call %s: %w", name, err)
	}

	var result ToolCallResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("mcp tools/call %s: unmarshal result: %w", name, err)
	}
	return &result, nil
}

// call sends a JSON-RPC request and returns the result.
func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	raw, _, err := c.callWithHeaders(ctx, method, params)
	return raw, err
}

// callWithHeaders sends a JSON-RPC request and returns the result along with response headers.
func (c *Client) callWithHeaders(ctx context.Context, method string, params any) (json.RawMessage, http.Header, error) {
	id := int(c.nextID.Add(1))
	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		ID:      id,
		Params:  params,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL, bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	if c.sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", c.sessionID)
	}

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("http request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, nil, fmt.Errorf("http status %d: %s", httpResp.StatusCode, string(respBody))
	}

	// Parse response — may be plain JSON or SSE (text/event-stream)
	ct := httpResp.Header.Get("Content-Type")
	var respBody []byte
	if strings.HasPrefix(ct, "text/event-stream") {
		respBody, err = extractSSEData(httpResp.Body)
		if err != nil {
			return nil, nil, fmt.Errorf("parse SSE response: %w", err)
		}
	} else {
		respBody, err = io.ReadAll(httpResp.Body)
		if err != nil {
			return nil, nil, fmt.Errorf("read response: %w", err)
		}
	}

	var rpcResp JSONRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, nil, fmt.Errorf("unmarshal response: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, nil, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, httpResp.Header, nil
}

// extractSSEData reads an SSE stream and returns the data from the first
// "message" event. The MCP streamable-http transport wraps JSON-RPC
// responses in SSE format: "event: message\ndata: {json}\n\n".
func extractSSEData(r io.Reader) ([]byte, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			return []byte(strings.TrimPrefix(line, "data: ")), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("no data line found in SSE stream")
}

// notify sends a JSON-RPC notification (no id, no response expected).
func (c *Client) notify(ctx context.Context, method string) error {
	// Notifications have no id field per JSON-RPC 2.0 spec
	type notification struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
	}
	body, err := json.Marshal(notification{JSONRPC: "2.0", Method: method})
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	if c.sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", c.sessionID)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)
	return nil
}
