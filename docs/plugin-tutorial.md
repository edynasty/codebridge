# CodeBridge ChatGPT/Codex 插件实用教程

面向使用 ChatGPT Web / Codex Desktop 里 **@CodeBridge** 插件的最终用户与运维者。以仓库内 `deploy/agent-plugin/` 的 manifest、`internal/manager/mcp_tools.go` 的工具注册表和 `docs/chatgpt-web.md` 为事实来源；与运行版本不一致时，以部署实际返回的 `tools/list` 与 `list_devices` 为准。

当前包内身份（见 `deploy/agent-plugin/plugin.json`、`.codex-plugin/plugin.json`、`.app.json`）：

| 项 | 值 |
|---|---|
| 插件内部名 | `dev-6ab63d167fbc819180d2386919fa3370` |
| Connected App id | `asdk_app_6ab63d167fbc819180d2386919fa3370` |
| 版本 | `1.0.5` |
| MCP 端点 | `https://cb.edynasty.asia/mcp` |

## 1. 架构总览

```text
ChatGPT Web / Codex Desktop（@CodeBridge 插件）
        |  HTTPS + OAuth 2.1（MCP Streamable HTTP）
        v
CodeBridge Manager（/mcp：MCP 工具层；gRPC AgentService：设备通道；/admin：设备身份）
        |  出站 gRPC / HTTP2（设备主动连接）
        v
本地 CodeBridge Client（工具策略 / 权限门 / 检查点）
        |
        v
本地工作区（逻辑 workspace 名 + workspace 相对路径）
```

要点：

- 连接方向是**设备出站连 Manager**，本地机器不需要暴露任何入站端口。
- 远端只见**逻辑 workspace 名**和相对路径；物理路径不进入 ChatGPT。
- 工具策略（哪些工具启用、哪些 workspace 可写、bash/子代理权限）全部由本地 Client 决定，远端调用者无法越权开启。

## 2. 两条插件加载路径

### ChatGPT Web（App 绑定路径）

```text
@CodeBridge
  → 插件包（.codex-plugin/plugin.json → apps: ./.app.json）
  → Connected App（.app.json 引用 connector id）
  → Remote MCP: https://cb.edynasty.asia/mcp（OAuth 2.1 + CIMD）
  → tools 注入
```

**ChatGPT Web 不读 `.mcp.json`。** 工具注入依赖 `.app.json` 里真实注册的 connector id（`asdk_app_...` 或 `connector_...`）。注册步骤：ChatGPT → 设置 → 应用与连接器 → 添加新连接器 → 填 `https://cb.edynasty.asia/mcp` → 完成授权 → 从连接器详情页取得 id → 填入 `.app.json` → 重新打包安装。

### Codex Desktop（bundled MCP 路径）

```text
@CodeBridge
  → .codex-plugin/plugin.json（skills + apps + mcpServers）
  → .mcp.json（type: "http"，Codex 专用格式）
  → Remote MCP: https://cb.edynasty.asia/mcp（同一 OAuth）
  → tools 注入
```

Codex 自动发现插件根的 `.mcp.json`（`type: "http"`）；它与 Agent Plugins portable 规范的 `mcp.json`（`type: "streamable-http"`）是两个并存的规范，不要"统一"它们。

## 3. 五个配置文件的职责

| 文件 | 职责 | 规范 |
|---|---|---|
| `plugin.json` | portable 插件元数据 + `extensions.com.openai` 界面信息，`apps` 指向 `./.app.json` | Agent Plugins 1.0.0 |
| `mcp.json` | portable MCP 声明（`streamable-http`） | Agent Plugins 1.0.0 |
| `.codex-plugin/plugin.json` | Codex 插件元数据，绑定 `skills` / `apps` / `mcpServers` 三项 | Codex 兼容层 |
| `.mcp.json` | Codex bundled MCP（`type: "http"`） | Codex |
| `.app.json` | ChatGPT Web Connected App 绑定（`apps.<插件内部名>.id` = connector id） | ChatGPT connector |

