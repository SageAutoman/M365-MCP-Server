[English](../runtime-dependency-audit.md) | **简体中文**

# 运行时依赖审计

Python 运行时及开发依赖在 `pyproject.toml` 中固定版本，`uv.lock` 记录完整解析结果及
分发文件哈希。CI 和生产镜像共同使用 `uv sync --frozen` 安装。

## 实际解析版本

| Package | 实际解析版本 |
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

项目有意同时保留两套 HTTP Client。项目自身的 Graph 和 OpenID Connect
请求继续使用直接依赖 `httpx`；FastMCP 4 与 MCP SDK v2 内部使用独立的传递依赖
`httpx2`。两者责任方不同，不是意外的重复依赖。

构建依赖为 `setuptools==84.2.1`、`wheel==0.48.0`，安装工具为 `uv==0.13.7`。
Python 保持受支持的 3.12 运行时边界。pypdf 升级到 7，移除了阻止
[GHSA-59jv-x5x2-gg4c](https://github.com/py-pdf/pypdf/security/advisories/GHSA-59jv-x5x2-gg4c)
安全修复的旧版本上限。

## 兼容性证据

CI 已配置为使用锁定依赖运行完整 Python 测试及 `pip-audit --skip-editable`，此次升级仍需
通过 CI 验证。测试继续覆盖生产 Application Factory、真实 ASGI/MCP Transport、
OAuth Refresh Rotation 和 Mock Graph/OBO 流程。日期 Schema 元数据已改用 Pydantic 支持的
`json_schema_extra` API。

测试在同一端点覆盖两代协议：

- Legacy `2025-03-26`：initialize、tools/list、全部 11 个 Mail Tools、
  Sanitized Failure，以及 Alice/Bob 并发身份隔离。
- Modern `2026-07-28`：携带 Request Metadata 且无 MCP Session ID 的
  server/discover、tools/list 和 tools/call。
- 完整 Unit/Integration Suite，包括 Health、未认证 MCP 拒绝和隔离附件解析检查。

CI 另外构建并 Smoke-test 生产 Docker Image、Non-root Worker 和 Compose 配置，脚本继续
拒绝已知 Authlib JOSE、HTTPX 迁移告警。所有构建和测试必须在 CI 中执行，此编辑环境禁止
创建测试环境或运行本地测试。

FastMCP 4 历史迁移证据：[迁移前](https://github.com/sagehou/M365-MCP-Server/actions/runs/35195153651)、
[迁移后](https://github.com/sagehou/M365-MCP-Server/actions/runs/35195810661)。

## 范围边界

Python 服务保留 Bearer Validation、Entra OBO 和原有 Feature Flag 后的 OAuth 服务。
[Windows 本机单用户版](windows-local-go.md) 使用独立 Go Module 和公共客户端浏览器登录，
不内置服务器凭据，也不替代多用户服务。Go 版本和依赖校验值记录在 `native/go.mod`、
`native/go.sum` 中。

原有 NativeAOT Windows 主机和流水线继续保留，SDK/运行时目标从 .NET 8 升级为 .NET 10 LTS；
它没有直接 NuGet 依赖。两套 Windows 流水线同步升级 GitHub Actions 依赖。Go 产物名包含
`windows-local-go`，不会替换原有 Windows 发布文件。

Go Module 为 [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932) 固定了上游
`x/crypto` 修复提交 `v0.58.1-0.20261009181718-2f1f834330b2`，该提交移除了停止维护的
OpenPGP 包；本程序不导入 OpenPGP。

依赖和锁文件更新后，必须通过 CI 执行 `uv sync --frozen --extra dev`、完整测试及
`pip-audit`，验证通过后再发布镜像。
实际部署的镜像 Digest 需另行记录。

---

Mock 测试不能代替真实租户和客户端验收。更新日期：2026-11-10。
