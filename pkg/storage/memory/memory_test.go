// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"context"
	"testing"
	"time"

	"github.com/leseb/openresponses-gw/pkg/core/state"
)

func makeSession(id string) *state.Session {
	return &state.Session{
		ID:        id,
		Metadata:  map[string]string{"key": "value"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func makeConversation(id, sessionID string) *state.Conversation {
	return &state.Conversation{
		ID:        id,
		SessionID: sessionID,
		Messages:  []state.Message{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func makeResponse(id, conversationID string) *state.Response {
	return &state.Response{
		ID:             id,
		ConversationID: conversationID,
		Status:         "completed",
		Request:        map[string]string{"model": "test"},
		CreatedAt:      time.Now(),
	}
}

// --- Session tests ---

func TestCreateAndGetSession(t *testing.T) {
	s := New()
	ctx := context.Background()

	session := makeSession("sess-1")
	if err := s.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	got, err := s.GetSession(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.ID != "sess-1" {
		t.Errorf("expected ID %q, got %q", "sess-1", got.ID)
	}
	if got.Metadata["key"] != "value" {
		t.Errorf("expected metadata key=value, got %v", got.Metadata)
	}
}

func TestCreateSession_Duplicate(t *testing.T) {
	s := New()
	ctx := context.Background()

	session := makeSession("sess-dup")
	if err := s.CreateSession(ctx, session); err != nil {
		t.Fatalf("first CreateSession: %v", err)
	}

	err := s.CreateSession(ctx, makeSession("sess-dup"))
	if err == nil {
		t.Error("expected error on duplicate session, got nil")
	}
}

func TestGetSession_NotFound(t *testing.T) {
	s := New()
	ctx := context.Background()

	_, err := s.GetSession(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for missing session, got nil")
	}
}

func TestUpdateSession(t *testing.T) {
	s := New()
	ctx := context.Background()

	session := makeSession("sess-upd")
	if err := s.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	session.Metadata["key"] = "updated"
	if err := s.UpdateSession(ctx, session); err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}

	got, err := s.GetSession(ctx, "sess-upd")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Metadata["key"] != "updated" {
		t.Errorf("expected metadata key=updated, got %v", got.Metadata)
	}
}

func TestDeleteSession(t *testing.T) {
	s := New()
	ctx := context.Background()

	session := makeSession("sess-del")
	if err := s.CreateSession(ctx, session); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := s.DeleteSession(ctx, "sess-del"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	_, err := s.GetSession(ctx, "sess-del")
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

// --- Conversation tests ---

func TestCreateAndGetConversation(t *testing.T) {
	s := New()
	ctx := context.Background()

	conv := makeConversation("conv-1", "sess-1")
	if err := s.CreateConversation(ctx, conv); err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	got, err := s.GetConversation(ctx, "conv-1")
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if got.ID != "conv-1" {
		t.Errorf("expected ID %q, got %q", "conv-1", got.ID)
	}
	if got.SessionID != "sess-1" {
		t.Errorf("expected SessionID %q, got %q", "sess-1", got.SessionID)
	}
}

func TestSaveConversation(t *testing.T) {
	s := New()
	ctx := context.Background()

	conv := makeConversation("conv-save", "sess-1")
	// SaveConversation should work as upsert (no prior CreateConversation needed)
	if err := s.SaveConversation(ctx, conv); err != nil {
		t.Fatalf("SaveConversation: %v", err)
	}

	got, err := s.GetConversation(ctx, "conv-save")
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if got.ID != "conv-save" {
		t.Errorf("expected ID %q, got %q", "conv-save", got.ID)
	}

	// Update via SaveConversation
	conv.Metadata = map[string]string{"updated": "true"}
	if err := s.SaveConversation(ctx, conv); err != nil {
		t.Fatalf("SaveConversation update: %v", err)
	}

	got, err = s.GetConversation(ctx, "conv-save")
	if err != nil {
		t.Fatalf("GetConversation after update: %v", err)
	}
	if got.Metadata["updated"] != "true" {
		t.Errorf("expected updated metadata, got %v", got.Metadata)
	}
}

func TestListConversations(t *testing.T) {
	s := New()
	ctx := context.Background()

	_ = s.SaveConversation(ctx, makeConversation("conv-a", "sess-1"))
	_ = s.SaveConversation(ctx, makeConversation("conv-b", "sess-1"))
	_ = s.SaveConversation(ctx, makeConversation("conv-c", "sess-2"))

	convs, err := s.ListConversations(ctx, "sess-1")
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(convs) != 2 {
		t.Errorf("expected 2 conversations for sess-1, got %d", len(convs))
	}

	convs2, err := s.ListConversations(ctx, "sess-2")
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(convs2) != 1 {
		t.Errorf("expected 1 conversation for sess-2, got %d", len(convs2))
	}
}

func TestDeleteConversation(t *testing.T) {
	s := New()
	ctx := context.Background()

	conv := makeConversation("conv-del", "sess-1")
	_ = s.CreateConversation(ctx, conv)

	if err := s.DeleteConversation(ctx, "conv-del"); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}

	_, err := s.GetConversation(ctx, "conv-del")
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestAddAndListConversationItems(t *testing.T) {
	s := New()
	ctx := context.Background()

	conv := makeConversation("conv-items", "sess-1")
	_ = s.CreateConversation(ctx, conv)

	items := []state.Message{
		{ID: "msg-1", Role: "user", Content: "hello"},
		{ID: "msg-2", Role: "assistant", Content: "hi there"},
	}
	if err := s.AddConversationItems(ctx, "conv-items", items); err != nil {
		t.Fatalf("AddConversationItems: %v", err)
	}

	msgs, hasMore, err := s.ListConversationItems(ctx, "conv-items", "", "", 50, "asc")
	if err != nil {
		t.Fatalf("ListConversationItems: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}
	if hasMore {
		t.Error("expected hasMore=false")
	}
	if msgs[0].ID != "msg-1" {
		t.Errorf("expected first message ID %q, got %q", "msg-1", msgs[0].ID)
	}
}

func TestListConversationItems_Pagination(t *testing.T) {
	s := New()
	ctx := context.Background()

	conv := makeConversation("conv-page", "sess-1")
	_ = s.CreateConversation(ctx, conv)

	var items []state.Message
	for i := 0; i < 5; i++ {
		items = append(items, state.Message{
			ID:   "msg-" + string(rune('a'+i)),
			Role: "user",
		})
	}
	_ = s.AddConversationItems(ctx, "conv-page", items)

	// Limit to 2
	msgs, hasMore, err := s.ListConversationItems(ctx, "conv-page", "", "", 2, "asc")
	if err != nil {
		t.Fatalf("ListConversationItems: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages with limit=2, got %d", len(msgs))
	}
	if !hasMore {
		t.Error("expected hasMore=true with 5 items and limit=2")
	}

	// After cursor
	msgs2, _, err := s.ListConversationItems(ctx, "conv-page", "msg-a", "", 50, "asc")
	if err != nil {
		t.Fatalf("ListConversationItems with after: %v", err)
	}
	if len(msgs2) != 4 {
		t.Errorf("expected 4 messages after 'msg-a', got %d", len(msgs2))
	}

	// Before cursor
	msgs3, _, err := s.ListConversationItems(ctx, "conv-page", "", "msg-c", 50, "asc")
	if err != nil {
		t.Fatalf("ListConversationItems with before: %v", err)
	}
	if len(msgs3) != 2 {
		t.Errorf("expected 2 messages before 'msg-c', got %d", len(msgs3))
	}
}

// --- Response tests ---

func TestSaveAndGetResponse(t *testing.T) {
	s := New()
	ctx := context.Background()

	resp := makeResponse("resp-1", "conv-1")
	if err := s.SaveResponse(ctx, resp); err != nil {
		t.Fatalf("SaveResponse: %v", err)
	}

	got, err := s.GetResponse(ctx, "resp-1")
	if err != nil {
		t.Fatalf("GetResponse: %v", err)
	}
	if got.ID != "resp-1" {
		t.Errorf("expected ID %q, got %q", "resp-1", got.ID)
	}
	if got.ConversationID != "conv-1" {
		t.Errorf("expected ConversationID %q, got %q", "conv-1", got.ConversationID)
	}
	if got.Status != "completed" {
		t.Errorf("expected Status %q, got %q", "completed", got.Status)
	}
}

func TestGetResponse_NotFound(t *testing.T) {
	s := New()
	ctx := context.Background()

	_, err := s.GetResponse(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for missing response, got nil")
	}
}

func TestListResponses(t *testing.T) {
	s := New()
	ctx := context.Background()

	_ = s.SaveResponse(ctx, makeResponse("resp-a", "conv-1"))
	_ = s.SaveResponse(ctx, makeResponse("resp-b", "conv-1"))
	_ = s.SaveResponse(ctx, makeResponse("resp-c", "conv-2"))

	resps, err := s.ListResponses(ctx, "conv-1")
	if err != nil {
		t.Fatalf("ListResponses: %v", err)
	}
	if len(resps) != 2 {
		t.Errorf("expected 2 responses for conv-1, got %d", len(resps))
	}
}

func TestDeleteResponse(t *testing.T) {
	s := New()
	ctx := context.Background()

	resp := makeResponse("resp-del", "conv-1")
	_ = s.SaveResponse(ctx, resp)

	if err := s.DeleteResponse(ctx, "resp-del"); err != nil {
		t.Fatalf("DeleteResponse: %v", err)
	}

	_, err := s.GetResponse(ctx, "resp-del")
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestDeleteResponse_NotFound(t *testing.T) {
	s := New()
	ctx := context.Background()

	err := s.DeleteResponse(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for missing response, got nil")
	}
}

func TestLinkResponses(t *testing.T) {
	s := New()
	ctx := context.Background()

	_ = s.SaveResponse(ctx, makeResponse("resp-cur", "conv-1"))
	_ = s.SaveResponse(ctx, makeResponse("resp-prev", "conv-1"))

	if err := s.LinkResponses(ctx, "resp-cur", "resp-prev"); err != nil {
		t.Fatalf("LinkResponses: %v", err)
	}

	got, err := s.GetResponse(ctx, "resp-cur")
	if err != nil {
		t.Fatalf("GetResponse: %v", err)
	}
	if got.PreviousResponseID != "resp-prev" {
		t.Errorf("expected PreviousResponseID %q, got %q", "resp-prev", got.PreviousResponseID)
	}
}

func TestLinkResponses_CurrentNotFound(t *testing.T) {
	s := New()
	ctx := context.Background()

	_ = s.SaveResponse(ctx, makeResponse("resp-prev", "conv-1"))

	err := s.LinkResponses(ctx, "nonexistent", "resp-prev")
	if err == nil {
		t.Error("expected error for missing current response")
	}
}

func TestLinkResponses_PreviousNotFound(t *testing.T) {
	s := New()
	ctx := context.Background()

	_ = s.SaveResponse(ctx, makeResponse("resp-cur", "conv-1"))

	err := s.LinkResponses(ctx, "resp-cur", "nonexistent")
	if err == nil {
		t.Error("expected error for missing previous response")
	}
}

func TestGetResponseInputItems(t *testing.T) {
	s := New()
	ctx := context.Background()

	resp := makeResponse("resp-input", "conv-1")
	resp.Request = map[string]string{"input": "hello"}
	_ = s.SaveResponse(ctx, resp)

	items, err := s.GetResponseInputItems(ctx, "resp-input")
	if err != nil {
		t.Fatalf("GetResponseInputItems: %v", err)
	}
	reqMap, ok := items.(map[string]string)
	if !ok {
		t.Fatalf("expected map[string]string, got %T", items)
	}
	if reqMap["input"] != "hello" {
		t.Errorf("expected input=hello, got %v", reqMap)
	}
}

func TestGetResponseInputItems_NotFound(t *testing.T) {
	s := New()
	ctx := context.Background()

	_, err := s.GetResponseInputItems(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for missing response")
	}
}

func TestListResponsesPaginated(t *testing.T) {
	s := New()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		resp := makeResponse("resp-p-"+string(rune('a'+i)), "conv-1")
		_ = s.SaveResponse(ctx, resp)
	}

	// Limit to 2
	resps, hasMore, err := s.ListResponsesPaginated(ctx, "", "", 2, "desc", "")
	if err != nil {
		t.Fatalf("ListResponsesPaginated: %v", err)
	}
	if len(resps) != 2 {
		t.Errorf("expected 2 responses with limit=2, got %d", len(resps))
	}
	if !hasMore {
		t.Error("expected hasMore=true with 5 items and limit=2")
	}

	// Default limit (0 -> 50)
	resps2, _, err := s.ListResponsesPaginated(ctx, "", "", 0, "", "")
	if err != nil {
		t.Fatalf("ListResponsesPaginated default: %v", err)
	}
	if len(resps2) != 5 {
		t.Errorf("expected 5 responses with default limit, got %d", len(resps2))
	}
}

func TestListConversationsPaginated(t *testing.T) {
	s := New()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		conv := makeConversation("conv-p-"+string(rune('a'+i)), "sess-1")
		_ = s.SaveConversation(ctx, conv)
	}

	// Limit to 2
	convs, hasMore, err := s.ListConversationsPaginated(ctx, "", "", 2, "desc")
	if err != nil {
		t.Fatalf("ListConversationsPaginated: %v", err)
	}
	if len(convs) != 2 {
		t.Errorf("expected 2 conversations with limit=2, got %d", len(convs))
	}
	if !hasMore {
		t.Error("expected hasMore=true with 5 items and limit=2")
	}

	// Default limit
	convs2, _, err := s.ListConversationsPaginated(ctx, "", "", 0, "")
	if err != nil {
		t.Fatalf("ListConversationsPaginated default: %v", err)
	}
	if len(convs2) != 5 {
		t.Errorf("expected 5 conversations with default limit, got %d", len(convs2))
	}
}

func TestDeleteConversation_NotFound(t *testing.T) {
	s := New()
	ctx := context.Background()

	err := s.DeleteConversation(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for missing conversation, got nil")
	}
}

func TestCreateConversation_Duplicate(t *testing.T) {
	s := New()
	ctx := context.Background()

	conv := makeConversation("conv-dup", "sess-1")
	if err := s.CreateConversation(ctx, conv); err != nil {
		t.Fatalf("first CreateConversation: %v", err)
	}

	err := s.CreateConversation(ctx, makeConversation("conv-dup", "sess-1"))
	if err == nil {
		t.Error("expected error on duplicate conversation, got nil")
	}
}
