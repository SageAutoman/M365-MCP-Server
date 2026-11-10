**English** | [简体中文](README.zh-CN.md)

# M365 MCP Server

A self-hosted Microsoft 365 MCP gateway with a deliberately bounded tool surface,
delegated Microsoft Entra identity, per-user isolation, and security-focused
handling of Outlook mail and attachments.

> **Released:** [`v0.1.1`](https://github.com/sagehou/M365-MCP-Server/releases/tag/v0.1.1)
> is available under the MIT License. CI validated the container image and Windows
> x64 NativeAOT executable. Deployment-specific Entra consent and client-side
> send authorization remain the responsibility of each installation.

## What works today

### Remote MCP server

Start with the [remote MCP server guide](docs/mcp-server.md) for the container,
client connection and attachment-upload paths.

- FastMCP Streamable HTTP endpoint at `/mcp/`.
- Microsoft Entra bearer-token validation with tenant and scope enforcement.
- Microsoft Graph delegated access through On-Behalf-Of (OBO).
- Per-request user identity isolation; Graph calls are confined to `/me`.
- Optional OAuth discovery, dynamic public-client registration, interactive
  sign-in, S256 PKCE, encrypted refresh sessions, rotation, and replay rejection.
- A WorkBuddy connector package under `workbuddy/` with a mocked end-to-end
  client flow in CI.
- Structured, redacted audit events with correlation IDs returned in safe errors.
- `mail_add_draft_attachment` adds up to 20 MiB to an existing draft without
  sending. A remote client explicitly uploads raw bytes, then passes a
  short-lived handle to MCP. For unattended client-side artifacts, OAuth mode
  can issue a draft-bound push grant; a trusted local uploader supplies the
  bytes. See [remote draft attachments](docs/mcp-server.md#add-a-file-to-a-remote-draft).

### Windows local executable

See the [Windows local guide](docs/windows-local.md) for `stdio`, local login,
relative-path draft attachments and the optional EXE helper for a remote
connector.

The Windows x64 NativeAOT local runtime is included in v0.1.1. The release
provides:

- `m365-mcp-windows-x64.exe`
- SHA-256 checksum
- MD5 and SHA-1 in the Release description for exact-file antivirus allowlisting
- GitHub Artifact Attestation provenance
- `LICENSE` legal notice (not a runtime dependency)

The executable does not require Python, .NET runtime, Docker, or an installer on
the target PC. Authenticode signing remains a future hardening item. If security
software quarantines the unsigned EXE, follow the
[false-positive handling guide](docs/windows-local.md#antivirus-false-positives);
the checksum and attestation establish provenance but do not override antivirus
decisions. MD5 and SHA-1 are only identifiers for legacy allowlist forms, not
authenticity checks; a changed EXE needs new hashes and a new allowlist review.

### Windows local Go executable

The [Go edition](docs/windows-local-go.md) uses its own `windows-local-go`
workflow to produce standalone AMD64 and ARM64 executables. It supports browser
sign-in, stdio, authenticated loopback HTTP, and attachment text extraction.
The existing NativeAOT workflow and release assets remain available.

## Release and validation status

| Area | Status |
|---|---|
| Python tests and bilingual documentation checks | Run in GitHub Actions |
| Production Docker image build | CI-covered |
| Container health smoke test | CI-covered |
| Docker Compose validation | CI-covered |
| Windows NativeAOT executable build | CI-covered |
| Windows SHA-256 and provenance attestation | Published and verified for v0.1.1 |
| Stable `v0.1.1` GitHub release | [Published](https://github.com/sagehou/M365-MCP-Server/releases/tag/v0.1.1) |

GitHub Actions is the only build and test environment for this repository.
Do not create local test environments, install test dependencies, or run local builds/tests.

## Remaining work

- Search continuation cursors and folder discovery.
- Reply and forward workflows; HTML composition.
- OCR, shared mailboxes, RBAC, read-only mode, dynamic tool exposure, and rate
  limiting.
- Calendar, OneDrive, SharePoint, and Teams.
- Authenticode signing for the Windows EXE; optional WAM evaluation and rich local attachment parsing.

See the [v0.1.1 release notes](docs/releases/v0.1.1.md) and
[deployment guide](docs/deployment.md) before deployment.

## License

The project source code and documentation are released under the [MIT License](LICENSE).
Third-party dependencies retain their own licenses.
