[English](../windows-local.md) | **简体中文**

# Windows 本地单文件版本

## 当前状态

首个 Windows 本地实现位于 `windows/M365Mcp.Local`，只通过 GitHub Actions
构建。它使用 .NET 10 NativeAOT，发布结果严格为一个 `m365-mcp.exe`；用户电脑
不需要 Python、.NET、Docker、Git 或安装器。

原有流水线继续保留；新增的 [Go 版](windows-local-go.md)使用独立流水线和产物名。
已发布的 v0.1.1 EXE 仍使用 .NET 8，本分支将构建目标升级为 .NET 10 LTS。

当前 EXE 已提供：

- `doctor`、`login`、`logout`、`status` 和 `stdio` 命令；
- 面向独立 Entra Desktop/Public Client App 的系统浏览器 Authorization Code +
  S256 PKCE 登录；
- 默认内置项目公共客户端 `M365-MCP-Localhost`，并允许企业覆盖 Client ID；
- 使用当前 Windows 用户 DPAPI 保护可选持久 Token State；
- 通过 MCP stdio 提供 11 个有边界的 Outlook Mail 工具；
- `--ephemeral` 模式，不读取也不写入持久状态；
- Windows Actions 门禁：构建结果不是单个 EXE 就失败；Smoke Test 时从
  `PATH` 移除语言 Runtime，并在 Ephemeral 运行产生文件时失败。

