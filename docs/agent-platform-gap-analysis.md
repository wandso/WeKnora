# 定制智能体平台 · 差距分析与功能补强建议

> 审计对象：`Tencent/WeKnora`（本地仓库 v0.8.3，2026-10-14）
> 审计视角：把它当作一个「对外提供定制智能体（GPTs 类）的网站/产品」来看，还缺什么
> 结论日期：2026-10-29

---

## 一、一句话结论

**智能体的「能力」已经很强，缺的是「产品化外壳」：配置完不能试、改了不能回滚、上线后不知道效果、也不知道花了多少钱。**

WeKnora 的 Agent 运行时（ReAct 循环、Skills、沙箱、MCP、BrowserSkill、多智能体协作、长期记忆、审批授权）深度明显超过多数开源同类；真正的短板不在内核，而在**版本发布、质量闭环、成本运营、分发获客**这四个产品层能力。因此不建议再往内核堆工具，建议按本报告的 P0 清单补齐产品层。

---

## 二、现有能力盘点（先明确「不用做」的部分）

| 能力域 | 现状 | 代码证据 |
|---|---|---|
| 智能体配置 | 极完整：模式/预设/提示词/模型/温度/推理强度/引用/多轮/检索参数/FAQ/联网/多模态/文件类型/解析引擎 | `internal/types/custom_agent.go`（配置字段 80+） |
| 工具与扩展 | 内置工具、MCP（含 OAuth）、Skills（ClawHub/Git/ZIP）、沙箱（Docker/E2B/Cube）、本地浏览器、小工具卡片 | `internal/agent/tools/`、`internal/agent/skills/`、`internal/sandbox/` |
| 多智能体 | 已有父子 Agent 协作运行时 + 前端子会话标签页 | `internal/agent/collaboration/`、`frontend/src/composables/useChildAgentTabs.ts` |
| 记忆 | 跨会话长期记忆，可按 Agent 关闭 | `config.MemoryEnabled` |
| 分发渠道 | 网站嵌入 widget（含主题/限流/webhook）、企业微信/飞书/Slack/Telegram 等 IM、MCP Server、OpenAI 兼容 API、CLI | `internal/types/embed_channel.go`、`internal/im/`、`internal/mcpserver/` |
| 权限 | 四角色 RBAC、按资源归属、组织共享、Agent 级授权 Grant、审计日志 | `internal/router/rbac.go`、`internal/types/agent_grant.go` |
| 自动化 | 定时/单次触发、错过策略、并发策略、审批中断 | `internal/automation/`、`internal/types/automation.go` |
| 可观测 | Langfuse 接入（外部依赖） | `internal/agent/langfuse_test.go`、`internal/tracing/` |
| 评测 | 有后端接口与数据集指标（Precision/Recall/NDCG/MRR/MAP） | `internal/handler/evaluation.go`、`internal/types/evaluation.go` |

**判断：内核能力已足够支撑「定制智能体网站」，不需要重写。**

---

## 三、差距清单（按优先级）

### P0-1 · 没有草稿 / 版本 / 回滚 / 灰度 —— 最大的生产风险

**现状证据**
- `CustomAgent` 结构体无 `Version`、无 `Status`、无 `PublishedAt` 字段（`internal/types/custom_agent.go`）。
- 数据库无 `agent_versions` 表（`migrations/` 全量检索无匹配）。
- `PUT /agents/:id` 直接覆盖生效（`internal/router/routes_agent.go:44`）。

**为什么是 P0**
一个 Agent 同时挂在网站嵌入、IM 群、自动化任务上。运营同学在后台改一句提示词、换一个模型，**所有线上会话立刻生效**；改坏了没有任何回退手段，也无法回答"上周那版为什么答得好"。这是从 Demo 走向生产必须补的第一块。

