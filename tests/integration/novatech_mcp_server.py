"""Minimal MCP server exposing NovaTech company information tools.

Used by integration tests to verify end-to-end MCP tool execution.
Requires the ``mcp`` package (``pip install mcp``).
"""

from mcp.server.fastmcp import FastMCP

mcp_server = FastMCP("novatech-mcp")


@mcp_server.tool()
def get_company_info(topic: str) -> str:
    """Get NovaTech company information on a specific topic.

    Args:
        topic: The topic to look up. Supported: pricing, security, support.
    """
    info = {
        "pricing": (
            "Starter: $9/user/month - 100 GB storage, 5 users max. "
            "Professional: $24/user/month - 1 TB storage, unlimited users. "
            "Enterprise: Custom pricing - unlimited storage, SSO, audit logs."
        ),
        "security": (
            "SOC 2 Type II certified, GDPR compliant. "
            "AES-256 encryption at rest, TLS 1.3 in transit. "
            "Annual third-party penetration testing."
        ),
        "support": (
            "Email support@novatech.example or call 1-800-NOVA-TECH. "
            "Enterprise customers get a dedicated account manager."
        ),
    }
    return info.get(topic, f"No information found for topic: {topic}")


if __name__ == "__main__":
    import sys

    port = int(sys.argv[1]) if len(sys.argv) > 1 else 9100
    mcp_server.run(transport="streamable-http", host="127.0.0.1", port=port)
