"""Integration tests for SSE events on file_search and web_search calls (Feature 2).

Tests verify that the gateway emits the 6 OpenAI-defined streaming events
for built-in tool calls:
  - response.file_search_call.{in_progress,searching,completed}
  - response.web_search_call.{in_progress,searching,completed}

These tests use raw SSE parsing via httpx since the OpenAI SDK may not
expose all raw event types through its stream iterator.
"""

import io
import json
import time

import httpx
import pytest


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


class TestFileSearchSSEEvents:
    """Tests for file_search call lifecycle SSE events."""

    def test_file_search_streaming_emits_lifecycle_events(
        self, client, base_url, api_key, model, create_vector_store, upload_file
    ):
        """Streaming with file_search tool should emit in_progress/searching/completed events.

        This test requires a vector backend to be configured so that the
        engine executes file_search server-side. If file_search is not
        triggered by the model, the test checks that the SSE stream
        completes without errors.
        """
        content = (
            b"NovaTech CloudSync provides enterprise cloud synchronization. "
            b"It uses AES-256 encryption and supports real-time file sync."
        )
        f = upload_file(content=content, filename="cloudsync-sse.txt")
        vs = create_vector_store(name="sse-file-search-test")

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

        # Make streaming request with file_search tool
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
                "input": "What encryption does CloudSync use?",
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

            lines = []
            for line in resp.iter_lines():
                lines.append(line)

        events = parse_sse_events(lines)
        event_types = [e[0] for e in events]

        # The stream should always include created and completed
        assert "response.created" in event_types
        assert "response.completed" in event_types

        # If file_search was triggered, verify lifecycle events
        fs_events = [
            t
            for t in event_types
            if t.startswith("response.file_search_call.")
        ]
        if fs_events:
            assert "response.file_search_call.in_progress" in event_types
            assert "response.file_search_call.searching" in event_types
            assert "response.file_search_call.completed" in event_types

            # Verify ordering: in_progress < searching < completed
            idx_in_progress = event_types.index(
                "response.file_search_call.in_progress"
            )
            idx_searching = event_types.index(
                "response.file_search_call.searching"
            )
            idx_completed = event_types.index(
                "response.file_search_call.completed"
            )
            assert idx_in_progress < idx_searching < idx_completed

            # Verify event payloads have required fields
            for event_type, data in events:
                if event_type.startswith("response.file_search_call."):
                    assert "type" in data
                    assert "output_index" in data
                    assert "item_id" in data
                    assert "sequence_number" in data

    def test_file_search_events_have_unique_item_id(
        self, client, base_url, api_key, model, create_vector_store, upload_file
    ):
        """All file_search lifecycle events for one call should share the same item_id."""
        content = b"CloudSync supports Windows, macOS, Linux, iOS, and Android."
        f = upload_file(content=content, filename="platforms-sse.txt")
        vs = create_vector_store(name="sse-fs-item-id-test")

        client.vector_stores.files.create(
            vector_store_id=vs.id,
            file_id=f.id,
        )

        for _ in range(30):
            check = client.vector_stores.files.retrieve(
                vector_store_id=vs.id,
                file_id=f.id,
            )
            if check.status in ("completed", "failed"):
                break
            time.sleep(0.5)

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
                "input": "What platforms does CloudSync support?",
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

        fs_item_ids = set()
        for event_type, data in events:
            if event_type.startswith("response.file_search_call.") and isinstance(
                data, dict
            ):
                fs_item_ids.add(data.get("item_id"))

        # If file_search was triggered, all lifecycle events should share one item_id
        if fs_item_ids:
            assert len(fs_item_ids) == 1
            item_id = fs_item_ids.pop()
            assert item_id.startswith("fs_")


class TestWebSearchSSEEvents:
    """Tests for web_search call lifecycle SSE events."""

    def test_web_search_streaming_emits_lifecycle_events(
        self, base_url, api_key, model
    ):
        """Streaming with web_search tool should emit in_progress/searching/completed events.

        This test requires a web search provider to be configured. If the
        model doesn't trigger web_search, the test verifies the stream
        completes without errors.
        """
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
                "input": "Search the web for the latest news about AI.",
                "stream": True,
                "tools": [{"type": "web_search"}],
            },
            timeout=120.0,
        ) as resp:
            assert resp.status_code == 200
            lines = list(resp.iter_lines())

        events = parse_sse_events(lines)
        event_types = [e[0] for e in events]

        # Stream should always complete
        assert "response.created" in event_types
        assert "response.completed" in event_types

        # If web_search was triggered, verify lifecycle events
        ws_events = [
            t
            for t in event_types
            if t.startswith("response.web_search_call.")
        ]
        if ws_events:
            assert "response.web_search_call.in_progress" in event_types
            assert "response.web_search_call.searching" in event_types
            assert "response.web_search_call.completed" in event_types

            # Verify ordering
            idx_in_progress = event_types.index(
                "response.web_search_call.in_progress"
            )
            idx_searching = event_types.index(
                "response.web_search_call.searching"
            )
            idx_completed = event_types.index(
                "response.web_search_call.completed"
            )
            assert idx_in_progress < idx_searching < idx_completed

            # Verify event payloads
            for event_type, data in events:
                if event_type.startswith("response.web_search_call."):
                    assert "type" in data
                    assert "output_index" in data
                    assert "item_id" in data
                    assert "sequence_number" in data

    def test_web_search_events_have_unique_item_id(self, base_url, api_key, model):
        """All web_search lifecycle events for one call should share the same item_id."""
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
                "input": "Search the web for the current weather in Paris.",
                "stream": True,
                "tools": [{"type": "web_search"}],
            },
            timeout=120.0,
        ) as resp:
            assert resp.status_code == 200
            lines = list(resp.iter_lines())

        events = parse_sse_events(lines)

        ws_item_ids = set()
        for event_type, data in events:
            if event_type.startswith("response.web_search_call.") and isinstance(
                data, dict
            ):
                ws_item_ids.add(data.get("item_id"))

        if ws_item_ids:
            assert len(ws_item_ids) == 1
            item_id = ws_item_ids.pop()
            assert item_id.startswith("ws_")


class TestSSESequenceNumbers:
    """Tests for sequence number monotonicity in SSE events."""

    def test_sequence_numbers_are_monotonically_increasing(
        self, base_url, api_key, model
    ):
        """All SSE events should have monotonically increasing sequence numbers."""
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
                "input": "Say hello briefly.",
                "stream": True,
            },
            timeout=120.0,
        ) as resp:
            assert resp.status_code == 200
            lines = list(resp.iter_lines())

        events = parse_sse_events(lines)

        seq_nums = []
        for _, data in events:
            if isinstance(data, dict) and "sequence_number" in data:
                seq_nums.append(data["sequence_number"])

        # Verify monotonically increasing
        for i in range(1, len(seq_nums)):
            assert seq_nums[i] > seq_nums[i - 1], (
                f"Sequence number {seq_nums[i]} at index {i} is not greater "
                f"than {seq_nums[i - 1]} at index {i - 1}"
            )
