package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/mcp"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// mcpServiceService implements MCPServiceService interface
type mcpServiceService struct {
	mcpServiceRepo interfaces.MCPServiceRepository
	mcpManager     *mcp.MCPManager
	oauthRepo      interfaces.MCPOAuthRepository
}

// NewMCPServiceService creates a new MCP service service
func NewMCPServiceService(
	mcpServiceRepo interfaces.MCPServiceRepository,
	mcpManager *mcp.MCPManager,
	oauthRepo interfaces.MCPOAuthRepository,
) interfaces.MCPServiceService {
	return &mcpServiceService{
		mcpServiceRepo: mcpServiceRepo,
		mcpManager:     mcpManager,
		oauthRepo:      oauthRepo,
	}
}

// CreateMCPService creates a new MCP service.
//
// The request body is bound straight onto this entity by the handler, so every
// field a client must not choose is cleared here rather than trusted: a caller
// could otherwise create a row with is_builtin=true (un-editable, un-deletable
// and visible everywhere), hand-pick its id, or publish it to the whole
// deployment in one POST.
func (s *mcpServiceService) CreateMCPService(ctx context.Context, service *types.MCPService) error {
	// Stdio transport is disabled for security reasons
	if service.TransportType == types.MCPTransportStdio {
		return fmt.Errorf("stdio transport is disabled for security reasons; please use SSE or HTTP Streamable transport instead")
	}
	if err := mcp.ValidateServiceOutboundURLs(service); err != nil {
		return err
	}

	service.ID = ""
	service.IsBuiltin = false
	service.CreatedBy, _ = types.UserIDFromContext(ctx)
	// New services start at the workspace level. Sharing is a separate,
	// audited action (SetMCPVisibility) rather than a create-time flag, which
	// matches how the knowledge base and mini tool menus behave.
	service.Visibility = types.MCPVisibilityWorkspace

	// Set default advanced config if not provided
	if service.AdvancedConfig == nil {
		service.AdvancedConfig = types.GetDefaultAdvancedConfig()
	}

	// Set timestamps
	service.CreatedAt = time.Now()
	service.UpdatedAt = time.Now()

	if err := s.mcpServiceRepo.Create(ctx, service); err != nil {
		logger.GetLogger(ctx).Errorf("Failed to create MCP service: %v", err)
		return fmt.Errorf("failed to create MCP service: %w", err)
	}

	// The handler serialises this same entity, so the permission flags have to
	// be filled here or the freshly created card would come back with every
	// affordance hidden.
	userID, _ := types.UserIDFromContext(ctx)
	decorateMCPService(service, service.TenantID, userID, types.TenantRoleFromContext(ctx))
	return nil
}

// requireOwnedMCP gates every write path on ownership.
//
// GetByID now also resolves public rows from other workspaces, so "found" no
// longer implies "mine". Without this check a reader in workspace B could
// rename a service published by workspace A — the entity the update carries
// still points at the owning tenant, so the repository's WHERE clause matched
// the owner's row and the write went through. Delete returned 200 with no
// effect for the opposite reason: its predicate matched nothing and GORM does
// not treat zero rows as an error.
func requireOwnedMCP(existing *types.MCPService, tenantID uint64) error {
	if existing == nil {
		return apperrors.NewNotFoundError("MCP service not found")
	}
	if tenantID == 0 || existing.TenantID != tenantID {
		return apperrors.NewForbiddenError("this MCP service belongs to another workspace")
	}
	return nil
}

// canManageMCPService reports whether the caller may edit or delete a service.
//
// tenantID is the caller's workspace and is checked first. A public row is
// reachable from anywhere, so without this a reader who happens to be Owner of
// their own workspace would pass the role test below and be handed write access
// to somebody else's service. Builtin rows are never manageable: they ship with
// the product and the next release would overwrite the edit.
func canManageMCPService(svc *types.MCPService, tenantID uint64, userID string, role types.TenantRole) bool {
	if svc == nil || svc.IsBuiltin {
		return false
	}
	if tenantID == 0 || svc.TenantID != tenantID {
		return false
	}
	if role == types.TenantRoleOwner || role == types.TenantRoleAdmin {
		return true
	}
	return userID != "" && svc.CreatedBy != "" && svc.CreatedBy == userID
}

