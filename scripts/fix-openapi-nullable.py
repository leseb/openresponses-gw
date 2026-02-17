#!/usr/bin/env python3
"""Post-process swag-generated OpenAPI 3.1 spec.

Applies several fixes that swag v2 cannot express natively:
  1. Nullable fields — pointer-typed Go fields become anyOf with {type: "null"}.
  2. Files API — fix POST /v1/files multipart schema and remove spurious
     ``type: object`` from the File schema so oasdiff sees full conformance.

Usage:
    python scripts/fix-openapi-nullable.py docs/openapi.yaml
"""

from __future__ import annotations

import sys
from pathlib import Path

import yaml


# ---------------------------------------------------------------------------
# Nullable field definitions
# ---------------------------------------------------------------------------
# Each entry: (schema_name_suffix, field_name, description_override)
# schema_name_suffix is matched against the end of the full schema key so we
# don't have to repeat the long Go module path prefix.
#
# When description_override is None the existing description is preserved.
# ---------------------------------------------------------------------------

NULLABLE_FIELDS: list[tuple[str, str, str | None]] = [
    # schema.Response — descriptions match the openresponses-spec.json reference
    ("schema.Response", "completed_at", None),
    ("schema.Response", "error", "The error that occurred, if the response failed."),
    ("schema.Response", "incomplete_details", "Details about why the response was incomplete, if applicable."),
    ("schema.Response", "usage", None),
    ("schema.Response", "previous_response_id", None),
    ("schema.Response", "conversation", None),
    ("schema.Response", "instructions", None),
    ("schema.Response", "reasoning", None),
    ("schema.Response", "max_output_tokens", None),
    ("schema.Response", "max_tool_calls", None),
]


def _make_nullable(field_schema: dict) -> dict:
    """Wrap a field schema in anyOf with a null variant."""
    # Already nullable (has anyOf with null)
    if "anyOf" in field_schema:
        variants = field_schema["anyOf"]
        if any(v.get("type") == "null" for v in variants):
            return field_schema

    # Build the non-null variant
    if "$ref" in field_schema:
        # $ref field: wrap ref in allOf with description
        non_null: dict = {"$ref": field_schema.pop("$ref")}
        desc = field_schema.pop("description", None)
        if desc:
            non_null = {"allOf": [non_null, {"description": desc}]}
    else:
        # Primitive type field: keep type + description together
        non_null = {}
        for key in list(field_schema.keys()):
            if key not in ("anyOf",):
                non_null[key] = field_schema.pop(key)

    field_schema.clear()
    field_schema["anyOf"] = [non_null, {"type": "null"}]
    return field_schema


def fix_nullable(spec: dict) -> dict:
    """Apply nullable transformations to the spec."""
    schemas = spec.get("components", {}).get("schemas", {})

    for schema_suffix, field_name, desc_override in NULLABLE_FIELDS:
        # Find the schema
        matched_key = None
        for key in schemas:
            if key.endswith(schema_suffix):
                matched_key = key
                break

        if matched_key is None:
            print(f"  warning: schema ending with '{schema_suffix}' not found", file=sys.stderr)
            continue

        props = schemas[matched_key].get("properties", {})
        if field_name not in props:
            print(f"  warning: field '{field_name}' not found in {matched_key}", file=sys.stderr)
            continue

        if desc_override is not None:
            props[field_name]["description"] = desc_override

        _make_nullable(props[field_name])

    return spec


# ---------------------------------------------------------------------------
# Custom YAML representer to quote "null" type values
# ---------------------------------------------------------------------------

class QuotedNull(str):
    """Marker for string values that must be quoted in YAML output."""


def _quoted_null_representer(dumper: yaml.Dumper, data: QuotedNull) -> yaml.Node:
    return dumper.represent_scalar("tag:yaml.org,2002:str", str(data), style='"')


yaml.add_representer(QuotedNull, _quoted_null_representer)


def _tag_null_types(obj):
    """Recursively find {type: "null"} dicts and replace the value with QuotedNull."""
    if isinstance(obj, dict):
        for key, value in obj.items():
            if key == "type" and value == "null":
                obj[key] = QuotedNull("null")
            else:
                _tag_null_types(value)
    elif isinstance(obj, list):
        for item in obj:
            _tag_null_types(item)


