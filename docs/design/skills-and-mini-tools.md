# 技能模块与小工具模块：AIOps 对照与选型

本文记录本次为 WeKnora 增加「技能治理元数据」与「小工具（Mini Tools）」两个能力时的对照结论与实现取舍。参考对象是本机 AIOps 项目的 `internal/skills`、`internal/schema/mini_tool.go`、`ui/src/pages/SkillsPage.vue`、`ui/src/pages/MiniToolsPage.vue`。

## 1. 两套实现的核心差异

| 维度 | AIOps | WeKnora（改动前） | 本次取舍 |
| --- | --- | --- | --- |
| 技能来源 | 数据库注册 + `skills/*/SKILL.md` 同步为内置技能 | 只认 `SKILL.md` 包，安装进沙箱镜像 | 保留 SKILL.md 作为唯一事实来源；把 AIOps 的治理字段解析进安装记录 |
| 渐进披露 | 无（整篇 content 注入或注册为工具） | 有（Level 1 元数据 / Level 2 指令 / Level 3 资源） | 保留 WeKnora 方案，更省 token，也支持脚本资源 |
| 执行方式 | 进程内工具函数（server/client 两种模式） | 沙箱内脚本，带审批与凭证作用域 | 保留 WeKnora 的沙箱隔离，安全性更强 |
| 治理字段 | category / risk_level / execution_mode / keywords / prompt_hint / is_active | 仅 name / description / enabled | 取长：新增 category / risk_level / keywords / prompt_hint，落库并随 API 返回 |
| 技能 CRUD | 完整 CRUD + 内置同步 | 目录注册、安装、卸载、启停 | 维持既有流程，不重复造注册中心 |
| 智能体绑定 | Agent 配置中按 key 启停 | `skills_selection_mode` + `selected_skills` | 已有，保留；新增小工具的同等绑定 |
| 小工具 | 卡片：formula + variables + knowledge + examples + visibility + 绑定 agent/model + 独立会话 | 无 | 新建模块，但复用现有会话与聊天管线，不引入独立会话存储 |

## 2. 结论（择优）

1. **技能不换底座。** WeKnora 的「SKILL.md + 沙箱执行 + 渐进披露」在隔离性和上下文成本上都优于 AIOps 的「DB 注册 + 进程内工具」，因此只补 AIOps 的治理维度，不引入第二套技能注册表。
2. **治理字段要能解释「未知」。** AIOps 的 category 描述副作用类型（safe / read_local / read_remote / write），risk_level 描述严重度（low / medium / high / critical）。WeKnora 对缺失值不猜 `safe`，而是 `unclassified` + `medium`：高估安全性比承认未知更危险。
3. **小工具按 WeKnora 的租户模型重做。** AIOps 的卡片带 `agent_id` 与独立会话，适合单机工具型产品；WeKnora 是多租户知识库产品，因此：
   - 卡片属于工作空间（`tenant_id`），可见性为 `private` / `workspace`；
   - 智能体按**卡片 ID 列表**绑定（多对多），改卡片内容不需要改智能体；
   - 不新建会话表，卡片只是**提示词预设**，在现有对话里注入；
   - 每次对话可额外附带 `mini_tool_ids`，与智能体默认选择叠加（不是替换）。

## 3. 技能治理元数据

`SKILL.md` frontmatter 新增四个**可选**字段，旧技能包不受影响：

```yaml
---
name: contract-review
description: 审阅合同并列出风险条款
category: read_local        # safe | read_local | read_remote | write
risk_level: medium          # low | medium | high | critical
keywords: [合同, 条款, 风险]
prompt_hint: 先列条款再给结论
---
```

- 解析：`internal/agent/skills/skill.go`（`keywords` 同时接受 YAML 数组与逗号分隔字符串）。
- 落库：`tenant_skills` 新增 `category` / `risk_level` / `keywords` / `prompt_hint`（迁移 000135，SQLite 000052）。
- 读取：`GET /api/v1/skills` 与 `GET /api/v1/skills/catalog` 都返回 `category` / `risk_level` / `keywords` / `prompt_hint`，缺失值归一为 `unclassified` / `medium`。归一函数是包级 `skills.NormalizeSkillCategory` / `skills.NormalizeSkillRiskLevel`，安装写入、接口读取、目录聚合三处共用同一份词表。
- 目录聚合：治理字段存在安装行上，`SkillCatalogView` 从「优先 ready、否则最新」的一条安装上取值，因此列表页不必打开任何安装就能显示徽章。
- 展示：
  - 技能列表卡片显示「能力类型 + 风险等级」两枚徽章，颜色与文字同时表意；tooltip 补充关键词与 `prompt_hint`，`unclassified` 额外说明原因。
  - 列表工具栏新增能力类型筛选下拉，选项只列出当前目录实际出现的类型。
  - 智能体编辑器的技能勾选列表在技能名后显示风险徽章——绑定那一刻才是真正决定授权范围的时刻，风险不该只藏在设置页。
