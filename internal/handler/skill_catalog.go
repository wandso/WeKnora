package handler

import (
	stderrors "errors"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// ListCatalog godoc
// @Summary      List workspace skills
// @Description  Returns every skill definition in this workspace and which sandbox configs it is installed on.
// @Tags         Skills
// @Produce      json
// @Success      200  {object}  handler.DataResponse{data=[]service.SkillCatalogView}
// @Security     Bearer
// @Security     ApiKeyAuth
// @x-agent      {"id":"skill.list_catalog","risk":"read","max_items":200}
// @x-agent-hint "Lists the skills already added to this workspace, with the sandbox config ids each is installed_on. skill.install_catalog puts one onto more sandboxes without fetching it again; for a skill not listed here, use skill.install with its source."
// @Router       /skills/catalog [get]
func (h *SkillHandler) ListCatalog(c *gin.Context) {
	if h.catalog == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []any{}})
		return
	}
	rows, err := h.catalog.ListCatalog(c.Request.Context(), sandboxConfigTenantID(c))
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

// GetCatalog godoc
// @Summary      Get a workspace skill
// @Description  Returns one skill definition of this workspace and which sandbox configs it is installed on.
// @Tags         Skills
// @Produce      json
// @Param        id   path  string  true  "Catalog skill ID"  extensions(x-agent=catalog_skill_id)
// @Success      200  {object}  handler.DataResponse{data=service.SkillCatalogView}
// @Failure      404  {object}  apperrors.AppError  "Skill not found"
// @Security     Bearer
// @Security     ApiKeyAuth
// @x-agent      {"id":"skill.get_catalog","risk":"read"}
// @Router       /skills/catalog/{id} [get]
func (h *SkillHandler) GetCatalog(c *gin.Context) {
	if h.catalog == nil {
		_ = c.Error(apperrors.NewNotFoundError("skill not found"))
		return
	}
	rows, err := h.catalog.ListCatalog(c.Request.Context(), sandboxConfigTenantID(c))
	if err != nil {
		_ = c.Error(err)
		return
	}
	id := c.Param("id")
	for _, row := range rows {
		if row.ID == id {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
			return
		}
	}
	_ = c.Error(apperrors.NewNotFoundError("skill not found"))
}

// RegisterCatalog godoc
// @Summary      Add a skill to the workspace catalog
// @Description  Records a skill without installing it. Send a zip as multipart field "file", or JSON {"source":"..."}.
// @Tags         Skills
// @Accept       json
// @Accept       multipart/form-data
// @Produce      json
// @Success      201  {object}  map[string]interface{}
// @Router       /skills/catalog [post]
func (h *SkillHandler) RegisterCatalog(c *gin.Context) {
	if h.catalog == nil {
		_ = c.Error(apperrors.NewInternalServerError("skill catalog is not configured"))
		return
	}
	maxBytes := secutils.GetMaxSkillBundleSize()
	limitSkillUploadBody(c, maxBytes)

	if strings.HasPrefix(c.ContentType(), "application/json") {
		h.registerCatalogFromSource(c)
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		if isRequestBodyTooLarge(err) {
			_ = c.Error(skillTooLargeError())
			return
		}
		_ = c.Error(apperrors.NewBadRequestError("file is required"))
		return
	}
	defer func() { _ = file.Close() }()
	if header.Size > maxBytes {
		_ = c.Error(skillTooLargeError())
		return
	}
	archive, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		_ = c.Error(apperrors.NewBadRequestError("failed to read the uploaded skill bundle"))
		return
	}
	if int64(len(archive)) > maxBytes {
		_ = c.Error(skillTooLargeError())
		return
	}

	// A single-skill archive keeps the legacy single-object response; a
	// multi-skill archive answers with the list of registered definitions.
	cats, err := h.catalog.RegisterCatalogsFromArchive(
		c.Request.Context(), sandboxConfigTenantID(c), archive,
	)
	if err != nil {
		respondSkillServiceError(c, err)
		return
	}
	if len(cats) == 1 {
		c.JSON(http.StatusCreated, gin.H{"success": true, "data": catalogIDResponse(cats[0])})
		return
	}
	resp := make([]gin.H, 0, len(cats))
	for _, cat := range cats {
		resp = append(resp, catalogIDResponse(cat))
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": resp})
}

