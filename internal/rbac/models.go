package rbac

import (
	"time"
)

// Role represents a user's role in the system
type Role string

const (
	RoleOwner  Role = "owner"  // Full access, can delete org, transfer ownership
	RoleAdmin  Role = "admin"  // Full access except org deletion
	RoleEditor Role = "editor" // Can create/edit resources
	RoleViewer Role = "viewer" // Read-only access
)

// Permission represents an action on a resource
type Permission struct {
	Resource string `json:"resource"` // dashboards, alerts, incidents, users, etc.
	Action   string `json:"action"`   // create, read, update, delete, execute
}

// Resource types
const (
	ResourceDashboards   = "dashboards"
	ResourceAlerts       = "alerts"
	ResourceIncidents    = "incidents"
	ResourceUsers        = "users"
	ResourceTeams        = "teams"
	ResourceSettings     = "settings"
	ResourceAPIKeys      = "apikeys"
	ResourceLogs         = "logs"
	ResourceTraces       = "traces"
	ResourceMetrics      = "metrics"
	ResourceSynthetics   = "synthetics"
	ResourceSLOs         = "slos"
	ResourceOnCall       = "oncall"
	ResourceIntegrations = "integrations"
	ResourceAll          = "*"
)

// Action types
const (
	ActionCreate  = "create"
	ActionRead    = "read"
	ActionUpdate  = "update"
	ActionDelete  = "delete"
	ActionExecute = "execute" // For running queries, triggering actions
	ActionAll     = "*"
)

// RolePermissions defines what each role can do
var RolePermissions = map[Role][]Permission{
	RoleOwner: {
		{Resource: ResourceAll, Action: ActionAll},
	},
	RoleAdmin: {
		{Resource: ResourceDashboards, Action: ActionAll},
		{Resource: ResourceAlerts, Action: ActionAll},
		{Resource: ResourceIncidents, Action: ActionAll},
		{Resource: ResourceUsers, Action: ActionAll},
		{Resource: ResourceTeams, Action: ActionAll},
		{Resource: ResourceSettings, Action: ActionRead},
		{Resource: ResourceSettings, Action: ActionUpdate},
		{Resource: ResourceAPIKeys, Action: ActionAll},
		{Resource: ResourceLogs, Action: ActionAll},
		{Resource: ResourceTraces, Action: ActionAll},
		{Resource: ResourceMetrics, Action: ActionAll},
		{Resource: ResourceSynthetics, Action: ActionAll},
		{Resource: ResourceSLOs, Action: ActionAll},
		{Resource: ResourceOnCall, Action: ActionAll},
		{Resource: ResourceIntegrations, Action: ActionAll},
	},
	RoleEditor: {
		{Resource: ResourceDashboards, Action: ActionCreate},
		{Resource: ResourceDashboards, Action: ActionRead},
		{Resource: ResourceDashboards, Action: ActionUpdate},
		{Resource: ResourceAlerts, Action: ActionAll},
		{Resource: ResourceIncidents, Action: ActionAll},
		{Resource: ResourceUsers, Action: ActionRead},
		{Resource: ResourceTeams, Action: ActionRead},
		{Resource: ResourceLogs, Action: ActionRead},
		{Resource: ResourceLogs, Action: ActionExecute},
		{Resource: ResourceTraces, Action: ActionRead},
		{Resource: ResourceMetrics, Action: ActionRead},
		{Resource: ResourceMetrics, Action: ActionCreate},
		{Resource: ResourceSynthetics, Action: ActionAll},
		{Resource: ResourceSLOs, Action: ActionAll},
		{Resource: ResourceOnCall, Action: ActionRead},
		{Resource: ResourceOnCall, Action: ActionUpdate},
	},
	RoleViewer: {
		{Resource: ResourceDashboards, Action: ActionRead},
		{Resource: ResourceAlerts, Action: ActionRead},
		{Resource: ResourceIncidents, Action: ActionRead},
		{Resource: ResourceUsers, Action: ActionRead},
		{Resource: ResourceTeams, Action: ActionRead},
		{Resource: ResourceLogs, Action: ActionRead},
		{Resource: ResourceTraces, Action: ActionRead},
		{Resource: ResourceMetrics, Action: ActionRead},
		{Resource: ResourceSynthetics, Action: ActionRead},
		{Resource: ResourceSLOs, Action: ActionRead},
		{Resource: ResourceOnCall, Action: ActionRead},
	},
}