// canShareMCPService reports whether the caller may change a service's
// visibility. Looser than canManageMCPService only in that it does not require
// the caller to be the creator: sharing is a workspace-level decision, so any
// Owner/Admin of the owning workspace may take it.
func canShareMCPService(svc *types.MCPService, tenantID uint64, userID string, role types.TenantRole) bool {
	if svc == nil || svc.IsBuiltin {
		return false
	}
	if tenantID == 0 || svc.TenantID != tenantID {
		return false
	}
	if role == types.TenantRoleOwner || role == types.TenantRoleAdmin {
		return true
	}
	return userID != "" && svc.CreatedBy != "" && svc.CreatedBy == userID
}

// decorateMCPService fills the per-request permission flags the UI renders.
//
// Pure decoration: it never touches the credential fields, so it is safe to run
// on the runtime lookup path as well as on the settings page.
func decorateMCPService(svc *types.MCPService, tenantID uint64, userID string, role types.TenantRole) {
	if svc == nil {
		return
	}
	svc.IsForeign = tenantID != 0 && svc.TenantID != tenantID
	svc.CanEdit = canManageMCPService(svc, tenantID, userID, role)
	svc.CanDelete = svc.CanEdit
	svc.CanShare = canShareMCPService(svc, tenantID, userID, role)
	if svc.IsForeign {
		// A published service is usable and readable from here, never writable:
		// its credentials belong to the workspace that published it.
		svc.CanEdit = false
		svc.CanDelete = false
		svc.CanShare = false
	}
}

func decorateMCPServices(services []*types.MCPService, ctx context.Context, tenantID uint64) {
	userID, _ := types.UserIDFromContext(ctx)
	role := types.TenantRoleFromContext(ctx)
	for _, svc := range services {
		decorateMCPService(svc, tenantID, userID, role)
	}
}

// SetMCPVisibility publishes, restricts or un-publishes a service.
//
// Deliberately narrower than UpdateMCPService: it writes one column, so it
// cannot accidentally rewrite credentials that changed concurrently, and it
// stays valid for rows whose connection settings the caller does not have to
// re-submit.
func (s *mcpServiceService) SetMCPVisibility(
	ctx context.Context, callerTenantID uint64, id string, visibility string,
) (*types.MCPService, error) {
	switch visibility {
	case types.MCPVisibilityPrivate, types.MCPVisibilityWorkspace, types.MCPVisibilityPublic:
	default:
		return nil, apperrors.NewValidationError("visibility must be private, workspace or public")
	}

	existing, err := s.mcpServiceRepo.GetByID(ctx, callerTenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP service: %w", err)
	}
	if existing == nil {
		return nil, apperrors.NewNotFoundError("MCP service not found")
	}
	if existing.IsBuiltin {
		return nil, apperrors.NewValidationError(
			"builtin MCP services are already visible to every workspace")
	}
	if callerTenantID == 0 || existing.TenantID != callerTenantID {
		return nil, apperrors.NewForbiddenError("you cannot change the visibility of this MCP service")
	}
	userID, _ := types.UserIDFromContext(ctx)
	role := types.TenantRoleFromContext(ctx)
	if !canShareMCPService(existing, callerTenantID, userID, role) {
		return nil, apperrors.NewForbiddenError("you cannot change the visibility of this MCP service")
	}

	if err := s.mcpServiceRepo.SetVisibility(ctx, callerTenantID, id, visibility); err != nil {
		return nil, fmt.Errorf("failed to set MCP visibility: %w", err)
	}
	existing.Visibility = visibility
	decorateMCPService(existing, callerTenantID, userID, role)
	return existing, nil
}