**建议实现**
1. 表 `agent_versions(id, agent_id, tenant_id, version_no, config jsonb, changelog, created_by, created_at)`，每次 `PUT/PATCH` 落一条快照（配置本就整体存 jsonb，改造成本低）。
2. `CustomAgent` 增加 `draft_config`（编辑中的草稿）与 `published_version_no`（线上生效版本），列表/聊天只读 published。
3. 接口：`POST /agents/:id/publish`（草稿→发布，带 changelog）、`POST /agents/:id/rollback`（指定 version_no）、`GET /agents/:id/versions`、`GET /agents/:id/versions/:no/diff`。
4. 渠道侧加 `pinned_version`（可选），支持"嵌入渠道锁旧版、站内灰度新版"。

---

### P0-2 · 7000 行编辑器里没有「试跑」

**现状证据**
- `frontend/src/views/agent/AgentEditorModal.vue` 共 **7003 行**，全文件检索无 `preview` / `testRun` / 试跑相关逻辑。
- 现有路径是：保存 → 关闭弹窗 → 去聊天页选这个 Agent → 提问（`isPostCreateSession` 提示即此意）。

**为什么是 P0**
配置项 80+，用户在完全看不到效果的情况下盲调提示词、阈值、工具白名单。这是"定制智能体"类产品最劝退的一环，也是客服工单的主要来源。

**建议实现**
1. 编辑器右侧加「试跑」抽屉（复用现有 SSE 聊天接口，`agent_id` 用草稿配置临时构造，不落库）。
2. 显示本轮实际命中的工具调用、检索片段、token 消耗、耗时——即把 `internal/agent/observe.go` 已有的观测事件渲染成时间线。
3. 进阶：A/B 双栏，同一问题分别跑「草稿」与「线上版本」，并排比对。

---

### P0-3 · 没有用户反馈闭环（官方 issue 已确认）

**现状证据**
- 全仓库无 message 级点赞/点踩：`internal/types/message.go`、`internal/handler/` 无 feedback 字段；`migrations/` 无 feedback 表。
- GitHub issue **#1248「知识库问答-点赞点踩」** 仍处于 open（51 条评论），属社区高关注需求。

**为什么是 P0**
没有反馈就没有"坏案例"，没有坏案例就无法做评测集、无法做提示词迭代、无法证明 Agent 变好了。这是所有 Agent 产品的飞轮起点。

**建议实现（最小闭环，约 1-2 天）**
1. 表 `message_feedback(message_id, session_id, agent_id, tenant_id, user_id, rating, reason_tags, comment, created_at)`，`rating ∈ {up, down}`。
2. `POST /messages/:id/feedback`；聊天消息气泡加 👍/👎 + 差评原因多选（答非所问/引用错误/太慢/不安全）。
3. 差评消息一键「加入评测集」（复用现有 dataset 结构），形成"反馈→数据集→评测→改提示词"闭环。

---

### P0-4 · 没有 Agent 级用量、成本与预算

**现状证据**
- 无 `token_usage` / `usage_record` / `cost` 相关表（`migrations/` 检索仅命中无关表）。
- 唯一的统计接口是**嵌入渠道**维度：`GET /agents/:id/embed-channels/:channel_id/stats`（`internal/router/routes_agent.go:296`）。
- 成本只能去 Langfuse 看，属于外部系统。

**为什么是 P0**
要做"网站"，就要回答：这个 Agent 这个月烧了多少钱、哪个租户超了、要不要限流、要不要按用量收费。当前无法回答，也无法做配额拦截。

**建议实现**
1. 每次 turn 结束写 `agent_usage(agent_id, tenant_id, session_id, model_id, prompt_tokens, completion_tokens, cost_micro, latency_ms, tool_calls, status, created_at)`（`Message.Usage` 已有 token 数据，落库即可）。
2. 看板：会话数/活跃用户/成功率/P95 延迟/工具调用 TopN/差评率/单会话成本。
3. 配额：`agent_quota(agent_id, monthly_token_limit, monthly_cost_limit, action=block|degrade)`，在 `internal/ratelimit` 旁加一层用量闸门。

---

### P1-1 · 没有确定性工作流编排（对标产品都有）

**现状证据**
- 全仓库无 workflow 引擎（`internal/` 检索命中均为注释或无关模块）。

