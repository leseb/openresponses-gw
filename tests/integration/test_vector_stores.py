"""Integration tests for the Vector Stores API."""

import io

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

    def _upload(content=b"vector store test content", filename="vs_test.txt"):
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


class TestVectorStores:
    def test_create_and_retrieve(self, client, create_vector_store):
        vs = create_vector_store(name="test-store")
        assert vs.id.startswith("vs_")
        assert vs.object == "vector_store"
        assert vs.name == "test-store"

        retrieved = client.vector_stores.retrieve(vs.id)
        assert retrieved.id == vs.id
        assert retrieved.name == "test-store"
        assert retrieved.object == "vector_store"

    def test_list_vector_stores(self, client, create_vector_store):
        vs = create_vector_store(name="list-test-store")
        result = client.vector_stores.list()
        store_ids = [item.id for item in result.data]
        assert vs.id in store_ids

    def test_update_vector_store(self, client, base_url, api_key, create_vector_store):
        vs = create_vector_store(name="original-name")
        # The gateway uses PUT for updates, but the OpenAI SDK sends POST.
        # Use httpx directly to match the gateway's route.
        resp = httpx.put(
            f"{base_url}/vector_stores/{vs.id}",
            headers={"Authorization": f"Bearer {api_key}"},
            json={"name": "updated-name", "metadata": {"key": "value"}},
        )
        resp.raise_for_status()
        updated = resp.json()
        assert updated["name"] == "updated-name"
        assert updated["metadata"] == {"key": "value"}

        retrieved = client.vector_stores.retrieve(vs.id)
        assert retrieved.name == "updated-name"
        assert retrieved.metadata == {"key": "value"}

    def test_delete_vector_store(self, client, create_vector_store):
        vs = create_vector_store(name="delete-test")
        deletion = client.vector_stores.delete(vs.id)
        assert deletion.deleted is True

    def test_add_file_to_vector_store(self, client, create_vector_store, upload_file):
        f = upload_file()
        vs = create_vector_store(name="file-test-store")

        vs_file = client.vector_stores.files.create(
            vector_store_id=vs.id,
            file_id=f.id,
        )
        assert vs_file.object == "vector_store.file"

        files = client.vector_stores.files.list(vector_store_id=vs.id)
        file_ids = [item.id for item in files.data]
        assert f.id in file_ids

    def test_remove_file_from_vector_store(
        self, client, create_vector_store, upload_file
    ):
        f = upload_file()
        vs = create_vector_store(name="remove-file-test")

        client.vector_stores.files.create(
            vector_store_id=vs.id,
            file_id=f.id,
        )

        deletion = client.vector_stores.files.delete(
            vector_store_id=vs.id,
            file_id=f.id,
        )
        assert deletion.deleted is True

    def test_file_batch(self, client, create_vector_store, upload_file):
        f1 = upload_file(content=b"batch file 1", filename="batch1.txt")
        f2 = upload_file(content=b"batch file 2", filename="batch2.txt")
        vs = create_vector_store(name="batch-test-store")

        batch = client.vector_stores.file_batches.create(
            vector_store_id=vs.id,
            file_ids=[f1.id, f2.id],
        )
        assert batch.id.startswith("vsfb_")
        assert batch.object == "vector_store.file_batch"

        retrieved_batch = client.vector_stores.file_batches.retrieve(
            vector_store_id=vs.id,
            batch_id=batch.id,
        )
        assert retrieved_batch.id == batch.id
        assert retrieved_batch.file_counts.total == 2
