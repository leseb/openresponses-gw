// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// EmbeddingClient generates vector embeddings from text inputs.
type EmbeddingClient interface {
	Embed(ctx context.Context, inputs []string) ([][]float32, error)
}

// OpenAIEmbeddingClient implements EmbeddingClient using the OpenAI SDK.
type OpenAIEmbeddingClient struct {
	client     openai.Client
	model      string
	dimensions int
}

// NewOpenAIEmbeddingClient creates an embedding client with its own base URL and API key.
func NewOpenAIEmbeddingClient(baseURL, apiKey, model string, dimensions int) *OpenAIEmbeddingClient {
	opts := []option.RequestOption{}

	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	} else {
		opts = append(opts, option.WithAPIKey("dummy"))
	}

	return &OpenAIEmbeddingClient{
		client:     openai.NewClient(opts...),
		model:      model,
		dimensions: dimensions,
	}
}

// Embed generates embeddings for the given text inputs.
func (c *OpenAIEmbeddingClient) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}

	// Build the input union: for a single string use OfString, otherwise OfArrayOfStrings
	var input openai.EmbeddingNewParamsInputUnion
	if len(inputs) == 1 {
		input = openai.EmbeddingNewParamsInputUnion{
			OfString: openai.String(inputs[0]),
		}
	} else {
		input = openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: inputs,
		}
	}

	params := openai.EmbeddingNewParams{
		Model:      openai.EmbeddingModel(c.model),
		Input:      input,
		Dimensions: openai.Int(int64(c.dimensions)),
	}

	resp, err := c.client.Embeddings.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}

	results := make([][]float32, len(resp.Data))
	for i, d := range resp.Data {
		vec := make([]float32, len(d.Embedding))
		for j, v := range d.Embedding {
			vec[j] = float32(v)
		}
		results[i] = vec
	}

	return results, nil
}
