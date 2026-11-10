**English** | [简体中文](zh-CN/windows-local-go.md)

# Windows local Go edition

## Install

Extract `m365-mcp-windows-local-go-amd64.zip` for Intel/AMD PCs, or
`m365-mcp-windows-local-go-arm64.zip` for Windows on ARM. Keep the extracted executable
in a stable folder. Python, Docker, administrator access, and a client secret
are not required. Check the archive against `SHA256SUMS.txt` before installing.

If downloading a single-EXE artifact, rename its executable to
`m365-mcp-windows-local-go.exe` before following the commands below.

## Register Microsoft sign-in

1. Create a Microsoft Entra app registration for the intended account type.
2. Under **Authentication**, add a **Mobile and desktop applications** platform
   with the redirect URI `http://localhost`.
3. Add Microsoft Graph **delegated** permissions `User.Read`, `Mail.ReadWrite`,
   and `Mail.Send`.
4. Copy the application (client) ID. Use a tenant ID for one organization, or
   `organizations` for work/school accounts across organizations. Use `common`
   only when the registration also supports personal Microsoft accounts.

Tenant policy may require administrator consent. Do not create a client secret
or grant application mailbox permissions for the local edition.

## Sign in

In PowerShell, from the extracted folder:

```powershell
.\m365-mcp-windows-local-go.exe login --client-id YOUR-APPLICATION-ID --tenant organizations
```

Complete Microsoft sign-in in the system browser. The application uses S256
PKCE and an ephemeral localhost callback. The selected account is bound to
subsequent calls; a refresh failure never selects another cached account.

## Connect an MCP client

For clients supporting stdio:

```powershell
.\m365-mcp-windows-local-go.exe connection
```

Copy the resulting `mcpServers` configuration into the client. The client starts
the executable with `serve`. All protocol traffic uses stdout; logs use stderr.

For clients supporting Streamable HTTP and custom request headers:

```powershell
.\m365-mcp-windows-local-go.exe connection --http --port 8000
.\m365-mcp-windows-local-go.exe serve --http --port 8000
```

Copy the first command's JSON into the client. Keep its bearer token private.
The listener binds only to `127.0.0.1`; every MCP request requires that token.
The local edition does not serve the container edition's OAuth/DCR endpoints.
Clients that require automatic OAuth discovery must use the Python server or
configure stdio/custom headers instead. WorkBuddy's existing OAuth connector
package is intended for the Python server.

## Mail tools

Twelve tools are available: `mail_search`, `mail_get`, `mail_list_attachments`,
`mail_read_attachment`, `mail_download_attachment`, `mail_mark_read`,
`mail_archive`, `mail_move`, `mail_set_category`, `mail_create_draft`,
`mail_send_draft`, and `mail_add_draft_attachment`. Every Graph request uses
the signed-in account's `/me` mailbox and delegated permissions.

Drafts use plain text. Sending requires per-message confirmation or explicit
bounded automation authorization at the MCP client; Graph acceptance does not
confirm delivery. Writes are never retried after ambiguous network failures.

To attach local PDF, DOCX, XLSX, PPTX, or ZIP files up to 20 MiB, configure an
operator-controlled attachment folder before generating client configuration:

```powershell
$env:M365_ATTACHMENT_ROOT = 'C:\M365Artifacts'
.\m365-mcp-windows-local-go.exe connection
```

Pass `draft_id` and a `relative_path` such as `reports/report.zip` to
`mail_add_draft_attachment`. Stdio configuration includes the configured folder
in `env`; HTTP mode inherits it when `serve` starts. Paths use forward slashes
and cannot escape that folder. This edition requires a relative path; the
existing NativeAOT edition also offers a native file picker and a remote push
helper. Attachments below 3,000,000 bytes use a direct Graph request; larger
files use a restricted Outlook upload session. Results include byte length and
SHA-256. Review the draft and attachments before authorizing a send.

Search returns one bounded page and a `next_link` hint. Attachment list `size`
is provider metadata; fetched `content_length` is the actual decoded byte count.
PDF, DOCX, XLSX, PPTX, TXT, and Markdown text extraction runs in a separate
process, with 512 MiB memory, 15 CPU seconds, 20 seconds wall time, and at most
two concurrent workers. Files are limited to 10 MiB, Office expansion to 64 MiB
and 2,048 entries, and output to 100,000 characters. XLSX extraction reads
cached formula results. Image-only PDFs require OCR, which is not included.

Downloads use an unguessable, five-minute, single-use HTTP URL bound to
`127.0.0.1`. Stdio mode starts its download listener on a random local port.
At most 16 downloads and 64 MiB are retained in memory. They expire on restart.
URLs are intended for a browser on the same PC. No remote download host is used.

## Sign out

Stop the MCP client/server first, then run:

```powershell
.\m365-mcp-windows-local-go.exe logout
```

Only one login/server/logout process can use the local account at a time.
Credentials and the local HTTP token live under `%LOCALAPPDATA%\M365MCP` with
a user-only Windows ACL and current-user DPAPI encryption. Sign-out removes
the local credentials; it does not revoke Microsoft browser sessions or tenant
consent. If Microsoft requires interactive sign-in again, stop the server and
repeat `login`; this also rotates the local HTTP token, so regenerate HTTP
client configuration afterward.

## Build

Builds and tests run only in GitHub Actions. The `windows-local-go` workflow uses
`scripts/build-windows-go.py` to package AMD64 and ARM64 executables, both usage
guides, and third-party license notices. After CI succeeds, download
`windows-local-go-amd64` or `windows-local-go-arm64` for a single EXE, or
`windows-local-go-packages` for ZIP packages and checksums. The workflow covers Linux and Windows tests, including
Windows DPAPI, process locking, and isolated workers. Do not build or test in
the editing environment.

The build uses `goversioninfo` v1.7.0 only in CI to embed Explorer file details
for both architectures. Product/file versions come from `pyproject.toml`.
The tag release workflow reuses this same build, verifies checksums, and
publishes the Go EXEs/ZIPs alongside the existing NativeAOT EXE.

---

The executable is unsigned. Go does not guarantee fewer antivirus detections;
SmartScreen reputation, code signing, and vendor false-positive review still
apply. Job Objects limit resources, not filesystem/network access. Real Entra
sign-in, client compatibility, representative documents, and antivirus behavior
require acceptance on the target Windows PC. Updated: 2026-11-10.