func (h *SkillHandler) registerCatalogFromSource(c *gin.Context) {
	var req skillSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if stderrors.As(err, &tooLarge) {
			_ = c.Error(skillSourceRequestTooLargeError())
			return
		}
		_ = c.Error(apperrors.NewBadRequestError("invalid skill source request"))
		return
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		_ = c.Error(apperrors.NewBadRequestError("source is required"))
		return
	}
	cat, err := h.catalog.RegisterCatalogFromSource(
		c.Request.Context(), sandboxConfigTenantID(c), source,
	)
	if err != nil {
		respondSkillServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": catalogIDResponse(cat)})
}

type catalogInstallRequest struct {
	// Sandbox config ids to install onto: an agent's sandbox_config_id, or host on WeKnora Lite
	SandboxConfigIDs []string `json:"sandbox_config_ids" validate:"required" extensions:"x-agent,x-agent-maximum=20"`
}

// catalogInstallData is the data of an accepted catalog install. installs and
// errors map a sandbox config id to its new skill id or to why it failed;
// started and failed carry the same as lists.
type catalogInstallData struct {
	Installs map[string]string `json:"installs"`
	Errors   map[string]string `json:"errors,omitempty"`
	// Sandbox config ids whose install started
	Started []string `json:"started" extensions:"x-agent"`
	// "sandbox config id: reason" for each sandbox that was not started
	Failed []string `json:"failed,omitempty" extensions:"x-agent"`
}

// catalogInstallResponse is the body of POST /skills/catalog/{id}/install.
type catalogInstallResponse struct {
	// false when any sandbox failed to start
	Success bool               `json:"success" extensions:"x-agent"`
	Data    catalogInstallData `json:"data" extensions:"x-agent"`
}

// InstallCatalog godoc
// @Summary      Install a catalog skill onto sandboxes
// @Description  Runs the existing snapshot install onto each named sandbox config.
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        id       path  string                 true  "Catalog skill ID"  extensions(x-agent=catalog_skill_id)
// @Param        request  body  catalogInstallRequest  true  "Target sandboxes"
// @Success      202  {object}  handler.catalogInstallResponse
// @Security     Bearer
// @Security     ApiKeyAuth
// @x-agent      {"id":"skill.install_catalog","risk":"write","role":"admin","target":"skill.get_catalog","label":"name","always_confirm":true,"root":""}
// @x-agent-hint "Installs a workspace skill from skill.list_catalog onto sandboxes, reusing its stored bundle. Like skill.install it runs as root in each image and always asks the user, and each install takes minutes; check them with skill.list_installed."
// @Router       /skills/catalog/{id}/install [post]
func (h *SkillHandler) InstallCatalog(c *gin.Context) {
	if h.catalog == nil {
		_ = c.Error(apperrors.NewInternalServerError("skill catalog is not configured"))
		return
	}
	limitJSONBody(c, skillSourceJSONMaxBytes)
	var req catalogInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if isRequestBodyTooLarge(err) {
			_ = c.Error(skillJSONRequestTooLargeError())
			return
		}
		_ = c.Error(apperrors.NewBadRequestError("invalid install request"))
		return
	}
	result, err := h.catalog.InstallCatalogToConfigs(
		c.Request.Context(), sandboxConfigTenantID(c), c.Param("id"), req.SandboxConfigIDs,
	)
	if err != nil {
		respondSkillServiceError(c, err)
		return
	}
	if result == nil {
		result = &service.CatalogInstallResult{}
	}
	data := catalogInstallData{Installs: result.Installs, Errors: result.Errors, Started: []string{}}
	data.Started = append(data.Started, slices.Sorted(maps.Keys(result.Installs))...)
	for _, configID := range slices.Sorted(maps.Keys(result.Errors)) {
		data.Failed = append(data.Failed, configID+": "+result.Errors[configID])
	}
	c.JSON(http.StatusAccepted, catalogInstallResponse{Success: len(result.Errors) == 0, Data: data})
}