[v0.1.1 GitHub Release](https://github.com/sagehou/M365-MCP-Server/releases/tag/v0.1.1)
已提供稳定版 Windows EXE、SHA-256 校验文件、MIT 许可声明和该精确 Smoke Test
二进制文件的 GitHub Artifact Attestation。普通分支的 Actions Artifact 仍只是
集成产物。已发布 EXE 尚无 Authenticode 签名；SBOM 与 PDF/Office 本地附件解析
属于独立加固事项。是否引入 WAM 取决于部署需求。已发布 v0.1.1 EXE 提供 11 个
邮件工具，包括添加草稿附件。

## Runtime 决策

远端/容器 Server 继续使用 Python 3.12。Windows 本地 Host 是一个小型、独立的
.NET NativeAOT 边界，只负责本地认证、MCP stdio 和受控 Graph/Mail 工具面。

这取代了早期 PyOxidizer 候选方案。PyOxidizer 最新稳定版内置 CPython 3.10，
不适合作为当前 Python 3.12 项目的可维护交付基础；PyInstaller 和 Nuitka 的
One-file 模式会在启动时解压 Runtime Tree，也不满足“不产生解压目录”的约束。
NativeAOT 生成原生 Windows EXE，用户电脑无需安装 Python 或 .NET Runtime。

## 分发约束

- 用户只接收并运行一个 `m365-mcp.exe`。
- 正常运行不解压 Runtime Tree。
- `stdio` 不安装 Service，不创建 Registry、Startup、Scheduled Task 或日志文件。
- stdout 只输出 MCP JSON-RPC；诊断信息只写 stderr。
- 持久模式最多创建一个已声明的当前用户文件：
  `%LOCALAPPDATA%\M365-MCP-Server\state.bin`。
- State 中的 OAuth Token 使用当前 Windows 用户 DPAPI 保护。
- `--ephemeral` 不读取、新建或修改 State 文件。
- Agent 将 `stdio` 作为子进程启动。当前版本不提供 Windows Service 安装，
  因为 Service 无法拥有 Agent 的 stdio 通道。

## 杀毒软件误报

未签名的 NativeAOT EXE 可能被终端安全软件隔离。构建参数、ZIP 包、Checksum 或
GitHub Attestation 都不能保证杀毒软件放行。Windows 构建已写入产品名称、文件描述
和版本号，有助于识别样本，但不等于发布者签名。

如果 EXE 被拦截：

1. 从项目 GitHub Release 获取精确的 EXE 和 `.sha256` 文件。用
   `Get-FileHash .\m365-mcp-windows-x64.exe -Algorithm SHA256` 与发布的校验值
   对比；如已安装 GitHub CLI，再运行
   `gh attestation verify m365-mcp-windows-x64.exe --repo sagehou/M365-MCP-Server`。
   任一验证失败都不要继续运行。
2. 记录安全产品及病毒库版本、检出名称、SHA-256、Release 来源链接和触发步骤。
   不要包含 Token、邮件内容或未经脱敏的用户/租户标识。
3. 将*被检出的那份二进制文件*作为疑似误报提交给对应厂商。奇安信使用其官方
   [样本上报页面](https://www.qianxin.com/other/sample-upload)；页面限制文件不
   超过 100 MB，并要求联系邮箱和验证码。附上仓库、Release 链接、构建来源证明
   和简要行为说明：这是单文件 MCP stdio 进程；登录时打开系统浏览器完成 Entra
   PKCE、仅监听本机 Loopback；访问 Microsoft Graph；可选地在已声明路径保存
   DPAPI 保护的状态。
4. 等待厂商判定和病毒库更新，再用同一 SHA-256 在受影响终端复测。企业受管终端
   如需临时例外，应由安全管理员评估对已验证的*精确文件哈希*设置窄范围、限时
   规则。不要关闭安全软件，也不要放行整个目录或同名进程家族。

二进制哈希改变后需重新核验；旧版本的判定不自动覆盖新构建。GitHub Attestation
证明构建来源，不证明文件绝对安全，也不代表所有杀毒引擎都会信任它。

若奇安信或其他受管终端安全产品要求提供 MD5 和 SHA-1 以按精确文件哈希加白，
请使用同一 GitHub Release 说明中 `m365-mcp-windows-x64.exe` 对应的值。先用
发布的 `.sha256` 文件核对 EXE 的 SHA-256，再由安全管理员评估窄范围白名单。
MD5 和 SHA-1 不是可信性证明；任何 EXE 变更都必须重新计算并审核白名单。

## Entra Public Client 配置

Windows 本地 EXE 默认使用项目维护的独立公共客户端：

```text
应用名称：M365-MCP-Localhost
Application (client) ID：6e35216e-2623-43cc-b867-83bec0865cf3
Authority：organizations
```

默认客户端使用以下 Entra 配置：

1. 添加 **移动和桌面应用程序（Mobile and desktop applications）**平台。
2. 注册精确的根 Redirect URI：`http://localhost`。EXE 每次登录会监听随机
   Loopback Port；Microsoft Entra 匹配 Native App 的 localhost Redirect 时忽略
   Port，但 Path 仍必须一致。
3. 在 **Advanced settings** 中把 **Allow public client flows** 设为 **Yes**。
4. 配置 Microsoft Graph 委托权限：`User.Read`、`Mail.ReadWrite` 和
   `Mail.Send`。
5. 不创建或分发 Client Secret 或证书。

用户不需要为了运行默认版本自行创建 App Registration。目标租户的策略仍可能要求
管理员针对此应用和当前权限集合完成一次同意；新增权限或撤销同意后需要重新批准。

企业也可以创建自己的 Desktop/Public Client App，并通过
`M365_LOCAL_CLIENT_ID` 覆盖内置 Client ID。自定义应用必须遵循上面相同的平台、
Redirect URI、Public Client Flow 和委托权限配置；需要限制到单一租户时，再同时设置
`M365_LOCAL_TENANT_ID`。Client ID 或 Tenant ID 变更后，已有 DPAPI State 不会跨应用
复用，需要重新执行 `login`。

远端 Server 现有的 Confidential-client/OBO App Registration 保持不变，不能把
它的凭据作为 Public Desktop Credential 分发。

## 运行

在 PowerShell 中：

```powershell
.\m365-mcp.exe doctor
.\m365-mcp.exe login
.\m365-mcp.exe status
```

默认运行不需要设置环境变量。使用企业自己的 App Registration 时再覆盖：

```powershell
$env:M365_LOCAL_CLIENT_ID = "<custom-desktop-public-client-id>"
$env:M365_LOCAL_TENANT_ID = "<tenant-id-or-organizations>"
```

`login` 会打开系统浏览器，通过 Loopback 完成 PKCE，然后保存 DPAPI 保护的 State。
不允许认证状态跨进程保留时，对 `login` 或 `stdio` 增加 `--ephemeral`。

MCP Client 配置示例：

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

使用自定义 Entra 应用时，在该 Server 配置的 `env` 中增加
`M365_LOCAL_CLIENT_ID`；需要时同时增加 `M365_LOCAL_TENANT_ID`。

如果没有可用状态，首次 Tool Call 也可以触发交互登录；Agent 必须给用户留出足够
时间完成浏览器流程。

## 当前本地工具面

EXE 当前提供：

- `mail_search`
- `mail_get`
- `mail_create_draft`
- `mail_send_draft`
- `mail_add_draft_attachment`：每次最多添加 20 MiB 的单个文件
- `mail_list_attachments`
- `mail_read_attachment`：支持有边界的 UTF-8 Text、CSV、JSON 和 XML
- `mail_mark_read`
- `mail_archive`
- `mail_move`
- `mail_set_category`

本地 stdio Runtime 没有 HTTP Download Endpoint，并且不得任意写文件，因此暂不
提供 `mail_download_attachment`。PDF/DOCX/XLSX/PPTX 解析仍在验收待办中。邮件
发送仍必须在 Client/Agent 层逐封确认，或具有明确的受限自动化授权。

### Windows 本地 Connector 的无人值守产物附件

使用 Windows 本地 stdio Connector 做无人值守自动化时，在 Connector 进程环境中
把 `M365_ATTACHMENT_ROOT` 配为可信的产物绝对目录，然后调用
`mail_add_draft_attachment(draft_id, relative_path="reports\\weekly.docx")`。
工具只读取最终路径仍在该目录内的已打开文件，限制为 20 MiB 以内的
DOCX/XLSX/PPTX/ZIP/PDF，并通过本地登录用户的 Graph 会话直接添加附件；
不需要浏览器上传、第二个 Connector、Bash 命令或远端 Push URL。不写临时附件
文件，也不通过 MCP 返回文件字节。不传 `relative_path` 时，才使用原有的
Windows 文件选择框。授权发送前须核对草稿和附件列表。

### 远端 Connector 的本机 EXE 辅助命令

已发布的同一个单文件 EXE 还提供
`inspect-attachment <relative-path>` 和 `push-attachment <relative-path>`。
这些命令不使用 Entra 登录，也不启动另一个 MCP Connector。把
`M365_ATTACHMENT_ROOT` 配为存放已完成自动化产物的可信绝对目录，把
`M365_ATTACHMENT_PUSH_URL` 配为 Server 固定的 HTTPS `/uploads/push` URL。
调用者只能给出该目录下的相对路径；EXE 校验打开文件的最终路径，仅允许
DOCX、XLSX、PPTX、ZIP 和 PDF，拒绝空文件及超过 20 MiB 的文件，不创建临时文件。

`inspect-attachment` 返回本机文件名、MIME 类型、字节数及 SHA-256。
`push-attachment` 从标准输入读取同一路径的完整 Grant JSON；EXE 会再次检查
配置的 URL 和文件，再推送原始字节，不接收用户 OAuth Token，也不跟随 HTTP
Redirect。Server 端的 Grant、上传、添加附件和发送完整步骤见
[远端 MCP Server 指南](mcp-server.md)。

例如 PowerShell 中已经把 MCP 工具返回的 Grant JSON 暂存在 `$grantJson` 时：

```powershell
$env:M365_ATTACHMENT_ROOT = 'C:\Automation\Output'
$env:M365_ATTACHMENT_PUSH_URL = 'https://mcp.example.com/uploads/push'
& 'C:\Tools\m365-mcp.exe' inspect-attachment 'reports\weekly.docx'
$grantJson | & 'C:\Tools\m365-mcp.exe' push-attachment 'reports\weekly.docx'
```

不要记录 Grant 或放在命令行参数中。如检查与推送之间文件已变化，重新签发
Grant，不要猜测哈希。WorkBuddy 撰写 Skill 可通过 Bash 工具调用这些命令；EXE 必须放在
管理员认可的位置。受限自动化授权仍须明确约束产物来源、收件人、大小、邮件
数量与有效期。EXE 的目录限制只约束该命令；WorkBuddy 的通用 Bash 权限仍可能
访问其他文件，还需要依赖客户端自身的沙箱与工具审批策略。

## 构建与取得集成产物

`Windows local executable` Workflow 会上传
`m365-mcp-windows-x64` Actions Artifact。该 Workflow：

1. 使用固定 Windows Runner；
2. 通过 NativeAOT 发布 `windows/M365Mcp.Local/M365Mcp.Local.csproj`；
3. 断言输出目录中只有 `m365-mcp.exe`；
4. 在 `PATH` 中没有语言 Runtime 的条件下执行 `doctor`、MCP
   `initialize` 和 `tools/list`；
5. 断言 Ephemeral 运行不会留下文件。

Actions Artifact 不能宣称为 Stable Release。Tag Release Workflow 会重新构建并
Smoke Test Windows x64 EXE，发布 SHA-256 校验文件和 MIT 许可声明，为该文件
记录 GitHub Artifact Attestation，并把精确通过测试的文件附加到 GitHub Release。

## 后续加固与部署检查

- 先验证 Browser PKCE，再根据账户选择、SSO 或 Tenant Policy 的实际需要决定
  是否增加可选 WAM 集成。
- 在不解压 Runtime 的前提下增加安全的富文档附件解析。
- 使用真实邮箱验证搜索、读取、草稿、确认发送、受限自动化发送、附件读取和代表性修改。
- 验证 Refresh、Logout、损坏 State 恢复、双账户隔离和 Tenant Policy。
- Authenticode 签名、时间戳和 SBOM 作为后续加固；v0.1.1 附有 SHA-256 Checksum 与 GitHub Artifact Attestation。
- 在干净 Windows x64 VM 上验证已发布的精确 EXE。
