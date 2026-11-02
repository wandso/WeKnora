package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// Publishing a card widens who can READ it. It must not widen who can edit it:
// write access follows the owning workspace, and these helpers are the single
// place that decides both.
func TestMiniToolVisibilityReadRules(t *testing.T) {
	const ownerTenant, otherTenant = uint64(1), uint64(2)

	private := &types.MiniTool{
		ID: "card-1", TenantID: ownerTenant, OwnerUserID: "alice",
		Visibility: types.MiniToolVisibilityPrivate,
	}
	workspace := &types.MiniTool{
		ID: "card-2", TenantID: ownerTenant, OwnerUserID: "alice",
		Visibility: types.MiniToolVisibilityWorkspace,
	}
	public := &types.MiniTool{
		ID: "card-3", TenantID: ownerTenant, OwnerUserID: "alice",
		Visibility: types.MiniToolVisibilityPublic,
	}

	for _, tt := range []struct {
		name   string
		tool   *types.MiniTool
		tenant uint64
		user   string
		role   types.TenantRole
		want   bool
	}{
		{"owner reads private", private, ownerTenant, "alice", types.TenantRoleContributor, true},
		{"colleague cannot read private", private, ownerTenant, "bob", types.TenantRoleContributor, false},
		{"workspace admin reads private", private, ownerTenant, "bob", types.TenantRoleAdmin, true},
		{"anyone reads workspace", workspace, ownerTenant, "bob", types.TenantRoleViewer, true},
		{"anyone reads public", public, ownerTenant, "bob", types.TenantRoleViewer, true},
		{"public is readable from another workspace", public, otherTenant, "carol", types.TenantRoleOwner, true},
		{"workspace card is not readable across workspaces", workspace, otherTenant, "carol", types.TenantRoleOwner, false},
		{"private card is not readable across workspaces", private, otherTenant, "carol", types.TenantRoleOwner, false},
		// An empty user id here is an API-key caller, not an anonymous request:
		// every entry point already requires a workspace in context, and the
		// synthetic user-id path carries none. Such a caller reads public cards
		// like any other member.
		{"api-key caller reads public", public, ownerTenant, "", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := canReadMiniTool(tt.tool, tt.tenant, tt.user, tt.role); got != tt.want {
				t.Fatalf("canReadMiniTool = %v, want %v", got, tt.want)
			}
		})
	}
}

// The hole this closes: a reader who is Owner of their OWN workspace used to
// pass the role check and be granted write access to somebody else's public
// card — the repository then failed with "not found" and surfaced a 500.
func TestForeignPublicMiniToolIsNeverManageable(t *testing.T) {
	const ownerTenant, readerTenant = uint64(1), uint64(2)
	card := &types.MiniTool{
		ID: "card-3", TenantID: ownerTenant, OwnerUserID: "alice",
		Visibility: types.MiniToolVisibilityPublic,
	}

	if canManageMiniTool(card, readerTenant, "carol", types.TenantRoleOwner) {
		t.Fatal("a workspace Owner must not manage a card published by another workspace")
	}
	// Even claiming the owner's user id does not help: the tenant gate runs first.
	if canManageMiniTool(card, readerTenant, "alice", types.TenantRoleOwner) {
		t.Fatal("the tenant gate must reject before the owner check")
	}
	if !canManageMiniTool(card, ownerTenant, "alice", types.TenantRoleContributor) {
		t.Fatal("the creator must keep write access in their own workspace")
	}
}

// A public card from another workspace is listed, injected into a turn, and
// must render as read-only. The rules to pin down: readable, but never
// editable, and flagged as foreign so the UI can label its origin.
func TestMiniToolDecoratesForeignPublicCardAsReadOnly(t *testing.T) {
	foreign := &types.MiniTool{
		ID: "card-3", TenantID: 1, OwnerUserID: "alice",
		Visibility: types.MiniToolVisibilityPublic,
	}
	// The reader is an Owner of their OWN workspace. That must not promote them
	// to editor of somebody else's published card.
	decorateMiniTool(foreign, 2, "bob", types.TenantRoleOwner)

	if !foreign.IsForeign {
		t.Error("expected a card from another tenant to be marked foreign")
	}
	if foreign.CanEdit || foreign.CanDelete {
		t.Errorf("foreign public card must be read-only, got can_edit=%v can_delete=%v",
			foreign.CanEdit, foreign.CanDelete)
	}

	own := &types.MiniTool{
		ID: "card-4", TenantID: 2, OwnerUserID: "bob",
		Visibility: types.MiniToolVisibilityPublic,
	}
	decorateMiniTool(own, 2, "bob", types.TenantRoleContributor)
	if own.IsForeign {
		t.Error("a card in the caller's own tenant must not be marked foreign")
	}
	if !own.CanEdit || !own.CanDelete {
		t.Errorf("the creator must keep write access to their published card, got can_edit=%v", own.CanEdit)
	}
}

// Normalize is the last line of defence for a value that arrives from a client:
// an unknown string must never be stored as "public".
func TestMiniToolNormalizeVisibility(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{types.MiniToolVisibilityPublic, types.MiniToolVisibilityPublic},
		{types.MiniToolVisibilityWorkspace, types.MiniToolVisibilityWorkspace},
		{types.MiniToolVisibilityPrivate, types.MiniToolVisibilityPrivate},
		{"publc", types.MiniToolVisibilityPrivate},
		{"", types.MiniToolVisibilityPrivate},
	} {
		tool := &types.MiniTool{Name: "x", Visibility: tt.in}
		tool.Normalize()
		if tool.Visibility != tt.want {
			t.Errorf("Normalize(%q) = %q, want %q", tt.in, tool.Visibility, tt.want)
		}
	}
}

// The shipped catalogue cards have no owner: they are re-seeded on every boot,
// so their content cannot be edited in place. That must not lock their
// visibility, otherwise the 108 preset cards could never be published outside
// their own workspace and the UI had no affordance to change it at all.
func TestShippedMiniToolCanBeSharedButNotEdited(t *testing.T) {
	shipped := &types.MiniTool{
		ID: "card-shipped", TenantID: 1, OwnerUserID: "",
		Visibility: types.MiniToolVisibilityWorkspace,
	}
	decorateMiniTool(shipped, 1, "alice", types.TenantRoleAdmin)

	if shipped.CanEdit || shipped.CanDelete {
		t.Errorf("a shipped card must not be editable in place, got can_edit=%v can_delete=%v",
			shipped.CanEdit, shipped.CanDelete)
	}
	if !shipped.CanShare {
		t.Error("a workspace admin must be able to publish a shipped card")
	}

	// A plain member cannot publish workspace-wide policy either, and a card
	// belonging to another workspace is never shareable from here.
	decorateMiniTool(shipped, 1, "bob", types.TenantRoleContributor)
	if shipped.CanShare {
		t.Error("a contributor must not change the visibility of an unowned card")
	}
	decorateMiniTool(shipped, 2, "carol", types.TenantRoleOwner)
	if shipped.CanShare {
		t.Error("a workspace owner must not change the visibility of another workspace's card")
	}

	// A member's own card stays shareable.
	mine := &types.MiniTool{
		ID: "card-mine", TenantID: 1, OwnerUserID: "bob",
		Visibility: types.MiniToolVisibilityPrivate,
	}
	decorateMiniTool(mine, 1, "bob", types.TenantRoleContributor)
	if !mine.CanShare {
		t.Error("the creator must be able to change their own card's visibility")
	}
}
