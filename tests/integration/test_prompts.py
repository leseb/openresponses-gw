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
            json={
                "name": "updated-name",
                "template": "Updated {{template}}",
                "version": 1,
            },
        )
        resp.raise_for_status()
        updated = resp.json()
        assert updated["name"] == "updated-name"
        assert updated["template"] == "Updated {{template}}"
        assert updated["version"] == 2

        # GET without version returns the default (latest)
        resp = http_client.get(f"/prompts/{prompt['id']}")
        resp.raise_for_status()
        retrieved = resp.json()
        assert retrieved["name"] == "updated-name"
        assert retrieved["template"] == "Updated {{template}}"

    def test_delete_prompt(self, http_client, create_prompt):
        prompt = create_prompt(name="delete-test", template="To be deleted")
        resp = http_client.delete(f"/prompts/{prompt['id']}")
        resp.raise_for_status()
        result = resp.json()
        assert result["deleted"] is True
        assert result["object"] == "prompt.deleted"


class TestPromptVersioning:
    def test_create_prompt_has_version(self, create_prompt):
        """Newly created prompt should have version 1 and is_default True."""
        prompt = create_prompt(
            name="versioned-prompt",
            template="Hello {{name}}!",
        )
        assert prompt["version"] == 1
        assert prompt["is_default"] is True

    def test_update_creates_new_version(self, http_client, create_prompt):
        """Updating a prompt should create a new version."""
        prompt = create_prompt(name="version-test", template="V1 template")
        resp = http_client.put(
            f"/prompts/{prompt['id']}",
            json={"template": "V2 {{template}}", "version": 1},
        )
        resp.raise_for_status()
        updated = resp.json()
        assert updated["version"] == 2
        assert updated["template"] == "V2 {{template}}"
        assert updated["is_default"] is True
        assert updated["name"] == "version-test"

    def test_get_prompt_returns_default(self, http_client, create_prompt):
        """GET without version returns the default version."""
        prompt = create_prompt(name="default-test", template="V1")
        http_client.put(
            f"/prompts/{prompt['id']}",
            json={"template": "V2", "version": 1},
        ).raise_for_status()

        resp = http_client.get(f"/prompts/{prompt['id']}")
        resp.raise_for_status()
        retrieved = resp.json()
        assert retrieved["version"] == 2
        assert retrieved["template"] == "V2"
        assert retrieved["is_default"] is True

    def test_get_prompt_specific_version(self, http_client, create_prompt):
        """GET with ?version=N returns that specific version."""
        prompt = create_prompt(name="specific-version-test", template="V1")
        http_client.put(
            f"/prompts/{prompt['id']}",
            json={"template": "V2", "version": 1},
        ).raise_for_status()

        # Get version 1 explicitly
        resp = http_client.get(
            f"/prompts/{prompt['id']}", params={"version": 1}
        )
        resp.raise_for_status()
        v1 = resp.json()
        assert v1["version"] == 1
        assert v1["template"] == "V1"

        # Get version 2 explicitly
        resp = http_client.get(
            f"/prompts/{prompt['id']}", params={"version": 2}
        )
        resp.raise_for_status()
        v2 = resp.json()
        assert v2["version"] == 2
        assert v2["template"] == "V2"

    def test_list_prompt_versions(self, http_client, create_prompt):
        """List all versions of a prompt."""
        prompt = create_prompt(name="list-versions-test", template="V1")
        http_client.put(
            f"/prompts/{prompt['id']}",
            json={"template": "V2", "version": 1},
        ).raise_for_status()
        http_client.put(
            f"/prompts/{prompt['id']}",
            json={"template": "V3", "version": 2},
        ).raise_for_status()

        resp = http_client.get(f"/prompts/{prompt['id']}/versions")
        resp.raise_for_status()
        data = resp.json()
        assert data["object"] == "list"
        versions = data["data"]
        assert len(versions) == 3
        assert versions[0]["version"] == 1
        assert versions[1]["version"] == 2
        assert versions[2]["version"] == 3
        assert versions[0]["template"] == "V1"
        assert versions[1]["template"] == "V2"
        assert versions[2]["template"] == "V3"

    def test_set_default_version(self, http_client, create_prompt):
        """Setting default version changes which version GET returns."""
        prompt = create_prompt(name="set-default-test", template="V1")
        http_client.put(
            f"/prompts/{prompt['id']}",
            json={"template": "V2", "version": 1},
        ).raise_for_status()

        # Default should be V2
        resp = http_client.get(f"/prompts/{prompt['id']}")
        resp.raise_for_status()
        assert resp.json()["version"] == 2

        # Set default back to V1
        resp = http_client.post(
            f"/prompts/{prompt['id']}/default_version",
            json={"version": 1},
        )
        resp.raise_for_status()
        result = resp.json()
        assert result["version"] == 1
        assert result["is_default"] is True

        # GET without version should now return V1
        resp = http_client.get(f"/prompts/{prompt['id']}")
        resp.raise_for_status()
        retrieved = resp.json()
        assert retrieved["version"] == 1
        assert retrieved["template"] == "V1"

    def test_update_requires_latest_version(self, http_client, create_prompt):
        """Updating with a stale version number returns an error."""
        prompt = create_prompt(name="stale-version-test", template="V1")
        http_client.put(
            f"/prompts/{prompt['id']}",
            json={"template": "V2", "version": 1},
        ).raise_for_status()

        # Try to update with version 1 (stale, latest is now 2)
        resp = http_client.put(
            f"/prompts/{prompt['id']}",
            json={"template": "V3", "version": 1},
        )
        assert resp.status_code == 409

    def test_update_requires_version_field(self, http_client, create_prompt):
        """Updating without version field returns 400."""
        prompt = create_prompt(name="no-version-test", template="V1")
        resp = http_client.put(
            f"/prompts/{prompt['id']}",
            json={"template": "V2"},
        )
        assert resp.status_code == 400

    def test_delete_removes_all_versions(self, http_client, create_prompt):
        """Deleting a prompt removes all versions."""
        prompt = create_prompt(name="delete-versions-test", template="V1")
        http_client.put(
            f"/prompts/{prompt['id']}",
            json={"template": "V2", "version": 1},
        ).raise_for_status()

        # Delete
        resp = http_client.delete(f"/prompts/{prompt['id']}")
        resp.raise_for_status()
        assert resp.json()["deleted"] is True

        # Verify all versions are gone
        resp = http_client.get(f"/prompts/{prompt['id']}")
        assert resp.status_code == 404

        resp = http_client.get(
            f"/prompts/{prompt['id']}", params={"version": 1}
        )
        assert resp.status_code == 404

        resp = http_client.get(f"/prompts/{prompt['id']}/versions")
        assert resp.status_code == 404

    def test_list_prompts_returns_default_only(self, http_client, create_prompt):
        """ListPrompts returns only the default version of each prompt."""
        prompt = create_prompt(name="list-default-test", template="V1")
        http_client.put(
            f"/prompts/{prompt['id']}",
            json={"template": "V2", "version": 1},
        ).raise_for_status()

        resp = http_client.get("/prompts")
        resp.raise_for_status()
        data = resp.json()
        matching = [p for p in data["data"] if p["id"] == prompt["id"]]
        assert len(matching) == 1
        assert matching[0]["version"] == 2
        assert matching[0]["is_default"] is True
