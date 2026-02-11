"""End-to-end demo: Files, Vector Stores, Prompts, and Responses API workflow.

Exercises the complete API surface in a single ordered test class using a
fictional "NovaTech Solutions" knowledge base scenario.  Three inline text
documents are uploaded, indexed into a vector store, then queried with
file_search, web_search, multi-turn conversations, structured input,
versioned prompt templates, and MCP tool execution.

Inspired by OpenAI's RAG on PDFs cookbook and the llama-stack RAG Lifecycle
notebook pattern of progressive stages.
"""

import io
import subprocess
import sys
import time
from pathlib import Path

import pytest

# ---------------------------------------------------------------------------
# Inline NovaTech Solutions documents
# ---------------------------------------------------------------------------

NOVATECH_DOCS = {
    "product-overview.txt": (
        "NovaTech Solutions - CloudSync Product Overview\n"
        "===============================================\n\n"
        "CloudSync is an enterprise-grade cloud synchronization platform.\n\n"
        "Key Features:\n"
        "- Real-time file synchronization across unlimited devices\n"
        "- End-to-end encryption with AES-256\n"
        "- Automatic conflict resolution with version history\n"
        "- Team collaboration with shared workspaces\n"
        "- API access for custom integrations\n\n"
        "Pricing Tiers:\n"
        "- Starter: $9/user/month - 100 GB storage, 5 users max\n"
        "- Professional: $24/user/month - 1 TB storage, unlimited users\n"
        "- Enterprise: Custom pricing - unlimited storage, SSO, audit logs\n\n"
        "CloudSync supports Windows, macOS, Linux, iOS, and Android.\n"
    ),
    "faq.txt": (
        "NovaTech Solutions - Frequently Asked Questions\n"
        "================================================\n\n"
        "Q: How do I reset my CloudSync password?\n"
        "A: Visit account.novatech.example/reset and follow the instructions.\n\n"
        "Q: What happens when two users edit the same file?\n"
        "A: CloudSync uses automatic conflict resolution. Both versions are\n"
        "   saved and the most recent edit becomes the primary version.\n\n"
        "Q: Is there a file size limit?\n"
        "A: Individual files can be up to 50 GB on Professional and Enterprise\n"
        "   plans. Starter plans have a 5 GB per-file limit.\n\n"
        "Q: Can I use CloudSync offline?\n"
        "A: Yes. CloudSync caches files locally and syncs changes when you\n"
        "   reconnect to the internet.\n\n"
        "Q: How do I contact support?\n"
        "A: Email support@novatech.example or call 1-800-NOVA-TECH.\n"
    ),
    "security-policy.txt": (
        "NovaTech Solutions - Data Security and Compliance Policy\n"
        "========================================================\n\n"
        "1. Encryption: All data is encrypted at rest (AES-256) and in transit\n"
        "   (TLS 1.3). Encryption keys are managed via AWS KMS.\n\n"
        "2. Access Control: Role-based access control (RBAC) with mandatory\n"
        "   multi-factor authentication for admin accounts.\n\n"
        "3. Compliance: CloudSync is SOC 2 Type II certified and GDPR\n"
        "   compliant. Annual penetration testing is performed by third-party\n"
        "   auditors.\n\n"
        "4. Data Residency: Customers can choose data storage regions:\n"
        "   US-East, US-West, EU-Frankfurt, AP-Tokyo.\n\n"
        "5. Incident Response: Security incidents are reported within 24 hours.\n"
        "   Our dedicated security team is available 24/7.\n"
    ),
}


# ---------------------------------------------------------------------------
# Class-scoped fixtures
# ---------------------------------------------------------------------------


@pytest.fixture(scope="class")
def uploaded_files(client):
    """Upload the three NovaTech docs, yield {filename: file_obj}, then delete."""
    files = {}
    for filename, content in NOVATECH_DOCS.items():
        f = client.files.create(
            file=(filename, io.BytesIO(content.encode())),
            purpose="assistants",
        )
        files[filename] = f
    yield files

    for f in files.values():
        try:
            client.files.delete(f.id)
        except Exception:
            pass


@pytest.fixture(scope="class")
def vector_store(client):
    """Create the novatech-knowledge-base vector store, delete on teardown."""
    vs = client.vector_stores.create(name="novatech-knowledge-base")
    yield vs

    try:
        client.vector_stores.delete(vs.id)
    except Exception:
        pass


