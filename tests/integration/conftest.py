"""Shared fixtures and constants for integration tests."""

import os

import openai
import pytest

BASE_URL = os.environ.get("OPENRESPONSES_BASE_URL", "http://localhost:8080/v1")
API_KEY = os.environ.get("OPENRESPONSES_API_KEY", "unused")
MODEL = os.environ.get("OPENRESPONSES_MODEL", "llama3.2:3b")


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
