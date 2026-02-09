// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Prompt represents a stored prompt template
type Prompt struct {
	ID          string
	Name        string
	Description string
	Template    string
	Variables   []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Metadata    map[string]string
}

// PromptsStore is an in-memory prompts store
type PromptsStore struct {
	mu      sync.RWMutex
	prompts map[string]*Prompt
}

// NewPromptsStore creates a new prompts store
func NewPromptsStore() *PromptsStore {
	return &PromptsStore{
		prompts: make(map[string]*Prompt),
	}
}

// CreatePrompt creates a new prompt
func (s *PromptsStore) CreatePrompt(ctx context.Context, prompt *Prompt) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.prompts[prompt.ID]; exists {
		return fmt.Errorf("prompt %s already exists", prompt.ID)
	}

	// Extract variables from template
	prompt.Variables = extractVariables(prompt.Template)

	s.prompts[prompt.ID] = prompt
	return nil
}

// GetPrompt retrieves a prompt by ID
func (s *PromptsStore) GetPrompt(ctx context.Context, promptID string) (*Prompt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prompt, exists := s.prompts[promptID]
	if !exists {
		return nil, fmt.Errorf("prompt %s not found", promptID)
	}

	return prompt, nil
}

// UpdatePrompt updates an existing prompt
func (s *PromptsStore) UpdatePrompt(ctx context.Context, prompt *Prompt) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.prompts[prompt.ID]; !exists {
		return fmt.Errorf("prompt %s not found", prompt.ID)
	}

	// Re-extract variables if template changed
	prompt.Variables = extractVariables(prompt.Template)
	prompt.UpdatedAt = time.Now()

	s.prompts[prompt.ID] = prompt
	return nil
}

// DeletePrompt deletes a prompt
func (s *PromptsStore) DeletePrompt(ctx context.Context, promptID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.prompts[promptID]; !exists {
		return fmt.Errorf("prompt %s not found", promptID)
	}

	delete(s.prompts, promptID)
	return nil
}

// ListPromptsPaginated lists prompts with pagination
func (s *PromptsStore) ListPromptsPaginated(ctx context.Context, after, before string, limit int, order string) ([]*Prompt, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Default limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// Collect all prompts
	var allPrompts []*Prompt
	for _, prompt := range s.prompts {
		allPrompts = append(allPrompts, prompt)
	}

	// Apply cursor-based pagination
	var filtered []*Prompt
	foundAfter := after == ""

	for _, prompt := range allPrompts {
		// Handle after cursor
		if after != "" && !foundAfter {
			if prompt.ID == after {
				foundAfter = true
			}
			continue
		}

		// Handle before cursor
		if before != "" && prompt.ID == before {
			break
		}

		filtered = append(filtered, prompt)

		// Limit results
		if len(filtered) >= limit {
			break
		}
	}

	// Check if there are more results
	hasMore := len(allPrompts) > len(filtered) && len(filtered) == limit

	return filtered, hasMore, nil
}

// extractVariables extracts variable names from a template
// Variables are in the format {{variable_name}}
func extractVariables(template string) []string {
	re := regexp.MustCompile(`\{\{([a-zA-Z_][a-zA-Z0-9_]*)\}\}`)
	matches := re.FindAllStringSubmatch(template, -1)

	// Collect unique variable names
	vars := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 {
			vars[match[1]] = true
		}
	}

	// Convert to slice
	result := make([]string, 0, len(vars))
	for v := range vars {
		result = append(result, v)
	}

	return result
}

// RenderPrompt renders a prompt template with given variables
func RenderPrompt(template string, variables map[string]string) string {
	result := template
	for key, value := range variables {
		placeholder := "{{" + key + "}}"
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}
