package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentRequiresRerankModel(t *testing.T) {
	tests := []struct {
		name  string
		agent *types.CustomAgent
		want  bool
	}{
		{
			name: "knowledge search with all knowledge bases",
			agent: &types.CustomAgent{Config: types.CustomAgentConfig{
				KBSelectionMode: "all",
				AllowedTools:    []string{tools.ToolSearchKnowledge},
			}},
			want: true,
		},
		{
			name: "knowledge search with selected knowledge bases",
			agent: &types.CustomAgent{Config: types.CustomAgentConfig{
				KBSelectionMode: "selected",
				AllowedTools:    []string{tools.ToolSearchKnowledge},
			}},
			want: true,
		},
		{
			name: "knowledge search with knowledge bases disabled",
			agent: &types.CustomAgent{Config: types.CustomAgentConfig{
				KBSelectionMode: "none",
				AllowedTools:    []string{tools.ToolSearchKnowledge},
			}},
			want: false,
		},
		{
			name: "default tools with knowledge bases disabled",
			agent: &types.CustomAgent{Config: types.CustomAgentConfig{
				KBSelectionMode: "none",
			}},
			want: false,
		},
		{
			// Rows written before KBSelectionMode existed, and agents created
			// through the API without the field, carry "". Every other reader
			// (share DTO, runtime) treats that as "none", so this check must
			// too — otherwise the operator is told to pick a rerank model the
			// agent can never use.
			name: "unset mode with default tools",
			agent: &types.CustomAgent{Config: types.CustomAgentConfig{
				ModelID: "model-1",
			}},
			want: false,
		},
		{
			name: "whitespace mode with knowledge search tool",
			agent: &types.CustomAgent{Config: types.CustomAgentConfig{
				KBSelectionMode: "  ",
				AllowedTools:    []string{tools.ToolSearchKnowledge},
			}},
			want: false,
		},
		{
			name: "wiki tools do not use reranker",
			agent: &types.CustomAgent{Config: types.CustomAgentConfig{
				KBSelectionMode: "all",
				AllowedTools:    []string{"wiki_search", "wiki_read_page"},
			}},
			want: false,
		},
		{
			name:  "nil agent",
			agent: nil,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, agentRequiresRerankModel(tt.agent))
		})
	}
}

// The assistant searches knowledge bases but cannot be given a rerank model,
// so it must take the workspace's instead of failing every run.
func TestWeKnoraAssistantTakesWorkspaceRerankModel(t *testing.T) {
	require.NoError(t, types.LoadBuiltinAgentsConfig(filepath.Join("..", "..", "..", "config")))
	agent := types.GetBuiltinAgent(types.BuiltinWeKnoraAssistantID, 1)
	require.NotNil(t, agent)
	assert.True(t, agentRequiresRerankModel(agent))
	assert.Empty(t, agent.Config.RerankModelID)
	assert.True(t, types.UsesWorkspaceModelFallback(agent))
}

func TestPickWorkspaceRerankModelID(t *testing.T) {
	model := func(id string, typ types.ModelType, status types.ModelStatus, isDefault bool) *types.Model {
		return &types.Model{ID: id, Type: typ, Status: status, IsDefault: isDefault}
	}
	active := types.ModelStatusActive
	tests := []struct {
		name   string
		models []*types.Model
		want   string
	}{
		{name: "no models", want: ""},
		{
			name: "default rerank model wins over an earlier one",
			models: []*types.Model{
				model("first", types.ModelTypeRerank, active, false),
				model("default", types.ModelTypeRerank, active, true),
			},
			want: "default",
		},
		{
			name: "first active rerank model without a default",
			models: []*types.Model{
				nil,
				model("chat", types.ModelTypeKnowledgeQA, active, true),
				model("first", types.ModelTypeRerank, active, false),
				model("second", types.ModelTypeRerank, active, false),
			},
			want: "first",
		},
		{
			name: "inactive rerank models are skipped, even the default",
			models: []*types.Model{
				model("broken-default", types.ModelTypeRerank, types.ModelStatusDownloadFailed, true),
				model("usable", types.ModelTypeRerank, active, false),
			},
			want: "usable",
		},
		{
			name:   "no rerank model",
			models: []*types.Model{model("chat", types.ModelTypeKnowledgeQA, active, true)},
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, pickWorkspaceRerankModelID(tt.models))
		})
	}
}

