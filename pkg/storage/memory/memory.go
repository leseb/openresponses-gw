// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/leseb/openai-responses-gateway/pkg/core/state"
)

// Store is an in-memory implementation of SessionStore
type Store struct {
	mu            sync.RWMutex
	sessions      map[string]*state.Session
	conversations map[string]*state.Conversation
	responses     map[string]*state.Response
}

// New creates a new in-memory store
func New() *Store {
	return &Store{
		sessions:      make(map[string]*state.Session),
		conversations: make(map[string]*state.Conversation),
		responses:     make(map[string]*state.Response),
	}
}

// CreateSession creates a new session
func (s *Store) CreateSession(ctx context.Context, session *state.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.sessions[session.ID]; exists {
		return fmt.Errorf("session %s already exists", session.ID)
	}

	s.sessions[session.ID] = session
	return nil
}

// GetSession retrieves a session by ID
func (s *Store) GetSession(ctx context.Context, sessionID string) (*state.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	return session, nil
}

// UpdateSession updates an existing session
func (s *Store) UpdateSession(ctx context.Context, session *state.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.sessions[session.ID]; !exists {
		return fmt.Errorf("session %s not found", session.ID)
	}

	s.sessions[session.ID] = session
	return nil
}

// DeleteSession deletes a session
func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, sessionID)
	return nil
}

// GetConversation retrieves a conversation by ID
func (s *Store) GetConversation(ctx context.Context, conversationID string) (*state.Conversation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conv, exists := s.conversations[conversationID]
	if !exists {
		return nil, fmt.Errorf("conversation %s not found", conversationID)
	}

	return conv, nil
}

// SaveConversation saves a conversation
func (s *Store) SaveConversation(ctx context.Context, conv *state.Conversation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.conversations[conv.ID] = conv
	return nil
}

// ListConversations lists conversations for a session
func (s *Store) ListConversations(ctx context.Context, sessionID string) ([]*state.Conversation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var convs []*state.Conversation
	for _, conv := range s.conversations {
		if conv.SessionID == sessionID {
			convs = append(convs, conv)
		}
	}

	return convs, nil
}

// GetResponse retrieves a response by ID
func (s *Store) GetResponse(ctx context.Context, responseID string) (*state.Response, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resp, exists := s.responses[responseID]
	if !exists {
		return nil, fmt.Errorf("response %s not found", responseID)
	}

	return resp, nil
}

// SaveResponse saves a response
func (s *Store) SaveResponse(ctx context.Context, resp *state.Response) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.responses[resp.ID] = resp
	return nil
}

// ListResponses lists responses for a conversation
func (s *Store) ListResponses(ctx context.Context, conversationID string) ([]*state.Response, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var resps []*state.Response
	for _, resp := range s.responses {
		if resp.ConversationID == conversationID {
			resps = append(resps, resp)
		}
	}

	return resps, nil
}

// LinkResponses links two responses (current points to previous)
func (s *Store) LinkResponses(ctx context.Context, currentID, previousID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, exists := s.responses[currentID]
	if !exists {
		return fmt.Errorf("current response %s not found", currentID)
	}

	if _, exists := s.responses[previousID]; !exists {
		return fmt.Errorf("previous response %s not found", previousID)
	}

	current.PreviousResponseID = previousID
	return nil
}

// ListResponsesPaginated lists responses with pagination
func (s *Store) ListResponsesPaginated(ctx context.Context, after, before string, limit int, order, model string) ([]*state.Response, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Default limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// Default order
	if order != "asc" && order != "desc" {
		order = "desc"
	}

	// Collect all responses into a slice
	var allResponses []*state.Response
	for _, resp := range s.responses {
		// Filter by model if specified
		if model != "" {
			// Note: We'd need to store model in state.Response for this to work
			// For now, skip model filtering in memory store
		}
		allResponses = append(allResponses, resp)
	}

	// Sort by created time
	// Note: This is a simplified implementation
	// In production, use a proper sorting library or DB query

	// Apply cursor-based pagination
	var filtered []*state.Response
	foundAfter := after == "" // If no after cursor, start from beginning

	for _, resp := range allResponses {
		// Handle after cursor
		if after != "" && !foundAfter {
			if resp.ID == after {
				foundAfter = true
			}
			continue
		}

		// Handle before cursor
		if before != "" && resp.ID == before {
			break
		}

		filtered = append(filtered, resp)

		// Limit results
		if len(filtered) >= limit {
			break
		}
	}

	// Check if there are more results
	hasMore := len(allResponses) > len(filtered) && len(filtered) == limit

	return filtered, hasMore, nil
}