// Organization represents a tenant/company
type Organization struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Slug      string            `json:"slug"` // URL-friendly name
	Plan      string            `json:"plan"` // free, pro, enterprise
	Settings  map[string]string `json:"settings"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// Team represents a group within an organization
type Team struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	MemberIDs   []string  `json:"member_ids"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// User represents a user account
type User struct {
	ID           string    `json:"id"`
	OrgID        string    `json:"org_id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // Never expose in JSON
	Name         string    `json:"name"`
	Role         Role      `json:"role"`
	TeamIDs      []string  `json:"team_ids"`
	AvatarURL    string    `json:"avatar_url,omitempty"`
	Timezone     string    `json:"timezone,omitempty"`
	IsActive     bool      `json:"is_active"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UserCreate is used for creating new users
type UserCreate struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Role     Role   `json:"role"`
}

// UserUpdate is used for updating users
type UserUpdate struct {
	Email    string   `json:"email,omitempty"`
	Name     string   `json:"name,omitempty"`
	Role     Role     `json:"role,omitempty"`
	TeamIDs  []string `json:"team_ids,omitempty"`
	IsActive *bool    `json:"is_active,omitempty"`
	Timezone string   `json:"timezone,omitempty"`
}

// Session represents an active user session
type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	OrgID     string    `json:"org_id"`
	Token     string    `json:"-"` // The actual token (hashed in DB)
	UserAgent string    `json:"user_agent"`
	IPAddress string    `json:"ip_address"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// APIKey represents an API key for programmatic access
type APIKey struct {
	ID          string       `json:"id"`
	OrgID       string       `json:"org_id"`
	UserID      string       `json:"user_id"` // Creator
	Name        string       `json:"name"`
	KeyPrefix   string       `json:"key_prefix"` // First 8 chars for identification
	KeyHash     string       `json:"-"`          // Hashed key
	Permissions []Permission `json:"permissions"`
	LastUsedAt  *time.Time   `json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time   `json:"expires_at,omitempty"`
	IsActive    bool         `json:"is_active"`
	CreatedAt   time.Time    `json:"created_at"`
}

// APIKeyCreate is used for creating new API keys
type APIKeyCreate struct {
	Name        string       `json:"name"`
	Permissions []Permission `json:"permissions"`
	ExpiresIn   string       `json:"expires_in,omitempty"` // e.g., "30d", "1y", "never"
}

// APIKeyCreated is returned after creating an API key (includes the full key)
type APIKeyCreated struct {
	APIKey
	Key string `json:"key"` // Only shown once at creation
}

// LoginRequest represents login credentials
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse is returned after successful login
type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      User      `json:"user"`
}

// InviteRequest represents an invitation to join an organization
type InviteRequest struct {
	Email string `json:"email"`
	Role  Role   `json:"role"`
}

// Invite represents a pending invitation
type Invite struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	Email     string    `json:"email"`
	Role      Role      `json:"role"`
	Token     string    `json:"-"`
	InvitedBy string    `json:"invited_by"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// AuditLog records user actions for compliance
type AuditLog struct {
	ID        string                 `json:"id"`
	OrgID     string                 `json:"org_id"`
	UserID    string                 `json:"user_id"`
	Action    string                 `json:"action"`
	Resource  string                 `json:"resource"`
	ResourceID string                `json:"resource_id"`
	Details   map[string]interface{} `json:"details"`
	IPAddress string                 `json:"ip_address"`
	UserAgent string                 `json:"user_agent"`
	CreatedAt time.Time              `json:"created_at"`
}

// HasPermission checks if a role has a specific permission
func HasPermission(role Role, resource, action string) bool {
	permissions, ok := RolePermissions[role]
	if !ok {
		return false
	}

	for _, p := range permissions {
		// Check for wildcard permissions
		if p.Resource == ResourceAll && p.Action == ActionAll {
			return true
		}
		if p.Resource == ResourceAll && p.Action == action {
			return true
		}
		if p.Resource == resource && p.Action == ActionAll {
			return true
		}
		if p.Resource == resource && p.Action == action {
			return true
		}
	}

	return false
}

// CanManageRole checks if a role can manage another role
func CanManageRole(managerRole, targetRole Role) bool {
	hierarchy := map[Role]int{
		RoleViewer: 1,
		RoleEditor: 2,
		RoleAdmin:  3,
		RoleOwner:  4,
	}

	return hierarchy[managerRole] > hierarchy[targetRole]
}
