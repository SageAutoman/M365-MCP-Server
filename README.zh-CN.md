[English](README.md) | **简体中文**

# M365 MCP Server

一个自托管的 Microsoft 365 MCP Gateway，采用受控工具面、Microsoft Entra
委托身份和按用户隔离，并重点保护 Outlook 邮件与附件处理边界。

> **已发布：**[`v0.1.1`](https://github.com/sagehou/M365-MCP-Server/releases/tag/v0.1.1)
> 已按 MIT 许可证发布。CI 验证了 Container Image 与 Windows x64 NativeAOT
> EXE。每个部署仍须自行配置 Entra 同意，并在客户端执行发送授权策略。

## 当前已经可用

### 远端 MCP Server

容器部署、客户端连接和附件上传路径从[远端 MCP Server 指南](docs/zh-CN/mcp-server.md)
开始阅读。

- FastMCP Streamable HTTP Endpoint：`/mcp/`。
- Microsoft Entra Bearer Token 校验，并执行 Tenant 与 Scope 约束。
- 通过 On-Behalf-Of（OBO）使用 Microsoft Graph 委托权限。
- 每个请求独立绑定用户身份；Graph 调用被限制在 `/me`。
- 可选的 OAuth Discovery、Dynamic Public-client Registration、Interactive
  Sign-in、S256 PKCE、加密 Refresh Session、Rotation 和 Replay Rejection。
- `workbuddy/` 中提供 WorkBuddy Connector Package，CI 覆盖完整的 Mock
  Client Flow。
- 结构化脱敏 Audit Event；安全错误会返回可关联的 Event ID。
- `mail_add_draft_attachment` 可为现有草稿添加单个不超过 20 MiB 的附件，
  但不会发送。远端客户端须主动上传原始字节，再向 MCP 提供短时句柄。无人值守
  本机产物可在 OAuth 模式下取得绑定草稿的 Push Grant，由受信任的本机程序
  提供文件字节。见[远端草稿附件](docs/zh-CN/mcp-server.md#为远端草稿添加文件)。

### Windows 本地 EXE

`stdio`、本地登录、相对路径草稿附件，以及配合远端 Connector 的可选 EXE
辅助命令，见 [Windows 本地指南](docs/zh-CN/windows-local.md)。

Windows x64 NativeAOT 本地 Runtime 已包含在 v0.1.1 中。Release 提供：

- `m365-mcp-windows-x64.exe`
- SHA-256 校验文件
- Release 说明中的 MD5 和 SHA-1，供按精确文件哈希加杀毒软件白名单
- GitHub Artifact Attestation 来源证明
- `LICENSE` 许可声明文件（不是运行依赖）

目标机器无需安装 Python、.NET Runtime、Docker 或安装器。Authenticode
签名属于后续加固项。如安全软件隔离未签名 EXE，请按
[误报处置说明](docs/zh-CN/windows-local.md#杀毒软件误报)处理；Checksum 与
Attestation 可核验来源，但不会覆盖杀毒软件的判定。
MD5 和 SHA-1 仅用于兼容旧式白名单表单，不是可信性验证；EXE 改变后须重新计算
哈希并审核白名单。

### Windows 本机 Go 程序

[Go 版](docs/zh-CN/windows-local-go.md)通过独立的 `windows-local-go` 流水线
生成 AMD64、ARM64 单文件 EXE，支持浏览器登录、stdio、本机鉴权 HTTP 和附件文本提取。
原有 NativeAOT 流水线和发布产物继续保留。

## 发布与验证状态

| 范围 | 状态 |
|---|---|
| Python 测试与双语文档检查 | GitHub Actions 执行 |
| Production Docker Image 构建 | CI 覆盖 |
| Container Health Smoke Test | CI 覆盖 |
| Docker Compose 校验 | CI 覆盖 |
| Windows NativeAOT EXE 构建 | CI 覆盖 |
| Windows SHA-256 与 Provenance Attestation | 已随 v0.1.1 发布并核验 |
| 稳定版 `v0.1.1` GitHub Release | [已发布](https://github.com/sagehou/M365-MCP-Server/releases/tag/v0.1.1) |

GitHub Actions 是本仓库唯一的构建与测试环境。此编辑环境禁止创建测试环境、安装测试依赖或执行本地构建和测试。

## 后续工作

- Search Continuation Cursor 与 Folder Discovery。
- Reply、Forward 工作流和 HTML 撰写。
- OCR、Shared Mailbox、RBAC、Read-only Mode、Dynamic Tool Exposure 和 Rate
  Limiting。
- Calendar、OneDrive、SharePoint 和 Teams。
- Windows EXE 的 Authenticode 签名；可选 WAM 评估与本地富文档附件解析。

部署前请阅读 [v0.1.1 发布说明](docs/zh-CN/releases/v0.1.1.md)和
[部署指南](docs/zh-CN/deployment.md)。

## 许可证

项目源代码与文档按 [MIT 许可证](LICENSE)发布。第三方依赖仍适用其各自的许可证。