// GetMCPServiceByID retrieves an MCP service by ID.
//
// Returns the raw stored entity including any AuthConfig credentials in plain
// form. Callers MUST convert to dto.MCPServiceResponse (which strips secret
// fields by construction) before serializing to a response body. Internal
// callers (e.g. MCP client construction, credential metadata derivation) need
// the unredacted form to function correctly.
func (s *mcpServiceService) GetMCPServiceByID(
	ctx context.Context,
	tenantID uint64,
	id string,
) (*types.MCPService, error) {
	service, err := s.mcpServiceRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		logger.GetLogger(ctx).Errorf("Failed to get MCP service: %v", err)
		return nil, fmt.Errorf("failed to get MCP service: %w", err)
	}

	if service == nil {
		return nil, fmt.Errorf("MCP service not found")
	}
	userID, _ := types.UserIDFromContext(ctx)
	decorateMCPService(service, tenantID, userID, types.TenantRoleFromContext(ctx))
	return service, nil
}

// ListMCPServices lists all MCP services for a tenant.
//
// Same contract as GetMCPServiceByID — returns raw entities; handlers MUST
// convert to dto.MCPServiceResponse before responding.
func (s *mcpServiceService) ListMCPServices(ctx context.Context, tenantID uint64) ([]*types.MCPService, error) {
	services, err := s.mcpServiceRepo.List(ctx, tenantID)
	if err != nil {
		logger.GetLogger(ctx).Errorf("Failed to list MCP services: %v", err)
		return nil, fmt.Errorf("failed to list MCP services: %w", err)
	}
	decorateMCPServices(services, ctx, tenantID)
	return services, nil
}

// ListMCPServicesByIDs retrieves multiple MCP services by IDs
func (s *mcpServiceService) ListMCPServicesByIDs(
	ctx context.Context,
	tenantID uint64,
	ids []string,
) ([]*types.MCPService, error) {
	if len(ids) == 0 {
		return []*types.MCPService{}, nil
	}

	services, err := s.mcpServiceRepo.ListByIDs(ctx, tenantID, ids)
	if err != nil {
		logger.GetLogger(ctx).Errorf("Failed to list MCP services by IDs: %v", err)
		return nil, fmt.Errorf("failed to list MCP services by IDs: %w", err)
	}

	return services, nil
}

