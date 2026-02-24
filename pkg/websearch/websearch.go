// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package websearch

import "context"

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
