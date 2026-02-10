"""Integration tests for the Files API."""

import io

import pytest


@pytest.fixture
def upload_file(client):
    """Helper fixture that uploads a file and returns its metadata."""
    created_ids = []

    def _upload(content=b"hello world", filename="test.txt", purpose="assistants"):
        f = client.files.create(
            file=(filename, io.BytesIO(content)),
            purpose=purpose,
        )
        created_ids.append(f.id)
        return f

    yield _upload

    for fid in created_ids:
        try:
            client.files.delete(fid)
        except Exception:
            pass


class TestFiles:
    def test_upload_and_retrieve(self, client, upload_file):
        f = upload_file()
        assert f.id.startswith("file_")
        assert f.status == "uploaded"
        assert f.object == "file"

        retrieved = client.files.retrieve(f.id)
        assert retrieved.id == f.id
        assert retrieved.filename == "test.txt"
        assert retrieved.purpose == "assistants"

    def test_list_files(self, client, upload_file):
        f = upload_file()
        result = client.files.list()
        file_ids = [item.id for item in result.data]
        assert f.id in file_ids

    def test_download_content(self, client, upload_file):
        content = b"integration test content"
        f = upload_file(content=content, filename="download.txt")
        downloaded = client.files.content(f.id)
        assert downloaded.read() == content

    def test_delete_file(self, client, upload_file):
        f = upload_file()
        deletion = client.files.delete(f.id)
        assert deletion.deleted is True

        with pytest.raises(Exception):
            client.files.retrieve(f.id)

    def test_list_files_filter_by_purpose(self, client, upload_file):
        f_assistants = upload_file(
            content=b"a", filename="a.txt", purpose="assistants"
        )
        f_vision = upload_file(content=b"b", filename="b.txt", purpose="vision")

        assistants_files = client.files.list(purpose="assistants")
        assistants_ids = [item.id for item in assistants_files.data]
        assert f_assistants.id in assistants_ids
        assert f_vision.id not in assistants_ids

        vision_files = client.files.list(purpose="vision")
        vision_ids = [item.id for item in vision_files.data]
        assert f_vision.id in vision_ids
        assert f_assistants.id not in vision_ids
