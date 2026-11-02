package access

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

// The visibility column adds two rules that the organization-share branches do
// not cover, and both are denial-shaped, so they get their own matrix:
//
//   - inside the owning workspace, a private KB belongs to its creator and the
//     workspace Owner/Admin, not to every colleague;
//   - outside it, a public KB is readable by anyone but only ever as a Viewer,
//     which is what keeps publishing read-only without a second check on each
//     write route.
func TestResolveKBVisibilityMatrix(t *testing.T) {
	const (
		ownerTenant   = uint64(1)
		readerTenant  = uint64(2)
		kbID          = "kb-1"
		creatorUserID = "creator"
	)

	baseKB := func(visibility string) *types.KnowledgeBase {
		return &types.KnowledgeBase{
			ID:         kbID,
			TenantID:   ownerTenant,
			CreatorID:  creatorUserID,
			Visibility: visibility,
		}
	}
	caller := func(tenant uint64, userID string, role types.TenantRole) types.Caller {
		return types.Caller{TenantID: tenant, UserID: userID, Role: role}
	}

	for _, tt := range []struct {
		name     string
		kb       *types.KnowledgeBase
		caller   types.Caller
		required types.OrgMemberRole
		want     types.OrgMemberRole
		wantErr  bool
	}{
		{
			name:     "public KB is readable from another workspace",
			kb:       baseKB(types.KnowledgeBaseVisibilityPublic),
			caller:   caller(readerTenant, "reader", types.TenantRoleContributor),
			required: types.OrgRoleViewer,
			want:     types.OrgRoleViewer,
		},
		{
			name: "public KB still refuses a write route",
			kb:   baseKB(types.KnowledgeBaseVisibilityPublic),
			// Even the reader's own Editor role does not promote them: the cap
			// comes from the KB's visibility, not from the caller's seniority.
			caller:   caller(readerTenant, "reader", types.TenantRoleOwner),
			required: types.OrgRoleEditor,
			wantErr:  true,
		},
		{
			name:     "private KB stays invisible to another workspace",
			kb:       baseKB(types.KnowledgeBaseVisibilityPrivate),
			caller:   caller(readerTenant, "reader", types.TenantRoleOwner),
			required: types.OrgRoleViewer,
			wantErr:  true,
		},
		{
			name:     "workspace KB is not readable across workspaces",
			kb:       baseKB(types.KnowledgeBaseVisibilityWorkspace),
			caller:   caller(readerTenant, "reader", types.TenantRoleOwner),
			required: types.OrgRoleViewer,
			wantErr:  true,
		},
		{
			name:     "creator reads their own private KB",
			kb:       baseKB(types.KnowledgeBaseVisibilityPrivate),
			caller:   caller(ownerTenant, creatorUserID, types.TenantRoleContributor),
			required: types.OrgRoleAdmin,
			want:     types.OrgRoleAdmin,
		},
		{
			name: "workspace admin reads a colleague's private KB",
			kb:   baseKB(types.KnowledgeBaseVisibilityPrivate),
			// Admin needs to reach it to reassign or clean up after someone
			// leaves; "only me" is not "only me, forever, unadministrable".
			caller:   caller(ownerTenant, "colleague", types.TenantRoleAdmin),
			required: types.OrgRoleViewer,
			want:     types.OrgRoleAdmin,
		},
		{
			name:     "colleague cannot read a private KB",
			kb:       baseKB(types.KnowledgeBaseVisibilityPrivate),
			caller:   caller(ownerTenant, "colleague", types.TenantRoleContributor),
			required: types.OrgRoleViewer,
			wantErr:  true,
		},
		{
			name: "legacy private KB with no creator stays workspace-readable",
			kb: &types.KnowledgeBase{
				ID: kbID, TenantID: ownerTenant, Visibility: types.KnowledgeBaseVisibilityPrivate,
			},
			// Rows from the API-key era have no creator to attribute them to;
			// hiding them would remove data nobody chose to hide.
			caller:   caller(ownerTenant, "colleague", types.TenantRoleViewer),
			required: types.OrgRoleViewer,
			want:     types.OrgRoleAdmin,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			grant, err := ResolveKB(
				ctx,
				KBRequest{Caller: tt.caller},
				tt.kb,
				tt.required,
				&shareLookup{}, // no organization share in play
				&agentLookup{}, // nor a shared agent
			)
			if tt.wantErr {
				require.Error(t, err)
				require.True(t, errors.Is(err, ErrForbidden), "want ErrForbidden, got %v", err)
				require.Nil(t, grant)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, grant)
			require.Equal(t, tt.want, grant.Permission)
			// Cross-tenant reads must scope retrieval to the KB's own
			// workspace, which is where its embedding store lives.
			require.Equal(t, ownerTenant, grant.EffectiveTenantID)
		})
	}
}

// A public KB must not become a back door into the write path: the resolver
// hands out Viewer, and Viewer never satisfies an Editor requirement.
func TestPublicKnowledgeBaseNeverGrantsWrite(t *testing.T) {
	kb := &types.KnowledgeBase{
		ID:         "kb-public",
		TenantID:   1,
		CreatorID:  "someone",
		Visibility: types.KnowledgeBaseVisibilityPublic,
	}
	request := KBRequest{Caller: types.Caller{TenantID: 2, UserID: "reader", Role: types.TenantRoleOwner}}

	for _, required := range []types.OrgMemberRole{types.OrgRoleEditor, types.OrgRoleAdmin} {
		_, err := ResolveKB(context.Background(), request, kb, required, nil, nil)
		require.ErrorIs(t, err, ErrForbidden, "required=%s", required)
	}

	grant, err := ResolveKB(context.Background(), request, kb, types.OrgRoleViewer, nil, nil)
	require.NoError(t, err)
	require.Equal(t, types.OrgRoleViewer, grant.Permission)
}
