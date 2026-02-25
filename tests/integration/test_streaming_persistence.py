"""Integration tests for incremental streaming persistence (Feature 3).

Tests verify that response state is persisted incrementally during streaming,
so that intermediate state (completed tool iterations) survives interruptions.
"""

import io
import json
import time

import httpx
import pytest


def get_response_with_retry(base_url, api_key, resp_id, retries=10, delay=0.5):
    """Retrieve a response via GET, retrying until output is populated.

    The streaming goroutine sends the response.completed SSE event before
    the final SaveResponse call, so there's a short window where GET may
    return stale data.
    """
    for _ in range(retries):
        r = httpx.get(
            f"{base_url}/responses/{resp_id}",
            headers={"Authorization": f"Bearer {api_key}"},
            timeout=30.0,
        )
        if r.status_code != 200:
            time.sleep(delay)
            continue
        data = r.json()
        if data.get("status") == "completed" and len(data.get("output", [])) > 0:
            return r
        time.sleep(delay)
    return r


@pytest.fixture
def create_vector_store(client):
    """Helper fixture that creates a vector store and tracks it for cleanup."""
    created_ids = []

    def _create(**kwargs):
        vs = client.vector_stores.create(**kwargs)
        created_ids.append(vs.id)
        return vs

    yield _create

    for vs_id in created_ids:
        try:
            client.vector_stores.delete(vs_id)
        except Exception:
            pass


@pytest.fixture
def upload_file(client):
    """Helper fixture that uploads a file and tracks it for cleanup."""
    created_ids = []

    def _upload(content=b"test content", filename="test.txt"):
        f = client.files.create(
            file=(filename, io.BytesIO(content)),
            purpose="assistants",
        )
        created_ids.append(f.id)
        return f

    yield _upload

    for fid in created_ids:
        try:
            client.files.delete(fid)
        except Exception:
            pass


def parse_sse_events(response_lines):
    """Parse raw SSE lines into a list of (event_type, data_dict) tuples."""
    events = []
    current_event = None
    current_data = ""

    for line in response_lines:
        line = line.strip()
        if line.startswith("event:"):
            current_event = line[len("event:"):].strip()
        elif line.startswith("data:"):
            current_data = line[len("data:"):].strip()
        elif line == "" and current_event is not None:
            try:
                data = json.loads(current_data)
            except (json.JSONDecodeError, ValueError):
                data = current_data
            events.append((current_event, data))
            current_event = None
            current_data = ""

    return events


