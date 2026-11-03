# 定制智能体平台 · P0/P1 补强实施说明

> 对应分析报告：`docs/agent-platform-gap-analysis.md`
> 实施日期：2026-10-29
> 状态：P0 全部后端完成 + P1 三项后端完成；反馈 UI 已接入聊天工具栏

---

## 一、本次交付清单

| 编号 | 能力 | 交付内容 | 状态 |
|---|---|---|---|
| P0-1 | 版本与回滚 | `agent_versions` 表、自动快照、发布/回滚/对比 API、运营页版本对比视图 | ✅ 前后端 |
| P0-2 | 编辑器试跑 | `AgentDryRunService` + 编辑器「试跑」抽屉 | ✅ 前后端 |
| P0-3 | 点赞点踩 | `message_feedback` 表、提交/查询/列表 API、聊天工具栏按钮 | ✅ 前后端 |
| P0-4 | 用量与成本 | 基于 messages 的 token 聚合、按天趋势、质量报告 API、运营页概览 | ✅ 前后端 |
| P1-2 | 模板市场 | `agent_templates` 表、列表/安装/另存/删除 API、4 个内置模板种子 | ✅ 前后端 |
| P1-3 | 公开主页 | `agent_public_pages` 表、配置 CRUD、匿名解析接口 | ✅ 后端 |
| P1-4 | 评测 | 质量报告（满意度 + 平均步数 + 平均耗时） | ✅ 部分 |
| P1-5 | 内容安全 | `moderation_rules` / `moderation_hits` 表、规则 CRUD、扫描与命中 API、运营页安全 tab（**观察模式，不拦截**） | ✅ 观察模式 |
| P1-1 | 工作流编排 | 设计规格（见第六节） | 📋 设计 |
| — | 运营页 i18n | 全部文案接入 zh-CN / en-US | ✅ |

---

## 二、数据库迁移

新增迁移（PostgreSQL 与 SQLite 双份）：

- `migrations/versioned/0001002_agent_ops.up.sql` / `.down.sql`
- `migrations/sqlite/0001002_agent_ops.up.sql` / `.down.sql`

四张表：

| 表 | 用途 | 关键约束 |
|---|---|---|
| `agent_versions` | 智能体配置的不可变快照 | 唯一键 `(tenant_id, agent_id, version_no)` |
| `message_feedback` | 单条答案的用户评价 | 唯一键 `(tenant_id, message_id, user_id)`，一人一票可改 |
| `agent_templates` | 可安装的智能体模板 | `tenant_id=0` 表示内置模板 |
| `agent_public_pages` | 对外落地页 | `slug` 全局唯一 |

启动时由 `internal/database/migration.go` 自动执行，无需手工操作。

**注意**：用量不单独建表。`messages` 已经保存了 `usage`、`agent_id`、`agent_steps`、`agent_duration_ms`，所有渠道（网页、嵌入、IM、自动化）都会写这条记录。从它聚合出来的数字不会和用户实际收到的答案脱节，也能直接统计历史数据。

---

## 三、API 清单

### 版本

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| POST | `/api/v1/agents/:id/versions` | Owner/Admin | 显式打一个版本快照 |
| GET | `/api/v1/agents/:id/versions` | Viewer | 版本列表（分页） |
| GET | `/api/v1/agents/:id/versions/:version_no` | Viewer | 单个快照（含完整配置） |
| GET | `/api/v1/agents/:id/version-diff?from=&to=` | Viewer | 顶层配置键差异 |
| POST | `/api/v1/agents/:id/versions/:version_no/rollback` | Owner/Admin | 回滚 |

自动快照已接入 `CustomAgentHandler`：创建（source=create）与每次保存（source=update）都会写一条，通过 `WithSnapshotRecorder` 注入，不改变原有构造函数签名，因此既有测试不受影响。

### 反馈

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/answer-feedback` | 提交/修改评价，body 含 `session_id`、`message_id`、`rating`、`reason_tags`、`comment` |
| GET | `/api/v1/answer-feedback?message_id=` | 查询我对某条答案的评价 |
| GET | `/api/v1/agents/:id/feedback?rating=` | 反馈列表（运营侧） |

服务端不信任客户端传来的 `agent_id`：用 `message_id + session_id` 反查消息，从消息上取真实的 agent，避免把差评挂到别的智能体上。

### 用量与质量

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/agents/:id/usage?days=30` | 轮次、prompt/completion/total tokens、平均步数、平均耗时、会话数 |
| GET | `/api/v1/agents/:id/usage/timeseries?days=30` | 按天趋势 |
| GET | `/api/v1/agents/:id/quality?days=30` | 用量 + 满意度合并报告 |

聚合 SQL 按方言分支（PostgreSQL / SQLite / MySQL），JSON 取值与日期分桶各写一份，见 `internal/application/repository/agentops.go` 的 `usageDialect`。

### 模板

