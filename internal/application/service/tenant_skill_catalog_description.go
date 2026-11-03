package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

// SkillDescriptionLangSource selects the canonical description parsed out of
// SKILL.md rather than a console translation of it.
const SkillDescriptionLangSource = "source"

// maxSkillDescriptionRunes bounds both a hand-written description and the text
// sent to the translation model. The column is a text column, so the limit is
// about prompt cost and card layout rather than storage.
const maxSkillDescriptionRunes = 4000

// SkillCatalogDescriptionView is the description state of one definition after
// an edit or a translation: the canonical text plus every stored variant.
type SkillCatalogDescriptionView struct {
	ID              string            `json:"id"`
	Description     string            `json:"description"`
	DescriptionI18n map[string]string `json:"description_i18n"`
}

// EditCatalogDescription writes the description of one language.
//
// lang == "" or SkillDescriptionLangSource rewrites the canonical
// description — the copy the console shows when no translation exists. Any
// other language writes that variant only. Nothing here touches the stored
// zip, so an installed image keeps running the SKILL.md it was built from;
// re-installing is what propagates a text change to the agent prompt.
func (s *TenantSkillService) EditCatalogDescription(
	ctx context.Context, callerTenantID uint64, catalogID, lang, description string,
) (*SkillCatalogDescriptionView, error) {
	text := strings.TrimSpace(description)
	if utf8.RuneCountInString(text) > maxSkillDescriptionRunes {
		return nil, apperrors.NewBadRequestError(
			fmt.Sprintf("description is longer than %d characters", maxSkillDescriptionRunes))
	}
	catalog, err := s.manageableCatalog(ctx, callerTenantID, catalogID)
	if err != nil {
		return nil, err
	}

	target := strings.TrimSpace(lang)
	switch {
	case target == "" || strings.EqualFold(target, SkillDescriptionLangSource):
		catalog.Description = text
	default:
		code, ok := types.NormalizeSkillDescriptionLang(target)
		if !ok {
			return nil, apperrors.NewBadRequestError("lang must be zh, en or source")
		}
		catalog.SetDescriptionVariant(code, text)
	}

	if err := s.skills.UpdateCatalogDescriptions(
		ctx, callerTenantID, catalog.ID, catalog.Description, catalog.DescriptionI18n,
	); err != nil {
		return nil, err
	}
	return descriptionViewOf(catalog), nil
}

// TranslateCatalogDescription translates a description into one console
// language and, when save is set, stores it as that language's variant.
//
// The model call happens before any write, so a translation that fails leaves
// the current text untouched. text defaults to the canonical description, which
// is what the card's one-click action sends; the editor passes whatever is
// currently in its textarea so a partly rewritten draft can be translated too.
func (s *TenantSkillService) TranslateCatalogDescription(
	ctx context.Context, callerTenantID uint64, catalogID, lang, text string, save bool,
) (string, *SkillCatalogDescriptionView, error) {
	code, ok := types.NormalizeSkillDescriptionLang(lang)
	if !ok {
		return "", nil, apperrors.NewBadRequestError("lang must be zh or en")
	}
	catalog, err := s.manageableCatalog(ctx, callerTenantID, catalogID)
	if err != nil {
		return "", nil, err
	}

	source := strings.TrimSpace(text)
	if source == "" {
		source = strings.TrimSpace(catalog.Description)
	}
	if source == "" {
		return "", nil, apperrors.NewBadRequestError("there is no description to translate")
	}
	if utf8.RuneCountInString(source) > maxSkillDescriptionRunes {
		return "", nil, apperrors.NewBadRequestError(
			fmt.Sprintf("description is longer than %d characters", maxSkillDescriptionRunes))
	}

	translated, err := s.translateSkillText(ctx, code, source)
	if err != nil {
		return "", nil, err
	}
	if !save {
		return translated, descriptionViewOf(catalog), nil
	}

	catalog.SetDescriptionVariant(code, translated)
	if err := s.skills.UpdateCatalogDescriptions(
		ctx, callerTenantID, catalog.ID, catalog.Description, catalog.DescriptionI18n,
	); err != nil {
		return "", nil, err
	}
	logger.Infof(ctx, "[skill] translated description of catalog %s into %s", catalog.ID, code)
	return translated, descriptionViewOf(catalog), nil
}