**影响**
WeKnora 的 Agent 是**自主规划（ReAct）**：灵活但不可控、不可复现、成本不可预估。企业客户大量场景其实是**确定性流程**——"先查订单库 → 若已发货走 A 话术 → 否则建工单 → 回执"。这类需求用 ReAct 做，稳定性和成本都不可接受。

**建议实现（新模块，与现有模式并存）**
- 新增第三种 `agent_mode = "workflow"`，节点类型先做最小集：`start / retrieval / llm / condition / http / code / tool / human_approval / end`。
- 复用现有能力：`retrieval` 直接调现有检索服务，`tool` 直接调 `internal/agent/tools`，`human_approval` 复用 `internal/agent/approval`，`code` 复用沙箱。
- 前端用 Vue Flow 类库画布；执行器做成 DAG 拓扑排序 + 节点级重试/超时，**不要**复用 ReAct 循环。
- 与 Agent 的关系：Workflow 可作为 Agent 的一个"工具"被调用，两者组合而不是互斥。

---

### P1-2 · 没有模板市场 / 一键克隆（分发能力缺失）

**现状证据**
- 仅有 5 个类型预设（`rag-qa / wiki-qa / hybrid-rag-wiki / data-analysis / custom`，见 `internal/types/agent_type_preset.go`）与 `POST /agents/:id/copy`。
- 无 marketplace / gallery / 模板库（全仓库检索无匹配）。

**影响**
新用户第一次进来面对 80+ 配置项，无从下手；优秀 Agent 无法在租户/组织间流通，平台没有"内容生态"。

**建议实现**
1. `agent_templates(id, name, category, icon, description, config jsonb, source, installs, rating)`，内置 15-20 个开箱模板（客服、合同审阅、财报问答、招标比对、代码库问答…）。
2. 从现有 Agent「另存为模板」；组织内模板墙；后续再做公开市场（带审核）。
3. 模板安装 = 复制 config + 引导绑定自己的模型/KB，避免"装完不能跑"。

---

### P1-3 · 只有嵌入 widget，没有 Agent 公开主页

**现状证据**
- `EmbedChannel` 提供 iframe/JS widget 与 publish token（`internal/types/embed_channel.go`），公开路由仅 `/embed/:channelId`（`internal/router/router.go:182`）。
- Agent 的分享是**组织内**共享（`agent_share_source.go`、`shared_agent_access.go`），需登录。

**影响**
无法像 GPTs / Coze 那样给出一个 `xxx.com/a/<slug>` 的匿名可访问主页，做 SEO、做投放、做"分享给客户直接试用"都缺一环。

**建议实现**
`agent_public_page(agent_id, slug, is_public, seo_title, seo_desc, og_image, access_mode=public|password|email_gate, daily_limit)`，前端复用 embed 的匿名会话通道，加独立落地页路由。注意与 embed 的限流/审计策略统一。

---

### P1-4 · 评测只有 RAG 指标，且没有界面

**现状证据**
- 指标仅 `Precision/Recall/NDCG/MRR/MAP`（`internal/types/evaluation.go:77-91`），评的是检索质量。
- 路由仅 `POST /evaluation` + `GET /evaluation`（`internal/router/routes_infra.go:96-99`），**前端无评测页面**（`frontend/src/views/` 无 evaluation）。

**影响**
无法回答"这个 Agent 的工具选对了吗、步骤绕了吗、有没有幻觉"。ReAct Agent 的评测必须是**轨迹级**的。

**建议实现**
1. 前端补「评测中心」页：数据集管理、跑批、指标对比、逐条 diff。
2. 新增 Agent 轨迹指标：工具选择准确率、无效步数、平均步数、任务完成率、引用支持率（groundedness）、人工标注通过率。
3. 支持"版本对比跑"——同一数据集跑 v3 与 v4，直接出结论（与 P0-1 版本表天然契合）。

---

### P1-5 · 没有内容安全 / 合规层

**现状证据**
- 无 moderation / 敏感词 / guardrail 模块（检索仅命中沙箱写权限与租户配置）。
- 工具侧有审批与风险分级（`internal/agent/approval/tool_policy.go`），但**输入输出文本**没有审核。

