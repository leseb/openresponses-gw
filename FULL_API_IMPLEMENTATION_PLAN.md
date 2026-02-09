# Full API Implementation Plan

## Overview

Transform the OpenAI Responses Gateway into a complete implementation supporting all major Llama Stack / OpenAI APIs.

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                     HTTP/ExtProc Layer                       │
│  POST /v1/responses              GET /v1/models             │
│  GET /v1/responses               POST /v1/chat/completions  │
│  POST /v1/conversations          POST /v1/prompts           │
│  POST /v1/files                  POST /v1/vector_stores     │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                       Core Engine                            │
│  - Response Orchestration    - Prompt Management            │
│  - Conversation Management   - File Management              │
│  - Vector Search             - Model Registry               │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                      Storage Layer                           │
│  - Responses Store   - Conversations Store                  │
│  - Prompts Store     - Files Store                          │
│  - Vector Store      - Models Registry                      │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                    Backend Services                          │
│  - LLM (OpenAI/Ollama/vLLM)                                 │
│  - Embedding Model                                           │
│  - Vector Database (optional)                                │
└─────────────────────────────────────────────────────────────┘
```

## Implementation Phases

### Phase 1: Models API (Simplest) ✅ START HERE
**Complexity:** Low
**Dependencies:** None
**Time:** ~30 minutes

**Endpoints:**
- `GET /v1/models` - List available models
- `GET /v1/models/{id}` - Get model details

**Implementation:**
1. Create `pkg/core/schema/models.go` - Model types
2. Create `pkg/core/services/models.go` - Models service
3. Add HTTP handlers in `pkg/adapters/http/models_handler.go`
4. Query backend for model list (or use static config)

**Files to Create:**
- `pkg/core/schema/models.go` (~50 lines)
- `pkg/core/services/models.go` (~100 lines)
- `pkg/adapters/http/models_handler.go` (~80 lines)

---

### Phase 2: Chat Completions API (Direct Backend Access)
**Complexity:** Low
**Dependencies:** None (already have ChatCompletionClient)
**Time:** ~45 minutes

**Endpoints:**
- `POST /v1/chat/completions` - Direct chat completions

**Implementation:**
1. Add handler that passes through to ChatCompletionClient
2. Support both streaming and non-streaming
3. No storage required (stateless)

**Files to Create:**
- `pkg/adapters/http/chat_handler.go` (~150 lines)

---

### Phase 3: Complete Responses API
**Complexity:** Medium
**Dependencies:** Storage layer
**Time:** ~2 hours

**Endpoints:**
- ✅ `POST /v1/responses` - Already implemented
- `GET /v1/responses` - List responses with pagination
- `GET /v1/responses/{id}` - Get specific response
- `DELETE /v1/responses/{id}` - Delete response
- `GET /v1/responses/{id}/input_items` - List input items

**Implementation:**
1. Extend `pkg/core/state/session_store.go` interface
2. Implement in `pkg/storage/memory/memory.go`
3. Add HTTP handlers

**Files to Modify:**
- `pkg/core/state/session_store.go` - Add list/get/delete methods
- `pkg/storage/memory/memory.go` - Implement new methods
- `pkg/adapters/http/handler.go` - Add new endpoints

**Files to Create:**
- `pkg/core/schema/pagination.go` (~40 lines)

---

### Phase 4: Conversations API
**Complexity:** Medium
**Dependencies:** Storage layer
**Time:** ~3 hours

**Endpoints:**
- `POST /v1/conversations` - Create conversation
- `GET /v1/conversations` - List conversations
- `GET /v1/conversations/{id}` - Get conversation
- `DELETE /v1/conversations/{id}` - Delete conversation
- `POST /v1/conversations/{id}/items` - Add items
- `GET /v1/conversations/{id}/items` - List items

**Implementation:**
1. Create conversation schema
2. Add ConversationStore interface
3. Implement in-memory storage
4. Add HTTP handlers
5. Integrate with Responses API (previous_response_id vs conversation)

**Files to Create:**
- `pkg/core/schema/conversations.go` (~200 lines)
- `pkg/core/state/conversation_store.go` (~100 lines)
- `pkg/storage/memory/conversations.go` (~250 lines)
- `pkg/adapters/http/conversations_handler.go` (~300 lines)

---

### Phase 5: Prompts API
**Complexity:** Medium
**Dependencies:** Storage layer
**Time:** ~2 hours

**Endpoints:**
- `POST /v1/prompts` - Create prompt template
- `GET /v1/prompts` - List prompts
- `GET /v1/prompts/{id}` - Get prompt
- `PUT /v1/prompts/{id}` - Update prompt
- `DELETE /v1/prompts/{id}` - Delete prompt

**Implementation:**
1. Create prompt schema with template variables
2. Add PromptStore interface
3. Implement template rendering (Go text/template)
4. Add HTTP handlers
5. Integrate with Responses API

**Files to Create:**
- `pkg/core/schema/prompts.go` (~150 lines)
- `pkg/core/state/prompt_store.go` (~80 lines)
- `pkg/storage/memory/prompts.go` (~200 lines)
- `pkg/core/services/prompt_renderer.go` (~120 lines)
- `pkg/adapters/http/prompts_handler.go` (~250 lines)

---

### Phase 6: Files API
**Complexity:** High
**Dependencies:** File system storage
**Time:** ~4 hours

**Endpoints:**
- `POST /v1/files` - Upload file (multipart)
- `GET /v1/files` - List files
- `GET /v1/files/{id}` - Get file metadata
- `GET /v1/files/{id}/content` - Download file
- `DELETE /v1/files/{id}` - Delete file

**Implementation:**
1. Create file schema
2. Add FileStore interface
3. Implement local file storage
4. Add multipart upload handling
5. Add HTTP handlers

**Files to Create:**
- `pkg/core/schema/files.go` (~120 lines)
- `pkg/core/state/file_store.go` (~100 lines)
- `pkg/storage/filesystem/files.go` (~300 lines)
- `pkg/adapters/http/files_handler.go` (~350 lines)

---

### Phase 7: Vector Stores API (Most Complex)
**Complexity:** Very High
**Dependencies:** Files API, Embedding API
**Time:** ~6 hours

**Endpoints:**
- `POST /v1/vector_stores` - Create vector store
- `GET /v1/vector_stores` - List vector stores
- `GET /v1/vector_stores/{id}` - Get vector store
- `DELETE /v1/vector_stores/{id}` - Delete vector store
- `POST /v1/vector_stores/{id}/files` - Add files
- `GET /v1/vector_stores/{id}/files` - List files in store
- `DELETE /v1/vector_stores/{id}/files/{file_id}` - Remove file

**Additional for Search:**
- Internal: Embedding generation
- Internal: Vector similarity search
- Internal: Chunking strategy

**Implementation:**
1. Create vector store schema
2. Add embedding client (OpenAI embeddings API)
3. Implement in-memory vector store (using cosine similarity)
4. Add chunking logic
5. Add HTTP handlers
6. Integrate with Responses API (file_search tool)

**Files to Create:**
- `pkg/core/schema/vector_stores.go` (~200 lines)
- `pkg/core/api/embeddings.go` (~80 lines)
- `pkg/core/api/openai_embeddings.go` (~120 lines)
- `pkg/core/state/vector_store.go` (~150 lines)
- `pkg/storage/memory/vector_store.go` (~400 lines)
- `pkg/core/services/chunker.go` (~150 lines)
- `pkg/adapters/http/vector_stores_handler.go` (~400 lines)

---

## Total Estimated Effort

| Phase | Time | Complexity | Priority |
|-------|------|------------|----------|
| Phase 1: Models API | 30 min | Low | High |
| Phase 2: Chat Completions | 45 min | Low | High |
| Phase 3: Complete Responses | 2 hours | Medium | High |
| Phase 4: Conversations | 3 hours | Medium | Medium |
| Phase 5: Prompts | 2 hours | Medium | Medium |
| Phase 6: Files | 4 hours | High | Low |
| Phase 7: Vector Stores | 6 hours | Very High | Low |
| **Total** | **~18 hours** | | |

## API Endpoint Summary

After implementation, we'll have:

```
┌─────────────────────────────────────────────────────────┐
│ Responses API (7 endpoints)                             │
├─────────────────────────────────────────────────────────┤
│ POST   /v1/responses                                    │
│ GET    /v1/responses                                    │
│ GET    /v1/responses/{id}                               │
│ DELETE /v1/responses/{id}                               │
│ GET    /v1/responses/{id}/input_items                   │
├─────────────────────────────────────────────────────────┤
│ Chat Completions API (1 endpoint)                       │
├─────────────────────────────────────────────────────────┤
│ POST   /v1/chat/completions                             │
├─────────────────────────────────────────────────────────┤
│ Conversations API (6 endpoints)                         │
├─────────────────────────────────────────────────────────┤
│ POST   /v1/conversations                                │
│ GET    /v1/conversations                                │
│ GET    /v1/conversations/{id}                           │
│ DELETE /v1/conversations/{id}                           │
│ POST   /v1/conversations/{id}/items                     │
│ GET    /v1/conversations/{id}/items                     │
├─────────────────────────────────────────────────────────┤
│ Prompts API (5 endpoints)                               │
├─────────────────────────────────────────────────────────┤
│ POST   /v1/prompts                                      │
│ GET    /v1/prompts                                      │
│ GET    /v1/prompts/{id}                                 │
│ PUT    /v1/prompts/{id}                                 │
│ DELETE /v1/prompts/{id}                                 │
├─────────────────────────────────────────────────────────┤
│ Files API (5 endpoints)                                 │
├─────────────────────────────────────────────────────────┤
│ POST   /v1/files                                        │
│ GET    /v1/files                                        │
│ GET    /v1/files/{id}                                   │
│ GET    /v1/files/{id}/content                           │
│ DELETE /v1/files/{id}                                   │
├─────────────────────────────────────────────────────────┤
│ Vector Stores API (7 endpoints)                         │
├─────────────────────────────────────────────────────────┤
│ POST   /v1/vector_stores                                │
│ GET    /v1/vector_stores                                │
│ GET    /v1/vector_stores/{id}                           │
│ DELETE /v1/vector_stores/{id}                           │
│ POST   /v1/vector_stores/{id}/files                     │
│ GET    /v1/vector_stores/{id}/files                     │
│ DELETE /v1/vector_stores/{id}/files/{file_id}           │
├─────────────────────────────────────────────────────────┤
│ Models API (2 endpoints)                                │
├─────────────────────────────────────────────────────────┤
│ GET    /v1/models                                       │
│ GET    /v1/models/{id}                                  │
├─────────────────────────────────────────────────────────┤
│ Health (1 endpoint)                                     │
├─────────────────────────────────────────────────────────┤
│ GET    /health                                          │
└─────────────────────────────────────────────────────────┘

