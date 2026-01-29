package rbac

import (
	"testing"
)

func TestHasPermission(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		role     Role
		resource string
		action   string
		expected bool
	}{
		// Owner tests
		{"owner has all permissions", RoleOwner, ResourceDashboards, ActionDelete, true},
		{"owner can manage users", RoleOwner, ResourceUsers, ActionDelete, true},
		{"owner can access settings", RoleOwner, ResourceSettings, ActionDelete, true},
		{"owner can access any resource", RoleOwner, ResourceAll, ActionAll, true},

		// Admin tests
		{"admin can manage dashboards", RoleAdmin, ResourceDashboards, ActionDelete, true},
		{"admin can manage users", RoleAdmin, ResourceUsers, ActionDelete, true},
		{"admin can read settings", RoleAdmin, ResourceSettings, ActionRead, true},
		{"admin can update settings", RoleAdmin, ResourceSettings, ActionUpdate, true},
		{"admin cannot delete settings", RoleAdmin, ResourceSettings, ActionDelete, false},
		{"admin can manage alerts", RoleAdmin, ResourceAlerts, ActionCreate, true},
		{"admin can manage incidents", RoleAdmin, ResourceIncidents, ActionUpdate, true},

		// Editor tests
		{"editor can create dashboards", RoleEditor, ResourceDashboards, ActionCreate, true},
		{"editor can read dashboards", RoleEditor, ResourceDashboards, ActionRead, true},
		{"editor can update dashboards", RoleEditor, ResourceDashboards, ActionUpdate, true},
		{"editor cannot delete dashboards", RoleEditor, ResourceDashboards, ActionDelete, false},
		{"editor can read users", RoleEditor, ResourceUsers, ActionRead, true},
		{"editor cannot create users", RoleEditor, ResourceUsers, ActionCreate, false},
		{"editor can manage alerts", RoleEditor, ResourceAlerts, ActionCreate, true},
		{"editor can read logs", RoleEditor, ResourceLogs, ActionRead, true},
		{"editor can execute log queries", RoleEditor, ResourceLogs, ActionExecute, true},

		// Viewer tests
		{"viewer can read dashboards", RoleViewer, ResourceDashboards, ActionRead, true},
		{"viewer cannot create dashboards", RoleViewer, ResourceDashboards, ActionCreate, false},
		{"viewer cannot update dashboards", RoleViewer, ResourceDashboards, ActionUpdate, false},
		{"viewer cannot delete dashboards", RoleViewer, ResourceDashboards, ActionDelete, false},
		{"viewer can read alerts", RoleViewer, ResourceAlerts, ActionRead, true},
		{"viewer cannot create alerts", RoleViewer, ResourceAlerts, ActionCreate, false},
		{"viewer can read metrics", RoleViewer, ResourceMetrics, ActionRead, true},
		{"viewer can read traces", RoleViewer, ResourceTraces, ActionRead, true},
		{"viewer can read logs", RoleViewer, ResourceLogs, ActionRead, true},

		// Invalid role
		{"invalid role has no permissions", Role("invalid"), ResourceDashboards, ActionRead, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasPermission(tt.role, tt.resource, tt.action)
			if result != tt.expected {
				t.Errorf("HasPermission(%s, %s, %s) = %v, want %v",
					tt.role, tt.resource, tt.action, result, tt.expected)
			}
		})
	}
}

func TestCanManageRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		managerRole Role
		targetRole  Role
		expected    bool
	}{
		// Owner can manage everyone except other owners
		{"owner can manage admin", RoleOwner, RoleAdmin, true},
		{"owner can manage editor", RoleOwner, RoleEditor, true},
		{"owner can manage viewer", RoleOwner, RoleViewer, true},
		{"owner cannot manage owner", RoleOwner, RoleOwner, false},

		// Admin can manage editors and viewers
		{"admin cannot manage owner", RoleAdmin, RoleOwner, false},
		{"admin cannot manage admin", RoleAdmin, RoleAdmin, false},
		{"admin can manage editor", RoleAdmin, RoleEditor, true},
		{"admin can manage viewer", RoleAdmin, RoleViewer, true},

		// Editor can manage viewers
		{"editor cannot manage owner", RoleEditor, RoleOwner, false},
		{"editor cannot manage admin", RoleEditor, RoleAdmin, false},
		{"editor cannot manage editor", RoleEditor, RoleEditor, false},
		{"editor can manage viewer", RoleEditor, RoleViewer, true},

		// Viewer cannot manage anyone
		{"viewer cannot manage owner", RoleViewer, RoleOwner, false},
		{"viewer cannot manage admin", RoleViewer, RoleAdmin, false},
		{"viewer cannot manage editor", RoleViewer, RoleEditor, false},
		{"viewer cannot manage viewer", RoleViewer, RoleViewer, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CanManageRole(tt.managerRole, tt.targetRole)
			if result != tt.expected {
				t.Errorf("CanManageRole(%s, %s) = %v, want %v",
					tt.managerRole, tt.targetRole, result, tt.expected)
			}
		})
	}
}

