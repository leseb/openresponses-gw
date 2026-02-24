// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBraveProvider_Search(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Subscription-Token") != "test-key" {
			t.Errorf("expected API key header, got %q", r.Header.Get("X-Subscription-Token"))
		}
		if r.URL.Query().Get("q") != "golang testing" {
			t.Errorf("expected query 'golang testing', got %q", r.URL.Query().Get("q"))
		}

		resp := braveSearchResponse{}
		resp.Web.Results = []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		}{
			{Title: "Go Testing", URL: "https://golang.org/testing", Description: "Testing in Go"},
			{Title: "Go Docs", URL: "https://golang.org/doc", Description: "Go documentation"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := &BraveProvider{
		apiKey:     "test-key",
		httpClient: server.Client(),
	}
	// Override the URL by using a custom transport
	origURL := "https://api.search.brave.com/res/v1/web/search"
	_ = origURL

	// For unit test, we test the provider with a mock server
	// We need to construct the provider differently for testing
	// Instead, test the full flow with a mock transport
	transport := &rewriteTransport{
		base:      server.Client().Transport,
		targetURL: server.URL,
	}
	provider.httpClient = &http.Client{Transport: transport}

	results, err := provider.Search(context.Background(), "golang testing", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Title != "Go Testing" {
		t.Errorf("expected title 'Go Testing', got %q", results[0].Title)
	}
	if results[0].URL != "https://golang.org/testing" {
		t.Errorf("expected URL 'https://golang.org/testing', got %q", results[0].URL)
	}
}

func TestTavilyProvider_Search(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var req tavilySearchRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.APIKey != "test-key" {
			t.Errorf("expected api_key 'test-key', got %q", req.APIKey)
		}
		if req.Query != "AI news" {
			t.Errorf("expected query 'AI news', got %q", req.Query)
		}

		resp := tavilySearchResponse{
			Results: []struct {
				Title   string `json:"title"`
				URL     string `json:"url"`
				Content string `json:"content"`
			}{
				{Title: "AI News", URL: "https://example.com/ai", Content: "Latest AI developments"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := &TavilyProvider{
		apiKey:     "test-key",
		httpClient: &http.Client{Transport: &rewriteTransport{targetURL: server.URL}},
	}

	results, err := provider.Search(context.Background(), "AI news", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Snippet != "Latest AI developments" {
		t.Errorf("expected snippet 'Latest AI developments', got %q", results[0].Snippet)
	}
}

// rewriteTransport rewrites requests to point at a test server.
type rewriteTransport struct {
	base      http.RoundTripper
	targetURL string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = "http"
	req.URL.Host = t.targetURL[len("http://"):]
	transport := t.base
	if transport == nil {
		transport = http.DefaultTransport
	}
	return transport.RoundTrip(req)
}