| 方法 | 路径 | 权限 |
|---|---|---|
| GET | `/api/v1/agent-templates?category=` | Viewer |
| POST | `/api/v1/agent-templates/:id/install` | Contributor |
| POST | `/api/v1/agents/:id/templates` | Contributor |
| DELETE | `/api/v1/agent-templates/:id` | Admin（仅工作区模板） |

安装走 `CustomAgentService.CreateAgent`，与手工创建共享校验、默认值和审计，不会出现"模板装出来的 Agent 和正常 Agent 行为不一致"。

### 公开主页

| 方法 | 路径 | 鉴权 |
|---|---|---|
| GET/PUT/DELETE | `/api/v1/agents/:id/public-page` | Owner/Admin |
| GET | `/api/v1/public/agents/:slug?password=` | 匿名 |

匿名接口只返回落地页元数据与 `embed_channel_id`，不返回提示词、工具白名单、知识库范围。密码使用 bcrypt 存储，`password_hash` 带 `json:"-"`，不会出现在任何响应里。

---

## 四、前端改动

| 文件 | 改动 |
|---|---|
| `frontend/src/api/agentops/index.ts` | 新增全部接口封装与类型 |
| `frontend/src/views/chat/components/AnswerFeedbackButtons.vue` | 新增点赞/点踩组件，点踩展开原因面板 |
| `frontend/src/views/chat/components/AgentStreamDisplay.vue` | Agent 回答工具栏接入 |
| `frontend/src/views/chat/components/botmsg.vue` | 普通 RAG 回答工具栏接入 |
| `frontend/src/i18n/locales/{zh-CN,en-US}.ts` | 新增 `agent.feedback.*` 文案 |

组件在挂载时读取已有评价并回填，刷新页面后状态不丢。点踩原因使用闭集（答非所问 / 引用错误 / 不完整 / 太慢 / 格式 / 工具失败 / 不安全 / 其他），保证跨 Agent 可比较。

---

## 五、P0-2 编辑器试跑（方案）

现有编辑器 7003 行，配置项 80+，保存前无法验证。建议：

1. 新增 `POST /api/v1/agents/:id/dry-run`：body 携带草稿 `config` 与 `query`，服务端用 `types.CustomAgent` 临时构造实例，**不落库**，复用现有 SSE 流式通道返回。
2. 前端在 `AgentEditorModal` 右侧加抽屉，展示：回答正文、命中的工具调用、检索片段、token 消耗、耗时。
3. 数据来源直接复用 `internal/agent/observe.go` 的观测事件，不新增埋点。
4. 验收：任意配置改动可在 3 秒内看到首字，且试跑不产生 `messages` 记录。

---

## 六、P1-1 工作流编排（设计规格）

现状：只有 ReAct 自主规划，没有确定性编排。企业场景（查库 → 判断 → 建单 → 回执）用 ReAct 跑，成本与稳定性都不可控。

**最小可用节点集（9 个）**

| 节点 | 复用现有能力 |
|---|---|
| start / end | 新增，纯路由 |
| retrieval | 现有检索服务 |
| llm | 现有模型调用 |
| condition | 新增，表达式求值 |
| http | 新增，带超时与重试 |
| code | 现有沙箱 |
| tool | `internal/agent/tools` |
| human_approval | `internal/agent/approval` |

**执行语义**

- 第三种 `agent_mode = "workflow"`，与 `quick-answer`、`smart-reasoning` 并列。
- DAG 拓扑排序执行；节点级超时、重试、失败分支。
- 变量作用域：`{{start.input.x}}`、`{{node_id.output}}`。
- 版本、评测、反馈全部复用本次新增的 P0 设施，不另起一套运营逻辑。
- Workflow 可作为工具被 ReAct Agent 调用，两者组合而非互斥。

不建议在本次直接实现：画布交互 + 执行器 + 调试器是 4-6 周量级，半成品比没有更糟。

---

## 七、未实施项与原因

| 项 | 状态 | 说明 |
|---|---|---|
| 编辑器试跑 UI | ✅ 已完成 | `AgentDryRunDrawer.vue` + 编辑器 footer「试跑」按钮 |
| 模板种子数据 | ✅ 已完成 | 4 个内置模板，`tenant_id=0`，每次启动幂等 upsert（见 7.1） |
| 版本对比 UI | ✅ 已完成 | 运营页版本 tab 勾选两个版本 → 对比对话框 |
| 内容安全 / 审核 | ✅ 观察模式 | 规则 + 命中记录 + 扫描，**不拦截**（见 7.2） |
| 运营页 i18n | ✅ 已完成 | 全部文案接入 zh-CN / en-US |
| 评测中心页面 | 📋 待办 | 后端 `/evaluation` 已有 RAG 指标，前端仍无入口 |
| 工作流编排 | 📋 设计 | 规格见第六节，实现量 4-6 周，不建议与本期混做 |
| 内容审核拦截 | 📋 待办 | 需先积累真实命中数据并确认误杀率 |

### 7.1 模板种子为什么用代码生成而不是手写 JSON

