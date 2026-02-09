// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package http

import (
	"encoding/json"
	"net/http"
)

// handleOpenAPI serves the OpenAPI specification
func (h *Handler) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	spec := getOpenAPISpec()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(spec)
}

// getOpenAPISpec returns the OpenAPI 3.0 specification
func getOpenAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":       "OpenAI Responses Gateway API",
			"description": "A complete implementation of OpenAI-compatible APIs including Responses, Chat Completions, Conversations, Prompts, Files, Vector Stores, and Models.",
			"version":     "1.0.0",
			"contact": map[string]string{
				"name": "OpenAI Responses Gateway",
			},
		},
		"servers": []map[string]interface{}{
			{
				"url":         "http://localhost:8080",
				"description": "Local development server",
			},
		},
		"tags": []map[string]interface{}{
			{"name": "Health", "description": "Health check endpoints"},
			{"name": "Models", "description": "Model discovery and information"},
			{"name": "Chat Completions", "description": "Direct chat completion inference"},
			{"name": "Responses", "description": "Responses API for managing inference responses"},
			{"name": "Conversations", "description": "Conversation state management"},
			{"name": "Prompts", "description": "Prompt template management"},
			{"name": "Files", "description": "File upload and management"},
			{"name": "Vector Stores", "description": "Vector store and embeddings management"},
		},
		"paths": map[string]interface{}{
			"/health": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Health"},
					"summary":     "Health check",
					"description": "Check if the service is healthy",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Service is healthy",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"status": map[string]string{"type": "string", "example": "healthy"},
										},
									},
								},
							},
						},
					},
				},
			},
			"/v1/models": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Models"},
					"summary":     "List available models",
					"description": "List all available models from the LLM backend",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of models",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/ListModelsResponse",
									},
								},
							},
						},
					},
				},
			},
			"/v1/models/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Models"},
					"summary":     "Get model details",
					"description": "Retrieve information about a specific model",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "Model ID",
							"schema":      map[string]string{"type": "string"},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Model details",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/Model",
									},
								},
							},
						},
						"404": map[string]interface{}{
							"description": "Model not found",
						},
					},
				},
			},
			"/v1/chat/completions": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Chat Completions"},
					"summary":     "Create chat completion",
					"description": "Create a chat completion with streaming or non-streaming response",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/ChatCompletionRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Chat completion response",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/ChatCompletionResponse",
									},
								},
								"text/event-stream": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "string",
									},
								},
							},
						},
					},
				},
			},
			"/v1/responses": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Responses"},
					"summary":     "Create response",
					"description": "Create a new response with streaming or non-streaming inference",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/ResponseRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Response created",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/Response",
									},
								},
							},
						},
					},
				},
				"get": map[string]interface{}{
					"tags":        []string{"Responses"},
					"summary":     "List responses",
					"description": "List responses with cursor-based pagination",
					"parameters": []map[string]interface{}{
						{
							"name":        "after",
							"in":          "query",
							"description": "Cursor for pagination (ID of last item from previous page)",
							"schema":      map[string]string{"type": "string"},
						},
						{
							"name":        "limit",
							"in":          "query",
							"description": "Number of items to return (1-100, default 50)",
							"schema":      map[string]interface{}{"type": "integer", "default": 50},
						},
						{
							"name":        "order",
							"in":          "query",
							"description": "Sort order",
							"schema":      map[string]interface{}{"type": "string", "enum": []string{"asc", "desc"}, "default": "desc"},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of responses",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/ListResponsesResponse",
									},
								},
							},
						},
					},
				},
			},
			"/v1/conversations": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Conversations"},
					"summary":     "Create conversation",
					"description": "Create a new conversation",
					"requestBody": map[string]interface{}{
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/CreateConversationRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Conversation created",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/Conversation",
									},
								},
							},
						},
					},
				},
				"get": map[string]interface{}{
					"tags":        []string{"Conversations"},
					"summary":     "List conversations",
					"description": "List conversations with pagination",
					"parameters": []map[string]interface{}{
						{
							"name":        "limit",
							"in":          "query",
							"description": "Number of items (1-100, default 50)",
							"schema":      map[string]interface{}{"type": "integer", "default": 50},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of conversations",
						},
					},
				},
			},
			"/v1/prompts": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Prompts"},
					"summary":     "Create prompt template",
					"description": "Create a new prompt template with variable support",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/CreatePromptRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Prompt created",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/Prompt",
									},
								},
							},
						},
					},
				},
				"get": map[string]interface{}{
					"tags":        []string{"Prompts"},
					"summary":     "List prompts",
					"description": "List prompt templates with pagination",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of prompts",
						},
					},
				},
			},
			"/v1/files": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Files"},
					"summary":     "Upload file",
					"description": "Upload a file for use with assistants, vision, batch, or fine-tuning",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"multipart/form-data": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"file": map[string]interface{}{
											"type":        "string",
											"format":      "binary",
											"description": "File to upload (max 512MB)",
										},
										"purpose": map[string]interface{}{
											"type":        "string",
											"description": "Purpose of the file",
											"enum":        []string{"assistants", "vision", "batch", "fine-tune"},
										},
									},
									"required": []string{"file", "purpose"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "File uploaded",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/File",
									},
								},
							},
						},
					},
				},
				"get": map[string]interface{}{
					"tags":        []string{"Files"},
					"summary":     "List files",
					"description": "List uploaded files with pagination",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of files",
						},
					},
				},
			},
			"/v1/vector_stores": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Vector Stores"},
					"summary":     "Create vector store",
					"description": "Create a new vector store for embeddings",
					"requestBody": map[string]interface{}{
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/CreateVectorStoreRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Vector store created",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/VectorStore",
									},
								},
							},
						},
					},
				},
				"get": map[string]interface{}{
					"tags":        []string{"Vector Stores"},
					"summary":     "List vector stores",
					"description": "List vector stores with pagination",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of vector stores",
						},
					},
				},
			},
		},
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{
				"Model": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id":       map[string]string{"type": "string", "example": "gpt-4"},
						"object":   map[string]string{"type": "string", "example": "model"},
						"created":  map[string]string{"type": "integer", "format": "int64"},
						"owned_by": map[string]string{"type": "string", "example": "openai"},
					},
				},
				"ListModelsResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"object": map[string]string{"type": "string", "example": "list"},
						"data": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"$ref": "#/components/schemas/Model",
							},
						},
					},
				},
				"ChatCompletionRequest": map[string]interface{}{
					"type": "object",
					"required": []string{"model", "messages"},
					"properties": map[string]interface{}{
						"model":       map[string]string{"type": "string", "example": "gpt-4"},
						"messages":    map[string]interface{}{"type": "array"},
						"temperature": map[string]interface{}{"type": "number", "format": "float"},
						"stream":      map[string]interface{}{"type": "boolean", "default": false},
					},
				},
				"ChatCompletionResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id":      map[string]string{"type": "string"},
						"object":  map[string]string{"type": "string", "example": "chat.completion"},
						"created": map[string]string{"type": "integer", "format": "int64"},
						"model":   map[string]string{"type": "string"},
						"choices": map[string]interface{}{"type": "array"},
					},
				},
				"ResponseRequest": map[string]interface{}{
					"type": "object",
					"required": []string{"model", "input"},
					"properties": map[string]interface{}{
						"model":  map[string]string{"type": "string"},
						"input":  map[string]interface{}{"type": "array"},
						"stream": map[string]interface{}{"type": "boolean", "default": false},
					},
				},
				"Response": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id":     map[string]string{"type": "string"},
						"object": map[string]string{"type": "string", "example": "response"},
						"status": map[string]string{"type": "string"},
						"output": map[string]interface{}{"type": "array"},
					},
				},
				"ListResponsesResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"object":   map[string]string{"type": "string", "example": "list"},
						"data":     map[string]interface{}{"type": "array"},
						"has_more": map[string]string{"type": "boolean"},
					},
				},
				"CreateConversationRequest": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"metadata": map[string]string{"type": "object"},
					},
				},
				"Conversation": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id":         map[string]string{"type": "string"},
						"object":     map[string]string{"type": "string", "example": "conversation"},
						"created_at": map[string]string{"type": "integer", "format": "int64"},
						"metadata":   map[string]string{"type": "object"},
					},
				},
				"CreatePromptRequest": map[string]interface{}{
					"type": "object",
					"required": []string{"name", "template"},
					"properties": map[string]interface{}{
						"name":        map[string]string{"type": "string"},
						"description": map[string]string{"type": "string"},
						"template":    map[string]string{"type": "string", "example": "Hello {{name}}!"},
						"metadata":    map[string]string{"type": "object"},
					},
				},
				"Prompt": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id":          map[string]string{"type": "string"},
						"object":      map[string]string{"type": "string", "example": "prompt"},
						"name":        map[string]string{"type": "string"},
						"template":    map[string]string{"type": "string"},
						"variables":   map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}},
						"created_at":  map[string]string{"type": "integer", "format": "int64"},
					},
				},
				"File": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id":         map[string]string{"type": "string"},
						"object":     map[string]string{"type": "string", "example": "file"},
						"bytes":      map[string]string{"type": "integer", "format": "int64"},
						"created_at": map[string]string{"type": "integer", "format": "int64"},
						"filename":   map[string]string{"type": "string"},
						"purpose":    map[string]string{"type": "string"},
					},
				},
				"CreateVectorStoreRequest": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name":     map[string]string{"type": "string"},
						"file_ids": map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}},
						"metadata": map[string]string{"type": "object"},
					},
				},
				"VectorStore": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id":          map[string]string{"type": "string"},
						"object":      map[string]string{"type": "string", "example": "vector_store"},
						"name":        map[string]string{"type": "string"},
						"status":      map[string]string{"type": "string"},
						"created_at":  map[string]string{"type": "integer", "format": "int64"},
					},
				},
			},
		},
	}
}