**影响**
面向公众的 Agent 网站，UGC 输入 + 模型输出双向风险；国内落地还涉及内容合规与 PII。

**建议实现**
1. 输入/输出双向 hook：`pre_input` / `post_output`，支持内置敏感词表 + 正则 PII（手机号/身份证/银行卡）+ 外部审核 API。
2. 命中策略：拦截 / 脱敏 / 改写 / 记录。默认记录不拦截，避免误杀。
3. 相关社区 PR 可参考：#3392「知识库文档脱敏」、#3139「可审计的质量与成本工作流」。

---

### P2 · 工程与体验债（可并行推进）

| 项 | 证据 | 建议 |
|---|---|---|
| 编辑器单体 | `AgentEditorModal.vue` 7003 行 | 按 section 拆子组件 + 独立表单 store，否则每次改动回归风险高 |
| 移动端适配 | issue **#917** 仍 open | 聊天页与 Agent 列表做响应式，输入框尺寸优先修 |
| MySQL 支持 | issue **#1418** 仍 open | 当前依赖 PG 特性（jsonb/paradedb），需评估方言抽象层成本 |
| 重建缓存复用 | issue **#1679**（77 评论） | OCR/Embedding/Wiki Map 内容寻址缓存，直接降本 |
| 可观测内置化 | 依赖 Langfuse 外部 | 内置 trace 查看器（读 `internal/tracing` 已有 span） |
| 对话转人工 | 无 handoff 机制 | IM 渠道加"转人工"事件 + webhook 外推 |
| GPTs 式私有知识直传 | 只能挂已有 KB（`KBSelectionMode`） | Agent 详情页支持直接上传文件，后台自动建专属 KB |
| 无 Agent 分类/标签 | 列表仅按 我的/空间/组织 分组 + 关键词 | 加 category/tags，为后续市场做准备 |

---

## 四、GitHub 同类项目对标

> 以下为各项目**公认的差异化能力**，具体以实现版本为准。

| 能力 | WeKnora | Dify | Coze Studio | FastGPT | MaxKB | RAGFlow | Langflow |
|---|---|---|---|---|---|---|---|
| 自主 Agent（ReAct + 工具） | ✅ 强 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **确定性工作流编排** | ❌ | ✅ 强 | ✅ 强 | ✅ 强 | ⚠️ 弱 | ⚠️ 弱 | ✅ 强 |
| 深度文档解析 | ✅ 强 | ⚠️ | ⚠️ | ⚠️ | ⚠️ | ✅ 强 | ❌ |
| 知识图谱 / Wiki | ✅ 强 | ❌ | ❌ | ❌ | ❌ | ⚠️ | ❌ |
| 沙箱 / 代码执行 | ✅ 强 | ⚠️ | ✅ | ⚠️ | ⚠️ | ❌ | ✅ |
| **Agent 版本与发布** | ❌ | ✅ | ✅ | ⚠️ | ⚠️ | ❌ | ❌ |
| **对话日志与标注** | ❌ | ✅ 强 | ✅ | ✅ | ✅ | ⚠️ | ❌ |
| **用量 / 成本统计** | ❌ | ✅ | ✅ | ✅ | ✅ | ⚠️ | ❌ |
| **模板 / 应用市场** | ❌ | ✅ | ✅ 商店 | ✅ | ✅ 应用商店 | ⚠️ | ⚠️ |
| 公开分享页 | ⚠️ 仅嵌入 | ✅ | ✅ | ✅ | ✅ | ⚠️ | ⚠️ |
| 多租户 RBAC | ✅ 强 | ✅ | ⚠️ | ⚠️ | ⚠️ | ⚠️ | ❌ |
| IM 多渠道 | ✅ 强 | ⚠️ | ✅ | ⚠️ | ⚠️ | ❌ | ❌ |

**读法**：WeKnora 在「知识侧」和「运行时」是领先的，在「应用侧」几乎全线缺失。这正好说明补强方向不是加更多工具，而是补应用层。

