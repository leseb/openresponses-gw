# Configuration Guide

This guide explains how to configure the Open Responses Gateway. The gateway works with any inference backend that supports `/v1/chat/completions` (default) or `/v1/responses`.

## Quick Start: Testing with OpenAI

### 1. Get an OpenAI API Key

Visit https://platform.openai.com/api-keys and create an API key.

### 2. Set Environment Variable

```bash
export OPENAI_API_KEY=sk-your-api-key-here
```

### 3. Run the Server

```bash
# The server will automatically detect the API key and use real OpenAI
make run

# Or with explicit config
./bin/openresponses-gw-server --config examples/standalone/config-openai.yaml
```

### 4. Test It!

```bash
# Simple test
curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "input": "What is 2+2?"
  }' | jq .

# Or use the comprehensive test script
OPENAI_API_KEY=sk-... ./examples/standalone/test-with-openai.sh
```

## Vector Store & Embedding Configuration

To enable vector search (RAG) and server-side `file_search` tool execution, configure an embedding service and a vector store backend.

### Environment Variables

```bash
# Embedding service (required for vector search)
export EMBEDDING_ENDPOINT="https://api.openai.com/v1"   # or any OpenAI-compatible endpoint
export EMBEDDING_API_KEY="sk-..."
export EMBEDDING_MODEL="text-embedding-3-small"          # default

# Vector store backend
export MILVUS_ADDRESS="localhost:19530"  # automatically selects Milvus backend
```

### YAML Configuration

```yaml
embedding:
  endpoint: https://api.openai.com/v1
  api_key: sk-...                        # prefer EMBEDDING_API_KEY env var
  model: text-embedding-3-small          # default
  dimensions: 1536                       # default

vector_store:
  type: milvus                           # "memory" (default, no-op) or "milvus"
  milvus_address: localhost:19530
```

### How It Works

1. **File ingestion:** When a file is added to a vector store, the gateway reads the file content, splits it into chunks, generates embeddings via the configured embedding service, and inserts the vectors into the Milvus collection.

2. **Search endpoint:** `POST /v1/vector_stores/{id}/search` embeds the query and performs vector similarity search against the Milvus collection.

3. **file_search tool:** When a `file_search` tool is included in a Responses API request and vector search is configured, the engine intercepts the tool call, executes the search server-side, and feeds the results back to the LLM — just like MCP tool execution.

### Without Configuration

If no `EMBEDDING_ENDPOINT` is set, the vector store feature is disabled. The search endpoint returns empty results, and `file_search` is passed through to the LLM as a client-side tool. No behavior changes for existing users.

### Starting Milvus

```bash
# Docker (standalone mode)
docker run -d --name milvus \
  -p 19530:19530 \
  -p 9091:9091 \
  milvusdb/milvus:latest standalone
```

---

## Web Search Configuration

To enable server-side `web_search` tool execution, configure a web search provider. The gateway supports Brave Search and Tavily.

### Environment Variables

```bash
# Brave Search
export WEB_SEARCH_PROVIDER=brave
export WEB_SEARCH_API_KEY="BSA..."   # Brave API subscription token

# Tavily Search
export WEB_SEARCH_PROVIDER=tavily
export WEB_SEARCH_API_KEY="tvly-..." # Tavily API key
```

### YAML Configuration

```yaml
web_search:
  provider: brave            # "brave" or "tavily"
  api_key: BSA...            # prefer WEB_SEARCH_API_KEY env var
```

### How It Works

1. **Tool expansion:** When a `web_search` tool is included in a Responses API request and a provider is configured, the engine replaces it with a synthetic function tool.

2. **Search execution:** When the LLM calls `web_search`, the engine executes the search server-side via the configured provider and feeds the results back to the LLM.

3. **Result sizing:** The `search_context_size` parameter controls result count: `low`=3 results, `medium`=5 (default), `high`=10.

4. **Citations:** Search results are attached as `url_citation` annotations on the final output text.

### Without Configuration

If no `WEB_SEARCH_PROVIDER` is set, the web search feature is disabled and `web_search` tools are passed through to the LLM as-is.

---

## Content Extraction

When files are added to a vector store, the gateway automatically extracts text based on the file extension:

