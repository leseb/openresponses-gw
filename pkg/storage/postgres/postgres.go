// Copyright Open Responses Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/leseb/openresponses-gw/pkg/core/state"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Store is a PostgreSQL-backed implementation of SessionStore.
type Store struct {
	db *sql.DB
}

// New creates a new PostgreSQL store. The dsn is a PostgreSQL connection string,
// e.g. "postgres://user:pass@host:5432/dbname?sslmode=disable".
func New(dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres open: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}

	s := &Store{db: db}
	if err := s.createTables(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) createTables() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			conversation_id TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL DEFAULT '{}',
			metadata TEXT NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS conversations (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL DEFAULT '',
			metadata TEXT NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT NOT NULL,
			conversation_id TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL DEFAULT '""',
			metadata TEXT NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL,
			position INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (conversation_id, id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_position ON messages(conversation_id, position)`,
		`CREATE TABLE IF NOT EXISTS responses (
			id TEXT PRIMARY KEY,
			conversation_id TEXT NOT NULL DEFAULT '',
			previous_response_id TEXT NOT NULL DEFAULT '',
			request TEXT NOT NULL DEFAULT 'null',
			output TEXT NOT NULL DEFAULT 'null',
			status TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT 'null',
			usage TEXT NOT NULL DEFAULT 'null',
			messages TEXT NOT NULL DEFAULT '[]',
			created_at TIMESTAMPTZ NOT NULL,
			completed_at TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_responses_created ON responses(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_responses_conversation ON responses(conversation_id)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("postgres create tables: %w", err)
		}
	}
	return nil
}

// --- helpers ---

func marshalJSON(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalMapStringInterface(data string) (map[string]interface{}, error) {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		return nil, err
	}
	return m, nil
}

func unmarshalMapStringString(data string) (map[string]string, error) {
	var m map[string]string
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		return nil, err
	}
	return m, nil
}

func unmarshalInterface(data string) (interface{}, error) {
	var v interface{}
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		return nil, err
	}
	return v, nil
}

func nullTimeToPtr(nt sql.NullTime) *time.Time {
	if nt.Valid {
		return &nt.Time
	}
	return nil
}

// --- Session methods ---

func (s *Store) CreateSession(ctx context.Context, session *state.Session) error {
	stateJSON, err := marshalJSON(session.State)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	metaJSON, err := marshalJSON(session.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, conversation_id, state, metadata, created_at, updated_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		session.ID, session.ConversationID, stateJSON, metaJSON,
		session.CreatedAt, session.UpdatedAt, session.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("session %s already exists", session.ID)
	}
	return nil
}

func (s *Store) GetSession(ctx context.Context, sessionID string) (*state.Session, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, conversation_id, state, metadata, created_at, updated_at, expires_at
		 FROM sessions WHERE id = $1`, sessionID)

	var (
		sess              state.Session
		stateStr, metaStr string
	)
	err := row.Scan(&sess.ID, &sess.ConversationID, &stateStr, &metaStr,
		&sess.CreatedAt, &sess.UpdatedAt, &sess.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	sess.State, err = unmarshalMapStringInterface(stateStr)
	if err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}
	sess.Metadata, err = unmarshalMapStringString(metaStr)
	if err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}
	return &sess, nil
}

func (s *Store) UpdateSession(ctx context.Context, session *state.Session) error {
	stateJSON, err := marshalJSON(session.State)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	metaJSON, err := marshalJSON(session.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET conversation_id=$1, state=$2, metadata=$3, updated_at=$4, expires_at=$5
		 WHERE id=$6`,
		session.ConversationID, stateJSON, metaJSON,
		session.UpdatedAt, session.ExpiresAt, session.ID,
	)
	if err != nil {
		return fmt.Errorf("update session: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("session %s not found", session.ID)
	}
	return nil
}

