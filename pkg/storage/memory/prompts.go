// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// VersionMismatchError indicates an optimistic concurrency conflict
type VersionMismatchError struct {
	ProvidedVersion int
	LatestVersion   int
}

func (e *VersionMismatchError) Error() string {
	return fmt.Sprintf("version mismatch: provided %d, latest is %d", e.ProvidedVersion, e.LatestVersion)
}

// Prompt represents a stored prompt template
type Prompt struct {
	ID          string
	Name        string
	Description string
	Template    string
	Variables   []string
	Version     int
	IsDefault   bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Metadata    map[string]string
}

// PromptsStore is an in-memory prompts store with versioning support
type PromptsStore struct {
	mu             sync.RWMutex
	versions       map[string]map[int]*Prompt // promptID -> version -> Prompt
	defaultVersion map[string]int             // promptID -> default version number
}

// NewPromptsStore creates a new prompts store
func NewPromptsStore() *PromptsStore {
	return &PromptsStore{
		versions:       make(map[string]map[int]*Prompt),
		defaultVersion: make(map[string]int),
	}
}

// CreatePrompt creates a new prompt (version 1)
func (s *PromptsStore) CreatePrompt(ctx context.Context, prompt *Prompt) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.versions[prompt.ID]; exists {
		return fmt.Errorf("prompt %s already exists", prompt.ID)
	}

	// Extract variables from template
	prompt.Variables = extractVariables(prompt.Template)
	prompt.Version = 1
	prompt.IsDefault = true

	s.versions[prompt.ID] = map[int]*Prompt{1: prompt}
	s.defaultVersion[prompt.ID] = 1
	return nil
}

// GetPrompt retrieves the default version of a prompt by ID
func (s *PromptsStore) GetPrompt(ctx context.Context, promptID string) (*Prompt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	versionMap, exists := s.versions[promptID]
	if !exists {
		return nil, fmt.Errorf("prompt %s not found", promptID)
	}

	defVer := s.defaultVersion[promptID]
	prompt, exists := versionMap[defVer]
	if !exists {
		return nil, fmt.Errorf("prompt %s default version %d not found", promptID, defVer)
	}

	return prompt, nil
}

// GetPromptVersion retrieves a specific version of a prompt
func (s *PromptsStore) GetPromptVersion(ctx context.Context, promptID string, version int) (*Prompt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	versionMap, exists := s.versions[promptID]
	if !exists {
		return nil, fmt.Errorf("prompt %s not found", promptID)
	}

	prompt, exists := versionMap[version]
	if !exists {
		return nil, fmt.Errorf("prompt %s version %d not found", promptID, version)
	}

	return prompt, nil
}

// latestVersion returns the highest version number for a prompt (caller must hold lock)
func (s *PromptsStore) latestVersion(promptID string) int {
	maxVer := 0
	for v := range s.versions[promptID] {
		if v > maxVer {
			maxVer = v
		}
	}
	return maxVer
}

// UpdatePrompt creates a new version of an existing prompt.
// The provided version must match the latest version (optimistic concurrency).
// Returns the newly created prompt version.
func (s *PromptsStore) UpdatePrompt(ctx context.Context, promptID string, version int, updates *Prompt, setAsDefault *bool) (*Prompt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	versionMap, exists := s.versions[promptID]
	if !exists {
		return nil, fmt.Errorf("prompt %s not found", promptID)
	}

	latest := s.latestVersion(promptID)
	if version != latest {
		return nil, &VersionMismatchError{ProvidedVersion: version, LatestVersion: latest}
	}

	currentPrompt := versionMap[latest]

	// Build new version from current + updates
	newVer := latest + 1
	now := time.Now()

	newPrompt := &Prompt{
		ID:          promptID,
		Name:        currentPrompt.Name,
		Description: currentPrompt.Description,
		Template:    currentPrompt.Template,
		Version:     newVer,
		CreatedAt:   currentPrompt.CreatedAt,
		UpdatedAt:   now,
		Metadata:    currentPrompt.Metadata,
	}

	if updates.Name != "" {
		newPrompt.Name = updates.Name
	}
	if updates.Description != "" {
		newPrompt.Description = updates.Description
	}
	if updates.Template != "" {
		newPrompt.Template = updates.Template
	}
	if updates.Metadata != nil {
		newPrompt.Metadata = updates.Metadata
	}

	// Re-extract variables
	newPrompt.Variables = extractVariables(newPrompt.Template)

	// Determine if this version should be the default (default: true)
	makeDefault := true
	if setAsDefault != nil {
		makeDefault = *setAsDefault
	}

	if makeDefault {
		// Unmark previous default
		prevDefVer := s.defaultVersion[promptID]
		if prev, ok := versionMap[prevDefVer]; ok {
			prev.IsDefault = false
		}
		newPrompt.IsDefault = true
		s.defaultVersion[promptID] = newVer
	}

	versionMap[newVer] = newPrompt

	return newPrompt, nil
}

// DeletePrompt deletes all versions of a prompt
func (s *PromptsStore) DeletePrompt(ctx context.Context, promptID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.versions[promptID]; !exists {
		return fmt.Errorf("prompt %s not found", promptID)
	}

	delete(s.versions, promptID)
	delete(s.defaultVersion, promptID)
	return nil
}

// ListPromptsPaginated lists the default version of each prompt with pagination
func (s *PromptsStore) ListPromptsPaginated(ctx context.Context, after, before string, limit int, order string) ([]*Prompt, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Default limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// Collect default version of each prompt
	var allPrompts []*Prompt
	for promptID, versionMap := range s.versions {
		defVer := s.defaultVersion[promptID]
		if prompt, ok := versionMap[defVer]; ok {
			allPrompts = append(allPrompts, prompt)
		}
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

// ListPromptVersions returns all versions of a prompt, sorted by version ascending
func (s *PromptsStore) ListPromptVersions(ctx context.Context, promptID string) ([]*Prompt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	versionMap, exists := s.versions[promptID]
	if !exists {
		return nil, fmt.Errorf("prompt %s not found", promptID)
	}

	var result []*Prompt
	for _, prompt := range versionMap {
		result = append(result, prompt)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Version < result[j].Version
	})

	return result, nil
}

// SetDefaultVersion sets the default version for a prompt
func (s *PromptsStore) SetDefaultVersion(ctx context.Context, promptID string, version int) (*Prompt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	versionMap, exists := s.versions[promptID]
	if !exists {
		return nil, fmt.Errorf("prompt %s not found", promptID)
	}

	newDefault, exists := versionMap[version]
	if !exists {
		return nil, fmt.Errorf("prompt %s version %d not found", promptID, version)
	}

	// Unmark previous default
	prevDefVer := s.defaultVersion[promptID]
	if prev, ok := versionMap[prevDefVer]; ok {
		prev.IsDefault = false
	}

	newDefault.IsDefault = true
	s.defaultVersion[promptID] = version

	return newDefault, nil
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