| Extension | Extraction Method |
|-----------|-------------------|
| `.pdf` | Page-by-page text extraction |
| `.html`, `.htm` | Strip tags, skip script/style elements |
| `.csv` | Tab-separated fields, newline-separated rows |
| `.json` | Pretty-printed JSON |
| `.jsonl` | Pretty-printed per line |
| Other | Plain text pass-through |

No configuration needed — extraction is automatic during file ingestion.

---

## File Store Configuration

By default, uploaded files are stored in memory and lost on restart. You can switch to a persistent backend via environment variables or YAML config.

### Environment Variables

```bash
# Filesystem backend
export FILE_STORE_TYPE=filesystem
export FILE_STORE_BASE_DIR=/var/lib/openresponses-gw/files

# S3 / MinIO backend
export FILE_STORE_TYPE=s3
export FILE_STORE_S3_BUCKET=my-files-bucket
export FILE_STORE_S3_REGION=us-east-1
export FILE_STORE_S3_PREFIX=files/               # optional key prefix
export FILE_STORE_S3_ENDPOINT=http://localhost:9000  # for MinIO
```

Setting `FILE_STORE_BASE_DIR` without `FILE_STORE_TYPE` auto-selects `filesystem`. Setting `FILE_STORE_S3_BUCKET` without `FILE_STORE_TYPE` auto-selects `s3`.

### YAML Configuration

```yaml
file_store:
  type: filesystem          # "memory" (default), "filesystem", or "s3"
  base_dir: /tmp/gw-files   # filesystem only

  # S3 / MinIO settings
  # type: s3
  # s3_bucket: my-bucket
  # s3_region: us-east-1
  # s3_prefix: files/
  # s3_endpoint: http://localhost:9000  # for MinIO
```

### Backends

| Backend | Persistence | Use Case |
|---------|-------------|----------|
| `memory` (default) | None — data lost on restart | Development, testing |
| `filesystem` | Local disk | Single-node deployments |
| `s3` | S3-compatible object storage | Production, multi-node, MinIO |

### Starting MinIO (for S3-compatible local testing)

```bash
docker run -d --name minio \
  -p 9000:9000 -p 9001:9001 \
  minio/minio server /data --console-address :9001

# Create a bucket
aws --endpoint-url http://localhost:9000 s3 mb s3://test-bucket

# Configure gateway
export FILE_STORE_TYPE=s3
export FILE_STORE_S3_BUCKET=test-bucket
export FILE_STORE_S3_ENDPOINT=http://localhost:9000
export FILE_STORE_S3_REGION=us-east-1
```

---

## Session Store Configuration

By default, sessions, conversations, and responses are stored in memory and lost on restart. You can switch to a persistent backend via environment variables or YAML config.

### Environment Variables

```bash
# SQLite
export SESSION_STORE_TYPE=sqlite
export SESSION_STORE_DSN=data/responses.db

# PostgreSQL
export SESSION_STORE_TYPE=postgres
export SESSION_STORE_DSN="postgres://user:pass@host:5432/dbname?sslmode=disable"
```

### YAML Configuration

```yaml
# SQLite
session_store:
  type: sqlite              # "sqlite" (default) or "postgres"
  dsn: data/responses.db    # SQLite file path

# PostgreSQL
session_store:
  type: postgres
  dsn: postgres://user:pass@host:5432/dbname?sslmode=disable
```

### Backends

| Backend | Persistence | Use Case |
|---------|-------------|----------|
| `memory` (default) | None — data lost on restart | Development, testing |
| `sqlite` | Local disk (pure Go, no CGO) | Single-node deployments |
| `postgres` | PostgreSQL database (via `pgx/v5`) | Production, multi-node deployments |

The SQLite backend uses WAL mode for concurrent read/write performance. The PostgreSQL backend supports connection pooling and concurrent writers, making it suitable for deployments with multiple replicas. Both store JSON columns for complex fields (request, output, usage, etc.).

---

## Configuration Methods

The gateway supports **3 ways** to configure the inference backend (in order of precedence):

### Method 1: Environment Variables (Recommended)

