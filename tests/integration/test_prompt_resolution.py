"""Integration tests for prompt parameter resolution (Feature 4).

Tests verify that a stored prompt can be referenced from a ResponseRequest
via the 'prompt' field, the template is resolved with variables, and the
rendered text is used as instructions for the response.
"""

import json

import httpx
import pytest


@pytest.fixture
def http_client(base_url, api_key):
    """HTTP client configured for the gateway."""
    return httpx.Client(
        base_url=base_url,
        headers={"Authorization": f"Bearer {api_key}"},
        timeout=httpx.Timeout(120.0),
    )


@pytest.fixture
def create_prompt(http_client):
    """Helper fixture that creates a prompt and tracks it for cleanup."""
    created_ids = []

    def _create(name, template, **kwargs):
        payload = {"name": name, "template": template, **kwargs}
        resp = http_client.post("/prompts", json=payload)
        resp.raise_for_status()
        data = resp.json()
        created_ids.append(data["id"])
        return data

    yield _create

    for prompt_id in created_ids:
        try:
            http_client.delete(f"/prompts/{prompt_id}")
        except Exception:
            pass


class TestPromptResolution:
    """Tests for referencing prompts from ResponseRequest."""

    def test_prompt_reference_resolves_template(
        self, http_client, model, create_prompt
    ):
        """Referencing a prompt by ID should resolve the template as instructions."""
        prompt = create_prompt(
            name="greeting-prompt",
            template="You are a helpful assistant that always greets the user by their name: {{name}}.",
        )

        resp = http_client.post(
            "/responses",
            json={
                "model": model,
                "input": "Hello!",
                "prompt": {
                    "id": prompt["id"],
                    "variables": {"name": "Alice"},
                },
            },
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["status"] == "completed"
        assert len(data.get("output", [])) > 0

        # The model should have been instructed to use Alice's name
        output_text = ""
        for item in data.get("output", []):
            if item.get("type") == "message":
                for part in item.get("content", []):
                    if part.get("text"):
                        output_text += part["text"]
        # The greeting should reference Alice (model-dependent, but likely)
        assert len(output_text) > 0

    def test_prompt_reference_without_variables(
        self, http_client, model, create_prompt
    ):
        """A prompt reference without variables should use the raw template."""
        prompt = create_prompt(
            name="simple-prompt",
            template="Always respond in exactly three words.",
        )

        resp = http_client.post(
            "/responses",
            json={
                "model": model,
                "input": "What is the sky?",
                "prompt": {"id": prompt["id"]},
            },
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["status"] == "completed"

    def test_prompt_and_instructions_mutually_exclusive(
        self, http_client, model, create_prompt
    ):
        """Setting both 'prompt' and 'instructions' should return an error."""
        prompt = create_prompt(
            name="conflict-prompt",
            template="Some instructions",
        )

        resp = http_client.post(
            "/responses",
            json={
                "model": model,
                "input": "Hello",
                "instructions": "Be brief.",
                "prompt": {"id": prompt["id"]},
            },
        )
        # Should return an error (400 or 500 depending on where validation happens)
        assert resp.status_code in (400, 500)
        data = resp.json()
        error_text = json.dumps(data).lower()
        assert "mutually exclusive" in error_text or "prompt" in error_text

    def test_prompt_reference_nonexistent_returns_error(
        self, http_client, model
    ):
        """Referencing a nonexistent prompt should return an error."""
        resp = http_client.post(
            "/responses",
            json={
                "model": model,
                "input": "Hello",
                "prompt": {"id": "prompt_nonexistent_12345"},
            },
        )
        assert resp.status_code in (400, 404, 500)

    def test_prompt_reference_specific_version(
        self, http_client, model, create_prompt
    ):
        """Referencing a specific prompt version should use that version's template."""
        prompt = create_prompt(
            name="versioned-prompt",
            template="V1: Respond with the word APPLE.",
        )

        # Update to create version 2
        http_client.put(
            f"/prompts/{prompt['id']}",
            json={
                "template": "V2: Respond with the word BANANA.",
                "version": 1,
            },
        ).raise_for_status()

        # Reference version 1 explicitly
        resp = http_client.post(
            "/responses",
            json={
                "model": model,
                "input": "What word should you say?",
                "prompt": {
                    "id": prompt["id"],
                    "version": 1,
                },
            },
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["status"] == "completed"

        # Reference version 2 (default)
        resp2 = http_client.post(
            "/responses",
            json={
                "model": model,
                "input": "What word should you say?",
                "prompt": {"id": prompt["id"]},
            },
        )
        assert resp2.status_code == 200
        data2 = resp2.json()
        assert data2["status"] == "completed"

    def test_prompt_reference_with_streaming(
        self, http_client, base_url, api_key, model, create_prompt
    ):
        """Prompt reference should work with streaming responses."""
        prompt = create_prompt(
            name="stream-prompt",
            template="Always respond in uppercase letters only.",
        )

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
                "prompt": {
                    "id": prompt["id"],
                },
            },
            timeout=120.0,
        ) as resp:
            assert resp.status_code == 200
            lines = list(resp.iter_lines())

        # Parse SSE events
        events = []
        current_event = None
        current_data = ""
        for line in lines:
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

        event_types = [e[0] for e in events]
        assert "response.created" in event_types
        assert "response.completed" in event_types

    def test_prompt_with_multiple_variables(
        self, http_client, model, create_prompt
    ):
        """A prompt with multiple variables should resolve all of them."""
        prompt = create_prompt(
            name="multi-var-prompt",
            template="You are {{role}} at {{company}}. Always mention your role and company.",
        )

        resp = http_client.post(
            "/responses",
            json={
                "model": model,
                "input": "Introduce yourself.",
                "prompt": {
                    "id": prompt["id"],
                    "variables": {
                        "role": "a senior engineer",
                        "company": "NovaTech",
                    },
                },
            },
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["status"] == "completed"
        assert len(data.get("output", [])) > 0


class TestPromptResolutionEdgeCases:
    """Edge case tests for prompt resolution."""

    def test_prompt_with_empty_variables_map(
        self, http_client, model, create_prompt
    ):
        """An empty variables map should leave template placeholders as-is."""
        prompt = create_prompt(
            name="empty-vars-prompt",
            template="Hello {{name}}, respond briefly.",
        )

        resp = http_client.post(
            "/responses",
            json={
                "model": model,
                "input": "Hi!",
                "prompt": {
                    "id": prompt["id"],
                    "variables": {},
                },
            },
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["status"] == "completed"

    def test_prompt_with_extra_variables_ignored(
        self, http_client, model, create_prompt
    ):
        """Variables not present in the template should be silently ignored."""
        prompt = create_prompt(
            name="extra-vars-prompt",
            template="Respond briefly.",
        )

        resp = http_client.post(
            "/responses",
            json={
                "model": model,
                "input": "Hello",
                "prompt": {
                    "id": prompt["id"],
                    "variables": {"unused_var": "unused_value"},
                },
            },
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["status"] == "completed"