func TestRoleConstants(t *testing.T) {
	t.Parallel()

	// Ensure role constants have expected values
	tests := []struct {
		role     Role
		expected string
	}{
		{RoleOwner, "owner"},
		{RoleAdmin, "admin"},
		{RoleEditor, "editor"},
		{RoleViewer, "viewer"},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			if string(tt.role) != tt.expected {
				t.Errorf("Role constant %v = %q, want %q", tt.role, tt.role, tt.expected)
			}
		})
	}
}

func TestResourceConstants(t *testing.T) {
	t.Parallel()

	// Ensure all expected resources exist
	resources := []string{
		ResourceDashboards,
		ResourceAlerts,
		ResourceIncidents,
		ResourceUsers,
		ResourceTeams,
		ResourceSettings,
		ResourceAPIKeys,
		ResourceLogs,
		ResourceTraces,
		ResourceMetrics,
		ResourceSynthetics,
		ResourceSLOs,
		ResourceOnCall,
		ResourceIntegrations,
		ResourceAll,
	}

	for _, r := range resources {
		t.Run(r, func(t *testing.T) {
			if r == "" {
				t.Error("resource constant should not be empty")
			}
		})
	}
}

func TestActionConstants(t *testing.T) {
	t.Parallel()

	// Ensure all expected actions exist
	actions := []string{
		ActionCreate,
		ActionRead,
		ActionUpdate,
		ActionDelete,
		ActionExecute,
		ActionAll,
	}

	for _, a := range actions {
		t.Run(a, func(t *testing.T) {
			if a == "" {
				t.Error("action constant should not be empty")
			}
		})
	}
}

func TestRolePermissionsCompleteness(t *testing.T) {
	t.Parallel()

	// Every defined role should have at least one permission
	roles := []Role{RoleOwner, RoleAdmin, RoleEditor, RoleViewer}

	for _, role := range roles {
		t.Run(string(role), func(t *testing.T) {
			perms, ok := RolePermissions[role]
			if !ok {
				t.Errorf("role %s has no permissions defined", role)
				return
			}
			if len(perms) == 0 {
				t.Errorf("role %s has empty permissions list", role)
			}
		})
	}
}

func TestPermissionStruct(t *testing.T) {
	t.Parallel()

	p := Permission{
		Resource: ResourceDashboards,
		Action:   ActionRead,
	}

	if p.Resource != "dashboards" {
		t.Errorf("Resource = %q, want %q", p.Resource, "dashboards")
	}
	if p.Action != "read" {
		t.Errorf("Action = %q, want %q", p.Action, "read")
	}
}

func TestRoleHierarchy(t *testing.T) {
	t.Parallel()

	// Test that higher roles have at least the permissions of lower roles
	// This is a logical consistency check

	// All roles should be able to read dashboards
	for _, role := range []Role{RoleOwner, RoleAdmin, RoleEditor, RoleViewer} {
		if !HasPermission(role, ResourceDashboards, ActionRead) {
			t.Errorf("%s should be able to read dashboards", role)
		}
	}

	// Only owner, admin, editor should be able to create alerts
	for _, role := range []Role{RoleOwner, RoleAdmin, RoleEditor} {
		if !HasPermission(role, ResourceAlerts, ActionCreate) {
			t.Errorf("%s should be able to create alerts", role)
		}
	}

	// Only owner and admin should be able to manage users
	for _, role := range []Role{RoleOwner, RoleAdmin} {
		if !HasPermission(role, ResourceUsers, ActionDelete) {
			t.Errorf("%s should be able to delete users", role)
		}
	}
}

// Benchmarks

func BenchmarkHasPermission(b *testing.B) {
	for i := 0; i < b.N; i++ {
		HasPermission(RoleEditor, ResourceDashboards, ActionRead)
	}
}

func BenchmarkCanManageRole(b *testing.B) {
	for i := 0; i < b.N; i++ {
		CanManageRole(RoleAdmin, RoleEditor)
	}
}
