// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package websearch

import (
	"context"

	"github.com/leseb/openresponses-gw/pkg/provider"
)

// Providers is the registry of web search provider implementations.
// Brave and Tavily are registered automatically via init().
var Providers = provider.NewRegistry[Provider]("web_search")

// SearchResult represents a single web search result.
type SearchResult struct {
	Title   string
	URL     string
	Snippet string
}

// Provider performs web searches against an external API.
type Provider interface {
	Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error)
}
