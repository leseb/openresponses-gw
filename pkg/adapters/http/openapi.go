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
// 100% Open Responses Specification Compliant Gateway
func getOpenAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":       "OpenAI Responses Gateway API",
			"description": "100% Open Responses Specification Compliant Gateway\n\nBased on: https://github.com/openresponses/openresponses\n\nThis gateway provides:\n- **Core API**: Full Open Responses spec compliance (POST /v1/responses)\n- **Extended APIs**: Conversations, Prompts, Files, Vector Stores, Models\n- **Dual Mode**: Standalone HTTP server or Envoy ExtProc integration\n\nStreaming: All 24 event types from Open Responses spec\nRequest Echo: All request parameters returned in response\nMultimodal: Support for text, images, files, video",
			"version": "1.0.0",
			"contact": map[string]string{
				"name": "OpenAI Responses Gateway",
				"url":  "https://github.com/leseb/openai-responses-gateway",
			},
		},
		"servers": []map[string]interface{}{
			{
				"url":         "http://localhost:8080",
				"description": "Local development server",
			},
		},
		"tags": []map[string]interface{}{
			{"name": "Health", "description": "Health check and API documentation"},
			{"name": "Responses", "description": "Open Responses API (100% spec compliant)"},
			{"name": "Conversations", "description": "Extended - Conversation state management"},
			{"name": "Prompts", "description": "Extended - Prompt template management"},
			{"name": "Files", "description": "Extended - File upload and management"},
			{"name": "Vector Stores", "description": "Extended - Vector store and embeddings"},
			{"name": "Models", "description": "Extended - Model discovery"},
		},
		"paths": map[string]interface{}{
			"/health": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Health"},
					"summary":     "Health check",
					"operationId": "getHealth",
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
			"/openapi.json": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Health"},
					"summary":     "Get OpenAPI specification (JSON)",
					"operationId": "getOpenAPISpec",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "OpenAPI specification",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
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
					"description": "**Open Responses API - 100% Spec Compliant**\n\nCreate a response with streaming or non-streaming output.\nAll request parameters are echoed back in the response.\n\nStreaming: Set `stream: true` (HTTP-specific, not in spec)\nReturns 24 granular event types via Server-Sent Events\n\nSupports:\n- Multi-turn conversations (previous_response_id)\n- Tool/function calling (tools, tool_choice)\n- Reasoning models (reasoning config for o1/o3)\n- Multimodal input (text, images, files, video)",
					"operationId": "createResponse",
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
			"/v1/conversations": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"Conversations"},
					"summary":     "Create conversation",
					"operationId": "createConversation",
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
						},
					},
				},
				"get": map[string]interface{}{
					"tags":        []string{"Conversations"},
					"summary":     "List conversations",
					"operationId": "listConversations",
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
					"operationId": "createPrompt",
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
						},
					},
				},
				"get": map[string]interface{}{
					"tags":        []string{"Prompts"},
					"summary":     "List prompts",
					"operationId": "listPrompts",
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
					"operationId": "uploadFile",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"multipart/form-data": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"required": []string{"file", "purpose"},
									"properties": map[string]interface{}{
										"file": map[string]interface{}{
											"type":   "string",
											"format": "binary",
										},
										"purpose": map[string]interface{}{
											"type": "string",
											"enum": []string{"assistants", "vision", "batch", "fine-tune"},
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "File uploaded",
						},
					},
				},
				"get": map[string]interface{}{
					"tags":        []string{"Files"},
					"summary":     "List files",
					"operationId": "listFiles",
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
					"operationId": "createVectorStore",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Vector store created",
						},
					},
				},
				"get": map[string]interface{}{
					"tags":        []string{"Vector Stores"},
					"summary":     "List vector stores",
					"operationId": "listVectorStores",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "List of vector stores",
						},
					},
				},
			},
			"/v1/models": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"Models"},
					"summary":     "List available models",
					"operationId": "listModels",
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
		},
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{
				"ResponseRequest": map[string]interface{}{
					"type":        "object",
					"description": "Request to create a response (Open Responses spec compliant)",
					"properties": map[string]interface{}{
						"model": map[string]interface{}{
							"type":        "string",
							"nullable":    true,
							"description": "Model ID (e.g., gpt-4, claude-3-opus)",
							"example":     "gpt-4",
						},
						"input": map[string]interface{}{
							"description": "String or array of input items",
							"oneOf": []map[string]interface{}{
								{"type": "string"},
								{"type": "array"},
							},
						},
						"previous_response_id": map[string]interface{}{
							"type":        "string",
							"nullable":    true,
							"description": "ID of previous response for multi-turn",
						},
						"tools": map[string]interface{}{
							"type":        "array",
							"description": "Tools available to the model",
						},
						"tool_choice": map[string]interface{}{
							"description": "Control which tool to use",
						},
						"instructions": map[string]interface{}{
							"type":        "string",
							"nullable":    true,
							"description": "System message / instructions",
						},
						"temperature": map[string]interface{}{
							"type":     "number",
							"format":   "float",
							"nullable": true,
							"minimum":  0,
							"maximum":  2,
						},
						"max_output_tokens": map[string]interface{}{
							"type":     "integer",
							"nullable": true,
						},
						"stream": map[string]interface{}{
							"type":        "boolean",
							"description": "HTTP-specific - enable SSE streaming",
							"default":     false,
						},
					},
				},
				"Response": map[string]interface{}{
					"type":        "object",
					"description": "Response object (Open Responses spec compliant)",
					"required":    []string{"id", "object", "created_at", "model", "status"},
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":    "string",
							"example": "resp_abc123",
						},
						"object": map[string]interface{}{
							"type":    "string",
							"example": "response",
						},
						"created_at": map[string]interface{}{
							"type":        "integer",
							"format":      "int64",
							"description": "Unix timestamp",
						},
						"completed_at": map[string]interface{}{
							"type":     "integer",
							"format":   "int64",
							"nullable": true,
						},
						"model": map[string]interface{}{
							"type":    "string",
							"example": "gpt-4",
						},
						"status": map[string]interface{}{
							"type": "string",
							"enum": []string{"queued", "in_progress", "completed", "failed", "incomplete"},
						},
						"output": map[string]interface{}{
							"type": "array",
						},
						"usage": map[string]interface{}{
							"type":        "object",
							"description": "Token usage statistics",
						},
						"text": map[string]interface{}{
							"type":        "string",
							"nullable":    true,
							"description": "Convenience field - concatenated output text",
						},
					},
				},
				"ListModelsResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"object": map[string]string{"type": "string", "example": "list"},
						"data": map[string]interface{}{
							"type": "array",
						},
					},
				},
				"CreateConversationRequest": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"metadata": map[string]string{"type": "object"},
					},
				},
				"CreatePromptRequest": map[string]interface{}{
					"type":     "object",
					"required": []string{"name", "template"},
					"properties": map[string]interface{}{
						"name":        map[string]string{"type": "string"},
						"description": map[string]string{"type": "string"},
						"template":    map[string]string{"type": "string", "example": "Hello {{name}}!"},
						"metadata":    map[string]string{"type": "object"},
					},
				},
			},
		},
	}
}
