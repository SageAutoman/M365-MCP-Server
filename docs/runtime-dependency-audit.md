**English** | [简体中文](zh-CN/runtime-dependency-audit.md)

# Runtime dependency audit

The Python runtime and development dependencies are pinned in `pyproject.toml`;
`uv.lock` records the complete resolution and distribution hashes. CI and the
production image both install with `uv sync --frozen`.

## Resolved versions

| Package | Resolved version |
| --- | ---: |
| `cryptography` | 50.0.2 |
| `fastapi` | 0.162.2 |
| `fastmcp` / `fastmcp-slim` | 4.1.7 |
| `mcp` / `mcp-types` | 2.3.0 |
| `authlib` | 1.9.5 |
| `joserfc` | 1.8.0 |
| `httpx` | 0.28.1 |
| `httpx2` | 2.16.0 |
| `msal` | 1.39.0 |
| `PyJWT` | 2.15.1 |
| `pypdf` | 7.0.0 |
| `python-docx` | 1.2.0 |
| `openpyxl` | 3.1.5 |
| `python-pptx` | 1.0.2 |
| `pydantic` | 2.14.3 |
| `pydantic-settings` | 2.16.2 |
| `starlette` | 1.10.0 |
| `uvicorn` | 0.54.0 |
| `pytest` | 9.2.0 |
| `pip-audit` | 2.10.1 |

The project intentionally contains both HTTP client families. Application-owned
Graph and OpenID Connect traffic continues to use the direct `httpx` dependency.
FastMCP 4 and MCP SDK v2 use the separate, transitive `httpx2` stack internally.
They have different owners and neither dependency is an accidental duplicate.

Build requirements are `setuptools==84.2.1` and `wheel==0.48.0`; installs use
`uv==0.13.7`. Python remains on the supported 3.12 runtime line.
The pypdf 7 upgrade removes the old upper bound that blocked
the fix for [GHSA-59jv-x5x2-gg4c](https://github.com/py-pdf/pypdf/security/advisories/GHSA-59jv-x5x2-gg4c).

## Compatibility evidence

CI is configured to run the complete Python suite and `pip-audit --skip-editable`
with the locked dependencies. This upgrade still requires a successful CI run.
The production application factory, real ASGI/MCP transport, OAuth refresh
rotation, and mocked Graph/OBO flows remain covered by the suite. Date schema
metadata uses Pydantic's supported `json_schema_extra` API.

The suite covers both protocol eras on that same endpoint:

- Legacy `2025-03-26`: initialize, tools/list, every one of the eleven mail tools,
  sanitized failures and concurrent Alice/Bob identity isolation.
- Modern `2026-07-28`: server/discover, tools/list and tools/call with request
  metadata and no MCP session identifier.
- The full unit/integration suite, including health, unauthenticated MCP
  rejection, and isolated extraction checks.

CI additionally builds and smoke-tests the production Docker image, non-root
worker, and Compose configuration. The smoke script rejects known Authlib JOSE
and HTTPX migration warnings. All builds and tests must run in CI; the editing
environment must not create test environments or execute local tests.

Historical FastMCP 4 migration evidence: [before](https://github.com/sagehou/M365-MCP-Server/actions/runs/35195153651)
and [after](https://github.com/sagehou/M365-MCP-Server/actions/runs/35195810661).

## Scope boundary

The Python server retains bearer validation, Entra OBO, and the existing
feature-flagged OAuth service. The [Windows single-user edition](windows-local-go.md)
has its own Go module and public-client browser login; it does not embed a
server credential or replace the multi-user server. Go versions and checksums
are recorded in `native/go.mod` and `native/go.sum`.

The existing NativeAOT Windows host and workflows are retained. Its SDK/runtime
target moves from .NET 8 to .NET 10 LTS; it has no direct NuGet dependencies.
Both Windows workflows use current GitHub Actions versions. The Go artifact
names include `windows-local-go` and cannot replace the existing Windows assets.

The Go module pins the upstream `x/crypto` fix
`v0.58.1-0.20261009181718-2f1f834330b2` for
[GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932). That commit removes the
retired OpenPGP package; the application does not import OpenPGP.

Dependency and lock-file updates require CI verification with
`uv sync --frozen --extra dev`, the complete tests, and `pip-audit` before images
are published. Record deployed image digests separately from source locks.

---

Mock tests do not certify real tenant/client behavior. Updated: 2026-11-10.
