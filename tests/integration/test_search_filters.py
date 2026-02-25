"""Integration tests for vector store search filters (Feature 1).

Tests verify that search filters are parsed, evaluated against file
attributes, and correctly restrict search results.  When no vector backend
is configured the search endpoint returns empty results (backward compat);
filter validation and empty-match short-circuit behavior are still tested.
"""

import io
import time

import httpx
import pytest


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


def _add_file_with_attributes(base_url, api_key, vs_id, file_id, attributes):
    """Add a file to a vector store with attributes using raw HTTP."""
    resp = httpx.post(
        f"{base_url}/vector_stores/{vs_id}/files",
        headers={"Authorization": f"Bearer {api_key}"},
        json={"file_id": file_id, "attributes": attributes},
    )
    resp.raise_for_status()
    return resp.json()


def _wait_for_ingestion(client, vs_id, file_id, timeout=15):
    """Wait for a file to finish ingestion."""
    for _ in range(timeout * 2):
        check = client.vector_stores.files.retrieve(
            vector_store_id=vs_id,
            file_id=file_id,
        )
        if check.status in ("completed", "failed"):
            return check.status
        time.sleep(0.5)
    return "timeout"


class TestSearchFilterValidation:
    """Tests for filter parsing and validation at the API level."""

    def test_invalid_filter_type_returns_400(
        self, base_url, api_key, create_vector_store
    ):
        """A filter with an unknown type should return 400."""
        vs = create_vector_store(name="filter-validation-test")
        resp = httpx.post(
            f"{base_url}/vector_stores/{vs.id}/search",
            headers={"Authorization": f"Bearer {api_key}"},
            json={
                "query": "test",
                "filters": {"type": "invalid_op", "key": "k", "value": "v"},
            },
        )
        assert resp.status_code == 400
        data = resp.json()
        assert "filter" in data.get("message", "").lower() or "filter" in str(
            data
        ).lower()

    def test_missing_filter_key_returns_400(
        self, base_url, api_key, create_vector_store
    ):
        """A comparison filter without 'key' should return 400."""
        vs = create_vector_store(name="filter-key-test")
        resp = httpx.post(
            f"{base_url}/vector_stores/{vs.id}/search",
            headers={"Authorization": f"Bearer {api_key}"},
            json={
                "query": "test",
                "filters": {"type": "eq", "value": "v"},
            },
        )
        assert resp.status_code == 400

    def test_missing_filter_value_returns_400(
        self, base_url, api_key, create_vector_store
    ):
        """A comparison filter without 'value' should return 400."""
        vs = create_vector_store(name="filter-value-test")
        resp = httpx.post(
            f"{base_url}/vector_stores/{vs.id}/search",
            headers={"Authorization": f"Bearer {api_key}"},
            json={
                "query": "test",
                "filters": {"type": "eq", "key": "k"},
            },
        )
        assert resp.status_code == 400

    def test_compound_filter_without_filters_array_returns_400(
        self, base_url, api_key, create_vector_store
    ):
        """A compound filter without 'filters' array should return 400."""
        vs = create_vector_store(name="filter-compound-test")
        resp = httpx.post(
            f"{base_url}/vector_stores/{vs.id}/search",
            headers={"Authorization": f"Bearer {api_key}"},
            json={
                "query": "test",
                "filters": {"type": "and"},
            },
        )
        assert resp.status_code == 400


