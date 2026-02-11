"""Integration tests for the Connectors API (llama-stack pattern)."""

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
def register_connector(http_client):
    """Helper fixture that registers a connector and tracks it for cleanup."""
    created_ids = []

    def _register(connector_id, url, **kwargs):
        payload = {
            "connector_id": connector_id,
            "connector_type": "mcp",
            "url": url,
            **kwargs,
        }
        resp = http_client.post("/connectors", json=payload)
        resp.raise_for_status()
        data = resp.json()
        created_ids.append(data["connector_id"])
        return data

    yield _register

    for cid in created_ids:
        try:
            http_client.delete(f"/connectors/{cid}")
        except Exception:
            pass


class TestConnectors:
    def test_register_connector(self, register_connector):
        connector = register_connector(
            connector_id="test-mcp-server",
            url="http://localhost:9090/mcp",
            server_label="Test MCP Server",
        )
        assert connector["connector_id"] == "test-mcp-server"
        assert connector["object"] == "connector"
        assert connector["connector_type"] == "mcp"
        assert connector["url"] == "http://localhost:9090/mcp"
        assert connector["server_label"] == "Test MCP Server"
        assert "created_at" in connector

    def test_get_connector(self, http_client, register_connector):
        connector = register_connector(
            connector_id="get-test",
            url="http://localhost:9091/mcp",
        )
        resp = http_client.get(f"/connectors/{connector['connector_id']}")
        resp.raise_for_status()
        retrieved = resp.json()
        assert retrieved["connector_id"] == "get-test"
        assert retrieved["url"] == "http://localhost:9091/mcp"
        assert retrieved["connector_type"] == "mcp"

    def test_list_connectors(self, http_client, register_connector):
        register_connector(connector_id="list-test-1", url="http://localhost:9092/mcp")
        register_connector(connector_id="list-test-2", url="http://localhost:9093/mcp")
        resp = http_client.get("/connectors")
        resp.raise_for_status()
        data = resp.json()
        connector_ids = [c["connector_id"] for c in data["data"]]
        assert "list-test-1" in connector_ids
        assert "list-test-2" in connector_ids

    def test_delete_connector(self, http_client, register_connector):
        connector = register_connector(
            connector_id="delete-test",
            url="http://localhost:9094/mcp",
        )
        resp = http_client.delete(f"/connectors/{connector['connector_id']}")
        resp.raise_for_status()
        result = resp.json()
        assert result["deleted"] is True
        assert result["object"] == "connector.deleted"
        assert result["connector_id"] == "delete-test"

    def test_register_existing_connector_overwrites(
        self, http_client, register_connector
    ):
        register_connector(
            connector_id="overwrite-test",
            url="http://localhost:9095/mcp",
        )
        # Re-register with different URL
        register_connector(
            connector_id="overwrite-test",
            url="http://localhost:9999/mcp-updated",
        )
        resp = http_client.get("/connectors/overwrite-test")
        resp.raise_for_status()
        retrieved = resp.json()
        assert retrieved["url"] == "http://localhost:9999/mcp-updated"
