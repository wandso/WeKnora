package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// descriptionChatModel answers every completion with one fixed string so a
// translation test does not need a provider.
type descriptionChatModel struct {
	reply    string
	lastMsgs []chat.Message
}

func (m *descriptionChatModel) Chat(
	_ context.Context, messages []chat.Message, _ *chat.ChatOptions,
) (*types.ChatResponse, error) {
	m.lastMsgs = append([]chat.Message(nil), messages...)
	return &types.ChatResponse{Content: m.reply}, nil
}

func (m *descriptionChatModel) ChatStream(
	context.Context, []chat.Message, *chat.ChatOptions,
) (<-chan types.StreamResponse, error) {
	return nil, nil
}

func (m *descriptionChatModel) GetModelName() string { return "description-test" }
func (m *descriptionChatModel) GetModelID() string   { return "model-1" }

// descriptionFixture is one workspace, one definition, and an Owner caller.
type descriptionFixture struct {
	svc    *TenantSkillService
	repo   repository.TenantSkillRepository
	ctx    context.Context
	model  *descriptionChatModel
	source string
}

func newDescriptionFixture(t *testing.T) *descriptionFixture {
	t.Helper()
	dsn := "file:skill-description-" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.TenantSkillCatalogEntity{}))
	repo := repository.NewTenantSkillRepository(db)

	const source = "Extract text and tables from PDF files."
	require.NoError(t, repo.CreateCatalog(context.Background(), &types.TenantSkillCatalogEntity{
		ID: "cat-pdf", TenantID: 7, Name: "pdf", Description: source, CreatedBy: "u-1",
	}))

	model := &descriptionChatModel{reply: "从 PDF 文件中提取文本与表格。"}
	svc := NewTenantSkillService(
		repo, nil, nil, nil, nil, nil, nil, nil,
		&stubModelService{
			chatModel:       model,
			availableModels: []*types.Model{{ID: "model-1", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive}},
		},
		nil, nil, nil, HostSandboxManager{},
	)

	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "u-1")
	ctx = context.WithValue(ctx, types.TenantRoleContextKey, types.TenantRoleOwner)
	return &descriptionFixture{svc: svc, repo: repo, ctx: ctx, model: model, source: source}
}

func TestEditCatalogDescriptionWritesOnlyTheTargetLanguage(t *testing.T) {
	f := newDescriptionFixture(t)

	view, err := f.svc.EditCatalogDescription(f.ctx, 7, "cat-pdf", "zh", "  从 PDF 提取文本  ")
	require.NoError(t, err)
	require.Equal(t, "从 PDF 提取文本", view.DescriptionI18n["zh"])
	// The canonical text parsed from SKILL.md must survive a translation edit.
	require.Equal(t, f.source, view.Description)

	stored, err := f.repo.GetCatalog(f.ctx, 7, "cat-pdf")
	require.NoError(t, err)
	require.Equal(t, "从 PDF 提取文本", stored.DescriptionVariants()["zh"])
	require.Equal(t, f.source, stored.Description)

	// lang="source" rewrites the canonical text and leaves the variant alone.
	view, err = f.svc.EditCatalogDescription(f.ctx, 7, "cat-pdf", SkillDescriptionLangSource, "New source text")
	require.NoError(t, err)
	require.Equal(t, "New source text", view.Description)
	require.Equal(t, "从 PDF 提取文本", view.DescriptionI18n["zh"])

	// An empty string clears the variant rather than storing a blank one.
	view, err = f.svc.EditCatalogDescription(f.ctx, 7, "cat-pdf", "zh", "   ")
	require.NoError(t, err)
	require.NotContains(t, view.DescriptionI18n, "zh")
}

func TestEditCatalogDescriptionRejectsOutsiders(t *testing.T) {
	f := newDescriptionFixture(t)

	viewer := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	viewer = context.WithValue(viewer, types.UserIDContextKey, "u-2")
	viewer = context.WithValue(viewer, types.TenantRoleContextKey, types.TenantRoleViewer)
	_, err := f.svc.EditCatalogDescription(viewer, 7, "cat-pdf", "zh", "hi")
	require.Error(t, err)

	// Another workspace can read a public definition but never edit it.
	otherTenant := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(9))
	otherTenant = context.WithValue(otherTenant, types.UserIDContextKey, "u-9")
	otherTenant = context.WithValue(otherTenant, types.TenantRoleContextKey, types.TenantRoleOwner)
	_, err = f.svc.EditCatalogDescription(otherTenant, 9, "cat-pdf", "zh", "hi")
	require.Error(t, err)

	_, err = f.svc.EditCatalogDescription(f.ctx, 7, "cat-missing", "zh", "hi")
	require.Error(t, err)
}

func TestTranslateCatalogDescriptionSavesOnlyWhenAsked(t *testing.T) {
	f := newDescriptionFixture(t)

	translated, view, err := f.svc.TranslateCatalogDescription(f.ctx, 7, "cat-pdf", "zh", "", false)
	require.NoError(t, err)
	require.Equal(t, "从 PDF 文件中提取文本与表格。", translated)
	require.NotContains(t, view.DescriptionI18n, "zh")
	require.Len(t, f.model.lastMsgs, 2)
	require.Contains(t, f.model.lastMsgs[0].Content, "Simplified Chinese")
	require.Equal(t, f.source, f.model.lastMsgs[1].Content)

	stored, err := f.repo.GetCatalog(f.ctx, 7, "cat-pdf")
	require.NoError(t, err)
	require.Empty(t, stored.DescriptionVariants()["zh"], "a preview must not be persisted")

	_, view, err = f.svc.TranslateCatalogDescription(f.ctx, 7, "cat-pdf", "zh", "", true)
	require.NoError(t, err)
	require.Equal(t, "从 PDF 文件中提取文本与表格。", view.DescriptionI18n["zh"])
}

func TestTranslateCatalogDescriptionRejectsUnsupportedLanguage(t *testing.T) {
	f := newDescriptionFixture(t)
	_, _, err := f.svc.TranslateCatalogDescription(f.ctx, 7, "cat-pdf", "fr", "", false)
	require.Error(t, err)
}

func TestCleanTranslatedTextStripsModelWrappers(t *testing.T) {
	require.Equal(t, "你好", cleanTranslatedText("```\n你好\n```"))
	require.Equal(t, "你好", cleanTranslatedText("“你好”"))
	require.Equal(t, "你好", cleanTranslatedText(`  "你好"  `))
	require.Equal(t, "keep me", cleanTranslatedText("keep me"))
}
