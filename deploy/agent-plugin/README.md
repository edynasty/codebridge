# CodeBridge Plugin（v1.0.2）

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
里**真实注册的 connector id**（`asdk_app_...` 或 `connector_...`）。注册方式：
ChatGPT → 设置 → 应用与连接器 → 添加新连接器 → 填
`https://cb.edynasty.asia/mcp` → 完成授权 → 从连接器详情页取得
id → 填入 `.app.json` → 重新打包安装。

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

## 打包

本目录**就是**包源文件。`tools/pluginpack` 从这里读包，只改部署相关的字段：

```bash
make plugin-web \
  MCP_URL=https://cb.edynasty.asia/mcp \
  APP_ID=plugin_asdk_app_... \
  PLUGIN_OUT=dist/codebridge-plugin.zip
```

改写 `plugin.json` / `.codex-plugin/plugin.json` 的 `version`（默认取本目录已有的版本）与 app 绑定、`mcp.json` / `.mcp.json` 的 `url`；`skills/`、`assets/`、`scripts/`、`README.md` 原样进包。

因此**改文案要直接改本目录的 manifest**，不要改生成器：界面文案、品牌、两套传输类型都由本目录决定，重新打包不会把它们改回去。`.app.json` 里的 id 用真实注册值（`asdk_app_...` 或 `connector_...`），该文件不入库。

## 验证

```bash
bash scripts/verify-plugin.sh https://cb.edynasty.asia <admin密码>
```
