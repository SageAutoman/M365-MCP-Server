"""Dependency gates for the MCP runtime modernization phase."""

from importlib import metadata

from packaging.version import Version


def test_fastmcp_4_uses_mcp_sdk_v2_and_split_http_stack() -> None:
    assert Version(metadata.version("fastmcp")) == Version("4.1.7")
    assert Version(metadata.version("mcp")).major == 2
    assert Version(metadata.version("mcp-types")).major == 2
    assert Version(metadata.version("httpx2")).major == 2

    # The application-owned Graph/OIDC transport intentionally remains on httpx.
    assert Version(metadata.version("httpx")).major == 0