内置模板的 `config` 必须与 `CustomAgentConfig` 的字段严格一致，手写 JSON 一旦字段改名就会静默失效（装出来的 Agent 行为与预期不符，且没有任何报错）。因此种子在 `internal/types/agent_templates_builtin.go` 里用结构体构造、再 `json.Marshal`：

- 字段改名时**编译失败**，而不是运行期静默降级
- `internal/types/agent_templates_builtin_test.go` 校验：ID 唯一、config 可反序列化、`agent_mode` 合法、`allowed_tools` 都是 `internal/agent/tools` 里的真实常量、两次调用字节一致（保证 upsert 幂等）
- 模板**不写 `model_id`**：模板无法知道工作区配了哪个模型，硬编码会导致安装后第一次提问就失败
- `mcp_selection_mode` / `skills_selection_mode` 固定为 `none`：安装模板不应静默获得工作区的 MCP 服务与技能权限

种子在 `BuildContainer` 末尾以 `container.Invoke` 执行，失败只告警不阻断启动；`UpsertBuiltin` 的更新列刻意不含 `installs`，升级重新播种不会把累计安装数清零。

### 7.2 内容审核为什么只记录不拦截

默认拦截的风险是不对称的：**误杀一条正常回答的代价，远高于观察期内漏检一次**。因此当前实现：

- 扫描 `messages`（经 `sessions` 关联租户），命中写入 `moderation_hits`，**不改变任何用户可见输出**
- 命中表对 `(tenant_id, message_id, rule_id)` 唯一，重复扫描不会重复计数
- 内置规则只有**结构性检测**（手机号、身份证、银行卡、邮箱），不内置任何敏感词表 —— 词表会过期、跨法域不一致，且错一条就会静默屏蔽正常业务内容
- 租户自定义规则存 `moderation_rules`，写入时即校验正则能否编译，避免坏规则在每次扫描时被静默跳过
- `moderation_rules.action` 字段已预留 `block`，将来开启拦截是改数据而不是改表结构

积累一段时间命中数据、确认误杀率可接受后，再决定哪些规则切到 `block`。

---

## 八、验证方式

本机已装好 CGO 工具链（`E:/xyh/mingw64`，GCC 14.2.0 posix/ucrt/seh），构建与测试均已实际跑通，不再是「只做静态检查」的状态。环境搭建见 `docs/dev-env-windows-cgo.md`。

**已执行的验证**

```bash
go build ./...                     # ✅ 通过
go vet ./internal/handler/ ./internal/router/ ./internal/application/service/ \
       ./internal/application/repository/ ./internal/types/... ./internal/container/   # ✅ 通过
go test ./internal/container/ -run TestRouterWiring          # ✅ lite / redis 两种模式均通过
go test ./internal/router/                                   # ✅
go test ./internal/types/ -run 'TestBuiltin'                 # ✅ 3 项
go test ./internal/application/service/ -run 'TestNormalizePage|TestSlugPattern'      # ✅
go test ./internal/application/service/ -run 'TestMatchModeration|TestModerationSnippet|TestBuiltinPII|TestBuiltinBankCard'  # ✅ 8 项
cd frontend && npm run type-check  # ✅ exit 0
cd frontend && npm run build       # ✅ 2m53s
```

**运行时验证（Lite 模式实跑）**

| 观察点 | 实际结果 |
|---|---|
| 迁移 | `Database migrated from version 53 to 54` |
| 模板种子 | `[Container] seeded 4 builtin agent templates`，库中 4 行 `tenant_id=0`、`source=builtin` |
| 审核建表 | `moderation_rules` / `moderation_hits` 均已创建 |
| 路由 | 557 → **562**（+5 个审核端点） |
| 端点可达 | `/moderation-rules`、`/agents/:id/moderation-hits`、`/agents/:id/moderation-scan` 均返回 401（已注册，需认证）而非 404 |

**重点回归路径**

1. 创建 Agent → 查看 `/agents/:id/versions` 是否出现 v1（source=create）。
2. 修改 Agent 保存 → 出现 v2；回滚到 v1 → 出现 v3（source=rollback），配置恢复。
3. 聊天中点踩 → `/agents/:id/feedback?rating=down` 能查到，`reason_tags` 正确。
4. `/agents/:id/quality?days=30` 的 turns 与消息数一致，平均步数非零。
5. 保存 Agent 为模板 → 安装 → 新 Agent 配置与原 Agent 一致。
6. 配置公开主页 → 匿名访问 `/api/v1/public/agents/:slug` 返回元数据；带密码时无密码返回 401。
7. 运营页模板 tab 能看到 4 个内置模板（排在租户模板之前），安装后生成新 Agent。
8. 运营页安全 tab → 点「扫描最近对话」→ 命中记录出现在表格；重复扫描 matched 为 0（幂等）。
9. 新建一条自定义规则（如关键词 `竞品名`）→ 扫描 → 命中该规则的记录出现，且对话内容未被修改。
