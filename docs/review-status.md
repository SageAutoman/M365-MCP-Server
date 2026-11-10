**English** | [简体中文](zh-CN/review-status.md)

# Implementation review follow-up

## Corrected gaps

- Configuration: empty alternative credentials from the environment template;
  positive/bounded runtime limits and non-empty required scopes.
- Authentication: signing-key rollover refresh with a one-minute miss cooldown;
  negative signature/audience/issuer/expiry/application-token tests.
- Runtime: stateless MCP HTTP identity per request; off-thread OBO; initialize,
  tools/list and concurrent two-user tools/call integration tests.
- Graph/mail: separate KQL search and OData filtering, preserve new IDs after
  move/archive, do not replay ambiguous writes, do not shorten Retry-After,
  reject path traversal and bound response bytes.
- Audit: actual JSON events with timestamp and identity under default logging;
  sanitize errors before framework handling.
- Attachments: metadata-only listings, pre-decode/expanded archive limits and
  supervised parser processes with resource, timeout and cancellation limits.
- Delivery: exclude credentials from Docker contexts, test configurable-port
  health checks/non-root worker startup, bind Compose privately and smoke-test
  the exact image before publishing it.
- Release hardening: pin hosted runners and action majors, verify that a safe MCP
  error reference exactly matches its audit event ID, and describe accepted date
  inputs as ISO 8601 timestamps with timezones.
- Release delivery: require tag/project-version equality and bilingual notes,
  smoke-test the exact Linux x64 image, validate Compose, publish explicit
  semantic image tags, and create the GitHub Release only after image publication.
- Local-runtime foundation: isolate Graph token acquisition behind a provider
  interface and allow tool identity context to be injected without an HTTP
  request. Existing HTTP/OBO behavior remains the hosted runtime.
- OAuth completion UX: WorkBuddy callbacks use a no-store, CSP-protected
  completion page that launches the private callback URI, attempts to close the
  tab, removes callback parameters from visible browser history, and retains a
  manual fallback. Loopback and HTTPS clients retain the normal redirect.
- Windows local integration build: add a .NET 10 NativeAOT MCP stdio host with
  public-client PKCE login, current-user DPAPI state, ephemeral mode, ten
  bounded mail tools, and a CI-produced single Windows x64 executable.

## What CI proves

Unit and integration tests exercise the real ASGI/FastMCP stack with mocked
Entra metadata, OBO and Graph transport. Docker smoke tests use the installed
non-root production image. All Python, Docker, and native Windows builds/tests
run in GitHub Actions; local tests are prohibited in the editing environment.
The mocked WorkBuddy client flow follows the 401 discovery challenge through
DCR, S256 authorization, an Entra callback, local token exchange, authenticated
MCP initialization, refresh-token rotation and another authenticated MCP call.
Connector-package tests also enforce the official directory shape, OAuth mode
without embedded credentials and the four scoped Skills.

The Windows workflow publishes a NativeAOT integration artifact only after the
publish directory contains exactly one `m365-mcp.exe`. Its isolated smoke test
runs with only Windows system directories on `PATH`, exercises
`doctor --ephemeral`, MCP `initialize`, and `tools/list`, verifies the eleven
expected tools, and checks that the isolated data root remains empty.

These tests do not read or change production mailbox data. CI proves the
artifact shape and mocked/protocol behavior; it does not prove live tenant
consent, real Graph behavior, code-signing trust, or target-agent compatibility.

## Current release and remaining deployment work

- `v0.1.1` is [published](https://github.com/sagehou/M365-MCP-Server/releases/tag/v0.1.1)
  under MIT; all four GHCR tags allow anonymous manifest access. Each deployment
  still needs its own Entra configuration and consent checks.
- OAuth discovery, dynamic public-client registration, MSAL-backed interactive
  authorization, S256 PKCE, one-time local authorization-code exchange and
  persistent encrypted refresh sessions with rotation/replay prevention are
  implemented behind a default-off feature flag. The WorkBuddy connector and CI
  client-flow coverage are present. Bounded automation sending is a client/agent
  authorization contract; the server cannot infer that authorization from a
  `draft_id` alone.
- Search returns one bounded page and a next_link hint, not an exhaustive mailbox
  export. The tool does not yet accept continuation cursors. Keyword search obeys
  Graph's search ordering/result limits; date-only queries use received time.
- Plain-text draft creation and draft sending are implemented. v0.1.1 adds
  draft-file attachments up to 20 MiB through a binary upload and one-use
  handle remotely, or a bounded artifact path or native file picker on Windows.
  Sending requires per-message
  confirmation or explicit bounded automation authorization at the client/agent
  layer. Direct send, HTML composition, and automatic retry remain excluded.
- OCR, replies, forwarding, shared mailboxes, enterprise RBAC/rate
  limiting/observability, Calendar, Drive, SharePoint and Teams remain roadmap
  items.
- The stable Windows executable is published unsigned with SHA-256 and GitHub
  provenance. Antivirus false-positive handling, optional signing, and SBOM are
  separate hardening concerns; they do not change the v0.1.1 artifact.
- Windows local mode intentionally omits `mail_download_attachment` and rich
  binary-document extraction. It does not install a Windows Service: the stdio
  process is launched and owned by the agent. Service mode would require a
  separate IPC and security design.