// DeleteCatalog godoc
// @Summary      Delete a catalog skill
// @Description  Refused while any sandbox still has an installation of this skill.
// @Tags         Skills
// @Param        id   path  string  true  "Catalog skill ID"
// @Success      200  {object}  handler.SuccessResponse
// @Router       /skills/catalog/{id} [delete]
func (h *SkillHandler) DeleteCatalog(c *gin.Context) {
	if h.catalog == nil {
		_ = c.Error(apperrors.NewInternalServerError("skill catalog is not configured"))
		return
	}
	if err := h.catalog.DeleteCatalog(
		c.Request.Context(), sandboxConfigTenantID(c), c.Param("id"),
	); err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// SetCatalogVisibility godoc
// @Summary      Set the visibility of a catalog skill
// @Description  private (creator plus workspace Owner/Admin) / workspace (all members) / public (discoverable and installable from every workspace). Only the owning workspace may change it.
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        id   path  string  true  "Catalog skill ID"
// @Success      200  {object}  handler.DataResponse{data=object}
// @Router       /skills/catalog/{id}/visibility [put]
func (h *SkillHandler) SetCatalogVisibility(c *gin.Context) {
	if h.catalog == nil {
		_ = c.Error(apperrors.NewInternalServerError("skill catalog is not configured"))
		return
	}
	var req struct {
		Visibility string `json:"visibility"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperrors.NewBadRequestError("Invalid request data").WithDetails(err.Error()))
		return
	}
	// sandboxConfigTenantID is the caller's own workspace; a guard must not be
	// allowed to rewrite it, since visibility is decided by the owner.
	catalog, err := h.catalog.SetCatalogVisibility(
		c.Request.Context(), sandboxConfigTenantID(c), c.Param("id"), req.Visibility,
	)
	if err != nil {
		respondSkillServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":         catalog.ID,
			"visibility": types.NormalizeSkillVisibility(catalog.Visibility),
		},
	})
}

// ListCatalogFiles godoc
// @Summary      List files of a catalog skill
// @Description  Lists the stored catalog bundle. Files belong to the skill definition, not a sandbox install.
// @Tags         Skills
// @Produce      json
// @Param        id   path  string  true  "Catalog skill ID"
// @Success      200  {object}  handler.DataResponse{data=[]service.SkillFileEntry}
// @Router       /skills/catalog/{id}/files [get]
func (h *SkillHandler) ListCatalogFiles(c *gin.Context) {
	if h.catalog == nil {
		_ = c.Error(apperrors.NewInternalServerError("skill catalog is not configured"))
		return
	}
	files, err := h.catalog.ListCatalogFiles(
		c.Request.Context(), sandboxConfigTenantID(c), c.Param("id"),
	)
	if err != nil {
		respondSkillServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": files})
}

// GetCatalogFile godoc
// @Summary      Read one file of a catalog skill
// @Tags         Skills
// @Produce      json
// @Param        id    path   string  true  "Catalog skill ID"
// @Param        path  query  string  true  "Skill-root-relative file path"
// @Success      200   {object}  handler.DataResponse{data=service.SkillFileContent}
// @Router       /skills/catalog/{id}/files/content [get]
func (h *SkillHandler) GetCatalogFile(c *gin.Context) {
	if h.catalog == nil {
		_ = c.Error(apperrors.NewInternalServerError("skill catalog is not configured"))
		return
	}
	file, err := h.catalog.ReadCatalogFile(
		c.Request.Context(), sandboxConfigTenantID(c), c.Param("id"), c.Query("path"),
	)
	if err != nil {
		respondSkillServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": file})
}

// catalogDescriptionRequest is the body of PUT /skills/catalog/{id}/description.
type catalogDescriptionRequest struct {
	// Lang selects what is being written: "" or "source" for the canonical
	// description parsed out of SKILL.md, "zh"/"en" for a console translation.
	Lang string `json:"lang"`
	// Description is the new text. Empty clears a translation variant, and
	// clears the canonical description when lang targets the source.
	Description string `json:"description"`
}

// PutCatalogDescription godoc
// @Summary      Edit the description of a workspace skill
// @Description  Rewrites the console description. lang "" or "source" writes the canonical SKILL.md text; "zh"/"en" write a console-only translation. The stored bundle is untouched, so installed sandboxes keep the SKILL.md they were built from.
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        id       path  string                     true  "Catalog skill ID"
// @Param        request  body  catalogDescriptionRequest  true  "New description"
// @Success      200  {object}  handler.DataResponse{data=service.SkillCatalogDescriptionView}
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills/catalog/{id}/description [put]
func (h *SkillHandler) PutCatalogDescription(c *gin.Context) {
	if h.catalog == nil {
		_ = c.Error(apperrors.NewInternalServerError("skill catalog is not configured"))
		return
	}
	limitJSONBody(c, skillSourceJSONMaxBytes)
	var req catalogDescriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if isRequestBodyTooLarge(err) {
			_ = c.Error(skillJSONRequestTooLargeError())
			return
		}
		_ = c.Error(apperrors.NewBadRequestError("invalid description request"))
		return
	}
	view, err := h.catalog.EditCatalogDescription(
		c.Request.Context(), sandboxConfigTenantID(c), c.Param("id"), req.Lang, req.Description,
	)
	if err != nil {
		respondSkillServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": view})
}

// catalogTranslateRequest is the body of POST /skills/catalog/{id}/translate.
type catalogTranslateRequest struct {
	// Lang is the language to translate into: "zh" or "en".
	Lang string `json:"lang"`
	// Text is what to translate. Empty means the definition's own description,
	// which is what the card's one-click action sends.
	Text string `json:"text"`
	// Save stores the result as Lang's variant; false only returns the text, so
	// the editor can preview a translation before committing it.
	Save bool `json:"save"`
}

// TranslateCatalogDescription godoc
// @Summary      Translate a workspace skill description
// @Description  Machine-translates a description with the workspace's chat model. With save=true the result becomes that language's console description; the run is refused, not silently skipped, when no chat model is configured.
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        id       path  string                   true  "Catalog skill ID"
// @Param        request  body  catalogTranslateRequest  true  "Target language"
// @Success      200  {object}  handler.DataResponse{data=object}
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills/catalog/{id}/translate [post]
func (h *SkillHandler) TranslateCatalogDescription(c *gin.Context) {
	if h.catalog == nil {
		_ = c.Error(apperrors.NewInternalServerError("skill catalog is not configured"))
		return
	}
	limitJSONBody(c, skillSourceJSONMaxBytes)
	var req catalogTranslateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if isRequestBodyTooLarge(err) {
			_ = c.Error(skillJSONRequestTooLargeError())
			return
		}
		_ = c.Error(apperrors.NewBadRequestError("invalid translation request"))
		return
	}
	translated, view, err := h.catalog.TranslateCatalogDescription(
		c.Request.Context(), sandboxConfigTenantID(c), c.Param("id"), req.Lang, req.Text, req.Save,
	)
	if err != nil {
		respondSkillServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"lang":        req.Lang,
		"description": translated,
		"saved":       req.Save,
		"catalog":     view,
	}})
}

func catalogIDResponse(cat *types.TenantSkillCatalogEntity) gin.H {
	if cat == nil {
		return gin.H{}
	}
	return gin.H{
		"id":          cat.ID,
		"name":        cat.Name,
		"version":     cat.Version,
		"description": cat.Description,
	}
}