@pytest.fixture(scope="class")
def mcp_server():
    """Start the NovaTech MCP server as a subprocess."""
    port = 9100
    server_script = str(Path(__file__).parent / "novatech_mcp_server.py")
    proc = subprocess.Popen(
        [sys.executable, server_script, str(port)],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )

    # Wait for server to be ready (retry health check)
    import httpx

    url = f"http://127.0.0.1:{port}/mcp"
    for _ in range(30):
        try:
            # Send an initialize JSON-RPC request as a health check
            resp = httpx.post(
                url,
                json={
                    "jsonrpc": "2.0",
                    "id": 1,
                    "method": "initialize",
                    "params": {
                        "protocolVersion": "2025-03-26",
                        "clientInfo": {"name": "test", "version": "0.1"},
                        "capabilities": {},
                    },
                },
                headers={"Content-Type": "application/json", "Accept": "application/json"},
                timeout=2.0,
            )
            if resp.status_code == 200:
                break
        except (httpx.ConnectError, httpx.ReadTimeout):
            pass
        time.sleep(0.5)
    else:
        proc.terminate()
        proc.wait(timeout=5)
        pytest.fail("MCP server did not start in time")

    yield {"url": url, "port": port, "process": proc}

    proc.terminate()
    try:
        proc.wait(timeout=5)
    except subprocess.TimeoutExpired:
        proc.kill()


@pytest.fixture(scope="class")
def mcp_connector(httpx_client, mcp_server):
    """Register the MCP server as a connector, delete on teardown."""
    resp = httpx_client.post(
        "/connectors",
        json={
            "connector_id": "novatech-mcp",
            "connector_type": "mcp",
            "url": mcp_server["url"],
            "server_label": "NovaTech MCP Server",
        },
    )
    resp.raise_for_status()
    yield resp.json()

    try:
        httpx_client.delete("/connectors/novatech-mcp")
    except Exception:
        pass


# ---------------------------------------------------------------------------
# Demo test class
# ---------------------------------------------------------------------------