// UpdateMCPService updates an MCP service
func (s *mcpServiceService) UpdateMCPService(
	ctx context.Context,
	service *types.MCPService,
	updateFields map[string]bool,
) error {
	// Check if service exists
	existing, err := s.mcpServiceRepo.GetByID(ctx, service.TenantID, service.ID)
	if err != nil {
		return fmt.Errorf("failed to get MCP service: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("MCP service not found")
	}

	// This path has no explicit tenantID argument: the handler stamps the
	// caller's workspace onto the entity, so that is what ownership is checked
	// against.
	if err := requireOwnedMCP(existing, service.TenantID); err != nil {
		return err
	}

	// Builtin MCP services cannot be updated
	if existing.IsBuiltin {
		return fmt.Errorf("builtin MCP services cannot be updated")
	}

	// Determine the final transport type after merge
	finalTransportType := existing.TransportType
	if service.TransportType != "" {
		finalTransportType = service.TransportType
	}

	// Stdio transport is disabled for security reasons
	if finalTransportType == types.MCPTransportStdio {
		return fmt.Errorf("stdio transport is disabled for security reasons; please use SSE or HTTP Streamable transport instead")
	}

	if updateFields["usage_instructions"] {
		existing.UsageInstructions = service.UsageInstructions
	}

	// Store old enabled state BEFORE any updates
	oldEnabled := existing.Enabled

	// Snapshot pre-merge values of fields that drive configChanged. We need
	// this because the in-place merge below reassigns pointer fields such as
	// existing.URL = service.URL, after which any post-merge comparison
	// between service.URL and existing.URL would trivially match.
	//
	// Only the non-secret AuthConfig fields below are snapshotted — credential
	// changes flow through the dedicated /credentials subresource which
	// handles its own CloseClient call. Main PUT does not accept secret
	// fields (see handler comment on UpdateMCPService).
	preURL := ""
	preURLSet := existing.URL != nil
	if preURLSet {
		preURL = *existing.URL
	}
	var preStdioCommand string
	var preStdioArgs []string
	preStdioSet := existing.StdioConfig != nil
	if preStdioSet {
		preStdioCommand = existing.StdioConfig.Command
		preStdioArgs = append([]string(nil), existing.StdioConfig.Args...)
	}
	preTransportType := existing.TransportType
	preHeaders := map[string]string{}
	if existing.AuthConfig != nil && existing.AuthConfig.CustomHeaders != nil {
		maps.Copy(preHeaders, existing.AuthConfig.CustomHeaders)
	}

	preAuthType := types.MCPAuthNone
	preAPIKeyHeader := ""
	preClientID := ""
	if existing.AuthConfig != nil {
		preAuthType = existing.AuthConfig.AuthType
		preAPIKeyHeader = existing.AuthConfig.APIKeyHeader
		preClientID = existing.AuthConfig.ClientID
	}

	// CustomHeaders flows through main PUT (it's structural, not a secret) —
	// nil preserves, non-nil replaces. Other AuthConfig fields (APIKey/Token)
	// are never accepted via main PUT; the handler strips them up front.
	//
	// auth_type / scopes / auth_server_metadata_url are non-secret OAuth
	// configuration and also flow through here.
	if service.AuthConfig != nil {
		if existing.AuthConfig == nil {
			existing.AuthConfig = &types.MCPAuthConfig{}
		}
		if service.AuthConfig.CustomHeaders != nil {
			existing.AuthConfig.CustomHeaders = service.AuthConfig.CustomHeaders
		}
		// Only overwrite OAuth config when explicitly provided, so a partial
		// PUT that carries only custom_headers does not wipe an existing
		// auth_type / scopes. (Empty/absent is treated as "no change".)
		if updateFields["auth_type"] || service.AuthConfig.AuthType != types.MCPAuthNone {
			existing.AuthConfig.AuthType = service.AuthConfig.AuthType
		}
		// APIKeyHeader is non-secret; empty means "use default X-API-Key", so a
		// partial PUT that omits it is treated as no-change (mirrors scopes).
		if updateFields["api_key_header"] || service.AuthConfig.APIKeyHeader != "" {
			existing.AuthConfig.APIKeyHeader = service.AuthConfig.APIKeyHeader
		}
		if service.AuthConfig.Scopes != nil {
			existing.AuthConfig.Scopes = service.AuthConfig.Scopes
		}
		if service.AuthConfig.AuthServerMetadataURL != "" {
			existing.AuthConfig.AuthServerMetadataURL = service.AuthConfig.AuthServerMetadataURL
		}
		// Empty means dynamic client registration, so only an explicitly
		// sent client_id may clear it.
		if updateFields["client_id"] {
			existing.AuthConfig.ClientID = service.AuthConfig.ClientID
		}
	}

	// Scalar zero values cannot distinguish omission from an explicit update,
	// so use the handler's field-presence map for name, description, and enabled.
	if updateFields["name"] {
		existing.Name = service.Name
	}
	if updateFields["description"] {
		existing.Description = service.Description
	}
	if updateFields["enabled"] {
		existing.Enabled = service.Enabled
	}
	if service.TransportType != "" {
		existing.TransportType = service.TransportType
	}
	if service.URL != nil {
		existing.URL = service.URL
	}
	if service.StdioConfig != nil {
		existing.StdioConfig = service.StdioConfig
	}
	if service.EnvVars != nil {
		existing.EnvVars = service.EnvVars
	}
	if service.Headers != nil {
		existing.Headers = service.Headers
	}
	if service.AdvancedConfig != nil {
		existing.AdvancedConfig = service.AdvancedConfig
	}
	if updateFields[types.MCPUpdateDropSecrets] {
		// Empty rather than nil: the repository skips nil maps.
		existing.Headers = types.MCPHeaders{}
		if existing.AuthConfig != nil {
			existing.AuthConfig.APIKey = ""
			existing.AuthConfig.Token = ""
			existing.AuthConfig.CustomHeaders = nil
		}
	}

	// Update timestamp
	if err := mcp.ValidateServiceOutboundURLs(existing); err != nil {
		return err
	}
	existing.UpdatedAt = time.Now()

	if err := s.mcpServiceRepo.Update(ctx, existing); err != nil {
		logger.GetLogger(ctx).Errorf("Failed to update MCP service: %v", err)
		return fmt.Errorf("failed to update MCP service: %w", err)
	}

	// Check if critical configuration changed (URL / StdioConfig / transport
	// type / custom headers). Comparisons MUST be against the pre-merge
	// snapshots captured above — after the in-place merge, service.URL and
	// existing.URL point to the same memory, making any post-merge compare
	// vacuously equal.
	//
	// AuthConfig API key / token changes do NOT go through this path; they
	// are handled by the /credentials subresource which triggers CloseClient
	// inline.
	configChanged := false
	currURLSet := existing.URL != nil
	switch {
	case currURLSet != preURLSet:
		configChanged = true
	case currURLSet && *existing.URL != preURL:
		configChanged = true
	}
	currStdioSet := existing.StdioConfig != nil
	switch {
	case currStdioSet != preStdioSet:
		configChanged = true
	case currStdioSet && (existing.StdioConfig.Command != preStdioCommand ||
		!slices.Equal(existing.StdioConfig.Args, preStdioArgs)):
		configChanged = true
	}
	if existing.TransportType != preTransportType {
		configChanged = true
	}
	currHeaders := map[string]string{}
	if existing.AuthConfig != nil && existing.AuthConfig.CustomHeaders != nil {
		currHeaders = existing.AuthConfig.CustomHeaders
	}
	if !maps.Equal(currHeaders, preHeaders) {
		configChanged = true
	}
	// Connections built with a pre-registered OAuth client hold its ID, so
	// they are rebuilt when it changes.
	if existing.AuthConfig != nil &&
		(existing.AuthConfig.AuthType != preAuthType ||
			existing.AuthConfig.APIKeyHeader != preAPIKeyHeader ||
			existing.AuthConfig.ClientID != preClientID) {
		configChanged = true
	}
	name := secutils.SanitizeForLog(existing.Name)
	// Close existing client connection if:
	// 1. Service is now disabled (need to close connection)
	// 2. Critical configuration changed (need to reconnect with new config)
	if !existing.Enabled {
		s.mcpManager.CloseClient(service.ID)
		logger.GetLogger(ctx).Infof("MCP service disabled, connection closed: %s (ID: %s)", name, service.ID)
	} else if configChanged {
		s.mcpManager.CloseClient(service.ID)
		logger.GetLogger(ctx).Infof("MCP service config changed, connection closed: %s (ID: %s)", name, service.ID)
	} else if oldEnabled != existing.Enabled && existing.Enabled {
		// Service was just enabled (was disabled, now enabled)
		// Close any existing connection to ensure clean state
		s.mcpManager.CloseClient(service.ID)
		logger.GetLogger(ctx).Infof("MCP service enabled, existing connection closed: %s (ID: %s)", name, service.ID)
	}

	logger.GetLogger(ctx).Infof("MCP service updated: %s (ID: %s), enabled: %v", name, service.ID, existing.Enabled)
	return nil
}

// DeleteMCPService deletes an MCP service
func (s *mcpServiceService) DeleteMCPService(ctx context.Context, tenantID uint64, id string) error {
	// Check if service exists
	existing, err := s.mcpServiceRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("failed to get MCP service: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("MCP service not found")
	}

	if err := requireOwnedMCP(existing, tenantID); err != nil {
		return err
	}

	// Builtin MCP services cannot be deleted
	if existing.IsBuiltin {
		return fmt.Errorf("builtin MCP services cannot be deleted")
	}

	// Close client connection
	s.mcpManager.CloseClient(id)

	if err := s.mcpServiceRepo.Delete(ctx, tenantID, id); err != nil {
		logger.GetLogger(ctx).Errorf("Failed to delete MCP service: %v", err)
		return fmt.Errorf("failed to delete MCP service: %w", err)
	}

	logger.GetLogger(ctx).Infof("MCP service deleted: %s (ID: %s)", secutils.SanitizeForLog(existing.Name), id)
	return nil
}

// TestMCPService tests the connection to an MCP service and returns available tools/resources
// mcpTestFailure builds a failed MCPTestResult, upgrading a generic connection
// error into an explicit "OAuth required" signal when the server answered with
// an RFC 9728 OAuth challenge. This lets the UI guide the user to switch the
// auth strategy to OAuth instead of showing a bare 401.
func mcpTestFailure(err error, prefix string) *types.MCPTestResult {
	var oerr *mcp.OAuthRequiredError
	if errors.As(err, &oerr) {
		return &types.MCPTestResult{
			Success:       false,
			OAuthRequired: true,
			Message: "This MCP server requires OAuth authorization. " +
				"Switch the auth method to OAuth 2.0 and authorize.",
		}
	}
	return &types.MCPTestResult{
		Success: false,
		Message: fmt.Sprintf("%s: %v", prefix, err),
	}
}

func (s *mcpServiceService) TestMCPService(
	ctx context.Context,
	tenantID uint64,
	id string,
) (*types.MCPTestResult, error) {
	// Get service
	service, err := s.mcpServiceRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP service: %w", err)
	}
	if service == nil {
		return nil, fmt.Errorf("MCP service not found")
	}

	// Create temporary client for testing. For OAuth services, wire the
	// per-user token store so the test connects with the current user's
	// authorization (and surfaces an authorization-required message when the
	// user has not authorized yet).
	config := &mcp.ClientConfig{
		Service: service,
	}
	if service.AuthConfig.IsOAuth() {
		config.OAuthRepo = s.oauthRepo
		config.TenantID, _ = types.TenantIDFromContext(ctx)
		config.Principal, _ = types.PrincipalFromContext(ctx)
	}

	client, err := mcp.NewMCPClient(config)
	if err != nil {
		return &types.MCPTestResult{
			Success: false,
			Message: fmt.Sprintf("Failed to create client: %v", err),
		}, nil
	}

	// Connect
	testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := client.Connect(testCtx); err != nil {
		return mcpTestFailure(err, "Connection failed"), nil
	}
	defer client.Disconnect()

	// Initialize
	initResult, err := client.Initialize(testCtx)
	if err != nil {
		return mcpTestFailure(err, "Initialization failed"), nil
	}

	tools, err := client.ListTools(testCtx)
	if err != nil {
		if initResult.Capabilities.Tools != nil {
			return mcpTestFailure(err, "Failed to list tools"), nil
		}
		logger.GetLogger(ctx).Warnf("Failed to list tools: %v", err)
		tools = []*types.MCPTool{}
	} else if s.mcpManager != nil {
		s.mcpManager.InvalidateToolSchemas(service.ID, client)
	}

	resources, err := client.ListResources(testCtx)
	if err != nil {
		if initResult.Capabilities.Resources != nil {
			return mcpTestFailure(err, "Failed to list resources"), nil
		}
		logger.GetLogger(ctx).Warnf("Failed to list resources: %v", err)
		resources = []*types.MCPResource{}
	}

	return &types.MCPTestResult{
		Success: true,
		Message: fmt.Sprintf(
			"Connected successfully to %s v%s",
			initResult.ServerInfo.Name,
			initResult.ServerInfo.Version,
		),
		Description: initResult.ServerInfo.Description,
		Tools:       tools,
		Resources:   resources,
	}, nil
}