def fix_files_api(spec: dict) -> dict:
    """Fix Files API schema issues that swag generates incorrectly.

    1. POST /v1/files — swag puts formData params under
       application/x-www-form-urlencoded instead of multipart/form-data.
       Replace the multipart schema with proper ``file`` and ``purpose``
       properties so oasdiff sees them.
    2. File schema — remove the top-level ``type: object`` that swag emits.
       The OpenAI spec omits it (properties imply it in OpenAPI 3.1) and
       oasdiff flags the extra type as a conformance issue.
    """
    # --- Fix 1: POST /v1/files multipart/form-data schema ---
    post_files = spec.get("paths", {}).get("/v1/files", {}).get("post", {})
    rb_content = post_files.get("requestBody", {}).get("content", {})
    if "multipart/form-data" in rb_content:
        rb_content["multipart/form-data"]["schema"] = {
            "type": "object",
            "required": ["file", "purpose"],
            "properties": {
                "file": {
                    "type": "string",
                    "format": "binary",
                    "description": "File to upload",
                },
                "purpose": {
                    "type": "string",
                    "description": "Purpose: assistants, vision, batch, or fine-tune",
                    "enum": [
                        "assistants",
                        "batch",
                        "fine-tune",
                        "vision",
                        "user_data",
                        "evals",
                    ],
                },
            },
        }
        # Remove the bogus application/x-www-form-urlencoded entry
        rb_content.pop("application/x-www-form-urlencoded", None)

    # --- Fix 2: Remove ``type: object`` from File schema ---
    schemas = spec.get("components", {}).get("schemas", {})
    for key in schemas:
        if key.endswith("schema.File"):
            schemas[key].pop("type", None)
            break

    return spec


def fix_request_body_oneof(spec: dict) -> dict:
    """Fix swag's bogus ``oneOf`` wrapper around requestBody schemas.

    Swag v2 generates requestBody schemas as::

        oneOf:
          - type: object          # empty placeholder
          - $ref: '#/components/schemas/...'

    This confuses oasdiff because the properties live inside the ``$ref``
    variant, not at the top level.  Replace with a direct ``$ref``.
    """
    for _path, methods in spec.get("paths", {}).items():
        for _method, operation in methods.items():
            if not isinstance(operation, dict):
                continue
            rb = operation.get("requestBody", {})
            for _ct, media in rb.get("content", {}).items():
                schema = media.get("schema", {})
                one_of = schema.get("oneOf")
                if not isinstance(one_of, list) or len(one_of) != 2:
                    continue
                # Find the variant that has a $ref
                ref_variant = None
                for variant in one_of:
                    if "$ref" in variant:
                        ref_variant = variant
                        break
                if ref_variant is None:
                    continue
                # Replace the entire schema with a direct $ref
                ref = ref_variant["$ref"]
                schema.clear()
                schema["$ref"] = ref
    return spec


