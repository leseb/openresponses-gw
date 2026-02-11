"""Integration tests for the Open Responses Gateway using the OpenAI Python client."""

import json


class TestNonStreamingResponse:
    def test_basic_response(self, client, model):
        response = client.responses.create(
            model=model,
            input="What is 2+2? Answer with just the number.",
        )
        assert response.id.startswith("resp_")
        assert response.status == "completed"
        assert len(response.output) > 0
        assert response.output[0].type == "message"
        assert len(response.output[0].content) > 0
        assert response.output[0].content[0].type == "output_text"
        assert len(response.output[0].content[0].text) > 0

    def test_usage_is_populated(self, client, model):
        response = client.responses.create(
            model=model,
            input="Say hello.",
        )
        assert response.usage is not None
        assert response.usage.input_tokens > 0
        assert response.usage.output_tokens > 0
        assert response.usage.total_tokens > 0
        assert (
            response.usage.total_tokens
            == response.usage.input_tokens + response.usage.output_tokens
        )

    def test_instructions(self, client, model):
        response = client.responses.create(
            model=model,
            input="What are you?",
            instructions="You are a helpful pirate. Always say 'Arrr' in your response.",
        )
        assert response.status == "completed"
        text = response.output[0].content[0].text.lower()
        assert "arrr" in text or "arr" in text


class TestStreamingResponse:
    def test_streaming_events(self, client, model):
        seen_events = set()
        final_text = ""

        with client.responses.stream(
            model=model,
            input="Say hello.",
        ) as stream:
            for event in stream:
                seen_events.add(event.type)
                if event.type == "response.output_text.delta":
                    final_text += event.delta

        assert "response.created" in seen_events
        assert "response.output_text.delta" in seen_events
        assert "response.completed" in seen_events
        assert len(final_text) > 0

    def test_stream_get_final_response(self, client, model):
        with client.responses.stream(
            model=model,
            input="Say hello.",
        ) as stream:
            response = stream.get_final_response()

        assert response.id.startswith("resp_")
        assert response.status == "completed"
        assert len(response.output) > 0


class TestMultiTurnConversation:
    def test_previous_response_id(self, client, model):
        first = client.responses.create(
            model=model,
            input="My name is Alice. Remember this.",
        )
        assert first.id.startswith("resp_")
        assert first.status == "completed"

        second = client.responses.create(
            model=model,
            input="What is my name?",
            previous_response_id=first.id,
        )
        assert second.status == "completed"
        text = second.output[0].content[0].text.lower()
        assert "alice" in text


class TestToolCalling:
    def test_function_tool(self, client, model):
        response = client.responses.create(
            model=model,
            input="What is the weather in Paris?",
            tools=[
                {
                    "type": "function",
                    "name": "get_weather",
                    "description": "Get the current weather for a location.",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "location": {
                                "type": "string",
                                "description": "City name",
                            },
                        },
                        "required": ["location"],
                    },
                },
            ],
        )

        function_calls = [
            item for item in response.output if item.type == "function_call"
        ]
        assert len(function_calls) > 0

        call = function_calls[0]
        assert call.name == "get_weather"
        assert call.arguments is not None
        assert len(call.arguments) > 0


class TestConversationIntegration:
    """Tests for the conversation field integration with the Responses API."""

    def test_response_auto_creates_conversation(self, httpx_client, model):
        """Every response should auto-create a conversation."""
        resp = httpx_client.post(
            "/responses",
            json={"model": model, "input": "Say hello."},
        )
        assert resp.status_code == 200
        data = resp.json()
        assert "conversation" in data
        assert data["conversation"] is not None
        assert data["conversation"].startswith("conv_")

    def test_continue_conversation(self, httpx_client, model):
        """Sending conversation=<id> should continue in the same conversation."""
        # First request: auto-creates a conversation
        resp1 = httpx_client.post(
            "/responses",
            json={"model": model, "input": "My name is Bob. Remember this."},
        )
        assert resp1.status_code == 200
        data1 = resp1.json()
        conv_id = data1["conversation"]
        assert conv_id.startswith("conv_")

        # Second request: continue in the same conversation
        resp2 = httpx_client.post(
            "/responses",
            json={
                "model": model,
                "input": "What is my name?",
                "conversation": conv_id,
            },
        )
        assert resp2.status_code == 200
        data2 = resp2.json()
        assert data2["conversation"] == conv_id

        # The model should recall the name from the conversation history
        output_text = ""
        for item in data2.get("output", []):
            if item.get("type") == "message":
                for part in item.get("content", []):
                    if part.get("text"):
                        output_text += part["text"]
        assert "bob" in output_text.lower()

    def test_conversation_items_populated(self, httpx_client, model):
        """After a response, conversation items should be available via the Conversations API."""
        # Create a response (auto-creates conversation)
        resp = httpx_client.post(
            "/responses",
            json={"model": model, "input": "Say hello."},
        )
        assert resp.status_code == 200
        data = resp.json()
        conv_id = data["conversation"]

        # List conversation items
        items_resp = httpx_client.get(f"/conversations/{conv_id}/items")
        assert items_resp.status_code == 200
        items_data = items_resp.json()

        # Should have at least a user message and an assistant message
        items = items_data.get("data", [])
        assert len(items) >= 2

        roles = [item.get("role") for item in items]
        assert "user" in roles
        assert "assistant" in roles

    def test_conversation_and_previous_response_id_mutually_exclusive(
        self, httpx_client, model
    ):
        """Sending both conversation and previous_response_id should return 400."""
        resp = httpx_client.post(
            "/responses",
            json={
                "model": model,
                "input": "Hello",
                "conversation": "conv_fake",
                "previous_response_id": "resp_fake",
            },
        )
        assert resp.status_code == 400
        data = resp.json()
        assert "mutually exclusive" in json.dumps(data).lower()

    def test_get_response_includes_conversation(self, httpx_client, model):
        """GET /v1/responses/{id} should include the conversation field."""
        # Create a response
        resp = httpx_client.post(
            "/responses",
            json={"model": model, "input": "Say hello."},
        )
        assert resp.status_code == 200
        data = resp.json()
        resp_id = data["id"]
        conv_id = data["conversation"]

        # Retrieve the response by ID
        get_resp = httpx_client.get(f"/responses/{resp_id}")
        assert get_resp.status_code == 200
        get_data = get_resp.json()
        assert get_data["conversation"] == conv_id
