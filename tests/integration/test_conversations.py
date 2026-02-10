"""Integration tests for the Conversations API."""

import pytest


@pytest.fixture
def create_conversation(client):
    """Helper fixture that creates a conversation and tracks it for cleanup."""
    created_ids = []

    def _create(**kwargs):
        conv = client.conversations.create(**kwargs)
        created_ids.append(conv.id)
        return conv

    yield _create

    for conv_id in created_ids:
        try:
            client.conversations.delete(conv_id)
        except Exception:
            pass


class TestConversations:
    def test_create_conversation(self, create_conversation):
        conv = create_conversation()
        assert conv.id.startswith("conv_")
        assert conv.object == "conversation"

    def test_retrieve_conversation(self, client, create_conversation):
        conv = create_conversation()
        retrieved = client.conversations.retrieve(conv.id)
        assert retrieved.id == conv.id
        assert retrieved.object == "conversation"

    def test_list_conversations(self, client, create_conversation):
        conv1 = create_conversation()
        conv2 = create_conversation()
        # The OpenAI SDK does not expose conversations.list(), so use the
        # SDK's raw HTTP helper to hit GET /conversations directly.
        result = client.get("/conversations", cast_to=object)
        conv_ids = [c["id"] for c in result["data"]]
        assert conv1.id in conv_ids
        assert conv2.id in conv_ids

    def test_delete_conversation(self, client, create_conversation):
        conv = create_conversation()
        deleted = client.conversations.delete(conv.id)
        assert deleted.deleted is True
        assert deleted.object == "conversation.deleted"

    def test_add_and_list_items(self, client, create_conversation):
        conv = create_conversation()
        added = client.conversations.items.create(
            conv.id,
            items=[
                {
                    "type": "message",
                    "role": "user",
                    "content": "Hello from integration test",
                }
            ],
        )
        assert len(added.data) == 1
        assert added.data[0].role == "user"

        items = client.conversations.items.list(conv.id)
        assert len(items.data) >= 1
        contents = [item.content for item in items.data]
        assert "Hello from integration test" in contents

    def test_full_workflow(self, client, create_conversation):
        # Create
        conv = create_conversation()
        assert conv.object == "conversation"

        # Add items
        client.conversations.items.create(
            conv.id,
            items=[
                {
                    "type": "message",
                    "role": "user",
                    "content": "First message",
                },
                {
                    "type": "message",
                    "role": "assistant",
                    "content": "First reply",
                },
            ],
        )

        # List items
        items = client.conversations.items.list(conv.id)
        assert len(items.data) >= 2

        # Delete
        deleted = client.conversations.delete(conv.id)
        assert deleted.deleted is True
