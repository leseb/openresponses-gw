// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

func init() {
	Providers.Register("brave", func(_ context.Context, params map[string]string) (Provider, error) {
		apiKey := params["api_key"]
		if apiKey == "" {
			return nil, fmt.Errorf("brave: api_key parameter is required")
		}
		return NewBraveProvider(apiKey), nil
	})
}

// BraveProvider performs web searches using the Brave Search API.
type BraveProvider struct {
	apiKey     string
	httpClient *http.Client
}

// NewBraveProvider creates a new Brave Search provider.
func NewBraveProvider(apiKey string) *BraveProvider {
	return &BraveProvider{
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

// Search queries the Brave Web Search API and returns results.
func (b *BraveProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	u, _ := url.Parse("https://api.search.brave.com/res/v1/web/search")
	q := u.Query()
	q.Set("q", query)
	q.Set("count", strconv.Itoa(maxResults))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", b.apiKey)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave search request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brave search returned status %d: %s", resp.StatusCode, string(body))
	}

	var result braveSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	var results []SearchResult
	for _, r := range result.Web.Results {
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Description,
		})
	}

	return results, nil
}

type braveSearchResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}