// DeleteResponse deletes a response
func (s *Store) DeleteResponse(ctx context.Context, responseID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.responses[responseID]; !exists {
		return fmt.Errorf("response %s not found", responseID)
	}

	delete(s.responses, responseID)
	return nil
}

// GetResponseInputItems retrieves input items for a response
func (s *Store) GetResponseInputItems(ctx context.Context, responseID string) (interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resp, exists := s.responses[responseID]
	if !exists {
		return nil, fmt.Errorf("response %s not found", responseID)
	}

	// Return the request field which contains input items
	return resp.Request, nil
}

// CreateConversation creates a new conversation
func (s *Store) CreateConversation(ctx context.Context, conv *state.Conversation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.conversations[conv.ID]; exists {
		return fmt.Errorf("conversation %s already exists", conv.ID)
	}

	s.conversations[conv.ID] = conv
	return nil
}

// ListConversationsPaginated lists conversations with pagination
func (s *Store) ListConversationsPaginated(ctx context.Context, after, before string, limit int, order string) ([]*state.Conversation, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Default limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// Default order
	if order != "asc" && order != "desc" {
		order = "desc"
	}

	// Collect all conversations
	var allConvs []*state.Conversation
	for _, conv := range s.conversations {
		allConvs = append(allConvs, conv)
	}

	// Apply cursor-based pagination
	var filtered []*state.Conversation
	foundAfter := after == ""

	for _, conv := range allConvs {
		// Handle after cursor
		if after != "" && !foundAfter {
			if conv.ID == after {
				foundAfter = true
			}
			continue
		}

		// Handle before cursor
		if before != "" && conv.ID == before {
			break
		}

		filtered = append(filtered, conv)

		// Limit results
		if len(filtered) >= limit {
			break
		}
	}

	// Check if there are more results
	hasMore := len(allConvs) > len(filtered) && len(filtered) == limit

	return filtered, hasMore, nil
}

// DeleteConversation deletes a conversation
func (s *Store) DeleteConversation(ctx context.Context, conversationID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.conversations[conversationID]; !exists {
		return fmt.Errorf("conversation %s not found", conversationID)
	}

	delete(s.conversations, conversationID)
	return nil
}

// AddConversationItems adds items to a conversation
func (s *Store) AddConversationItems(ctx context.Context, conversationID string, items []state.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	conv, exists := s.conversations[conversationID]
	if !exists {
		return fmt.Errorf("conversation %s not found", conversationID)
	}

	conv.Messages = append(conv.Messages, items...)
	return nil
}

// ListConversationItems lists items in a conversation with pagination
func (s *Store) ListConversationItems(ctx context.Context, conversationID string, after, before string, limit int, order string) ([]state.Message, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conv, exists := s.conversations[conversationID]
	if !exists {
		return nil, false, fmt.Errorf("conversation %s not found", conversationID)
	}

	// Default limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// Apply cursor-based pagination
	var filtered []state.Message
	foundAfter := after == ""

	for _, msg := range conv.Messages {
		// Handle after cursor
		if after != "" && !foundAfter {
			if msg.ID == after {
				foundAfter = true
			}
			continue
		}

		// Handle before cursor
		if before != "" && msg.ID == before {
			break
		}

		filtered = append(filtered, msg)

		// Limit results
		if len(filtered) >= limit {
			break
		}
	}

	// Check if there are more results
	hasMore := len(conv.Messages) > len(filtered) && len(filtered) == limit

	return filtered, hasMore, nil
}
