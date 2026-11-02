# MCP 服务移植配置（来自 WeKnora 实例 10.33.120.194:8088）

## 文件说明

| 文件 | 内容 |
|------|------|
| `mcpServers.json` | **可直接粘贴的标准 `mcpServers` 配置**（仅含两个启用中的服务） |
| `mcp-services.json` | WeKnora 实例返回的原始服务配置（5 个服务，含停用项） |
| `oauth-discovery.json` | 两个启用服务的 OAuth 发现探测结果（授权服务器 / 保护资源元数据） |

## 两个启用服务的移植要点

### 1) 台州水雨情业务查询（`taizhou-water-rain-query`）
- **传输**：HTTP Streamable，`https://10.33.120.178:50216/mcp`
- **认证**：OAuth 2.0 授权码 + **PKCE（S256）**。授权服务器元数据在
  `https://10.33.120.178:50216/.well-known/oauth-authorization-server`（RFC 8414 标准路径，可自动发现），
  `token_endpoint_auth_methods_supported` 含 `none` → **新客户端可不带 client_secret，直接走公共客户端 + PKCE**。
- **client_id**：该实例固定为 `weknora-mcp-client`。若客户端支持指定 client_id，请填它；否则尝试动态注册公共客户端。
- **scopes**：48 个（`tool:query_*` 等，见 `mcp-services.json` 中该服务的 `scopes` 数组）。授权时按需勾选。
- **TLS ⚠️**：证书由**内网私有 CA**（`SJY Water Internal Root CA 2026`）签发（`CN=water178.invalid`）。
  客户端默认校验证书会失败。需要：导入该 CA 到系统信任库，或在该客户端里开启"允许自签/忽略 TLS 校验"。

### 2) 台州人事证书投标助手（`taizhou-hr-cert-tender`）
- **传输**：HTTP Streamable，`http://10.33.120.178:50310/prod-api/mcp`（**明文 HTTP**，无 TLS 问题）
- **认证**：RFC 8707 保护资源。无认证探测返回
  `WWW-Authenticate: Bearer resource_metadata=".../.well-known/oauth-protected-resource"`，
  元数据给出 `authorization_servers: ["http://10.33.120.178:50310/prod-api"]`、`scopes_supported: ["hrm:bidding:mcp"]`。
- **⚠️ 已知风险**：该服务的授权服务器元数据端点（`.../prod-api/.well-known/oauth-authorization-server`）
  **匿名访问返回 401**，客户端自动发现可能受阻。若自动 OAuth 失败，需要：
  - 手动拿一个 `hrm:bidding:mcp` 的 access token，以 `Authorization: Bearer <token>` 静态头方式配置；或
  - 联系该服务方确认授权服务器实现 / 提供可用 client 凭据。

### 3) 三个停用服务（万方 / WPS / 腾讯文档）
- 均为 `transport_type: http`，当前 WeKnora 版本**不支持该传输**，且 `enabled=false`。
- 万方、腾讯文档：无认证；WPS：需要自备 `OFDH_API_KEY`（实例中未配置）。
- 如需使用，先确认远端是否实际提供 HTTP Streamable / SSE 端点。

## 本地配置提示

本机（Windows）未检测到 Claude Desktop / Cursor / VS Code 的现成 MCP 配置。常见落点：

| 客户端 | 配置文件 |
|--------|----------|
| Claude Desktop | `%APPDATA%\Claude\claude_desktop_config.json` |
| Cursor（用户级） | `~/.cursor/mcp.json` |
| Cursor（项目级） | `<项目>/.cursor/mcp.json` |
| VS Code（工作区） | `<项目>/.vscode/mcp.json` |

直接把 `mcpServers.json` 中 `mcpServers` 的内容合并进对应文件即可。

若目标是把这两个服务加到**另一台 WeKnora 实例**：走 `POST /api/v1/mcp-services`（URL / transport / scopes / client_id 均为明文，可直接录入），
密钥在目标实例上重新配置（重走 OAuth 或重填 token）。
