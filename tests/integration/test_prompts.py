"""Integration tests for the Prompts API (gateway-specific)."""

import httpx
import pytest


@pytest.fixture
def http_client(base_url, api_key):
    """HTTP client configured for the gateway."""
    return httpx.Client(
        base_url=base_url,
        headers={"Authorization": f"Bearer {api_key}"},
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


class TestPrompts:
    def test_create_prompt(self, create_prompt):
        prompt = create_prompt(
            name="test-prompt",
            template="Hello {{name}}, welcome to {{place}}!",
        )
        assert prompt["id"].startswith("prompt_")
        assert prompt["object"] == "prompt"
        assert prompt["name"] == "test-prompt"
        assert prompt["template"] == "Hello {{name}}, welcome to {{place}}!"

    def test_retrieve_prompt(self, http_client, create_prompt):
        prompt = create_prompt(
            name="retrieve-test",
            template="Tell me about {{topic}}",
        )
        resp = http_client.get(f"/prompts/{prompt['id']}")
        resp.raise_for_status()
        retrieved = resp.json()
        assert retrieved["id"] == prompt["id"]
        assert retrieved["name"] == "retrieve-test"
        assert retrieved["template"] == "Tell me about {{topic}}"

    def test_list_prompts(self, http_client, create_prompt):
        p1 = create_prompt(name="list-test-1", template="Template 1")
        p2 = create_prompt(name="list-test-2", template="Template 2")
        resp = http_client.get("/prompts")
        resp.raise_for_status()
        data = resp.json()
        prompt_ids = [p["id"] for p in data["data"]]
        assert p1["id"] in prompt_ids
        assert p2["id"] in prompt_ids

    def test_update_prompt(self, http_client, create_prompt):
        prompt = create_prompt(name="update-test", template="Original template")
        resp = http_client.put(
            f"/prompts/{prompt['id']}",
            json={"name": "updated-name", "template": "Updated {{template}}"},
        )
        resp.raise_for_status()

        resp = http_client.get(f"/prompts/{prompt['id']}")
        resp.raise_for_status()
        updated = resp.json()
        assert updated["name"] == "updated-name"
        assert updated["template"] == "Updated {{template}}"

    def test_delete_prompt(self, http_client, create_prompt):
        prompt = create_prompt(name="delete-test", template="To be deleted")
        resp = http_client.delete(f"/prompts/{prompt['id']}")
        resp.raise_for_status()
        result = resp.json()
        assert result["deleted"] is True
        assert result["object"] == "prompt.deleted"