func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id=$1`, sessionID)
	return err
}

// --- Conversation methods ---

func (s *Store) CreateConversation(ctx context.Context, conv *state.Conversation) error {
	metaJSON, err := marshalJSON(conv.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO conversations (id, session_id, metadata, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		conv.ID, conv.SessionID, metaJSON, conv.CreatedAt, conv.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("conversation %s already exists", conv.ID)
	}
	return nil
}

func (s *Store) GetConversation(ctx context.Context, conversationID string) (*state.Conversation, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, session_id, metadata, created_at, updated_at
		 FROM conversations WHERE id = $1`, conversationID)

	var (
		conv    state.Conversation
		metaStr string
	)
	err := row.Scan(&conv.ID, &conv.SessionID, &metaStr, &conv.CreatedAt, &conv.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("conversation %s not found", conversationID)
	}
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}

	conv.Metadata, err = unmarshalMapStringString(metaStr)
	if err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}

	// Load messages
	conv.Messages, err = s.loadMessages(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	return &conv, nil
}

func (s *Store) SaveConversation(ctx context.Context, conv *state.Conversation) error {
	metaJSON, err := marshalJSON(conv.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO conversations (id, session_id, metadata, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (id) DO UPDATE SET session_id=$2, metadata=$3, created_at=$4, updated_at=$5`,
		conv.ID, conv.SessionID, metaJSON, conv.CreatedAt, conv.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("save conversation: %w", err)
	}

	// Sync messages: delete existing then re-insert to handle updates
	if _, err := s.db.ExecContext(ctx, `DELETE FROM messages WHERE conversation_id=$1`, conv.ID); err != nil {
		return fmt.Errorf("delete old messages: %w", err)
	}
	for i, msg := range conv.Messages {
		if err := s.insertMessage(ctx, conv.ID, msg, i); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) ListConversations(ctx context.Context, sessionID string) ([]*state.Conversation, error) {
	convs, err := s.scanConversationRows(ctx,
		`SELECT id, session_id, metadata, created_at, updated_at
		 FROM conversations WHERE session_id=$1`, sessionID)
	if err != nil {
		return nil, err
	}
	for _, conv := range convs {
		conv.Messages, err = s.loadMessages(ctx, conv.ID)
		if err != nil {
			return nil, err
		}
	}
	return convs, nil
}

func (s *Store) ListConversationsPaginated(ctx context.Context, after, before string, limit int, order string) ([]*state.Conversation, bool, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if order != "asc" && order != "desc" {
		order = "desc"
	}

	query := `SELECT id, session_id, metadata, created_at, updated_at FROM conversations`
	var args []interface{}
	var where []string
	argIdx := 1

	if after != "" {
		where = append(where, fmt.Sprintf("created_at > (SELECT created_at FROM conversations WHERE id = $%d)", argIdx))
		args = append(args, after)
		argIdx++
	}
	if before != "" {
		where = append(where, fmt.Sprintf("created_at < (SELECT created_at FROM conversations WHERE id = $%d)", argIdx))
		args = append(args, before)
		argIdx++
	}
	if len(where) > 0 {
		query += " WHERE " + where[0]
		for _, w := range where[1:] {
			query += " AND " + w
		}
	}

	query += fmt.Sprintf(" ORDER BY created_at %s LIMIT $%d", order, argIdx)
	args = append(args, limit+1)

	convs, err := s.scanConversationRows(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	for _, conv := range convs {
		conv.Messages, err = s.loadMessages(ctx, conv.ID)
		if err != nil {
			return nil, false, err
		}
	}

	hasMore := len(convs) > limit
	if hasMore {
		convs = convs[:limit]
	}
	return convs, hasMore, nil
}

func (s *Store) DeleteConversation(ctx context.Context, conversationID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM conversations WHERE id=$1`, conversationID)
	if err != nil {
		return fmt.Errorf("delete conversation: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("conversation %s not found", conversationID)
	}
	// Clean up associated messages
	_, _ = s.db.ExecContext(ctx, `DELETE FROM messages WHERE conversation_id=$1`, conversationID)
	return nil
}

func (s *Store) AddConversationItems(ctx context.Context, conversationID string, items []state.Message) error {
	// Verify conversation exists
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM conversations WHERE id=$1`, conversationID).Scan(&exists)
	if err == sql.ErrNoRows {
		return fmt.Errorf("conversation %s not found", conversationID)
	}
	if err != nil {
		return fmt.Errorf("check conversation: %w", err)
	}

	// Get current max position
	var maxPos int
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(position), -1) FROM messages WHERE conversation_id=$1`,
		conversationID).Scan(&maxPos)
	if err != nil {
		return fmt.Errorf("get max position: %w", err)
	}

	for i, msg := range items {
		if err := s.insertMessage(ctx, conversationID, msg, maxPos+1+i); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListConversationItems(ctx context.Context, conversationID string, after, before string, limit int, order string) ([]state.Message, bool, error) {
	// Verify conversation exists
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM conversations WHERE id=$1`, conversationID).Scan(&exists)
	if err == sql.ErrNoRows {
		return nil, false, fmt.Errorf("conversation %s not found", conversationID)
	}
	if err != nil {
		return nil, false, fmt.Errorf("check conversation: %w", err)
	}

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	query := `SELECT id, role, content, metadata, created_at FROM messages WHERE conversation_id=$1`
	args := []interface{}{conversationID}
	argIdx := 2

	if after != "" {
		query += fmt.Sprintf(` AND position > (SELECT position FROM messages WHERE conversation_id=$%d AND id=$%d)`, argIdx, argIdx+1)
		args = append(args, conversationID, after)
		argIdx += 2
	}
	if before != "" {
		query += fmt.Sprintf(` AND position < (SELECT position FROM messages WHERE conversation_id=$%d AND id=$%d)`, argIdx, argIdx+1)
		args = append(args, conversationID, before)
		argIdx += 2
	}

	query += fmt.Sprintf(` ORDER BY position ASC LIMIT $%d`, argIdx)
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("list conversation items: %w", err)
	}
	defer rows.Close()

	var msgs []state.Message
	for rows.Next() {
		var (
			msg                 state.Message
			contentStr, metaStr string
		)
		if err := rows.Scan(&msg.ID, &msg.Role, &contentStr, &metaStr, &msg.CreatedAt); err != nil {
			return nil, false, fmt.Errorf("scan message: %w", err)
		}
		msg.Content, err = unmarshalInterface(contentStr)
		if err != nil {
			return nil, false, fmt.Errorf("unmarshal content: %w", err)
		}
		msg.Metadata, err = unmarshalMapStringString(metaStr)
		if err != nil {
			return nil, false, fmt.Errorf("unmarshal metadata: %w", err)
		}
		msgs = append(msgs, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}

	hasMore := len(msgs) > limit
	if hasMore {
		msgs = msgs[:limit]
	}
	return msgs, hasMore, nil
}

// --- Response methods ---

func (s *Store) GetResponse(ctx context.Context, responseID string) (*state.Response, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, conversation_id, previous_response_id, request, output, status,
		        error, usage, messages, created_at, completed_at
		 FROM responses WHERE id = $1`, responseID)

	return s.scanResponse(row)
}

func (s *Store) SaveResponse(ctx context.Context, resp *state.Response) error {
	requestJSON, err := marshalJSON(resp.Request)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	outputJSON, err := marshalJSON(resp.Output)
	if err != nil {
		return fmt.Errorf("marshal output: %w", err)
	}
	errorJSON, err := marshalJSON(resp.Error)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}
	usageJSON, err := marshalJSON(resp.Usage)
	if err != nil {
		return fmt.Errorf("marshal usage: %w", err)
	}
	messagesJSON, err := marshalJSON(resp.Messages)
	if err != nil {
		return fmt.Errorf("marshal messages: %w", err)
	}

	var completedAt sql.NullTime
	if resp.CompletedAt != nil {
		completedAt = sql.NullTime{Time: *resp.CompletedAt, Valid: true}
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO responses
		 (id, conversation_id, previous_response_id, request, output, status, error, usage, messages, created_at, completed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT (id) DO UPDATE SET
		   conversation_id=$2, previous_response_id=$3, request=$4, output=$5,
		   status=$6, error=$7, usage=$8, messages=$9, created_at=$10, completed_at=$11`,
		resp.ID, resp.ConversationID, resp.PreviousResponseID,
		requestJSON, outputJSON, resp.Status, errorJSON, usageJSON, messagesJSON,
		resp.CreatedAt, completedAt,
	)
	if err != nil {
		return fmt.Errorf("save response: %w", err)
	}
	return nil
}

func (s *Store) ListResponses(ctx context.Context, conversationID string) ([]*state.Response, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, conversation_id, previous_response_id, request, output, status,
		        error, usage, messages, created_at, completed_at
		 FROM responses WHERE conversation_id=$1`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list responses: %w", err)
	}
	defer rows.Close()

	return s.scanResponses(rows)
}

func (s *Store) LinkResponses(ctx context.Context, currentID, previousID string) error {
	// Verify both exist
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM responses WHERE id=$1`, currentID).Scan(&exists)
	if err == sql.ErrNoRows {
		return fmt.Errorf("current response %s not found", currentID)
	}
	if err != nil {
		return fmt.Errorf("check current response: %w", err)
	}

	err = s.db.QueryRowContext(ctx, `SELECT 1 FROM responses WHERE id=$1`, previousID).Scan(&exists)
	if err == sql.ErrNoRows {
		return fmt.Errorf("previous response %s not found", previousID)
	}
	if err != nil {
		return fmt.Errorf("check previous response: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE responses SET previous_response_id=$1 WHERE id=$2`,
		previousID, currentID)
	return err
}

func (s *Store) ListResponsesPaginated(ctx context.Context, after, before string, limit int, order, model string) ([]*state.Response, bool, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if order != "asc" && order != "desc" {
		order = "desc"
	}

	query := `SELECT id, conversation_id, previous_response_id, request, output, status,
	                 error, usage, messages, created_at, completed_at
	          FROM responses`
	var args []interface{}
	var where []string
	argIdx := 1

	if after != "" {
		where = append(where, fmt.Sprintf("created_at > (SELECT created_at FROM responses WHERE id = $%d)", argIdx))
		args = append(args, after)
		argIdx++
	}
	if before != "" {
		where = append(where, fmt.Sprintf("created_at < (SELECT created_at FROM responses WHERE id = $%d)", argIdx))
		args = append(args, before)
		argIdx++
	}
	if len(where) > 0 {
		query += " WHERE " + where[0]
		for _, w := range where[1:] {
			query += " AND " + w
		}
	}

	query += fmt.Sprintf(" ORDER BY created_at %s LIMIT $%d", order, argIdx)
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("list responses paginated: %w", err)
	}
	defer rows.Close()

	resps, err := s.scanResponses(rows)
	if err != nil {
		return nil, false, err
	}

	hasMore := len(resps) > limit
	if hasMore {
		resps = resps[:limit]
	}
	return resps, hasMore, nil
}

