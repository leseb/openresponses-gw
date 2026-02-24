// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func init() {
	Providers.Register("tavily", func(_ context.Context, params map[string]string) (Provider, error) {
		apiKey := params["api_key"]
		if apiKey == "" {
			return nil, fmt.Errorf("tavily: api_key parameter is required")
		}
		return NewTavilyProvider(apiKey), nil
	})
}

// TavilyProvider performs web searches using the Tavily Search API.
type TavilyProvider struct {
	apiKey     string
	httpClient *http.Client
}

// NewTavilyProvider creates a new Tavily Search provider.
func NewTavilyProvider(apiKey string) *TavilyProvider {
	return &TavilyProvider{
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

// Search queries the Tavily Search API and returns results.
func (t *TavilyProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	reqBody := tavilySearchRequest{
		APIKey:     t.apiKey,
		Query:      query,
		MaxResults: maxResults,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily search request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tavily search returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result tavilySearchResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	var results []SearchResult
	for _, r := range result.Results {
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
		})
	}

	return results, nil
}

type tavilySearchRequest struct {
	APIKey     string `json:"api_key"`
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

type tavilySearchResponse struct {
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	} `json:"results"`
}
