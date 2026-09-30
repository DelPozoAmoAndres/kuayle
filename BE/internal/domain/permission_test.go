package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkspaceTransferPermissionIsLimitedToOwnerAndAdmin(t *testing.T) {
	require.True(t, HasPermission(RoleOwner, PermWorkspaceTransfer))
	require.True(t, HasPermission(RoleAdmin, PermWorkspaceTransfer))
	require.False(t, HasPermission(RoleMember, PermWorkspaceTransfer))
	require.False(t, HasPermission(RoleGuest, PermWorkspaceTransfer))
}

// Invited admins must be able to manage the workspace itself (integrations,
// webhooks, statuses...), otherwise only the owner can configure anything.
func TestWorkspaceManageIsAvailableToOwnerAndAdmin(t *testing.T) {
	require.True(t, HasPermission(RoleOwner, PermWorkspaceManage))
	require.True(t, HasPermission(RoleAdmin, PermWorkspaceManage))
	require.False(t, HasPermission(RoleMember, PermWorkspaceManage))
	require.False(t, HasPermission(RoleGuest, PermWorkspaceManage))
}

// Any invited member can review linked repositories/projects and create new ones.
func TestProjectManageIsAvailableToOwnerAdminAndMember(t *testing.T) {
	require.True(t, HasPermission(RoleOwner, PermProjectManage))
	require.True(t, HasPermission(RoleAdmin, PermProjectManage))
	require.True(t, HasPermission(RoleMember, PermProjectManage))
	require.False(t, HasPermission(RoleGuest, PermProjectManage))
}

// Teams and cycles no longer exist as a product concept.
func TestRemovedFeaturePermissions(t *testing.T) {
	for _, role := range []string{RoleOwner, RoleAdmin, RoleMember, RoleGuest} {
		require.False(t, HasPermission(role, "team:manage"))
		require.False(t, HasPermission(role, "cycle:manage"))
	}
}