// GetMCPServiceTools retrieves the list of tools from an MCP service
func (s *mcpServiceService) GetMCPServiceTools(
	ctx context.Context,
	tenantID uint64,
	id string,
) ([]*types.MCPTool, error) {
	// Get service
	service, err := s.mcpServiceRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP service: %w", err)
	}
	if service == nil {
		return nil, fmt.Errorf("MCP service not found")
	}

	// Get or create client
	client, err := s.mcpManager.GetOrCreateClient(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP client: %w", err)
	}

	// List tools
	tools, err := client.ListTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	return tools, nil
}

// UpdateMCPCredentials writes one or more credential fields and recycles any
// active client connection so the next upstream call uses the new credential.
//
// Implementation notes:
//   - apiKey == nil and token == nil → no-op, returns current state.
//   - apiKey == &"" → explicit empty string; treated as no-op because clearing
//     is the dedicated ClearMCPCredential path. The handler enforces this
//     contract by accepting empty as no-op too; this is defense-in-depth.
//   - apiKey == &"sk-..." → replaces stored value.
//   - Builtin services cannot have credentials updated (mirrors the
//     UpdateMCPService restriction).
//   - Always re-fetches existing AuthConfig before merge to avoid clobbering
//     CustomHeaders or the unrelated credential field.
func (s *mcpServiceService) UpdateMCPCredentials(
	ctx context.Context, tenantID uint64, id string, apiKey, token, clientSecret *string,
) (*types.MCPService, error) {
	existing, err := s.mcpServiceRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP service: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("MCP service not found")
	}
	if err := requireOwnedMCP(existing, tenantID); err != nil {
		return nil, err
	}
	if existing.IsBuiltin {
		return nil, fmt.Errorf("builtin MCP services cannot have credentials modified")
	}

	if existing.AuthConfig == nil {
		existing.AuthConfig = &types.MCPAuthConfig{}
	}
	changed := false
	if apiKey != nil && *apiKey != "" && *apiKey != existing.AuthConfig.APIKey {
		existing.AuthConfig.APIKey = *apiKey
		changed = true
	}
	if token != nil && *token != "" && *token != existing.AuthConfig.Token {
		existing.AuthConfig.Token = *token
		changed = true
	}
	if clientSecret != nil && *clientSecret != "" && *clientSecret != existing.AuthConfig.ClientSecret {
		existing.AuthConfig.ClientSecret = *clientSecret
		changed = true
	}
	if !changed {
		return existing, nil
	}

	existing.UpdatedAt = time.Now()
	if err := s.mcpServiceRepo.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("failed to update MCP service: %w", err)
	}

	// Credential changed → recycle client so the next call reconnects.
	s.mcpManager.CloseClient(id)
	logger.GetLogger(ctx).Infof(
		"MCP credentials updated, connection closed: %s (ID: %s)",
		secutils.SanitizeForLog(existing.Name), id,
	)
	return existing, nil
}

