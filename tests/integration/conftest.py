"""Shared fixtures and constants for integration tests."""

import os

import httpx
import openai
import pytest

BASE_URL = os.environ.get("OPENRESPONSES_BASE_URL", "http://localhost:8080/v1")
API_KEY = os.environ.get("OPENRESPONSES_API_KEY", "unused")
MODEL = os.environ.get("OPENRESPONSES_MODEL", "Qwen/Qwen3-0.6B")
ADAPTER = os.environ.get("OPENRESPONSES_ADAPTER", "http")


def pytest_collection_modifyitems(config, items):
    if ADAPTER != "envoy":
        return
    skip_envoy = pytest.mark.skip(reason="Not supported through Envoy ExtProc")
    for item in items:
        if "envoy_skip" in item.keywords:
            item.add_marker(skip_envoy)


@pytest.fixture(scope="session")
def client():
    return openai.OpenAI(base_url=BASE_URL, api_key=API_KEY)


@pytest.fixture(scope="session")
def base_url():
    return BASE_URL


@pytest.fixture(scope="session")
def model():
    return MODEL


@pytest.fixture(scope="session")
def api_key():
    return API_KEY


@pytest.fixture(scope="session")
def httpx_client():
    return httpx.Client(
        base_url=BASE_URL,
        headers={"Authorization": f"Bearer {API_KEY}"},
        timeout=httpx.Timeout(120.0),
    )
