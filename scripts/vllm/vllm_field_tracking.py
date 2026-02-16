#!/usr/bin/env python3
"""Compare vLLM, gateway, and OpenAI Responses API request fields.

Reads the vLLM OpenAPI spec, the gateway's OpenAPI spec, and the OpenAI
reference spec to produce a structured JSON report of field coverage for
POST /v1/responses.

The report categorises each field into one of:
  - forwarded:      accepted by gateway AND present in vLLM
  - not_forwarded:  present in vLLM but NOT sent by the gateway
  - accepted_not_forwarded:   accepted by gateway but NOT forwarded to vLLM
  - vllm_only:      vLLM extension not in the OpenAI spec
  - not_implemented: in OpenAI spec but in neither gateway nor vLLM

Usage:
    python scripts/vllm/vllm_field_tracking.py [--update]
    make vllm-field-tracking
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

import yaml


def _load_json(path: Path) -> dict:
    with open(path) as f:
        return json.load(f)


def _load_yaml(path: Path) -> dict:
    with open(path) as f:
        return yaml.safe_load(f)


def _extract_vllm_fields(spec: dict) -> set[str]:
    """Extract field names from vLLM's ResponsesRequest schema."""
    schemas = spec.get("components", {}).get("schemas", {})
    for name, schema in schemas.items():
        if "ResponsesRequest" in name:
            return set(schema.get("properties", {}).keys())
    return set()


def _extract_gateway_fields(spec: dict) -> set[str]:
    """Extract field names from gateway's ResponseRequest schema."""
    schemas = spec.get("components", {}).get("schemas", {})
    for name, schema in schemas.items():
        if name.endswith("ResponseRequest"):
            return set(schema.get("properties", {}).keys())
    return set()


def _extract_openai_fields(spec: dict) -> set[str]:
    """Extract field names from OpenAI's CreateResponseBody schema."""
    schemas = spec.get("components", {}).get("schemas", {})
    body = schemas.get("CreateResponseBody", {})
    return set(body.get("properties", {}).keys())


def _extract_forwarded_fields(go_struct_path: Path) -> set[str]:
    """Extract field names from the Go ResponsesAPIRequest struct (what we actually send to vLLM).

    Parses json tags from the Go source to get the exact field list.
    """
    import re

    fields: set[str] = set()
    in_struct = False

    with open(go_struct_path) as f:
        for line in f:
            if "type ResponsesAPIRequest struct" in line:
                in_struct = True
                continue
            if in_struct:
                if line.strip() == "}":
                    break
                m = re.search(r'json:"(\w+)', line)
                if m:
                    fields.add(m.group(1))

    return fields


def build_report(
    vllm_spec_path: Path,
    gateway_spec_path: Path,
    openai_spec_path: Path,
    go_client_path: Path,
) -> dict:
    """Build the field tracking report."""
    vllm_spec = _load_json(vllm_spec_path)
    gateway_spec = _load_yaml(gateway_spec_path)
    openai_spec = _load_json(openai_spec_path)

    vllm_fields = _extract_vllm_fields(vllm_spec)
    gateway_fields = _extract_gateway_fields(gateway_spec)
    openai_fields = _extract_openai_fields(openai_spec)
    forwarded_fields = _extract_forwarded_fields(go_client_path)

    # All fields across all three specs
    all_fields = vllm_fields | gateway_fields | openai_fields

    # Categorise — priority order matters:
    # 1. If we forward it to vLLM → forwarded (regardless of vLLM schema)
    # 2. If vLLM has it + OpenAI has it but we don't send → not_forwarded
    # 3. If gateway accepts but doesn't forward and vLLM doesn't have → accepted_not_forwarded
    # 4. If only in vLLM (not in OpenAI) → vllm_only
    # 5. If only in OpenAI (not in gateway) → not_implemented
    forwarded: list[str] = []
    not_forwarded: list[str] = []
    accepted_not_forwarded: list[str] = []
    vllm_only: list[str] = []
    not_implemented: list[str] = []

    for field in sorted(all_fields):
        in_vllm = field in vllm_fields
        in_gateway = field in gateway_fields
        in_openai = field in openai_fields
        in_forwarded = field in forwarded_fields

        if in_forwarded:
            # We send it to vLLM — it's forwarded
            forwarded.append(field)
        elif in_gateway and not in_forwarded and (in_vllm or not in_vllm):
            # Gateway accepts but doesn't forward to vLLM
            accepted_not_forwarded.append(field)
        elif in_vllm and in_openai and not in_gateway:
            # vLLM supports it, OpenAI spec has it, but we don't accept it
            not_forwarded.append(field)
        elif in_vllm and not in_openai:
            # vLLM extension only
            vllm_only.append(field)
        elif in_openai and not in_gateway:
            # OpenAI spec only — we don't support it at all
            not_implemented.append(field)
        else:
            # Remaining: in vLLM + maybe openai, not in gateway
            if in_vllm and not in_forwarded:
                not_forwarded.append(field)
            elif in_openai:
                not_implemented.append(field)

    # Build detailed field info
    def _field_info(field: str) -> dict:
        info: dict = {"field": field}
        info["in_openai_spec"] = field in openai_fields
        info["in_vllm"] = field in vllm_fields
        info["in_gateway_request"] = field in gateway_fields
        info["forwarded_to_vllm"] = field in forwarded_fields
        return info

    report = {
        "vllm_spec": str(vllm_spec_path),
        "gateway_spec": str(gateway_spec_path),
        "openai_spec": str(openai_spec_path),
        "summary": {
            "forwarded": len(forwarded),
            "not_forwarded": len(not_forwarded),
            "accepted_not_forwarded": len(accepted_not_forwarded),
            "vllm_only": len(vllm_only),
            "not_implemented": len(not_implemented),
            "total_fields": len(all_fields),
        },
        "forwarded": {
            "description": "Fields accepted by gateway AND forwarded to vLLM",
            "fields": [_field_info(f) for f in forwarded],
        },
        "not_forwarded": {
            "description": "Fields vLLM supports (and in OpenAI spec) but gateway does not forward yet",
            "fields": [_field_info(f) for f in not_forwarded],
        },
        "accepted_not_forwarded": {
            "description": "Fields gateway accepts but does not forward to vLLM (handled by gateway or not yet wired)",
            "fields": [_field_info(f) for f in accepted_not_forwarded],
        },
        "vllm_only": {
            "description": "vLLM-specific extensions not in the OpenAI spec",
            "fields": [_field_info(f) for f in vllm_only],
        },
        "not_implemented": {
            "description": "Fields in OpenAI spec not yet accepted by gateway or forwarded to vLLM",
            "fields": [_field_info(f) for f in not_implemented],
        },
    }

    return report