func (s *Store) DeleteResponse(ctx context.Context, responseID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM responses WHERE id=$1`, responseID)
	if err != nil {
		return fmt.Errorf("delete response: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("response %s not found", responseID)
	}
	return nil
}

func (s *Store) GetResponseInputItems(ctx context.Context, responseID string) (interface{}, error) {
	var requestStr string
	err := s.db.QueryRowContext(ctx,
		`SELECT request FROM responses WHERE id=$1`, responseID).Scan(&requestStr)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("response %s not found", responseID)
	}
	if err != nil {
		return nil, fmt.Errorf("get response input items: %w", err)
	}
	return unmarshalInterface(requestStr)
}

// --- internal helpers ---

func (s *Store) insertMessage(ctx context.Context, conversationID string, msg state.Message, position int) error {
	contentJSON, err := marshalJSON(msg.Content)
	if err != nil {
		return fmt.Errorf("marshal content: %w", err)
	}
	metaJSON, err := marshalJSON(msg.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO messages (id, conversation_id, role, content, metadata, created_at, position)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (conversation_id, id) DO UPDATE SET role=$3, content=$4, metadata=$5, created_at=$6, position=$7`,
		msg.ID, conversationID, msg.Role, contentJSON, metaJSON, msg.CreatedAt, position,
	)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	return nil
}

