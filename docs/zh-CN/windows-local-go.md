[English](../windows-local-go.md) | **简体中文**

# Windows 本机 Go 版

## 安装

Intel/AMD 电脑解压 `m365-mcp-windows-local-go-amd64.zip`，Windows ARM 电脑解压
`m365-mcp-windows-local-go-arm64.zip`，将程序放在固定目录。无需 Python、Docker、管理员权限或
Client Secret。安装前用 `SHA256SUMS.txt` 核对压缩包。

若下载单 EXE 的 Artifact，先将程序重命名为 `m365-mcp-windows-local-go.exe`，
再使用下文命令。

## 配置 Microsoft 登录

1. 在 Microsoft Entra 注册应用，选择需要支持的账户类型。
2. 在「身份验证」添加「移动和桌面应用程序」平台，回调地址填写 `http://localhost`。
3. 添加 Microsoft Graph **委托权限**：`User.Read`、`Mail.ReadWrite`、`Mail.Send`。
4. 记录应用程序（客户端）ID。单组织使用 Tenant ID；跨组织工作/学校账户使用
   `organizations`。只有应用同时支持个人 Microsoft 账户时才使用 `common`。

租户策略可能要求管理员同意。本机版不需要创建 Client Secret，也不需要邮箱 Application 权限。

## 登录

在解压目录打开 PowerShell：

```powershell
.\m365-mcp-windows-local-go.exe login --client-id 你的应用ID --tenant organizations
```

在系统浏览器完成 Microsoft 登录。程序使用 S256 PKCE 和随机端口的本机回调，后续调用绑定
此次选择的账户；刷新失败时不会切换到缓存中的其他账户。

## 连接 MCP 客户端

支持 stdio 的客户端执行：

```powershell
.\m365-mcp-windows-local-go.exe connection
```

将生成的 `mcpServers` 配置填入客户端。客户端以 `serve` 参数启动程序；stdout 只输出协议数据，
日志写入 stderr。

支持 Streamable HTTP 和自定义请求头的客户端执行：

```powershell
.\m365-mcp-windows-local-go.exe connection --http --port 8000
.\m365-mcp-windows-local-go.exe serve --http --port 8000
```

将第一条命令输出的 JSON 填入客户端，妥善保管其中的 Bearer Token。监听地址固定为
`127.0.0.1`，所有 MCP 请求均需携带该 Token。本机版不提供容器版的 OAuth/DCR 端点；
仅支持自动 OAuth Discovery 的客户端应连接 Python 服务，或改用 stdio/自定义请求头。
仓库现有的 WorkBuddy OAuth 连接器包用于 Python 服务。

## 邮件工具

提供 12 项工具：`mail_search`、`mail_get`、`mail_list_attachments`、
`mail_read_attachment`、`mail_download_attachment`、`mail_mark_read`、
`mail_archive`、`mail_move`、`mail_set_category`、`mail_create_draft`、
`mail_send_draft`、`mail_add_draft_attachment`。所有 Graph 请求通过委托权限访问登录账户的
`/me` 邮箱。

草稿使用纯文本。发送前，MCP 客户端必须取得逐封确认或明确且有范围的自动化授权。
Graph 接受发送请求不代表最终送达；网络失败且结果不明时，不会自动重试写入。

添加本地 PDF、DOCX、XLSX、PPTX、ZIP 附件前，设置由操作者控制的目录，再生成客户端配置：

```powershell
$env:M365_ATTACHMENT_ROOT = 'C:\M365Artifacts'
.\m365-mcp-windows-local-go.exe connection
```

调用 `mail_add_draft_attachment` 时传入 `draft_id` 和 `relative_path`，例如
`reports/report.zip`。stdio 配置会将目录写入 `env`；HTTP 模式在启动 `serve` 时继承环境变量。
路径使用正斜杠，不能越出配置目录，附件最大 20 MiB。Go 版需要相对路径；原有 NativeAOT 版还
提供原生文件选择器和远程附件推送辅助程序。小于 3,000,000 字节的附件直接上传，更大的附件
使用受限的 Outlook Upload Session。返回结果包含实际字节数和 SHA-256；发送授权前需核对
草稿与附件。

搜索返回一页有界结果和 `next_link` 提示。附件列表的 `size` 是服务提供方元数据，下载后的
`content_length` 是实际解码字节数。PDF、DOCX、XLSX、PPTX、TXT、Markdown 文本提取使用
独立进程，限制为 512 MiB 内存、15 秒 CPU、20 秒总时间，最多同时运行两个 Worker。
附件不超过 10 MiB，Office 解压不超过 64 MiB / 2,048 个文件，提取文本不超过 100,000 字符。
XLSX 读取公式缓存结果；纯图片 PDF 所需的 OCR 尚未提供。

下载链接为绑定 `127.0.0.1` 的随机 HTTP URL，五分钟过期且仅可兑换一次。stdio 模式在随机
本机端口启动下载监听器。内存中最多保留 16 个下载和 64 MiB 数据，重启后全部失效。
链接供同一台电脑上的浏览器使用，不经过远程下载服务。

## 退出登录

先停止 MCP 客户端或服务进程，再执行：

```powershell
.\m365-mcp-windows-local-go.exe logout
```

同一时刻仅允许一个登录、服务或退出进程使用该账户。本地凭据与 HTTP Token 保存在
`%LOCALAPPDATA%\M365MCP`，使用当前 Windows 用户的 DPAPI 加密和用户专属 ACL。
退出登录删除本地凭据，不会撤销 Microsoft 浏览器会话或租户授权。Microsoft 要求交互式登录时，
停止服务并重新执行 `login`；这会同时轮换本地 HTTP Token，需重新生成 HTTP 客户端配置。

## 构建

所有构建和测试仅在 GitHub Actions 执行。`windows-local-go` 工作流使用
`scripts/build-windows-go.py` 打包 AMD64、ARM64 程序、中英文使用说明和第三方许可证。
CI 成功后，下载 `windows-local-go-amd64` 或 `windows-local-go-arm64` 获取单文件 EXE，
下载 `windows-local-go-packages` 获取 ZIP 包和校验和。工作流覆盖 Linux、Windows 测试，包括
Windows DPAPI、进程互斥和隔离 Worker。禁止在此编辑环境构建或测试。

构建仅在 CI 使用 `goversioninfo` v1.7.0，为两个架构嵌入资源管理器文件详细信息，
产品/文件版本取自 `pyproject.toml`。Tag 发布流水线复用同一构建，核对校验和，
将 Go EXE、ZIP 与原有 NativeAOT EXE 一起发布。

---

程序尚未代码签名。Go 不保证更少的杀软检测，SmartScreen 信誉、代码签名和误报申诉仍需按
发布环境处理。Job Object 限制资源，不隔离文件系统或网络。真实 Entra 登录、客户端兼容性、
代表性文档和杀软行为仍需在目标 Windows 电脑验收。更新日期：2026-11-10。