class TestDemoWorkflow:
    """Ordered end-to-end demo exercising Files, Vector Stores, Prompts, and Responses."""

    _state: dict = {}

    # -- Stage 1: Files API ------------------------------------------------

    def test_01_upload_documents(self, uploaded_files):
        """Upload 3 NovaTech docs via the Files API."""
        assert len(uploaded_files) == 3
        for filename, f in uploaded_files.items():
            assert f.id.startswith("file_")
            assert f.status == "uploaded"
            assert f.object == "file"
            assert f.filename == filename

    def test_02_list_uploaded_files(self, client, uploaded_files):
        """List files and verify all 3 NovaTech docs are present."""
        result = client.files.list()
        listed_ids = {item.id for item in result.data}
        for f in uploaded_files.values():
            assert f.id in listed_ids

    def test_03_download_file_content(self, client, uploaded_files):
        """Download a file and verify its content matches the original."""
        f = uploaded_files["product-overview.txt"]
        downloaded = client.files.content(f.id)
        assert downloaded.read() == NOVATECH_DOCS["product-overview.txt"].encode()

    # -- Stage 2: Vector Stores API ----------------------------------------

    def test_04_create_vector_store(self, vector_store):
        """Create a named vector store."""
        assert vector_store.id.startswith("vs_")
        assert vector_store.object == "vector_store"
        assert vector_store.name == "novatech-knowledge-base"

    def test_05_add_file_individually(self, client, uploaded_files, vector_store):
        """Add 1 file to the vector store individually."""
        f = uploaded_files["product-overview.txt"]
        vs_file = client.vector_stores.files.create(
            vector_store_id=vector_store.id,
            file_id=f.id,
        )
        assert vs_file.object == "vector_store.file"
        assert vs_file.id == f.id

    def test_06_add_files_via_batch(self, client, uploaded_files, vector_store):
        """Batch-add remaining 2 files to the vector store."""
        remaining_ids = [
            uploaded_files[name].id for name in ("faq.txt", "security-policy.txt")
        ]
        batch = client.vector_stores.file_batches.create(
            vector_store_id=vector_store.id,
            file_ids=remaining_ids,
        )
        assert batch.id.startswith("vsfb_")
        assert batch.object == "vector_store.file_batch"
        assert batch.file_counts.total == 2

    def test_07_verify_vector_store_files(self, client, uploaded_files, vector_store):
        """List vector store files and verify all 3 are present."""
        files = client.vector_stores.files.list(vector_store_id=vector_store.id)
        vs_file_ids = {item.id for item in files.data}
        for f in uploaded_files.values():
            assert f.id in vs_file_ids

    # -- Stage 3: Responses API with tools ---------------------------------

    def test_08_query_with_file_search(
        self, client, httpx_client, model, uploaded_files, vector_store
    ):
        """Responses API + file_search tool; tool is echoed back."""
        response = client.responses.create(
            model=model,
            input="What are the pricing tiers for CloudSync?",
            tools=[
                {
                    "type": "file_search",
                    "vector_store_ids": [vector_store.id],
                }
            ],
        )
        assert response.id.startswith("resp_")
        assert response.status == "completed"

        # file_search tool is echoed
        assert len(response.tools) > 0
        fs_tools = [t for t in response.tools if t.type == "file_search"]
        assert len(fs_tools) == 1
        assert vector_store.id in fs_tools[0].vector_store_ids

        # Output contains at least one message with non-empty text
        assert len(response.output) > 0
        assert response.output[0].type == "message"
        assert len(response.output[0].content) > 0
        assert response.output[0].content[0].type == "output_text"
        assert len(response.output[0].content[0].text) > 0

        # Save state for multi-turn tests
        TestDemoWorkflow._state["response_id"] = response.id
        # conversation is a gateway extension; retrieve via httpx
        get_resp = httpx_client.get(f"/responses/{response.id}")
        TestDemoWorkflow._state["conversation_id"] = get_resp.json().get(
            "conversation"
        )

    def test_09_query_with_web_search(self, httpx_client, model):
        """Responses API + web_search tool with options; tool is echoed.

        Uses httpx because the gateway echoes the tool type as "web_search"
        which doesn't match the SDK's expected "web_search_preview" literal.
        """
        resp = httpx_client.post(
            "/responses",
            json={
                "model": model,
                "input": "What is NovaTech Solutions?",
                "tools": [
                    {
                        "type": "web_search",
                        "search_context_size": "medium",
                        "user_location": {
                            "type": "approximate",
                            "city": "San Francisco",
                            "region": "California",
                            "country": "US",
                        },
                    }
                ],
            },
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["status"] == "completed"

        # web_search tool echoed with options preserved
        tools = data.get("tools", [])
        ws_tools = [t for t in tools if t["type"] == "web_search"]
        assert len(ws_tools) == 1
        assert ws_tools[0].get("search_context_size") == "medium"
        assert ws_tools[0].get("user_location", {}).get("city") == "San Francisco"

    # -- Stage 4: Multi-turn conversations ---------------------------------

    def test_10_multi_turn_previous_response(self, client, model):
        """Follow-up query using previous_response_id."""
        prev_id = TestDemoWorkflow._state.get("response_id")
        assert prev_id is not None, "test_08 must run first to set response_id"

        response = client.responses.create(
            model=model,
            input="Can you tell me more about the Enterprise tier?",
            previous_response_id=prev_id,
        )
        assert response.status == "completed"
        assert response.id.startswith("resp_")
        assert response.id != prev_id

    def test_11_multi_turn_conversation(self, httpx_client, model):
        """Follow-up query using the conversation field.

        Uses httpx because ``conversation`` is a gateway extension not
        present in the OpenAI SDK's request/response types.
        """
        conv_id = TestDemoWorkflow._state.get("conversation_id")
        assert conv_id is not None, "test_08 must run first to set conversation_id"

        resp = httpx_client.post(
            "/responses",
            json={
                "model": model,
                "input": "What security certifications does CloudSync have?",
                "conversation": conv_id,
            },
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["status"] == "completed"
        assert data["conversation"] == conv_id

    # -- Stage 5: Input modes ----------------------------------------------

    def test_12_structured_input(self, client, model):
        """Structured input with input_text content parts."""
        response = client.responses.create(
            model=model,
            input=[
                {
                    "type": "message",
                    "role": "user",
                    "content": [
                        {
                            "type": "input_text",
                            "text": "Summarize the CloudSync product in one sentence.",
                        },
                    ],
                }
            ],
        )
        assert response.status == "completed"
        assert len(response.output) > 0

    # -- Stage 6: Conversations API ----------------------------------------

    def test_13_verify_conversation_items(self, httpx_client):
        """List conversation items via the Conversations API.

        Uses httpx because the Conversations API is a gateway extension
        with no corresponding SDK method.
        """
        conv_id = TestDemoWorkflow._state.get("conversation_id")
        assert conv_id is not None, "test_08 must run first to set conversation_id"

        resp = httpx_client.get(f"/conversations/{conv_id}/items")
        assert resp.status_code == 200
        data = resp.json()

        items = data.get("data", [])
        assert len(items) >= 2  # at least user + assistant from test_08

        roles = {item.get("role") for item in items}
        assert "user" in roles
        assert "assistant" in roles

    def test_14_retrieve_response(self, client, httpx_client):
        """GET a response by ID and verify it includes conversation."""
        resp_id = TestDemoWorkflow._state.get("response_id")
        assert resp_id is not None, "test_08 must run first to set response_id"

        # Retrieve via SDK
        response = client.responses.retrieve(resp_id)
        assert response.id == resp_id
        assert response.status == "completed"

        # Verify conversation field via httpx (gateway extension)
        raw = httpx_client.get(f"/responses/{resp_id}")
        assert raw.status_code == 200
        assert "conversation" in raw.json()

    # -- Stage 7: Versioned Prompts API -----------------------------------

    def test_15_create_prompt_template(self, httpx_client):
        """Create a versioned prompt template for customer inquiries.

        Uses httpx because the Prompts API is a gateway extension with no
        corresponding SDK method.
        """
        resp = httpx_client.post(
            "/prompts",
            json={
                "name": "novatech-inquiry",
                "template": (
                    "You are a NovaTech support agent. "
                    "A customer asks: {{question}}"
                ),
                "description": "Standard customer inquiry template",
            },
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["id"].startswith("prompt_")
        assert data["object"] == "prompt"
        assert data["version"] == 1
        assert data["is_default"] is True
        assert "question" in data["variables"]

        TestDemoWorkflow._state["prompt_id"] = data["id"]

    def test_16_update_prompt_creates_version(self, httpx_client):
        """Update the prompt template; verify a new version is created."""
        prompt_id = TestDemoWorkflow._state["prompt_id"]
        resp = httpx_client.put(
            f"/prompts/{prompt_id}",
            json={
                "template": (
                    "You are a NovaTech CloudSync support agent. "
                    "Answer concisely. Customer asks: {{question}}"
                ),
                "version": 1,
            },
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["version"] == 2
        assert data["is_default"] is True
        assert data["name"] == "novatech-inquiry"

    def test_17_list_prompt_versions(self, httpx_client):
        """List all versions and verify both are present."""
        prompt_id = TestDemoWorkflow._state["prompt_id"]
        resp = httpx_client.get(f"/prompts/{prompt_id}/versions")
        assert resp.status_code == 200
        data = resp.json()
        assert data["object"] == "list"
        versions = data["data"]
        assert len(versions) == 2
        assert versions[0]["version"] == 1
        assert versions[1]["version"] == 2

    def test_18_get_specific_version(self, httpx_client):
        """Retrieve a specific prompt version via query param."""
        prompt_id = TestDemoWorkflow._state["prompt_id"]

        # Version 1 should still have the original template
        resp = httpx_client.get(f"/prompts/{prompt_id}", params={"version": 1})
        assert resp.status_code == 200
        v1 = resp.json()
        assert v1["version"] == 1
        assert "Answer concisely" not in v1["template"]

        # Default (no param) should return version 2
        resp = httpx_client.get(f"/prompts/{prompt_id}")
        assert resp.status_code == 200
        assert resp.json()["version"] == 2

    def test_19_set_default_version(self, httpx_client):
        """Set default back to version 1, verify GET returns it."""
        prompt_id = TestDemoWorkflow._state["prompt_id"]
        resp = httpx_client.post(
            f"/prompts/{prompt_id}/default_version",
            json={"version": 1},
        )
        assert resp.status_code == 200
        assert resp.json()["version"] == 1
        assert resp.json()["is_default"] is True

        # GET without version should now return version 1
        resp = httpx_client.get(f"/prompts/{prompt_id}")
        assert resp.status_code == 200
        assert resp.json()["version"] == 1

        # Restore default to version 2 for subsequent tests
        httpx_client.post(
            f"/prompts/{prompt_id}/default_version",
            json={"version": 2},
        ).raise_for_status()

    def test_20_stale_version_update_rejected(self, httpx_client):
        """Updating with a stale version number returns 409 Conflict."""
        prompt_id = TestDemoWorkflow._state["prompt_id"]
        resp = httpx_client.put(
            f"/prompts/{prompt_id}",
            json={"template": "stale update", "version": 1},
        )
        assert resp.status_code == 409

    # -- Stage 8: Mixed tools ----------------------------------------------

    def test_21_mixed_tools(self, client, model, vector_store):
        """Combine file_search + function tools in a single request."""
        response = client.responses.create(
            model=model,
            input="Look up CloudSync pricing and convert 24 USD to EUR.",
            tools=[
                {
                    "type": "file_search",
                    "vector_store_ids": [vector_store.id],
                },
                {
                    "type": "function",
                    "name": "convert_currency",
                    "description": "Convert an amount from one currency to another.",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "amount": {"type": "number"},
                            "from_currency": {"type": "string"},
                            "to_currency": {"type": "string"},
                        },
                        "required": ["amount", "from_currency", "to_currency"],
                    },
                },
            ],
        )

        # Both tools should be echoed
        tool_types = {t.type for t in response.tools}
        assert "file_search" in tool_types
        assert "function" in tool_types

    # -- Stage 9: MCP tool integration ------------------------------------

    def test_22_register_mcp_connector(self, mcp_connector):
        """Register the NovaTech MCP server via the Connectors API."""
        assert mcp_connector["connector_id"] == "novatech-mcp"
        assert mcp_connector["connector_type"] == "mcp"
        assert mcp_connector["object"] == "connector"

    def test_23_list_connectors(self, httpx_client, mcp_connector):
        """List connectors and verify the MCP registration is present."""
        resp = httpx_client.get("/connectors")
        assert resp.status_code == 200
        data = resp.json()
        connector_ids = [c["connector_id"] for c in data.get("data", [])]
        assert "novatech-mcp" in connector_ids

    def test_24_query_with_mcp_tool(self, httpx_client, model, mcp_connector):
        """Responses API + mcp tool: engine discovers and executes MCP tool server-side.

        Uses httpx because the SDK does not support type="mcp" tools.
        """
        resp = httpx_client.post(
            "/responses",
            json={
                "model": model,
                "input": "What are the pricing tiers for NovaTech CloudSync?",
                "tools": [{"type": "mcp", "server_label": "novatech-mcp"}],
            },
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["status"] == "completed"

        # The engine should have executed the MCP tool server-side
        # and returned a final text response (not a function_call break)
        messages = [o for o in data.get("output", []) if o["type"] == "message"]
        assert len(messages) > 0
        assert len(messages[0]["content"]) > 0
        assert len(messages[0]["content"][0]["text"]) > 0

        # Save for next test
        TestDemoWorkflow._state["mcp_output"] = data.get("output", [])

    def test_25_verify_mcp_output(self):
        """Verify the MCP response includes function_call and function_call_output items."""
        output = TestDemoWorkflow._state.get("mcp_output", [])
        types = [o["type"] for o in output]

        # Should have function_call, function_call_output, and message
        assert "function_call" in types, f"Expected function_call in output types: {types}"
        assert "function_call_output" in types, f"Expected function_call_output in output types: {types}"
        assert "message" in types, f"Expected message in output types: {types}"

    # -- Stage 10: Explicit cleanup ----------------------------------------

    def test_26_cleanup(self, client, httpx_client, uploaded_files, vector_store):
        """Delete prompts, files from VS, VS, files, and MCP connector."""
        # Delete prompt (all versions)
        prompt_id = TestDemoWorkflow._state.get("prompt_id")
        if prompt_id:
            resp = httpx_client.delete(f"/prompts/{prompt_id}")
            assert resp.status_code == 200
            assert resp.json()["deleted"] is True

            # Verify all versions are gone
            resp = httpx_client.get(f"/prompts/{prompt_id}/versions")
            assert resp.status_code == 404

        # Remove files from vector store
        for f in uploaded_files.values():
            deletion = client.vector_stores.files.delete(
                vector_store_id=vector_store.id,
                file_id=f.id,
            )
            assert deletion.deleted is True

        # Verify vector store is empty
        remaining = client.vector_stores.files.list(vector_store_id=vector_store.id)
        assert len(remaining.data) == 0

        # Delete vector store
        vs_deletion = client.vector_stores.delete(vector_store.id)
        assert vs_deletion.deleted is True

        # Delete uploaded files
        for f in uploaded_files.values():
            file_deletion = client.files.delete(f.id)
            assert file_deletion.deleted is True

        # Delete MCP connector
        try:
            resp = httpx_client.delete("/connectors/novatech-mcp")
            assert resp.status_code == 200
        except Exception:
            pass  # may already be cleaned up by fixture