def fix_chunking_strategy_union(spec: dict) -> dict:
    """Rewrite ChunkingStrategy from a flat schema to a oneOf union.

    The OpenAI spec models ``chunking_strategy`` as a ``oneOf`` with two
    discriminated variants (static / other).  Swag generates a flat object
    because Go doesn't have sum types.  Rewrite the schema in-place so
    oasdiff sees the union variants.
    """
    schemas = spec.get("components", {}).get("schemas", {})

    # Find the ChunkingStrategy schema key
    cs_key = None
    for key in schemas:
        if key.endswith("schema.ChunkingStrategy"):
            cs_key = key
            break
    if cs_key is None:
        return spec

    # Create named variant schemas that oasdiff can match by component name
    schemas["StaticChunkingStrategyResponseParam"] = {
        "type": "object",
        "title": "Static Chunking Strategy",
        "additionalProperties": False,
        "properties": {
            "type": {
                "type": "string",
                "description": "Always `static`.",
                "enum": ["static"],
            },
            "static": {
                "type": "object",
                "additionalProperties": False,
                "properties": {
                    "max_chunk_size_tokens": {
                        "type": "integer",
                        "minimum": 100,
                        "maximum": 4096,
                        "description": (
                            "The maximum number of tokens in each chunk. "
                            "The default value is `800`. The minimum value is "
                            "`100` and the maximum value is `4096`."
                        ),
                    },
                    "chunk_overlap_tokens": {
                        "type": "integer",
                        "description": (
                            "The number of tokens that overlap between chunks. "
                            "The default value is `400`.\n\nNote that the overlap "
                            "must not exceed half of `max_chunk_size_tokens`.\n"
                        ),
                    },
                },
                "required": ["max_chunk_size_tokens", "chunk_overlap_tokens"],
            },
        },
        "required": ["type", "static"],
    }
    schemas["OtherChunkingStrategyResponseParam"] = {
        "type": "object",
        "title": "Other Chunking Strategy",
        "description": (
            "This is returned when the chunking strategy is unknown. "
            "Typically, this is because the file was indexed before the "
            "`chunking_strategy` concept was introduced in the API."
        ),
        "additionalProperties": False,
        "properties": {
            "type": {
                "type": "string",
                "description": "Always `other`.",
                "enum": ["other"],
            },
        },
        "required": ["type"],
    }

    schemas[cs_key] = {
        "type": "object",
        "description": "The strategy used to chunk the file.",
        "oneOf": [
            {"$ref": "#/components/schemas/StaticChunkingStrategyResponseParam"},
            {"$ref": "#/components/schemas/OtherChunkingStrategyResponseParam"},
        ],
    }
    return spec


def fix_request_chunking_strategy(spec: dict) -> dict:
    """Replace chunking_strategy $ref in request schemas with request-specific union.

    The OpenAI spec uses different variants for request (auto/static) vs
    response (static/other).  The response variants were handled by
    ``fix_chunking_strategy_union``.  Here we add the request variants and
    patch the request schemas to reference them instead.
    """
    schemas = spec.get("components", {}).get("schemas", {})

    # Add request variant schemas
    schemas["AutoChunkingStrategyRequestParam"] = {
        "type": "object",
        "title": "Auto Chunking Strategy",
        "description": (
            "The default strategy. This strategy currently uses a "
            "`max_chunk_size_tokens` of `800` and `chunk_overlap_tokens` of `400`."
        ),
        "additionalProperties": False,
        "properties": {
            "type": {
                "type": "string",
                "description": "Always `auto`.",
                "enum": ["auto"],
            },
        },
        "required": ["type"],
    }
    schemas["StaticChunkingStrategyRequestParam"] = {
        "type": "object",
        "title": "Static Chunking Strategy",
        "description": (
            "Customize your own chunking strategy by setting chunk size "
            "and chunk overlap."
        ),
        "additionalProperties": False,
        "properties": {
            "type": {
                "type": "string",
                "description": "Always `static`.",
                "enum": ["static"],
            },
            "static": {
                "type": "object",
                "additionalProperties": False,
                "properties": {
                    "max_chunk_size_tokens": {
                        "type": "integer",
                        "minimum": 100,
                        "maximum": 4096,
                        "description": (
                            "The maximum number of tokens in each chunk. "
                            "The default value is `800`. The minimum value is "
                            "`100` and the maximum value is `4096`."
                        ),
                    },
                    "chunk_overlap_tokens": {
                        "type": "integer",
                        "description": (
                            "The number of tokens that overlap between chunks. "
                            "The default value is `400`.\n\nNote that the overlap "
                            "must not exceed half of `max_chunk_size_tokens`.\n"
                        ),
                    },
                },
                "required": ["max_chunk_size_tokens", "chunk_overlap_tokens"],
            },
        },
        "required": ["type", "static"],
    }

    request_cs = {
        "type": "object",
        "description": (
            "The chunking strategy used to chunk the file(s). "
            "If not set, will use the `auto` strategy."
        ),
        "oneOf": [
            {"$ref": "#/components/schemas/AutoChunkingStrategyRequestParam"},
            {"$ref": "#/components/schemas/StaticChunkingStrategyRequestParam"},
        ],
    }

    # Patch request schemas that have chunking_strategy
    request_schema_suffixes = [
        "schema.CreateVectorStoreRequest",
        "schema.AddVectorStoreFileRequest",
        "schema.CreateVectorStoreFileBatchRequest",
    ]
    for key in schemas:
        for suffix in request_schema_suffixes:
            if key.endswith(suffix):
                props = schemas[key].get("properties", {})
                if "chunking_strategy" in props:
                    props["chunking_strategy"] = request_cs
                break

    return spec