### 值得直接借鉴的 4 个模块

1. **Dify 的「应用」抽象**：Agent/Workflow/Chatbot 统一为 App，共享发布、日志、标注、密钥、监控。WeKnora 现在是 Agent 单一形态，可引入 App 壳层。
2. **Coze 的「商店 + 一键复制」**：模板即内容，是冷启动最有效的手段。
3. **FastGPT / MaxKB 的「对话日志 + 标注」**：把线上真实对话变成训练/评测资产，比任何自动指标都有效。
4. **Langflow / n8n 的节点编排范式**：若做工作流，直接采用成熟交互范式，不必自创。

---

## 五、建议落地的三个新模块（按投入产出排序）

### 模块 A：Agent 版本与发布流水线（2-3 周）

```
编辑草稿 ──试跑──▶ 保存版本 ──发布──▶ 线上生效
                        │              │
                        └── 回滚 ◀─────┘
```
- 交付：`agent_versions` 表、发布/回滚 API、版本 diff 视图、编辑器试跑抽屉。
- 验收：任意一次线上事故可在 30 秒内回退到上一版本；发布前必须至少跑通 3 条冒烟问题。

### 模块 B：质量运营中心（3-4 周）

```
用户反馈 ──▶ 差评池 ──▶ 评测集 ──▶ 版本对比跑 ──▶ 发布决策
                ▲                        │
                └──── 用量/成本看板 ◀─────┘
```
- 交付：反馈表 + 差评池、评测中心页面、轨迹级指标、Agent 用量看板、配额闸门。
- 验收：能回答"v4 比 v3 好多少、每月花多少钱、哪个 Agent 最该优化"。

### 模块 C：Workflow 模式（4-6 周，可后置）

- 交付：第三种 agent_mode、8 类节点、DAG 执行器、画布编辑器。
- 与 A/B 的关系：Workflow 也应纳入同一套版本/评测体系，避免形成两套运营逻辑。

---

## 六、路线图建议

| 阶段 | 目标 | 内容 | 衡量 |
|---|---|---|---|
| 第 1 期（2-3 周） | 敢上线 | 版本/回滚 + 编辑器试跑 + 点赞点踩 | 无回滚事故；配置类工单下降 50% |
| 第 2 期（3-4 周） | 看得清 | 用量成本看板 + 评测中心 + 差评池 | 能出月度 Agent 质量与成本报告 |
| 第 3 期（4-6 周） | 长得大 | 模板市场 + 公开主页 + 工作流模式 | 新用户 5 分钟内跑通第一个 Agent |

---

## 七、可以立刻做的三件小事（1-2 天量级）

1. **点赞点踩最小闭环**：`message_feedback` 表 + 两个接口 + 消息气泡按钮。直接对应 issue #1248，也最容易提 PR 被合并。
2. **编辑器试跑抽屉**：复用现有聊天 SSE，草稿配置不落库。纯前端 + 一个临时会话接口。
3. **Agent 列表加标签与分类**：为模板市场铺路，改动小、体感明显。

---

## 附：本次审计的代码定位索引

| 主题 | 文件 |
|---|---|
| 智能体配置结构 | `internal/types/custom_agent.go` |
| 智能体路由与权限 | `internal/router/routes_agent.go`、`internal/router/rbac.go` |
| 运行时引擎 | `internal/agent/engine.go`、`internal/agent/act.go` |
| 多智能体协作 | `internal/agent/collaboration/` |
| 工具注册 | `internal/agent/tools/definitions.go` |
| 审批与授权 | `internal/agent/approval/`、`internal/types/agent_grant.go` |
| 嵌入渠道 | `internal/types/embed_channel.go`、`internal/router/router.go:182` |
| 自动化 | `internal/automation/`、`internal/types/automation.go` |
| 评测 | `internal/types/evaluation.go`、`internal/handler/evaluation.go` |
| 前端编辑器 | `frontend/src/views/agent/AgentEditorModal.vue` |
| 前端列表 | `frontend/src/views/agent/AgentList.vue` |
