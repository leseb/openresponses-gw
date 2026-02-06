// Copyright OpenAI Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"context"
	"time"
)

// SessionStore defines the interface for state storage
type SessionStore interface {
	// Session lifecycle
	CreateSession(ctx context.Context, session *Session) error
	GetSession(ctx context.Context, sessionID string) (*Session, error)
	UpdateSession(ctx context.Context, session *Session) error
	DeleteSession(ctx context.Context, sessionID string) error

	// Conversation management
	GetConversation(ctx context.Context, conversationID string) (*Conversation, error)
	SaveConversation(ctx context.Context, conv *Conversation) error
	ListConversations(ctx context.Context, sessionID string) ([]*Conversation, error)

	// Response history
	GetResponse(ctx context.Context, responseID string) (*Response, error)
	SaveResponse(ctx context.Context, resp *Response) error
	ListResponses(ctx context.Context, conversationID string) ([]*Response, error)
	LinkResponses(ctx context.Context, currentID, previousID string) error
}

// Session represents a user session
type Session struct {
	ID             string
	ConversationID string
	State          map[string]interface{}
	Metadata       map[string]string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ExpiresAt      time.Time
}

// Conversation represents a conversation
type Conversation struct {
	ID        string
	SessionID string
	Messages  []Message
	Metadata  map[string]string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Message represents a message in a conversation
type Message struct {
	ID        string
	Role      string
	Content   interface{}
	Metadata  map[string]string
	CreatedAt time.Time
}

// Response represents a stored response
type Response struct {
	ID                 string
	ConversationID     string
	PreviousResponseID string
	Request            interface{}
	Output             interface{}
	Status             string
	Error              interface{}
	Usage              interface{}
	CreatedAt          time.Time
	CompletedAt        *time.Time
}