func (s *Store) loadMessages(ctx context.Context, conversationID string) ([]state.Message, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, role, content, metadata, created_at
		 FROM messages WHERE conversation_id=$1 ORDER BY position ASC`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("load messages: %w", err)
	}
	defer rows.Close()

	var msgs []state.Message
	for rows.Next() {
		var (
			msg                 state.Message
			contentStr, metaStr string
		)
		if err := rows.Scan(&msg.ID, &msg.Role, &contentStr, &metaStr, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		msg.Content, err = unmarshalInterface(contentStr)
		if err != nil {
			return nil, fmt.Errorf("unmarshal content: %w", err)
		}
		msg.Metadata, err = unmarshalMapStringString(metaStr)
		if err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
		msgs = append(msgs, msg)
	}
	return msgs, rows.Err()
}

type scannable interface {
	Scan(dest ...interface{}) error
}

func (s *Store) scanResponse(row scannable) (*state.Response, error) {
	var (
		resp                                                   state.Response
		requestStr, outputStr, errorStr, usageStr, messagesStr string
		completedAt                                            sql.NullTime
	)
	err := row.Scan(&resp.ID, &resp.ConversationID, &resp.PreviousResponseID,
		&requestStr, &outputStr, &resp.Status, &errorStr, &usageStr, &messagesStr,
		&resp.CreatedAt, &completedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("response %s not found", resp.ID)
	}
	if err != nil {
		return nil, fmt.Errorf("scan response: %w", err)
	}

	resp.CompletedAt = nullTimeToPtr(completedAt)

	resp.Request, err = unmarshalInterface(requestStr)
	if err != nil {
		return nil, fmt.Errorf("unmarshal request: %w", err)
	}
	resp.Output, err = unmarshalInterface(outputStr)
	if err != nil {
		return nil, fmt.Errorf("unmarshal output: %w", err)
	}
	resp.Error, err = unmarshalInterface(errorStr)
	if err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}
	resp.Usage, err = unmarshalInterface(usageStr)
	if err != nil {
		return nil, fmt.Errorf("unmarshal usage: %w", err)
	}
	if err := json.Unmarshal([]byte(messagesStr), &resp.Messages); err != nil {
		return nil, fmt.Errorf("unmarshal messages: %w", err)
	}
	return &resp, nil
}

func (s *Store) scanConversationRows(ctx context.Context, query string, args ...interface{}) ([]*state.Conversation, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query conversations: %w", err)
	}
	defer rows.Close()

	var convs []*state.Conversation
	for rows.Next() {
		var (
			conv    state.Conversation
			metaStr string
		)
		if err := rows.Scan(&conv.ID, &conv.SessionID, &metaStr, &conv.CreatedAt, &conv.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		conv.Metadata, err = unmarshalMapStringString(metaStr)
		if err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
		convs = append(convs, &conv)
	}
	return convs, rows.Err()
}

func (s *Store) scanResponses(rows *sql.Rows) ([]*state.Response, error) {
	var resps []*state.Response
	for rows.Next() {
		resp, err := s.scanResponse(rows)
		if err != nil {
			return nil, err
		}
		resps = append(resps, resp)
	}
	return resps, rows.Err()
}