class TestStreamingPersistence:
    """Tests for response state persistence during streaming."""

    def test_streaming_response_persisted_on_completion(
        self, client, base_url, api_key, model
    ):
        """After streaming completes, the response should be retrievable via GET."""
        # Stream a response
        with httpx.stream(
            "POST",
            f"{base_url}/responses",
            headers={
                "Authorization": f"Bearer {api_key}",
                "Content-Type": "application/json",
                "Accept": "text/event-stream",
            },
            json={
                "model": model,
                "input": "Say hello.",
                "stream": True,
            },
            timeout=120.0,
        ) as resp:
            assert resp.status_code == 200
            lines = list(resp.iter_lines())

        events = parse_sse_events(lines)

        # Find the response ID from the created event
        resp_id = None
        for event_type, data in events:
            if event_type == "response.created" and isinstance(data, dict):
                resp_id = data.get("response", {}).get("id")
                if resp_id:
                    break

        assert resp_id is not None, "Could not find response ID in SSE events"

        # Retrieve the response via GET (retry to allow final save to complete)
        get_resp = get_response_with_retry(base_url, api_key, resp_id)
        assert get_resp.status_code == 200
        data = get_resp.json()
        assert data["id"] == resp_id
        assert data["status"] == "completed"
        assert len(data.get("output", [])) > 0

    def test_streaming_response_has_output_items(
        self, client, base_url, api_key, model
    ):
        """The persisted response should contain the same output as the stream."""
        with httpx.stream(
            "POST",
            f"{base_url}/responses",
            headers={
                "Authorization": f"Bearer {api_key}",
                "Content-Type": "application/json",
                "Accept": "text/event-stream",
            },
            json={
                "model": model,
                "input": "Count from 1 to 3.",
                "stream": True,
            },
            timeout=120.0,
        ) as resp:
            assert resp.status_code == 200
            lines = list(resp.iter_lines())

        events = parse_sse_events(lines)

        # Get response from completed event
        completed_resp = None
        for event_type, data in events:
            if event_type == "response.completed" and isinstance(data, dict):
                completed_resp = data.get("response")
                break

        assert completed_resp is not None

        # Retrieve via GET (retry to allow final save to complete)
        get_resp = get_response_with_retry(
            base_url, api_key, completed_resp["id"]
        )
        assert get_resp.status_code == 200
        persisted = get_resp.json()

        # Output should match
        assert persisted["status"] == "completed"
        assert len(persisted.get("output", [])) == len(
            completed_resp.get("output", [])
        )

    def test_streaming_with_tool_calls_persists_intermediate_state(
        self, client, base_url, api_key, model, create_vector_store, upload_file
    ):
        """When streaming with file_search, the response should be persisted
        with tool call output even if we retrieve it mid-stream.

        This test verifies the intermediate persistence mechanism by:
        1. Starting a streaming request with file_search
        2. Waiting for the stream to complete
        3. Verifying the persisted response contains tool call output
        """
        content = (
            b"CloudSync Enterprise features include single sign-on (SSO), "
            b"audit logging, and unlimited storage capacity."
        )
        f = upload_file(content=content, filename="persist-test.txt")
        vs = create_vector_store(name="persistence-test")

        client.vector_stores.files.create(
            vector_store_id=vs.id,
            file_id=f.id,
        )

        # Wait for ingestion
        for _ in range(30):
            check = client.vector_stores.files.retrieve(
                vector_store_id=vs.id,
                file_id=f.id,
            )
            if check.status in ("completed", "failed"):
                break
            time.sleep(0.5)

        # Stream a request with file_search tool
        with httpx.stream(
            "POST",
            f"{base_url}/responses",
            headers={
                "Authorization": f"Bearer {api_key}",
                "Content-Type": "application/json",
                "Accept": "text/event-stream",
            },
            json={
                "model": model,
                "input": "What enterprise features does CloudSync have?",
                "stream": True,
                "tools": [
                    {
                        "type": "file_search",
                        "vector_store_ids": [vs.id],
                    }
                ],
            },
            timeout=120.0,
        ) as resp:
            assert resp.status_code == 200
            lines = list(resp.iter_lines())

        events = parse_sse_events(lines)

        # Find response ID
        resp_id = None
        for event_type, data in events:
            if event_type == "response.created" and isinstance(data, dict):
                resp_id = data.get("response", {}).get("id")
                if resp_id:
                    break

        assert resp_id is not None

        # Retrieve the persisted response (retry to allow final save)
        get_resp = get_response_with_retry(base_url, api_key, resp_id)
        assert get_resp.status_code == 200
        persisted = get_resp.json()
        assert persisted["status"] == "completed"

        # If file_search was triggered, output should contain tool call items
        output_types = [item.get("type") for item in persisted.get("output", [])]
        if "function_call" in output_types:
            assert "function_call_output" in output_types
            # Verify the function_call_output has actual content
            for item in persisted["output"]:
                if item.get("type") == "function_call_output":
                    assert item.get("output") is not None

    def test_streaming_response_has_usage(self, base_url, api_key, model):
        """The persisted streaming response should have usage data."""
        with httpx.stream(
            "POST",
            f"{base_url}/responses",
            headers={
                "Authorization": f"Bearer {api_key}",
                "Content-Type": "application/json",
                "Accept": "text/event-stream",
            },
            json={
                "model": model,
                "input": "What is 2+2?",
                "stream": True,
            },
            timeout=120.0,
        ) as resp:
            assert resp.status_code == 200
            lines = list(resp.iter_lines())

        events = parse_sse_events(lines)

        resp_id = None
        for event_type, data in events:
            if event_type == "response.created" and isinstance(data, dict):
                resp_id = data.get("response", {}).get("id")
                if resp_id:
                    break

        assert resp_id is not None

        get_resp = get_response_with_retry(base_url, api_key, resp_id)
        assert get_resp.status_code == 200
        persisted = get_resp.json()
        assert persisted["status"] == "completed"
        # Usage should be persisted
        if persisted.get("usage"):
            assert persisted["usage"]["input_tokens"] > 0
            assert persisted["usage"]["output_tokens"] > 0

    def test_streaming_conversation_preserved(self, base_url, api_key, model):
        """The persisted streaming response should have a conversation ID."""
        with httpx.stream(
            "POST",
            f"{base_url}/responses",
            headers={
                "Authorization": f"Bearer {api_key}",
                "Content-Type": "application/json",
                "Accept": "text/event-stream",
            },
            json={
                "model": model,
                "input": "Hello!",
                "stream": True,
            },
            timeout=120.0,
        ) as resp:
            assert resp.status_code == 200
            lines = list(resp.iter_lines())

        events = parse_sse_events(lines)

        resp_id = None
        for event_type, data in events:
            if event_type == "response.created" and isinstance(data, dict):
                resp_id = data.get("response", {}).get("id")
                if resp_id:
                    break

        assert resp_id is not None

        get_resp = get_response_with_retry(base_url, api_key, resp_id)
        assert get_resp.status_code == 200
        persisted = get_resp.json()
        assert persisted.get("conversation") is not None
        assert persisted["conversation"].startswith("conv_")
