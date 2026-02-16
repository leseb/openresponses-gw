#!/usr/bin/env python3
"""Post-process swag-generated OpenAPI 3.1 spec to fix nullable fields.

Swag v2 does not emit anyOf/null for Go pointer types. This script reads the
generated YAML, transforms pointer-typed fields into proper nullable schemas
(anyOf with {type: "null"}), and writes the result back.

The list of nullable fields is derived from the Go source structs where pointer
types (*int64, *string, *SomeStruct) indicate nullable fields per JSON semantics.

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

    # Tag null types for proper YAML quoting
    _tag_null_types(spec)

    with open(spec_path, "w") as f:
        yaml.dump(spec, f, default_flow_style=False, sort_keys=False, allow_unicode=True, width=120)

    print(f"Fixed nullable fields in {spec_path}")


if __name__ == "__main__":
    main()