```bash
# Required — LLM backend
export OPENAI_API_KEY=sk-your-key-here
export OPENAI_API_ENDPOINT=https://api.openai.com/v1  # optional, this is the default

# Optional — backend API mode (default: chat_completions)
export BACKEND_API=chat_completions  # "chat_completions" (default) or "responses"

# Optional — embedding service (enables vector search / RAG)
export EMBEDDING_ENDPOINT=https://api.openai.com/v1
export EMBEDDING_API_KEY=sk-your-key-here
export EMBEDDING_MODEL=text-embedding-3-small  # default

# Optional — vector store backend (auto-selects Milvus when set)
export MILVUS_ADDRESS=localhost:19530

# Optional — web search (enables server-side web_search tool)
export WEB_SEARCH_PROVIDER=brave  # or "tavily"
export WEB_SEARCH_API_KEY=BSA...

# Optional — persistent session store (default: in-memory)
export SESSION_STORE_TYPE=sqlite          # or "postgres"
export SESSION_STORE_DSN=data/responses.db  # or "postgres://user:pass@host:5432/dbname?sslmode=disable"

# Run
./bin/openresponses-gw-server
```

**Pros:**
- Most flexible
- Works with Docker/Kubernetes
- No secrets in config files

---

### Method 2: Configuration File

Create `config.yaml`:

```yaml
server:
  host: 0.0.0.0
  port: 8080
  timeout: 60s

engine:
  model_endpoint: https://api.openai.com/v1
  api_key: sk-your-key-here  # Not recommended - use env vars instead
  backend_api: chat_completions  # "chat_completions" (default) or "responses"
  max_tokens: 4096
  timeout: 60s

session_store:
  type: sqlite               # "sqlite" (default) or "postgres"
  dsn: data/responses.db
```

Run:

```bash
./bin/openresponses-gw-server --config config.yaml
```

**Pros:**
- Explicit configuration
- Easy to version control (without api_key)

---

### Method 3: Command-Line Flags

```bash
./bin/openresponses-gw-server --port 9090
```

**Available flags:**
- `--config <path>` - Path to config file
- `--port <port>` - HTTP port (default: 8080)
- `--version` - Print version

---

## Compatible Backends

The gateway supports two backend API modes, controlled by the `BACKEND_API` env var (or `backend_api` in YAML config):

| Mode | Endpoint Called | Compatible Backends |
|------|----------------|---------------------|
| `chat_completions` (default) | `/v1/chat/completions` | vLLM, Ollama, TGI, OpenAI, any OpenAI-compatible server |
| `responses` | `/v1/responses` | vLLM, Ollama, OpenAI |

### vLLM

```bash
python -m vllm.entrypoints.openai.api_server --model <model>

export OPENAI_API_KEY=unused
export OPENAI_API_ENDPOINT=http://localhost:8000/v1
./bin/openresponses-gw-server
```

### Ollama

```bash
ollama serve

export OPENAI_API_KEY=unused
export OPENAI_API_ENDPOINT=http://localhost:11434/v1
./bin/openresponses-gw-server
```

### OpenAI

```bash
export OPENAI_API_KEY=sk-proj-...
export OPENAI_API_ENDPOINT=https://api.openai.com/v1
./bin/openresponses-gw-server
```

### Using the Responses API backend

If your backend supports the `/v1/responses` endpoint natively (e.g., vLLM):

```bash
export BACKEND_API=responses
export OPENAI_API_ENDPOINT=http://localhost:8000/v1
./bin/openresponses-gw-server
```

---

## Configuration Examples

### Example 1: vLLM (Local)

```yaml
# config-vllm.yaml
server:
  host: 0.0.0.0
  port: 8080
  timeout: 60s

engine:
  model_endpoint: http://localhost:8000/v1
  max_tokens: 4096
  timeout: 60s
```

```bash
python -m vllm.entrypoints.openai.api_server --model <model>
export OPENAI_API_KEY=unused
./bin/openresponses-gw-server --config config-vllm.yaml
```

---

### Example 2: Production OpenAI

```yaml
# config-production.yaml
server:
  host: 0.0.0.0
  port: 8080
  timeout: 60s

engine:
  model_endpoint: https://api.openai.com/v1
  # api_key loaded from OPENAI_API_KEY env var
  max_tokens: 4096
  timeout: 60s
```

```bash
export OPENAI_API_KEY=sk-proj-...
./bin/openresponses-gw-server --config config-production.yaml
```

---

## Request Parameters

When making requests, you can control model behavior:

```bash
curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "input": "Tell me a joke",

    # Optional parameters
    "instructions": "You are a comedian",
    "temperature": 1.5,
    "max_output_tokens": 500,
    "stream": true
  }'
```

### Available Parameters