// ClearMCPCredential removes a single credential field. Idempotent: clearing
// an already-empty field returns nil without writing or reconnecting.
func (s *mcpServiceService) ClearMCPCredential(
	ctx context.Context, tenantID uint64, id, field string,
) error {
	existing, err := s.mcpServiceRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("failed to get MCP service: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("MCP service not found")
	}
	if err := requireOwnedMCP(existing, tenantID); err != nil {
		return err
	}
	if existing.IsBuiltin {
		return fmt.Errorf("builtin MCP services cannot have credentials modified")
	}
	if existing.AuthConfig == nil {
		return nil // nothing to clear
	}

	changed := false
	switch field {
	case "api_key":
		if existing.AuthConfig.APIKey != "" {
			existing.AuthConfig.APIKey = ""
			changed = true
		}
	case "token":
		if existing.AuthConfig.Token != "" {
			existing.AuthConfig.Token = ""
			changed = true
		}
	case "client_secret":
		if existing.AuthConfig.ClientSecret != "" {
			existing.AuthConfig.ClientSecret = ""
			changed = true
		}
	default:
		return fmt.Errorf("unknown credential field: %s", field)
	}
	if !changed {
		return nil
	}

	existing.UpdatedAt = time.Now()
	if err := s.mcpServiceRepo.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update MCP service: %w", err)
	}

	s.mcpManager.CloseClient(id)
	logger.GetLogger(ctx).Infof(
		"MCP credential cleared by user: id=%s field=%s, connection closed",
		secutils.SanitizeForLog(id), field,
	)
	return nil
}

// GetMCPServiceResources retrieves the list of resources from an MCP service
func (s *mcpServiceService) GetMCPServiceResources(
	ctx context.Context,
	tenantID uint64,
	id string,
) ([]*types.MCPResource, error) {
	// Get service
	service, err := s.mcpServiceRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP service: %w", err)
	}
	if service == nil {
		return nil, fmt.Errorf("MCP service not found")
	}

	// Get or create client
	client, err := s.mcpManager.GetOrCreateClient(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP client: %w", err)
	}

	// List resources
	resources, err := client.ListResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}

	return resources, nil
}
