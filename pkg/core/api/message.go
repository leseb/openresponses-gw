// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package api

// Message represents an internal chat message used for conversation history.
type Message struct {
	Role         string               `json:"role"`                    // "system", "user", "assistant", "tool", "developer"
	Content      string               `json:"content"`                 // Message text content
	ContentParts []MessageContentPart `json:"content_parts,omitempty"` // Multimodal content parts (takes precedence over Content when non-empty)
	ToolCalls    []ToolCall           `json:"tool_calls,omitempty"`    // Tool calls (assistant messages)
	ToolCallID   string               `json:"tool_call_id,omitempty"`  // Tool call ID (tool messages)
}

// MessageContentPart represents a content part in a multimodal message.
type MessageContentPart struct {
	Type     string           `json:"type"`                // "text", "image_url", "file"
	Text     string           `json:"text,omitempty"`      // Text content (when Type="text")
	ImageURL *MessageImageURL `json:"image_url,omitempty"` // Image URL (when Type="image_url")
	File     *MessageFile     `json:"file,omitempty"`      // File content (when Type="file")
}

// MessageImageURL represents an image URL in a content part.
type MessageImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // "auto", "low", "high"
}

// MessageFile represents a file in a content part.
type MessageFile struct {
	FileData string `json:"file_data,omitempty"`
	FileID   string `json:"file_id,omitempty"`
	Filename string `json:"filename,omitempty"`
}

// ToolCall represents a tool call made by the assistant.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"` // "function"
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction contains the function name and arguments for a tool call.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