def fix_search_request(spec: dict) -> dict:
    """Fix search request schema to match OpenAI spec.

    1. ``query`` — must be oneOf string or array of strings.
    2. ``filters`` — must be oneOf ComparisonFilter / CompoundFilter.
    3. ``max_num_results`` — needs default: 10.
    4. ``rewrite_query`` — needs default: false.
    """
    schemas = spec.get("components", {}).get("schemas", {})

    for key in schemas:
        if not key.endswith("schema.SearchVectorStoreRequest"):
            continue
        props = schemas[key].get("properties", {})

        # Fix query: oneOf [string, array]
        if "query" in props:
            props["query"] = {
                "description": "A query string for a search",
                "oneOf": [
                    {"type": "string"},
                    {
                        "type": "array",
                        "items": {
                            "type": "string",
                            "description": "A list of queries to search for.",
                            "minItems": 1,
                        },
                    },
                ],
            }

        # Fix filters: oneOf [ComparisonFilter, CompoundFilter]
        if "filters" in props:
            # Add named filter schemas
            schemas["ComparisonFilter"] = {
                "type": "object",
                "title": "Comparison filter",
                "description": "A filter used to compare a specified attribute key to a given value using a defined comparison operation.",
                "additionalProperties": False,
                "properties": {
                    "type": {
                        "type": "string",
                        "enum": ["eq", "ne", "gt", "gte", "lt", "lte"],
                    },
                    "key": {"type": "string"},
                    "value": {
                        "oneOf": [
                            {"type": "string"},
                            {"type": "number"},
                            {"type": "boolean"},
                        ],
                    },
                },
                "required": ["type", "key", "value"],
            }
            schemas["CompoundFilter"] = {
                "type": "object",
                "title": "Compound filter",
                "description": "Combine multiple filters using `and` or `or`.",
                "additionalProperties": False,
                "properties": {
                    "type": {
                        "type": "string",
                        "enum": ["and", "or"],
                    },
                    "filters": {
                        "type": "array",
                        "items": {
                            "oneOf": [
                                {"$ref": "#/components/schemas/ComparisonFilter"},
                                {"$ref": "#/components/schemas/CompoundFilter"},
                            ],
                        },
                    },
                },
                "required": ["type", "filters"],
            }
            props["filters"] = {
                "description": "A filter to apply based on file attributes.",
                "oneOf": [
                    {"$ref": "#/components/schemas/ComparisonFilter"},
                    {"$ref": "#/components/schemas/CompoundFilter"},
                ],
            }

        # Fix max_num_results: add default
        if "max_num_results" in props:
            props["max_num_results"]["default"] = 10

        # Fix rewrite_query: add default
        if "rewrite_query" in props:
            props["rewrite_query"]["default"] = False

        break

    return spec


def main():
    if len(sys.argv) != 2:
        print(f"Usage: {sys.argv[0]} <openapi.yaml>", file=sys.stderr)
        sys.exit(1)

    spec_path = Path(sys.argv[1])
    if not spec_path.exists():
        print(f"Error: {spec_path} does not exist", file=sys.stderr)
        sys.exit(1)

    with open(spec_path) as f:
        spec = yaml.safe_load(f)

    fix_nullable(spec)
    fix_files_api(spec)
    fix_request_body_oneof(spec)
    fix_chunking_strategy_union(spec)
    fix_request_chunking_strategy(spec)
    fix_search_request(spec)

    # Tag null types for proper YAML quoting
    _tag_null_types(spec)

    with open(spec_path, "w") as f:
        yaml.dump(spec, f, default_flow_style=False, sort_keys=False, allow_unicode=True, width=120)

    print(f"Fixed nullable fields in {spec_path}")


if __name__ == "__main__":
    main()