`deploy/agent-plugin/` 目录就是包源文件：界面文案、品牌、两套传输类型都由本目录决定；`tools/pluginpack` 打包时只改写部署相关字段（version、MCP URL、app 绑定），改文案要直接改本目录 manifest。

## 4. 当前 Skills

| Skill | 用途 |
|---|---|
| `codebridge-ops` | 设备/工作区发现、文件浏览、tool_policy 理解、权限门控 bash。用户要求检查、搜索、诊断、跑命令时用 |
| `codebridge-write` | `write` / `edit` / `apply_patch` / `rollback_patch` 安全写流程：先 preview 后 confirm，保留 checkpoint |
| `codebridge-subagent` | 把自包含任务委托给本地子代理（默认 omp；profile 锁定 opencode/codex 时用对应端）。任务时长未知时默认用持久 `agent_start` 并立即结束当前 ChatGPT turn；`agent_status/result` 只用于显式检查、恢复和终态取结果 |

## 5. 当前 MCP 工具面

按 `internal/manager/mcp_tools.go` 的注册表（verify-plugin.sh 的在线检查也逐一验证这 17 个名字）：

**只读（`readOnlyHint=true`）**

- `list_devices` — 列出账户下已连接设备，含 `workspaces` 与 `tool_policy`。
- `list_workspaces` — 单设备的工作区视图。
- `list` — 列 workspace 相对目录。
- `read` — 读单个文本文件，单次上限 256 KiB。
- `agents_list` — 列出设备上实际配置的子代理 profile。
- `agent_status` / `agent_result` / `agent_runs` — 长任务跟进与恢复。
- `permission_grant` — 批准/拒绝挂起的权限请求。

**变更（`readOnlyHint=false` / `destructiveHint=true`）**

- `write` — 全文写文件（当前 Client 在目标已存在时拒绝 `write`，改文件用 `edit`/`apply_patch`）。
- `edit` — 精确替换一处 `old_text` → `new_text`。
- `apply_patch` — 多文件原子编辑（create/replace/delete 组合）。
- `rollback_patch` — 用 `checkpoint_id` 回滚一次成功写入；检查点保存在设备上，最多保留最近 20 个。
- `bash` — 在 workspace 根执行命令：30 秒超时、有界输出、无交互输入，需要本地权限。
- `agent` — 单次调用同步完成的子代理任务。
- `agent_start` — 启动长任务并立即返回 `run_id`；运行在本地独立于 MCP 会话，断线不取消。
- `agent_cancel` — 取消排队/运行中的 run。

除名字外的公共约定：每个调用带 `device_id` + `workspace`；路径一律 workspace 相对；每个工具声明 JSON Schema `outputSchema` 并在 `structuredContent` 返回匹配结果。本地 Client 还可以定义 `custom_tools`（窄化的包装工具），以 `tool_policy` 报告为准。

## 6. 工作区与 tool_policy 发现

写任何东西之前先做能力发现：

1. `list_devices` → 选在线设备。
2. 看设备广播的每个 workspace 的 `writable` 标志——只有本地 Client 在自己的配置 `writable_workspaces` 里显式列出的 workspace 才会带 `writable: true`（本地 UI 勾选"写模式"写入同一字段）。
3. 看 `tool_policy.enabled_tools` / `disabled_tools` / `custom_tools`——不要假设每台设备都有全部内置工具。
4. 需要更新的工作区视图时再调 `list_workspaces`。

本地 Client 是工具策略的唯一权威；不可写的 workspace 不允许用 `bash` 绕过写限制。

## 7. 权限流（bash 与子代理共用模式）

`bash`、`agent`、`agent_start` 都可能不执行而返回一个**权限请求**：

```text
调用 bash / agent / agent_start
        |
        v  命令或 harness 未命中本地 allow 规则
返回 request_id（命令未执行）
        |
        v
permission_grant { request_id, decision }
   once   — 只放行这一次执行
   always — 为该命令前缀持久化规则
   deny   — 拒绝
        |
        v  批准后
原样重试同一个调用 → 执行
```