- 兼容：未声明字段的第三方技能照常安装，只是显示为「未分类 / 中风险」。

## 4. 小工具模块

### 数据模型

`mini_tools`：`id`、`tenant_id`、`owner_user_id`、`name`、`icon`、`category`、`description`、`formula`、`formula_note`、`variables`(JSON)、`knowledge`、`examples`(JSON)、`visibility`、`enabled`、`usage_count`、时间戳与软删除。

- `variables[]`：`key` / `label` / `unit` / `default` / `remark` / `required` / `min` / `max`，用于告诉模型「还缺哪个数」。
- `examples[]`：`title` / `input` / `output`，作为输出格式提示。

### API

| 方法 | 路径 | 说明 | 最低角色 |
| --- | --- | --- | --- |
| GET | `/api/v1/mini-tools` | 列表，支持 `category` / `keyword` / `scope=all\|mine\|workspace` / `include_disabled` | Viewer |
| GET | `/api/v1/mini-tools/categories` | 分类集合 | Viewer |
| GET | `/api/v1/mini-tools/:id` | 详情 | Viewer |
| POST | `/api/v1/mini-tools` | 创建 | Contributor |
| PUT | `/api/v1/mini-tools/:id` | 局部更新 | 创建者或 Admin |
| DELETE | `/api/v1/mini-tools/:id` | 删除（软删） | 创建者或 Admin |
| POST | `/api/v1/mini-tools/:id/duplicate` | 复制为私有副本 | Contributor |

私有卡片对非创建者表现为「不存在」（404 而不是 403），避免用 ID 探测他人卡片。

### 运行时注入

1. 计算生效集合：智能体 `mini_tools_selection_mode`（`all` / `selected` / `none`）+ 本轮 `mini_tool_ids`。
2. 过滤：`enabled`、调用方可读、去重；失效 ID 静默丢弃。
3. 渲染：`types.MiniTool.RenderPrompt()` 生成公式、输入、领域知识、示例与「缺少必填项要追问」的收尾约束。
4. 位置：系统提示词 `mini_tools` 小节，排在技能目录之后、记忆/协议之前（`internal/agent/prompts.go`）。
5. 计数：`usage_count` 在注入时递增，失败不影响本轮回答。

### 入口

- 「工具」页新增 **小工具** 标签（`/platform/toolbox/minitools`），支持搜索、分类筛选、卡片详情、创建/编辑/复制/删除。
- 智能体编辑器新增 **小工具** 小节（Agent 模式），选择全部 / 指定 / 禁用，并按分类勾选卡片。
- 卡片详情提示绑定位置；空状态引导到工具页创建。

## 5. 迁移与验证

- `migrations/versioned/000134_mini_tools.{up,down}.sql`
- `migrations/versioned/000135_skill_governance.{up,down}.sql`
- `migrations/sqlite/000051_mini_tools.{up,down}.sql`
- `migrations/sqlite/000052_skill_governance.{up,down}.sql`

启动时由既有 golang-migrate 流程执行（实测 118 → 135 一次到位）。回滚顺序为 000135 → 000134。

## 6. 在本机构建与发布

`docker compose build app` 在本机走不通：`docker/Dockerfile.app` 的 `browserskill` 阶段要 clone `github.com/Tencent/BrowserSkill`，`WITH_ANYDOC=1` 阶段要装 Rust 工具链，两处都被网络阻断。实际采用的发布方式：

1. 用 `golang:1.26-bookworm` 容器编译（需 `apt-get install libsqlite3-dev`）：
   ```bash
   make build-prod     # 产物 ./WeKnora
   ```
2. 把二进制与迁移脚本叠加到已发布的 app 镜像上（`.workbuddy/tmp/overlay/Dockerfile`）：
   ```dockerfile
   FROM wechatopenai/weknora-app:latest
   COPY .workbuddy/tmp/overlay/WeKnora /app/WeKnora
   COPY migrations /app/migrations
   ```
   `COPY migrations` 不能省：迁移由进程内按 `file://migrations/versioned` 读取，不打进镜像新迁移不会执行。
3. `docker build -f .workbuddy/tmp/overlay/Dockerfile -t wechatopenai/weknora-app:latest .`
4. `docker compose up -d --no-build --force-recreate app frontend`

代价：这样编出来的二进制没有 `-tags anydoc`，进程内 anydoc 解析引擎是 stub，Office 解析仍走 docreader 服务。要恢复该引擎，需要在能联网的环境用官方 Dockerfile 构建。

前端不用绕：`docker compose build frontend` 正常（`NPM_REGISTRY=https://registry.npmmirror.com` 加速）。

