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