规则：

- `always` 只在用户明确希望命令模式被记住时使用。
- 权限是**按工具隔离**的：批了 Agent 不等于批了 bash。
- 较窄的命令被 deny 后，不要悄悄改宽命令重试。
- 远端调用者永远无法给自己授权——`permission_grant` 释放的是本地挂起的请求。

## 8. Agent 长任务与超时策略

**默认不要设置总任务超时。** ChatGPT/Skill 无法可靠预知本地 Agent 需要读多少仓库文件、调用多少工具或等待模型多久，因此：

- `timeout_seconds` 省略或为 `0` = **无 wall-clock 总时限**；
- 默认使用 `agent_start` 启动持久 run，然后结束当前 ChatGPT turn；Runtime 事件独立于请求持续记录并在重连后补齐，禁止用 `agent_status / agent_result` 持续轮询等待；
- 只有用户明确要求 deadline、执行预算或硬性总时长时，才传正数 `timeout_seconds`；
- 不要把“长任务”“深度分析”“写文档”之类描述自行换算成 5 分钟、10 分钟或其他猜测值；
- MCP 会话断开不应取消 `agent_start` 的本地 run；
- 需要停止时显式调用 `agent_cancel`。

典型模式：

```text
agent_start (不传 timeout_seconds)
    ↓
run_id
    ↓
当前 ChatGPT turn 结束
    ↓
Runtime 事件 / 重连补偿
    ↓
terminal 已知
    ↓
agent_result（一次）
```

用户主动询问运行进度、诊断异常或恢复丢失上下文时，才调用 `agent_status` / `agent_runs`。

`timeout` 状态仍然保留，只用于调用方**明确传入正数 timeout_seconds** 的场景，而不是默认策略。

## 9. 安全写：preview → confirm → checkpoint → rollback

以 `apply_patch` 为例（`write`/`edit` 同构）：

```jsonc
// 第 1 步：preview，不改文件
{ "device_id": "mbp-m1", "workspace": "pms", "edits": [
    { "path": "src/main/java/.../RefundService.java",
      "old_text": "    BigDecimal amount = total;",
      "new_text": "    BigDecimal amount = applyThreshold(total);" } ],
  "preview": true }

// 第 2 步：检查返回的 diff 和受影响文件列表；符合预期后原样重试并 confirm
{ ...同上..., "confirm": true }

// 第 3 步：返回 checkpoint_id —— 验证期结束前保存好它
{ "checkpoint_id": "..." }

// 第 4 步：验证失败且应整体撤销时
rollback_patch { "device_id": "...", "workspace": "...", "checkpoint_id": "..." }
```

不变式：

- `preview: true` 绝不改文件；不要声称 preview 已写入。
- `confirm: true` 才真正落盘，写入是全有或全无，写前自动建检查点。
- `edit` / replace 型 patch 的 `old_text` 必须与当前内容**精确匹配且唯一**；文件变了就重读重建编辑。删除条目用整个当前文件作为 `old_text` 保持并发安全。
- 敏感路径（`.env`、私钥、`.git` 内部等）与二进制内容会被拒绝，且不可写，即使本地开了敏感读开关。
- 回滚是"整体撤销一个逻辑变更"，不是修部分错误的方法——部分错误应准备新的精确 patch。
- 验证：改后重读改动区域，必要时跑聚焦测试/build。

## 10. 打包与更新既有 App-backed 插件

```bash
make plugin-web \
  MCP_URL=https://cb.edynasty.asia/mcp \
  APP_ID=asdk_app_6ab63d167fbc819180d2386919fa3370 \
  PLUGIN_OUT=dist/codebridge-plugin.zip
```

`tools/pluginpack` 从 `deploy/agent-plugin/` 读包，只改写：两个 manifest 的 `version`（默认沿用本目录已有版本，不带 `--version` 重建不会降级已发布包）、app 绑定、`mcp.json` / `.mcp.json` 的 `url`；`skills/`、`assets/`、`scripts/`、`README.md` 原样进包。开发者模式拿到的 `plugin_asdk_app_...` id 会被规范化成 `.app.json` 需要的 `asdk_app_...` 形式。