type shareTestKBRepo struct {
	interfaces.KnowledgeBaseRepository
	kb *types.KnowledgeBase
}

// shareTestOrgRepo stops ShareAgent right after the configuration checks:
// with no organization, everything that comes next stays out of the test.
type shareTestOrgRepo struct {
	interfaces.OrganizationRepository
}

func (shareTestOrgRepo) GetByID(context.Context, string) (*types.Organization, error) {
	return nil, repository.ErrOrganizationNotFound
}

func (r shareTestKBRepo) GetKnowledgeBaseByIDs(
	_ context.Context, _ []string,
) ([]*types.KnowledgeBase, error) {
	return []*types.KnowledgeBase{r.kb}, nil
}

// A share that cannot go through used to fail with a single message naming
// both the chat model and the rerank model, so the console could not tell the
// operator which setting was actually missing. Each cause now reports itself.
func TestShareAgentReportsMissingChatModel(t *testing.T) {
	svc := &agentShareService{
		agentRepo: &builtinShareAgentRepo{agent: &types.CustomAgent{
			ID: "agent-no-model", TenantID: 84,
			Config: types.CustomAgentConfig{KBSelectionMode: "none"},
		}},
		kbRepo: shareTestKBRepo{},
	}

	_, err := svc.ShareAgent(
		context.Background(), "agent-no-model", "org-1", "user-1", 84, types.OrgRoleViewer)
	require.ErrorIs(t, err, ErrAgentMissingChatModel)
	require.NotErrorIs(t, err, ErrAgentMissingRerankModel)
}

// An unset KB mode must not be mistaken for "searches knowledge bases": an
// agent with a chat model but no KB mode is shareable without a reranker.
func TestShareAgentAllowsUnsetKBModeWithoutRerank(t *testing.T) {
	svc := &agentShareService{
		agentRepo: &builtinShareAgentRepo{agent: &types.CustomAgent{
			ID: "agent-unset-kb", TenantID: 84,
			Config: types.CustomAgentConfig{ModelID: "model-1"},
		}},
		kbRepo:  shareTestKBRepo{},
		orgRepo: shareTestOrgRepo{},
	}

	// Reaches the organization lookup instead of stopping on configuration.
	_, err := svc.ShareAgent(
		context.Background(), "agent-unset-kb", "org-1", "user-1", 84, types.OrgRoleViewer)
	require.ErrorIs(t, err, ErrOrgNotFound)
}

func TestShareAgentReportsMissingRerankModel(t *testing.T) {
	svc := &agentShareService{
		agentRepo: &builtinShareAgentRepo{agent: &types.CustomAgent{
			ID: "agent-rerank", TenantID: 84,
			Config: types.CustomAgentConfig{
				ModelID:         "model-1",
				KBSelectionMode: "selected",
				KnowledgeBases:  []string{"kb-1"},
				AllowedTools:    []string{tools.ToolSearchKnowledge},
			},
		}},
		kbRepo: shareTestKBRepo{kb: &types.KnowledgeBase{ID: "kb-1", TenantID: 84, CreatorID: "user-1"}},
	}

	_, err := svc.ShareAgent(
		context.Background(), "agent-rerank", "org-1", "user-1", 84, types.OrgRoleViewer)
	require.ErrorIs(t, err, ErrAgentMissingRerankModel)
	require.NotErrorIs(t, err, ErrAgentMissingChatModel)
}