def format_summary(report: dict) -> str:
    """Format a human-readable summary."""
    s = report["summary"]
    lines: list[str] = []
    w = lines.append

    w("")
    w("=" * 60)
    w("vLLM Field Tracking Report — POST /v1/responses")
    w("=" * 60)
    w("")
    w(f"Total unique fields across all specs: {s['total_fields']}")
    w("")
    w(f"  Forwarded to vLLM:      {s['forwarded']:>3}")
    w(f"  Not yet forwarded:      {s['not_forwarded']:>3}")
    w(f"  Accepted, not forwarded:{s['accepted_not_forwarded']:>3}")
    w(f"  vLLM-only extensions:   {s['vllm_only']:>3}")
    w(f"  Not implemented:        {s['not_implemented']:>3}")
    w("")

    for category in ("forwarded", "not_forwarded", "accepted_not_forwarded", "vllm_only", "not_implemented"):
        data = report[category]
        fields = data["fields"]
        if not fields:
            continue
        w(f"{'—' * 50}")
        w(f"{category.upper()} ({len(fields)}): {data['description']}")
        w(f"{'—' * 50}")
        for f in fields:
            markers = []
            if f["in_openai_spec"]:
                markers.append("openai")
            if f["in_vllm"]:
                markers.append("vllm")
            if f["in_gateway_request"]:
                markers.append("gw-req")
            if f["forwarded_to_vllm"]:
                markers.append("fwd")
            w(f"  {f['field']:<30} [{', '.join(markers)}]")
        w("")

    return "\n".join(lines)


def main():
    parser = argparse.ArgumentParser(description="vLLM field tracking for POST /v1/responses")
    parser.add_argument(
        "--vllm-spec",
        type=Path,
        default=Path("scripts/vllm/vllm_0.16.0+cpu_openapi.json"),
        help="Path to vLLM OpenAPI spec",
    )
    parser.add_argument(
        "--gateway-spec",
        type=Path,
        default=Path("docs/openapi.yaml"),
        help="Path to gateway OpenAPI spec",
    )
    parser.add_argument(
        "--openai-spec",
        type=Path,
        default=Path("scripts/conformance/openresponses-spec.json"),
        help="Path to OpenAI/OpenResponses reference spec",
    )
    parser.add_argument(
        "--go-client",
        type=Path,
        default=Path("pkg/core/api/responses_client.go"),
        help="Path to Go ResponsesAPIRequest struct",
    )
    parser.add_argument(
        "--output",
        type=Path,
        default=Path("scripts/vllm/field-tracking.json"),
        help="Output JSON path",
    )
    parser.add_argument(
        "--update",
        action="store_true",
        help="Write results to output file",
    )
    parser.add_argument(
        "--quiet",
        action="store_true",
        help="Only output errors",
    )

    args = parser.parse_args()

    for path in (args.vllm_spec, args.gateway_spec, args.openai_spec, args.go_client):
        if not path.exists():
            print(f"Error: {path} does not exist", file=sys.stderr)
            sys.exit(1)

    report = build_report(args.vllm_spec, args.gateway_spec, args.openai_spec, args.go_client)
    summary = format_summary(report)

    if not args.quiet:
        print(summary)

    if args.update:
        with open(args.output, "w") as f:
            json.dump(report, f, indent=2)
            f.write("\n")

        txt_path = args.output.with_suffix(".txt")
        with open(txt_path, "w") as f:
            f.write(summary.lstrip("\n"))
            f.write("\n")

        if not args.quiet:
            print(f"Written to {args.output} and {txt_path}")


if __name__ == "__main__":
    main()