**为什么内部插件名与 `.app.json` key 必须匹配既有插件**：ChatGPT 以插件内部名（`plugin.json` / `.codex-plugin/plugin.json` 的 `name`）标识"同一个插件"，`.app.json` 的 `apps` 对象以同一个名字为 key 关联 Connected App。更新已有插件时三者不一致会出现两种失败之一——上传被当作**新插件**拒绝（"Plugin upload name must match the existing plugin"），或者插件安装成功但没有工具（app 绑定落空）。所以当前三处都必须是 `dev-6ab63d167fbc819180d2386919fa3370`。

更新节奏：Manager 镜像携带工具描述与输入 schema，插件包携带界面文案/skills/app 绑定——**两层要一起重新部署**，然后重装包、刷新 ChatGPT、开新会话。改了 MCP 注册或 app 映射，必须重建重装包，不要指望已有会话热加载。

无 ChatGPT app 绑定的 portable/Codex 包用 `make plugin MCP_URL=...`；该包刻意不含 `.app.json`，不宣称是完整的 ChatGPT Web 注册应用包。

## 11. verify-plugin.sh：打包校验

```bash
bash scripts/verify-plugin.sh https://cb.edynasty.asia <admin密码>
```

静态检查（不传密码也执行）：

- 五个 manifest 均为合法 JSON；
- `mcp.json` transport = `streamable-http`，`.mcp.json` transport = `http`；
- `.app.json` 引用真实 connector id（非占位符，`connector_` 或 `asdk_app_` 前缀）；
- 插件身份一致：两个 manifest 的 `name` 与 `.app.json` 的 apps key 都是 `dev-6ab63d167fbc819180d2386919fa3370`；
- `.codex-plugin/plugin.json` 三项绑定（apps/skills/mcpServers）齐全；
- 三个必需 Skills 存在且 front-matter `name` 匹配；
- 两个 manifest 版本一致。

在线检查（传 admin 密码或 `CODEBRIDGE_AS_ADMIN_PASSWORD`）：走完整 OAuth 2.1（DCR 注册 + PKCE S256 + 授权码换 token），然后 `initialize`，再 `tools/list` 逐一确认 17 个工具全部暴露。

## 12. 故障排查

**"Plugin upload name must match the existing plugin"**
上传包的 `plugin.json` / `.codex-plugin/plugin.json` 内部名不是目标账户已装插件的内部名。对照第 10 节：内部名、`.app.json` apps key、已注册插件三者必须同为 `dev-6ab63d167fbc819180d2386919fa3370`。用 verify-plugin.sh 的 "plugin identity" 检查确认后再打包。

**插件可见但没有工具（missing app binding / 注入失败）**
按顺序查：① 服务端发现——MCP 客户端/Doctor 能列出工具；② 认证——token 的 resource/audience 与 `codebridge.read` scope 正确；③ 注册的 MCP 连接——开发者模式显示预期端点；④ 插件映射——`.app.json` 含注册的 connector id，两个 manifest 指向 `./.app.json`；⑤ 安装范围——包装给拥有该连接器的同一用户/workspace；⑥ 刷新并开新会话。不要用加重复 MCP 注册或放宽 OAuth 校验来"修复"。

**会话里还是旧版本（stale cached version）**
重装包后必须刷新 ChatGPT 并开**新会话**；已有会话不会热加载新 manifest。同时确认 Manager 镜像同步更新（schema/工具描述在镜像里，不在包里）。

**缺失某个工具（missing tool）**
先 `list_devices` 看 `tool_policy.enabled_tools` / `disabled_tools`——本地 Client 可能禁用了它或用 `custom_tools` 窄化了表面。工具在 `tools/list` 里也不见时，检查 Manager 镜像是否为当前版本（旧 Manager 早于 outputSchema 声明），用 `codebridge-doctor --url ...` 验证。