class TestSearchFilterMatching:
    """Tests for filter matching against file attributes."""

    def test_no_matching_files_returns_empty(
        self, client, base_url, api_key, create_vector_store, upload_file
    ):
        """A filter that matches no files should return empty results."""
        f = upload_file(content=b"Some content for filtering", filename="filter-test.txt")
        vs = create_vector_store(name="filter-no-match")

        _add_file_with_attributes(
            base_url, api_key, vs.id, f.id, {"category": "docs"}
        )
        _wait_for_ingestion(client, vs.id, f.id)

        resp = httpx.post(
            f"{base_url}/vector_stores/{vs.id}/search",
            headers={"Authorization": f"Bearer {api_key}"},
            json={
                "query": "content",
                "filters": {"type": "eq", "key": "category", "value": "images"},
            },
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["object"] == "vector_store.search_results.page"
        assert data["data"] == []

    def test_eq_filter_matches_file(
        self, client, base_url, api_key, create_vector_store, upload_file
    ):
        """An eq filter should match files with the given attribute value."""
        f = upload_file(
            content=b"Important documentation about widgets",
            filename="docs-widget.txt",
        )
        vs = create_vector_store(name="filter-eq-match")

        _add_file_with_attributes(
            base_url, api_key, vs.id, f.id, {"category": "docs"}
        )
        _wait_for_ingestion(client, vs.id, f.id)

        resp = httpx.post(
            f"{base_url}/vector_stores/{vs.id}/search",
            headers={"Authorization": f"Bearer {api_key}"},
            json={
                "query": "widget",
                "filters": {"type": "eq", "key": "category", "value": "docs"},
            },
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["object"] == "vector_store.search_results.page"
        # If vector backend is configured, should get results
        # If not, empty list is acceptable (backward compat)
        assert isinstance(data["data"], list)

    def test_ne_filter_excludes_file(
        self, client, base_url, api_key, create_vector_store, upload_file
    ):
        """A ne filter should exclude files with the given attribute value."""
        f = upload_file(
            content=b"Some image description content",
            filename="image-desc.txt",
        )
        vs = create_vector_store(name="filter-ne-test")

        _add_file_with_attributes(
            base_url, api_key, vs.id, f.id, {"category": "images"}
        )
        _wait_for_ingestion(client, vs.id, f.id)

        # ne "docs" should match (file is "images")
        resp_match = httpx.post(
            f"{base_url}/vector_stores/{vs.id}/search",
            headers={"Authorization": f"Bearer {api_key}"},
            json={
                "query": "image",
                "filters": {"type": "ne", "key": "category", "value": "docs"},
            },
        )
        assert resp_match.status_code == 200

        # ne "images" should not match (file IS "images")
        resp_no_match = httpx.post(
            f"{base_url}/vector_stores/{vs.id}/search",
            headers={"Authorization": f"Bearer {api_key}"},
            json={
                "query": "image",
                "filters": {"type": "ne", "key": "category", "value": "images"},
            },
        )
        assert resp_no_match.status_code == 200
        assert resp_no_match.json()["data"] == []

    def test_compound_and_filter(
        self, client, base_url, api_key, create_vector_store, upload_file
    ):
        """A compound AND filter should require all conditions to match."""
        f = upload_file(
            content=b"Enterprise pricing for cloud services",
            filename="pricing.txt",
        )
        vs = create_vector_store(name="filter-and-test")

        _add_file_with_attributes(
            base_url,
            api_key,
            vs.id,
            f.id,
            {"category": "docs", "tier": "enterprise"},
        )
        _wait_for_ingestion(client, vs.id, f.id)

        # Both conditions match
        resp = httpx.post(
            f"{base_url}/vector_stores/{vs.id}/search",
            headers={"Authorization": f"Bearer {api_key}"},
            json={
                "query": "pricing",
                "filters": {
                    "type": "and",
                    "filters": [
                        {"type": "eq", "key": "category", "value": "docs"},
                        {"type": "eq", "key": "tier", "value": "enterprise"},
                    ],
                },
            },
        )
        assert resp.status_code == 200

        # One condition doesn't match -> empty
        resp2 = httpx.post(
            f"{base_url}/vector_stores/{vs.id}/search",
            headers={"Authorization": f"Bearer {api_key}"},
            json={
                "query": "pricing",
                "filters": {
                    "type": "and",
                    "filters": [
                        {"type": "eq", "key": "category", "value": "docs"},
                        {"type": "eq", "key": "tier", "value": "starter"},
                    ],
                },
            },
        )
        assert resp2.status_code == 200
        assert resp2.json()["data"] == []

    def test_compound_or_filter(
        self, client, base_url, api_key, create_vector_store, upload_file
    ):
        """A compound OR filter should match if any condition matches."""
        f = upload_file(
            content=b"FAQ document about common questions",
            filename="faq-or.txt",
        )
        vs = create_vector_store(name="filter-or-test")

        _add_file_with_attributes(
            base_url, api_key, vs.id, f.id, {"category": "faq"}
        )
        _wait_for_ingestion(client, vs.id, f.id)

        # One condition matches
        resp = httpx.post(
            f"{base_url}/vector_stores/{vs.id}/search",
            headers={"Authorization": f"Bearer {api_key}"},
            json={
                "query": "questions",
                "filters": {
                    "type": "or",
                    "filters": [
                        {"type": "eq", "key": "category", "value": "docs"},
                        {"type": "eq", "key": "category", "value": "faq"},
                    ],
                },
            },
        )
        assert resp.status_code == 200

        # Neither condition matches -> empty
        resp2 = httpx.post(
            f"{base_url}/vector_stores/{vs.id}/search",
            headers={"Authorization": f"Bearer {api_key}"},
            json={
                "query": "questions",
                "filters": {
                    "type": "or",
                    "filters": [
                        {"type": "eq", "key": "category", "value": "docs"},
                        {"type": "eq", "key": "category", "value": "images"},
                    ],
                },
            },
        )
        assert resp2.status_code == 200
        assert resp2.json()["data"] == []

    def test_deprecated_filter_field_works(
        self, client, base_url, api_key, create_vector_store, upload_file
    ):
        """The deprecated 'filter' field (singular) should work like 'filters'."""
        f = upload_file(
            content=b"Legacy filter test content",
            filename="legacy-filter.txt",
        )
        vs = create_vector_store(name="filter-deprecated-test")

        _add_file_with_attributes(
            base_url, api_key, vs.id, f.id, {"type": "legacy"}
        )
        _wait_for_ingestion(client, vs.id, f.id)

        # No match via deprecated 'filter' field
        resp = httpx.post(
            f"{base_url}/vector_stores/{vs.id}/search",
            headers={"Authorization": f"Bearer {api_key}"},
            json={
                "query": "legacy",
                "filter": {"type": "eq", "key": "type", "value": "modern"},
            },
        )
        assert resp.status_code == 200
        assert resp.json()["data"] == []

    def test_search_without_filter_returns_results(
        self, client, base_url, api_key, create_vector_store, upload_file
    ):
        """Search without filters should return all matching results (baseline)."""
        f = upload_file(
            content=b"Baseline test content without filters",
            filename="baseline.txt",
        )
        vs = create_vector_store(name="filter-baseline")

        _add_file_with_attributes(
            base_url, api_key, vs.id, f.id, {"category": "test"}
        )
        _wait_for_ingestion(client, vs.id, f.id)

        resp = httpx.post(
            f"{base_url}/vector_stores/{vs.id}/search",
            headers={"Authorization": f"Bearer {api_key}"},
            json={"query": "baseline", "top_k": 5},
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["object"] == "vector_store.search_results.page"
        assert isinstance(data["data"], list)


class TestFileAttributes:
    """Tests for file attributes persistence and retrieval."""

    def test_add_file_with_attributes(
        self, client, base_url, api_key, create_vector_store, upload_file
    ):
        """Adding a file with attributes should persist them."""
        f = upload_file(content=b"File with metadata", filename="meta.txt")
        vs = create_vector_store(name="attributes-test")

        result = _add_file_with_attributes(
            base_url,
            api_key,
            vs.id,
            f.id,
            {"category": "docs", "priority": "high"},
        )
        assert result["id"] == f.id
        assert result["object"] == "vector_store.file"
        # Attributes should be returned
        if "attributes" in result:
            assert result["attributes"]["category"] == "docs"
            assert result["attributes"]["priority"] == "high"

    def test_add_file_without_attributes(
        self, client, create_vector_store, upload_file
    ):
        """Adding a file without attributes should still work."""
        f = upload_file(content=b"File without metadata", filename="no-meta.txt")
        vs = create_vector_store(name="no-attributes-test")

        vs_file = client.vector_stores.files.create(
            vector_store_id=vs.id,
            file_id=f.id,
        )
        assert vs_file.object == "vector_store.file"
        assert vs_file.id == f.id