Total: 39 endpoints across 8 APIs
```

## Design Principles

### 1. **Storage Abstraction**
All storage uses interfaces for pluggability:
- In-memory (default, for testing)
- SQL (PostgreSQL, SQLite)
- Cloud (S3 for files, Pinecone for vectors)

### 2. **Stateless Core**
Engine layer is stateless - all state in storage layer

### 3. **OpenAI Compatible**
All APIs match OpenAI's format for compatibility

### 4. **Extensible**
Easy to add new backends, storage, or features

## Testing Strategy

Each phase includes:
- Unit tests for core logic
- Integration tests for HTTP endpoints
- Example curl commands
- Postman collection (optional)

## Documentation

Each API gets:
- OpenAPI/Swagger spec
- README with examples
- Integration guide

## Deployment Considerations

**Minimum for Phase 1-3 (Core functionality):**
- Single binary
- In-memory storage
- Perfect for testing/development

**Production (All Phases):**
- PostgreSQL for persistence
- S3/MinIO for file storage
- Qdrant/Pinecone for vector search
- Redis for caching

## Success Criteria

✅ All 39 endpoints implemented and tested
✅ OpenAPI spec generated
✅ Compatible with OpenAI SDK
✅ Compatible with Llama Stack client
✅ All tests passing
✅ Documentation complete
✅ Docker Compose example works
✅ ExtProc mode supports all endpoints

## Next Steps

1. Start with Phase 1 (Models API) - simplest, immediate value
2. Then Phase 2 (Chat Completions) - unlocks direct LLM access
3. Then Phase 3 (Complete Responses) - finish what we started
4. Continue through phases based on user needs

Let's begin! 🚀