**workspace 不可写**
`list_devices` 里该 workspace 没带 `writable: true`——本地 Client 的 `writable_workspaces` 未包含它（或本地路径未以读写方式挂载）。在本地 Client 配置/UI 中显式开启后重连，远端无法代开。禁止用 `bash` 绕过。

**权限请求阻塞**
`bash` / `agent` / `agent_start` 返回 `request_id` 而非结果时，取 `request_id` 调 `permission_grant`（`once` / `always` / `deny`），批准后**原样重试**同一调用。命令/harness 需要本地 allow 规则（如 `allow agent omp`）才会直接放行。

**Agent 运行中断与恢复**
`agent_start` 的 run 在本地独立执行，MCP 断连、聊天关闭都不会取消它。恢复顺序：先 `agent_runs`（按设备+workspace 列出 run，历史跨 Client 重启保留），找回 `run_id`，再 `agent_status` 看状态（`queued` / `running` / `completed` / `failed` / `cancelled` / `timeout` / `interrupted`），终态后 `agent_result` 取输出。状态调用被中断**不要**直接再起一个重复 run——先查 `agent_runs`。确认无用才 `agent_cancel`。失败的 run 先看其 error 和进度再重试。

## 13. 端到端示例：@CodeBridge 委托本地 OMP 子代理

场景：让 ChatGPT 检查本地 `pms` 工作区的权限流实现并汇报，全程不改文件。

```text
@CodeBridge 在 pms 工作区检查 CodeBridge 的 Agent 权限流。找出为什么长任务 run
能在 MCP 断连后存活，验证相关测试，汇报具体文件与行为。不要修改任何文件。
```

模型按 Skills 与工具面依次执行（每步的真实参数来自前一步返回）：

1. **能力发现**（codebridge-ops）：`list_devices` → 选在线设备，确认 `pms` 在 `workspaces` 中、`tool_policy` 显示 Agent 工具已启用。
2. **子代理发现**（codebridge-subagent）：`agents_list` → 从返回的 profile 描述里挑一个探索/读向 profile，而不是凭记忆猜名字。
3. **委托**：任务时长不可可靠预估，因此默认用持久长任务模式，并且**不传** `timeout_seconds`——

```jsonc
agent_start {
  "device_id": "<第 1 步返回的设备 id>",
  "workspace": "pms",
  "task": "In this workspace, inspect the CodeBridge Agent permission flow. Find why a long-running run can survive an MCP disconnect, verify the relevant tests, and report the exact files and behavior. Do not modify files."
}
```

4. **权限门**（如本地未放行该 harness）：调用返回 `request_id` 而非 `run_id` → `permission_grant { decision: "once" }` → 原样重试 `agent_start` → 得到 `run_id`。
5. **跟进**：得到 `run_id` 后结束当前 ChatGPT turn，不持续轮询。Runtime 事件在本地独立记录并通过 gRPC 推送/重连补偿；只有用户主动询问状态或执行恢复时才调 `agent_status` / `agent_runs`，终态已知后调用一次 `agent_result`。
6. **验收**：把子代理结果当证据而非真相——必要时用 `read` / `list` 抽查它引用的文件；本任务只读，全程不触发写工具。
7. **异常分支**：若中途连接断开，Client 的 durable RunEvent journal 继续记录；重连后按序列自动补齐。若聊天上下文已丢失，可用 `agent_runs` 找回该 run，再按 5–6 恢复。

若任务改成"修复该问题并提交修改"，第 3 步换成允许编辑的任务描述，第 6 步改为：检查改动 diff → 聚焦测试 → 若需整体撤销用 `rollback_patch`（codebridge-write 流程）。

---

来源文件：`deploy/agent-plugin/`（全部 manifest、三个 SKILL.md、verify-plugin.sh）、`internal/manager/mcp_tools.go`、`internal/manager/server.go`（tool_policy 字段）、`Makefile`（plugin/plugin-web 目标）、`README.md`、`docs/chatgpt-web.md`。
