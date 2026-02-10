// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"fmt"
	"time"

	"github.com/leseb/openresponses-gw/pkg/core/api"
	"github.com/leseb/openresponses-gw/pkg/core/schema"
)

// ModelsService handles model listing and information
type ModelsService struct {
	client api.ChatCompletionClient
}

// NewModelsService creates a new models service
func NewModelsService(client api.ChatCompletionClient) *ModelsService {
	return &ModelsService{
		client: client,
	}
}

// ListModels returns available models
func (s *ModelsService) ListModels(ctx context.Context) (*schema.ListModelsResponse, error) {
	// Try to get models from backend if it's an OpenAI client
	if openaiClient, ok := s.client.(*api.OpenAIClient); ok {
		return s.listFromOpenAI(ctx, openaiClient)
	}

	// Fall back to static model list for other backends
	return s.staticModelList(), nil
}

// GetModel returns information about a specific model
func (s *ModelsService) GetModel(ctx context.Context, modelID string) (*schema.Model, error) {
	// Get all models
	models, err := s.ListModels(ctx)
	if err != nil {
		return nil, err
	}

	// Find the requested model
	for _, model := range models.Data {
		if model.ID == modelID {
			return &model, nil
		}
	}

	return nil, fmt.Errorf("model not found: %s", modelID)
}

// listFromOpenAI gets models from OpenAI API
func (s *ModelsService) listFromOpenAI(ctx context.Context, client *api.OpenAIClient) (*schema.ListModelsResponse, error) {
	// Access the underlying OpenAI client
	// Note: This requires exposing the client field or using reflection
	// For now, fall back to static list
	return s.staticModelList(), nil
}

// staticModelList returns a static list of common models
func (s *ModelsService) staticModelList() *schema.ListModelsResponse {
	now := time.Now().Unix()

	return &schema.ListModelsResponse{
		Object: "list",
		Data: []schema.Model{
			// OpenAI Models
			{
				ID:          "gpt-4",
				Object:      "model",
				Created:     now,
				OwnedBy:     "openai",
				Description: "GPT-4 - Most capable model, best for complex tasks",
			},
			{
				ID:          "gpt-4-turbo",
				Object:      "model",
				Created:     now,
				OwnedBy:     "openai",
				Description: "GPT-4 Turbo - Faster and more efficient GPT-4",
			},
			{
				ID:          "gpt-4o",
				Object:      "model",
				Created:     now,
				OwnedBy:     "openai",
				Description: "GPT-4o - Optimized for speed and cost",
			},
			{
				ID:          "gpt-3.5-turbo",
				Object:      "model",
				Created:     now,
				OwnedBy:     "openai",
				Description: "GPT-3.5 Turbo - Fast and cost-effective",
			},
			// Ollama Models (common local models)
			{
				ID:          "llama3.2:3b",
				Object:      "model",
				Created:     now,
				OwnedBy:     "meta",
				Description: "Llama 3.2 3B - Lightweight local model",
			},
			{
				ID:          "llama3.1:8b",
				Object:      "model",
				Created:     now,
				OwnedBy:     "meta",
				Description: "Llama 3.1 8B - Balanced local model",
			},
			{
				ID:          "llama3.1:70b",
				Object:      "model",
				Created:     now,
				OwnedBy:     "meta",
				Description: "Llama 3.1 70B - High-capability local model",
			},
			{
				ID:          "mistral:7b",
				Object:      "model",
				Created:     now,
				OwnedBy:     "mistralai",
				Description: "Mistral 7B - Efficient open-source model",
			},
			{
				ID:          "qwen2.5:7b",
				Object:      "model",
				Created:     now,
				OwnedBy:     "alibaba",
				Description: "Qwen 2.5 7B - Multilingual model",
			},
		},
	}
}
