"""Integration tests for the Open Responses Gateway using the OpenAI Python client."""

import os

import openai
import pytest

BASE_URL = os.environ.get("OPENRESPONSES_BASE_URL", "http://localhost:8080/v1")
API_KEY = os.environ.get("OPENRESPONSES_API_KEY", "unused")
MODEL = os.environ.get("OPENRESPONSES_MODEL", "llama3.2:3b")


@pytest.fixture
def client():
    return openai.OpenAI(base_url=BASE_URL, api_key=API_KEY)


class TestNonStreamingResponse:
    def test_basic_response(self, client):
        response = client.responses.create(
            model=MODEL,
            input="What is 2+2? Answer with just the number.",
        )
        assert response.id.startswith("resp_")
        assert response.status == "completed"
        assert len(response.output) > 0
        assert response.output[0].type == "message"
        assert len(response.output[0].content) > 0
        assert response.output[0].content[0].type == "output_text"
        assert len(response.output[0].content[0].text) > 0

    def test_usage_is_populated(self, client):
        response = client.responses.create(
            model=MODEL,
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

    def test_instructions(self, client):
        response = client.responses.create(
            model=MODEL,
            input="What are you?",
            instructions="You are a helpful pirate. Always say 'Arrr' in your response.",
        )
        assert response.status == "completed"
        text = response.output[0].content[0].text.lower()
        assert "arrr" in text or "arr" in text


class TestStreamingResponse:
    def test_streaming_events(self, client):
        seen_events = set()
        final_text = ""

        with client.responses.stream(
            model=MODEL,
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

    def test_stream_get_final_response(self, client):
        with client.responses.stream(
            model=MODEL,
            input="Say hello.",
        ) as stream:
            response = stream.get_final_response()

        assert response.id.startswith("resp_")
        assert response.status == "completed"
        assert len(response.output) > 0


class TestMultiTurnConversation:
    def test_previous_response_id(self, client):
        first = client.responses.create(
            model=MODEL,
            input="My name is Alice. Remember this.",
        )
        assert first.id.startswith("resp_")
        assert first.status == "completed"

        second = client.responses.create(
            model=MODEL,
            input="What is my name?",
            previous_response_id=first.id,
        )
        assert second.status == "completed"
        text = second.output[0].content[0].text.lower()
        assert "alice" in text


class TestToolCalling:
    def test_function_tool(self, client):
        response = client.responses.create(
            model=MODEL,
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
