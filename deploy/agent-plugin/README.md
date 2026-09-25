# CodeBridge Plugin（v1.0.1）

双端加载链路。**两套配置、两种传输类型，不要混用。**

## ChatGPT Web

```text
@CodeBridge
  → 插件包（.codex-plugin/plugin.json → apps: ./.app.json）
  → Connected App（.app.json 引用 connector id）
  → Remote MCP: https://cb.edynasty.asia/mcp（OAuth 2.1 + CIMD）
  → tools 注入（list_devices / list_workspaces / read / bash / ...）
```

**关键**：ChatGPT Web 不读 `.mcp.json`。MCP tools 注入依赖 `.app.json`
里**真实注册的 connector id**（`connector_...` 格式）。注册方式：
ChatGPT → 设置 → 应用与连接器 → 添加新连接器 → 填
`https://cb.edynasty.asia/mcp` → 完成授权 → 从连接器详情页取得
connector id → 填入 `.app.json` → 重新打包安装。

## Codex Desktop

```text
@CodeBridge
  → .codex-plugin/plugin.json（skills + apps + mcpServers）
  → .mcp.json（type: "http" — Codex 专用格式）
  → Remote MCP: https://cb.edynasty.asia/mcp（同一 OAuth）
  → tools 注入
```

Codex 自动发现插件根的 `.mcp.json`（`type: "http"`），与 Agent Plugins
portable 的 `mcp.json`（`type: "streamable-http"`）是**两个不同规范**，
两者并存且各自正确。

## 配置职责

| 文件 | 职责 | 规范 |
|---|---|---|
| `plugin.json` | portable 元数据 + extensions.com.openai | Agent Plugins 1.0.0 |
| `mcp.json` | portable MCP 声明（streamable-http） | Agent Plugins 1.0.0 |
| `.codex-plugin/plugin.json` | Codex 插件元数据 + 三个绑定 | Codex 兼容层 |
| `.mcp.json` | Codex bundled MCP（http） | Codex |
| `.app.json` | ChatGPT Web Connected App 绑定 | ChatGPT connector |

## 验证

```bash
bash scripts/verify-plugin.sh https://cb.edynasty.asia <admin密码>
```
