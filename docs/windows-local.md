**English** | [简体中文](zh-CN/windows-local.md)

# Windows local single-executable

## Current status

The first Windows-local implementation is available in
`windows/M365Mcp.Local` and is built only by GitHub Actions. It is a .NET 10
NativeAOT application that publishes as exactly one `m365-mcp.exe`; the target
PC does not need Python, .NET, Docker, Git, or an installer.

The existing workflow is retained. The separately built
[Go edition](windows-local-go.md) has its own workflow and artifact names.
The published v0.1.1 executable continues to use .NET 8; this branch upgrades
the build target to .NET 10 LTS.

The executable currently provides:

- `doctor`, `login`, `logout`, `status`, and `stdio` commands
- system-browser authorization code plus S256 PKCE for a dedicated Entra
  desktop/public-client application
- the project `M365-MCP-Localhost` public client built in by default, with an
  enterprise client-ID override
- current-user DPAPI protection for the optional persistent token state
- MCP stdio with eleven bounded Outlook mail tools
- an `--ephemeral` mode that neither reads nor writes persistent state
- a Windows Actions gate that rejects anything except one executable, removes
  language runtimes from `PATH` during smoke testing, and fails if the
  ephemeral smoke run creates files

The [v0.1.1 GitHub Release](https://github.com/sagehou/M365-MCP-Server/releases/tag/v0.1.1)
contains the stable Windows asset, its SHA-256 checksum, MIT notice, and GitHub
Artifact Attestation for the exact smoke-tested binary. Actions artifacts from
ordinary branch builds remain integration builds. The released EXE is unsigned;
Authenticode signing, SBOM, and rich local attachment extraction remain separate
hardening work. Optional WAM depends on deployment requirements. The published
v0.1.1 EXE has eleven mail tools, including draft attachments.

## Runtime decision

The remote/container server remains Python 3.12. The Windows-local host is a
small separate .NET NativeAOT boundary that implements only local authentication,
MCP stdio, and the bounded Graph/mail surface.

This replaces the earlier PyOxidizer candidate. PyOxidizer's latest stable
release embeds CPython 3.10 and is not a supportable basis for this Python 3.12
application. PyInstaller and Nuitka one-file modes extract a runtime tree before
execution, so they do not satisfy the no-extraction contract. NativeAOT produces
a native Windows executable and needs no Python or .NET runtime on the user PC.

## Distribution contract

- The user receives and executes one `m365-mcp.exe` file.
- Normal execution does not extract a runtime tree.
- `stdio` does not install a service, write registry/startup/scheduled-task
  entries, or create log files.
- MCP JSON-RPC is the only stdout content; diagnostics use stderr.
- Persistent mode may create one documented current-user file:
  `%LOCALAPPDATA%\M365-MCP-Server\state.bin`.
- The state contains OAuth tokens protected with current-user Windows DPAPI.
- `--ephemeral` does not read, create, or modify the state file.
- The agent launches `stdio` as a child process. Windows service installation
  is not part of this build because a service cannot own the agent's stdio
  channel.

## Antivirus false positives

The unsigned NativeAOT executable may be quarantined by endpoint security
software. No build option, ZIP wrapper, checksum, or GitHub attestation can
guarantee an antivirus allow decision. The Windows build embeds the product
name, file description, and version; these help identify a sample but are not
a publisher signature.

If a security product blocks the EXE:

1. Obtain the exact EXE and `.sha256` file from the project's GitHub Release.
   Compare `Get-FileHash .\m365-mcp-windows-x64.exe -Algorithm SHA256` with the
   published checksum and, if GitHub CLI is available, run
   `gh attestation verify m365-mcp-windows-x64.exe --repo sagehou/M365-MCP-Server`.
   Stop if either verification fails.
2. Record the security product and definition versions, detection name, SHA-256,
   source Release URL, and the step that triggered detection. Do not include
   tokens, mail content, or unredacted user/tenant IDs.
3. Submit the *exact detected binary* as a suspected false positive to that
   vendor. For Qianxin, use its official
   [sample upload form](https://www.qianxin.com/other/sample-upload); the form
   has a 100 MB file limit and asks for a contact email and CAPTCHA. Include
   the repository and Release URL, build provenance, and a short explanation:
   this is a single-file MCP stdio process, opens the system browser for Entra
   PKCE, listens only on loopback during sign-in, calls Microsoft Graph, and
   optionally stores DPAPI-protected state at the documented path.
4. Wait for the vendor's determination and updated definitions, then retest
   the same SHA-256 on the affected endpoint. If a managed business endpoint
   needs an interim exception, let its security administrator assess a narrow,
   time-limited rule for the verified exact file hash. Do not disable the
   security product or exempt a whole directory or process family.

Repeat the sample review for a changed binary hash; a prior verdict does not
automatically cover new builds. GitHub attestation proves build provenance,
not that the binary is malware-free or trusted by every antivirus engine.

For Qianxin or another managed endpoint product that requests MD5 and SHA-1
for exact-file allowlisting, use the values for `m365-mcp-windows-x64.exe` in
the same GitHub Release description. First confirm the EXE's SHA-256 against
the published `.sha256` file; then give the MD5 and SHA-1 to the security
administrator for a narrowly scoped rule. These older hashes are not proof of
authenticity and must be recalculated for any changed executable.

## Entra public-client setup

The Windows-local executable uses this project-managed public client by default:

```text
Application name: M365-MCP-Localhost
Application (client) ID: 6e35216e-2623-43cc-b867-83bec0865cf3
Authority: organizations
```

The built-in client uses this Entra configuration:

1. Add the **Mobile and desktop applications** platform.
2. Register the exact root URI `http://localhost` as the redirect URI. The
   executable listens on a random loopback port for each sign-in; Microsoft
   Entra ignores the port when matching localhost native-app redirects, but the
   path must still match.
3. Under **Advanced settings**, set **Allow public client flows** to **Yes**.
4. Configure delegated Microsoft Graph permissions:
   `User.Read`, `Mail.ReadWrite`, and `Mail.Send`.
5. Do not create or distribute a client secret or certificate.

Users do not need to create an App Registration to run the default build. The
target tenant policy can still require an administrator to consent once for this
application and its current permission set. New permissions or revoked consent
require another approval.

An enterprise can instead create its own desktop/public-client application and
override the built-in client ID with `M365_LOCAL_CLIENT_ID`. The custom app must
use the same platform, redirect URI, public-client-flow, and delegated-permission
configuration above. Set `M365_LOCAL_TENANT_ID` as well when the app must be
restricted to one tenant. Existing DPAPI state is never reused across a client-ID
or tenant-ID change; run `login` again after changing either value.

The remote server's confidential-client/OBO App Registration is unchanged and
must not be reused as a public desktop credential.

## Run

In PowerShell:

```powershell
.\m365-mcp.exe doctor
.\m365-mcp.exe login
.\m365-mcp.exe status
```

The default build needs no environment variables. Override it only when using
an enterprise-owned App Registration:

```powershell
$env:M365_LOCAL_CLIENT_ID = "<custom-desktop-public-client-id>"
$env:M365_LOCAL_TENANT_ID = "<tenant-id-or-organizations>"
```

`login` opens the system browser, completes PKCE on loopback, then saves the
DPAPI-protected state. Use `--ephemeral` with `login` or `stdio` when no
authentication state may survive process exit.

Example MCP client configuration:

```json
{
  "mcpServers": {
    "m365-local": {
      "command": "C:\\Tools\\m365-mcp.exe",
      "args": ["stdio"]
    }
  }
}
```

For a custom Entra application, add `M365_LOCAL_CLIENT_ID` to this server's
`env` block and add `M365_LOCAL_TENANT_ID` when required.

The first tool call can also start interactive sign-in if no usable state exists.
The agent must allow enough time for the user to finish the browser flow.

## Current local tool surface

The executable exposes these tools:

- `mail_search`
- `mail_get`
- `mail_create_draft`
- `mail_send_draft`
- `mail_add_draft_attachment` (one file up to 20 MiB per call)
- `mail_list_attachments`
- `mail_read_attachment` for bounded UTF-8 text, CSV, JSON, and XML
- `mail_mark_read`
- `mail_archive`
- `mail_move`
- `mail_set_category`

`mail_download_attachment` is intentionally absent because the local stdio
runtime has no HTTP download endpoint and must not write arbitrary files.
PDF/DOCX/XLSX/PPTX extraction remains on the acceptance backlog. Sending still
requires per-message confirmation or an explicit bounded automation
authorization at the client/agent layer.

### Unattended artifacts for a Windows-local connector

For unattended automation using the Windows-local stdio connector, set
`M365_ATTACHMENT_ROOT` in the connector process environment to a trusted,
absolute output directory. Call
`mail_add_draft_attachment(draft_id, relative_path="reports\\weekly.docx")`.
The tool reads only an opened file whose resolved path stays inside that root,
with the same 20 MiB and DOCX/XLSX/PPTX/ZIP/PDF limits as the EXE upload
commands. It attaches directly through the locally signed-in user's Graph
session: no browser upload, second connector, Bash command, or remote push URL
is needed. It never writes a temporary attachment file or returns file bytes
through MCP. Omit `relative_path` for the existing interactive Windows file
picker. Review the draft and attachment list before an authorized send.

### Local EXE helper for a remote connector

The same released single EXE also provides
`inspect-attachment <relative-path>` and `push-attachment <relative-path>`.
These commands do not use Entra login or start an MCP connector. Set
`M365_ATTACHMENT_ROOT` to a trusted, absolute directory containing completed
automation outputs, and `M365_ATTACHMENT_PUSH_URL` to the server's fixed HTTPS
`/uploads/push` URL. The caller may specify only a relative path beneath that
root; the EXE checks the opened file's resolved path, permits DOCX, XLSX,
PPTX, ZIP, and PDF, and rejects empty files or files over 20 MiB. It creates
no temporary file.

`inspect-attachment` returns the local file's name, MIME type, byte length and
SHA-256. `push-attachment` accepts the complete grant JSON through standard
input for the same path. The EXE checks the configured URL and the file again
before posting raw bytes; it receives no user OAuth token and does not follow
HTTP redirects. The full server-side grant, upload, attachment and send
sequence belongs to the [remote MCP server guide](mcp-server.md).

For example, in PowerShell with an already received MCP grant JSON held only in
`$grantJson`:

```powershell
$env:M365_ATTACHMENT_ROOT = 'C:\Automation\Output'
$env:M365_ATTACHMENT_PUSH_URL = 'https://mcp.example.com/uploads/push'
& 'C:\Tools\m365-mcp.exe' inspect-attachment 'reports\weekly.docx'
$grantJson | & 'C:\Tools\m365-mcp.exe' push-attachment 'reports\weekly.docx'
```

Do not log the grant or place it in command arguments. If inspection and push
differ, regenerate the grant; never guess the hash.
The WorkBuddy compose Skill can invoke these commands through its Bash tool;
the EXE must be installed at an operator-approved path. A bounded automation
authorization must still name the permitted output source, recipients, sizes,
message limits, and expiry. The EXE's file-root check limits only this command:
WorkBuddy's general Bash permission may access other files, so use the client's
own sandbox and tool-approval policy for that broader capability.

## Build and obtain the integration artifact

The `Windows local executable` workflow publishes the
`m365-mcp-windows-x64` Actions artifact. The workflow:

1. runs on a pinned Windows runner;
2. publishes `windows/M365Mcp.Local/M365Mcp.Local.csproj` with NativeAOT;
3. asserts that the output contains exactly `m365-mcp.exe`;
4. runs `doctor`, MCP `initialize`, and `tools/list` with no language runtime
   on `PATH`;
5. asserts that the ephemeral run leaves no files.

Do not describe an Actions artifact as a stable release. The tagged release
workflow rebuilds and smoke-tests the Windows x64 executable, publishes its
SHA-256 checksum and MIT notice, records GitHub Artifact Attestation for that
binary, and attaches the exact tested files to the GitHub Release.

## Follow-up hardening and deployment checks

- Validate browser PKCE first, then decide whether optional WAM integration is
  justified by account-selection, SSO, or tenant-policy requirements.
- Add safe rich attachment extraction without runtime extraction.
- Exercise search, read, draft, confirmed send, bounded automation send,
  attachment reading, and representative mutations against a real mailbox.
- Verify refresh, logout, corrupted-state recovery, two-account isolation, and
  tenant policy behavior.
- Add Authenticode signing, timestamping, and an SBOM as follow-up hardening; SHA-256 checksums and GitHub Artifact Attestation accompany v0.1.1.
- Validate the published executable on a clean Windows x64 VM.