| Parameter | Type | Description | Default |
|-----------|------|-------------|---------|
| `model` | string | Model to use (required) | - |
| `input` | string | User input (required) | - |
| `instructions` | string | System message | - |
| `temperature` | float | Creativity (0-2) | 1.0 |
| `max_output_tokens` | int | Max response tokens | 4096 |
| `stream` | bool | Enable streaming | false |
| `previous_response_id` | string | For multi-turn | - |
| `seed` | int | Deterministic sampling seed | - |
| `stop` | string/[]string | Stop sequences | - |
| `service_tier` | string | Service tier preference | - |

---

## Validation

### How to Verify Backend Connection

1. **Check server logs:**
```bash
./bin/openresponses-gw-server

# Look for:
# INFO Initialized engine
```

2. **Make a test request:**
```bash
curl -X POST http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{"model":"<model>","input":"ping"}' | jq .
```

---

## Troubleshooting

### "backend returned status 401"

**Problem:** Invalid API key

**Solution:**
```bash
# Check your API key
echo $OPENAI_API_KEY

# Verify it's correct
curl https://api.openai.com/v1/models \
  -H "Authorization: Bearer $OPENAI_API_KEY"
```

---

### "request to backend failed: connection refused"

**Problem:** Backend endpoint not reachable

**Solution:**
```bash
# Ensure your backend is running and the endpoint is correct
curl $OPENAI_API_ENDPOINT/responses \
  -H "Content-Type: application/json" \
  -d '{"model":"<model>","input":"test"}'
```

---

### Streaming doesn't work

**Problem:** Some backends don't support streaming

**Solution:**
```bash
# Test without streaming first
curl -X POST http://localhost:8080/v1/responses \
  -d '{"model":"gpt-4o-mini","input":"hi","stream":false}'

# If that works, backend doesn't support streaming
# Use non-streaming mode
```

---

## Docker Configuration

### Using Environment Variables

```bash
docker run -p 8080:8080 \
  -e OPENAI_API_KEY=sk-... \
  -e OPENAI_API_ENDPOINT=https://api.openai.com/v1 \
  -e BACKEND_API=chat_completions \
  -e EMBEDDING_ENDPOINT=https://api.openai.com/v1 \
  -e EMBEDDING_API_KEY=sk-... \
  -e MILVUS_ADDRESS=host.docker.internal:19530 \
  -e WEB_SEARCH_PROVIDER=brave \
  -e WEB_SEARCH_API_KEY=BSA... \
  -e FILE_STORE_TYPE=s3 \
  -e FILE_STORE_S3_BUCKET=my-bucket \
  -e FILE_STORE_S3_REGION=us-east-1 \
  -e SESSION_STORE_TYPE=sqlite \
  -e SESSION_STORE_DSN=/data/responses.db \
  -v $(pwd)/data:/data \
  openresponses-gw:latest
```

### Using Config File

```bash
docker run -p 8080:8080 \
  -v $(pwd)/config.yaml:/config.yaml \
  openresponses-gw:latest --config /config.yaml
```

### Docker Compose

```yaml
version: '3.8'
services:
  milvus:
    image: milvusdb/milvus:latest
    command: milvus run standalone
    ports:
      - "19530:19530"

  gateway:
    image: openresponses-gw:latest
    ports:
      - "8080:8080"
    depends_on:
      - milvus
    environment:
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - OPENAI_API_ENDPOINT=https://api.openai.com/v1
      - EMBEDDING_ENDPOINT=https://api.openai.com/v1
      - EMBEDDING_API_KEY=${OPENAI_API_KEY}
      - MILVUS_ADDRESS=milvus:19530
      - SESSION_STORE_TYPE=sqlite
      - SESSION_STORE_DSN=/data/responses.db
    volumes:
      - gateway-data:/data

volumes:
  gateway-data:
```

Run:
```bash
OPENAI_API_KEY=sk-... docker-compose up
```

---

## Security Best Practices

1. **Never commit API keys** to version control
2. **Use environment variables** instead of config files
3. **Rotate keys** regularly
4. **Use separate keys** for dev/staging/prod
5. **Monitor usage** in your provider dashboard

---

## Next Steps

- See [QUICKSTART.md](./QUICKSTART.md) for testing examples
- See [CONTRIBUTING.md](../CONTRIBUTING.md) to add new backends
