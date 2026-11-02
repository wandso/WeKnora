package types

import (
	"encoding/json"
	"fmt"
)

// Built-in template IDs. They are the upsert key, so they must never change:
// a tenant agent installed from a template keeps pointing at the same row, and
// re-seeding must update that row instead of creating a duplicate.
const (
	BuiltinTemplateKBQA    = "builtin-tpl-kb-qa"
	BuiltinTemplateAnalyst = "builtin-tpl-analyst"
	BuiltinTemplateSupport = "builtin-tpl-support"
	BuiltinTemplateWriter  = "builtin-tpl-writer"
)

// builtinTemplateSpec is the human-authored part of a template. The agent
// config is built from these fields instead of being written as a JSON blob,
// so a renamed or retyped config field fails at compile time rather than
// silently producing an agent that behaves differently from the intent.
type builtinTemplateSpec struct {
	id          string
	name        string
	category    string
	icon        string
	description string
	// Agent identity that install reproduces.
	agentName        string
	agentDescription string
	agentAvatar      string

	agentMode     string
	agentType     string
	systemPrompt  string
	temperature   float64
	allowedTools  []string
	maxIterations int
}

// BuiltinAgentTemplates returns the seed set of templates that ship with the
// product. Seeding is idempotent: callers upsert by ID.
//
// Design notes:
//   - ModelID is deliberately empty. A template cannot know which model a
//     workspace has configured, and guessing one produces an agent that fails
//     on first use. The installer is expected to pick a model.
//   - MCP and skill selection are pinned to "none" so installing a template
//     never silently grants access to a workspace's MCP servers or skills.
//     Knowledge base selection is left at "all", which is the same behaviour a
//     hand-created agent gets.
//   - Tool names are string literals on purpose: importing internal/agent/tools
//     here would invert the package dependency direction (tools already depends
//     on types). The values are pinned by TestBuiltinTemplateToolNames.
func BuiltinAgentTemplates() ([]*AgentTemplate, error) {
	specs := []builtinTemplateSpec{
		{
			id:               BuiltinTemplateKBQA,
			name:             "知识库问答助手",
			category:         "knowledge",
			icon:             "book",
			description:      "严格基于知识库回答，找不到依据时明确说明，并给出来源引用。适合产品手册、内部制度、FAQ 等场景。",
			agentName:        "知识库问答助手",
			agentDescription: "基于知识库的严谨问答，答案附带来源",
			agentAvatar:      "📚",
			agentMode:        AgentModeQuickAnswer,
			agentType:        AgentTypeRAGQA,
			systemPrompt: "你是企业知识库问答助手。回答规则：\n" +
				"1. 只使用检索到的知识库内容作答，不要引入外部知识或凭印象补充。\n" +
				"2. 如果检索结果不足以回答，直接说明「知识库中未找到相关内容」，并指出还需要什么信息，不要编造。\n" +
				"3. 回答先给结论，再给依据；涉及流程或步骤时用有序列表。\n" +
				"4. 引用具体条款、数值、时间时保留原文表述，不要改写关键信息。\n" +
				"5. 使用与提问相同的语言回答。",
			temperature:   0.2,
			maxIterations: 1,
		},
		{
			id:               BuiltinTemplateAnalyst,
			name:             "数据分析助手",
			category:         "analysis",
			icon:             "chart-bar",
			description:      "面向结构化数据的分析型智能体：先看表结构再取数，结论必须能追溯到具体数据。适合报表解读、指标排查。",
			agentName:        "数据分析助手",
			agentDescription: "先看表结构再取数的分析型智能体",
			agentAvatar:      "📊",
			agentMode:        AgentModeSmartReasoning,
			agentType:        AgentTypeDataAnalysis,
			systemPrompt: "你是数据分析助手。工作方式：\n" +
				"1. 动手取数之前，先用 data_schema 看清表结构和字段含义，不要假设字段名。\n" +
				"2. 需要业务口径时优先查知识库（search_knowledge），不要自行定义指标。\n" +
				"3. 每次取数都要说明用了哪张表、哪个口径、什么时间范围。\n" +
				"4. 给出结论时同时给出支撑数据；数据不足以支撑结论时，明确说明缺什么。\n" +
				"5. 发现数据异常（为空、突增突降、口径矛盾）先报告，不要直接下结论。",
			temperature:   0.2,
			allowedTools:  []string{"search_knowledge", "data_schema", "data_analysis"},
			maxIterations: 20,
		},
		{
			id:               BuiltinTemplateSupport,
			name:             "客服助手",
			category:         "service",
			icon:             "service",
			description:      "面向终端用户的客服话术：先共情、再给可执行方案，信息不足时主动追问，不确定时转人工。",
			agentName:        "客服助手",
			agentDescription: "共情、给方案、必要时转人工的客服话术",
			agentAvatar:      "🎧",
			agentMode:        AgentModeQuickAnswer,
			agentType:        AgentTypeRAGQA,
			systemPrompt: "你是客服助手，面对的是终端用户。回答规则：\n" +
				"1. 先简短回应用户的情绪或处境，再给解决方案，不要一上来就贴操作步骤。\n" +
				"2. 方案要可执行：分步骤、说明在哪操作、预期看到什么结果。\n" +
				"3. 只承诺知识库里写明的政策、时效和补偿口径，不要自行放宽。\n" +
				"4. 信息不足时先问清关键信息（订单号、错误提示、发生时间），不要猜。\n" +
				"5. 涉及退款争议、账号安全、投诉升级时，说明会转接人工客服。\n" +
				"6. 语气自然、简洁，不使用「亲」「尊敬的客户」这类套话。",
			temperature:   0.5,
			maxIterations: 1,
		},
		{
			id:               BuiltinTemplateWriter,
			name:             "内容创作助手",
			category:         "content",
			icon:             "edit",
			description:      "带联网能力的写作助手：先确认受众与目标，再产出结构清晰的初稿，事实性内容会先查证。",
			agentName:        "内容创作助手",
			agentDescription: "先确认受众与目标再动笔的写作助手",
			agentAvatar:      "✍️",
			agentMode:        AgentModeSmartReasoning,
			agentType:        AgentTypeCustom,
			systemPrompt: "你是内容创作助手。工作方式：\n" +
				"1. 动笔前先确认三件事：写给谁、要达到什么目的、期望篇幅。用户没说就主动问，最多问一轮。\n" +
				"2. 涉及事实、数据、时效性信息时先联网查证（web_search / web_fetch），并在文中标注来源。\n" +
				"3. 先给结构大纲，确认后再展开成稿；用户明确要求直接成稿时跳过确认。\n" +
				"4. 用具体表达替代空话：删掉「赋能」「抓手」「闭环」这类没有信息量的词。\n" +
				"5. 保持用户指定的语气和受众；未指定时默认专业但不生硬。",
			temperature:   0.8,
			allowedTools:  []string{"web_search", "web_fetch"},
			maxIterations: 15,
		},
	}

	out := make([]*AgentTemplate, 0, len(specs))
	for _, spec := range specs {
		cfg, err := builtinTemplateConfig(spec)
		if err != nil {
			return nil, err
		}
		out = append(out, &AgentTemplate{
			ID:               spec.id,
			TenantID:         0,
			Name:             spec.name,
			Category:         spec.category,
			Icon:             spec.icon,
			Description:      spec.description,
			Config:           cfg,
			AgentName:        spec.agentName,
			AgentDescription: spec.agentDescription,
			AgentAvatar:      spec.agentAvatar,
			Source:           AgentTemplateSourceBuiltin,
		})
	}
	return out, nil
}

// builtinTemplateConfig turns a spec into the same JSON shape a saved agent
// uses, so Install is a plain decode + CreateAgent.
func builtinTemplateConfig(spec builtinTemplateSpec) (JSON, error) {
	maxIterations := spec.maxIterations
	if spec.agentMode == AgentModeSmartReasoning && maxIterations == 0 {
		maxIterations = UnlimitedMaxIterations
	}
	if spec.agentMode == "" {
		maxIterations = 0
	}
	cfg := CustomAgentConfig{
		AgentMode:    spec.agentMode,
		AgentType:    spec.agentType,
		SystemPrompt: spec.systemPrompt,
		Temperature:  spec.temperature,
		// ModelID stays empty: the installer picks a workspace model.
		MaxIterations:       maxIterations,
		AllowedTools:        spec.allowedTools,
		MCPSelectionMode:    "none",
		SkillsSelectionMode: "none",
		KBSelectionMode:     "all",
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal builtin template %q: %w", spec.id, err)
	}
	return JSON(raw), nil
}