// manageableCatalog loads a definition this caller may rewrite: it must belong
// to the caller's workspace, and the caller must be Owner/Admin or its creator.
func (s *TenantSkillService) manageableCatalog(
	ctx context.Context, callerTenantID uint64, catalogID string,
) (*types.TenantSkillCatalogEntity, error) {
	catalog, err := s.skills.GetCatalog(ctx, callerTenantID, catalogID)
	if err != nil {
		return nil, err
	}
	if catalog == nil {
		return nil, apperrors.NewNotFoundError("skill not found")
	}
	if catalog.TenantID != callerTenantID {
		return nil, apperrors.NewForbiddenError("this skill belongs to another workspace")
	}
	if !canManageSkillCatalog(catalog, skillCallerFrom(ctx)) {
		return nil, apperrors.NewForbiddenError("you cannot edit this skill")
	}
	return catalog, nil
}

// translateSkillText runs one non-streaming completion against the workspace's
// default chat model.
//
// The prompt asks for bare text rather than JSON: a description is prose, and
// wrapping it in an object only adds a parse step that can fail after the
// tokens are already paid for.
func (s *TenantSkillService) translateSkillText(
	ctx context.Context, lang, text string,
) (string, error) {
	if s.models == nil {
		return "", apperrors.NewInternalServerError("no model service is configured")
	}
	models, err := s.models.ListModels(ctx)
	if err != nil {
		return "", fmt.Errorf("list models for translation: %w", err)
	}
	picked := types.PickChatModel(models, "")
	if picked == nil {
		return "", apperrors.NewBadRequestError("no chat model is configured for this workspace")
	}
	chatModel, err := s.models.GetChatModel(ctx, picked.ID)
	if err != nil {
		return "", fmt.Errorf("load chat model for translation: %w", err)
	}

	systemPrompt := fmt.Sprintf(`You translate short product copy for a developer console.
Translate the user's text into %s.
Rules: return only the translated text, with no quotes, no explanation and no markdown fences;
keep product names, CLI commands, file paths, code identifiers and version numbers unchanged;
preserve the meaning and tone rather than the sentence structure;
if the text is already in %s, return it unchanged.`,
		types.SkillDescriptionLanguageName(lang), types.SkillDescriptionLanguageName(lang))

	thinking := false
	result, err := chatModel.Chat(
		types.WithLLMCallMetadata(ctx, "skill_description_translate", ""),
		[]chat.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: text},
		},
		&chat.ChatOptions{Temperature: 0.1, MaxTokens: 2048, Thinking: &thinking},
	)
	if err != nil {
		return "", fmt.Errorf("translate skill description: %w", err)
	}
	translated := cleanTranslatedText(result.Content)
	if translated == "" {
		return "", apperrors.NewInternalServerError("the model returned an empty translation")
	}
	return translated, nil
}

// cleanTranslatedText removes the wrappers a chat model adds even when told not
// to: a fenced block, and a pair of quotes around the whole answer.
func cleanTranslatedText(raw string) string {
	out := strings.TrimSpace(raw)
	if strings.HasPrefix(out, "```") {
		if idx := strings.Index(out, "\n"); idx >= 0 {
			out = out[idx+1:]
		}
		out = strings.TrimSuffix(strings.TrimSpace(out), "```")
		out = strings.TrimSpace(out)
	}
	// Compare runes, not bytes: the CJK quotation marks below are multi-byte,
	// so an out[i] byte comparison would never match them.
	runes := []rune(out)
	if len(runes) >= 2 {
		first, last := runes[0], runes[len(runes)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') ||
			(first == '“' && last == '”') || (first == '「' && last == '」') {
			out = strings.TrimSpace(string(runes[1 : len(runes)-1]))
		}
	}
	return out
}

func descriptionViewOf(catalog *types.TenantSkillCatalogEntity) *SkillCatalogDescriptionView {
	if catalog == nil {
		return nil
	}
	return &SkillCatalogDescriptionView{
		ID:              catalog.ID,
		Description:     catalog.Description,
		DescriptionI18n: catalog.DescriptionVariants(),
	}
}
