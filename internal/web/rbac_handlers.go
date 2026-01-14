package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"dogwatch/internal/rbac"
)

// rbacAuth holds the RBAC auth instance
var rbacAuth *rbac.Auth
var rbacMiddleware *rbac.Middleware
var rbacStore *rbac.Store

// SetRBACAuth sets the RBAC auth for the handlers
func SetRBACAuth(auth *rbac.Auth, mw *rbac.Middleware, store *rbac.Store) {
	rbacAuth = auth
	rbacMiddleware = mw
	rbacStore = store
}

// RegisterRBACRoutes registers RBAC API routes
func RegisterRBACRoutes(mux *http.ServeMux) {
	// Auth routes (public)
	mux.HandleFunc("/api/auth/login", handleLogin)
	mux.HandleFunc("/api/auth/logout", handleLogout)
	mux.HandleFunc("/api/auth/me", handleMe)

	// Invite routes (public accept, protected create)
	mux.HandleFunc("/api/auth/invite/accept", handleAcceptInvite)
	mux.HandleFunc("/api/auth/invites", handleInvites)
	mux.HandleFunc("/api/auth/invites/", handleInvite)

	// User routes (protected)
	mux.HandleFunc("/api/users", handleUsers)
	mux.HandleFunc("/api/users/", handleUser)

	// Team routes (protected)
	mux.HandleFunc("/api/teams", handleTeams)
	mux.HandleFunc("/api/teams/", handleTeam)

	// API key routes (protected)
	mux.HandleFunc("/api/apikeys", handleAPIKeys)
	mux.HandleFunc("/api/apikeys/", handleAPIKey)

	// Organization routes (protected)
	mux.HandleFunc("/api/org", handleOrg)

	// Sessions routes (protected)
	mux.HandleFunc("/api/sessions", handleSessions)

	// Password change (protected)
	mux.HandleFunc("/api/auth/password", handlePasswordChange)
}

// handleLogin handles user login
func handleLogin(w http.ResponseWriter, r *http.Request) {
	if rbacAuth == nil {
		http.Error(w, `{"error":"rbac not configured"}`, http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req rbac.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Get user agent and IP
	userAgent := r.Header.Get("User-Agent")
	ipAddress := r.RemoteAddr
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ipAddress = strings.Split(xff, ",")[0]
	}

	// Try login across all orgs
	resp, err := rbacAuth.LoginAnyOrg(req.Email, req.Password, userAgent, ipAddress)
	if err != nil {
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    resp.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   7 * 24 * 60 * 60, // 7 days
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleLogout handles user logout
func handleLogout(w http.ResponseWriter, r *http.Request) {
	if rbacAuth == nil {
		http.Error(w, `{"error":"rbac not configured"}`, http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Get token from cookie or header
	token := ""
	if cookie, err := r.Cookie("session_token"); err == nil {
		token = cookie.Value
	} else if auth := r.Header.Get("Authorization"); auth != "" {
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			token = parts[1]
		}
	}

	if token != "" {
		rbacAuth.Logout(token)
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "logged out"})
}

// handleMe returns the current authenticated user
func handleMe(w http.ResponseWriter, r *http.Request) {
	if rbacAuth == nil {
		http.Error(w, `{"error":"rbac not configured"}`, http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	user := rbac.GetUserFromContext(r.Context())
	if user == nil {
		// Try to authenticate
		token := ""
		if cookie, err := r.Cookie("session_token"); err == nil {
			token = cookie.Value
		} else if auth := r.Header.Get("Authorization"); auth != "" {
			parts := strings.SplitN(auth, " ", 2)
			if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
				token = parts[1]
			}
		}

		if token == "" {
			http.Error(w, `{"error":"not authenticated"}`, http.StatusUnauthorized)
			return
		}

		var err error
		user, _, err = rbacAuth.ValidateSession(token)
		if err != nil {
			http.Error(w, `{"error":"invalid session"}`, http.StatusUnauthorized)
			return
		}
	}

	// Get org info
	org, _ := rbacStore.GetOrganization(user.OrgID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user": user,
		"org":  org,
	})
}

// handleUsers handles GET/POST for users
func handleUsers(w http.ResponseWriter, r *http.Request) {
	if rbacAuth == nil {
		http.Error(w, `{"error":"rbac not configured"}`, http.StatusServiceUnavailable)
		return
	}

	user := getUserFromRequest(r)
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Check read permission
		if !rbac.HasPermission(user.Role, rbac.ResourceUsers, rbac.ActionRead) {
			http.Error(w, `{"error":"permission denied"}`, http.StatusForbidden)
			return
		}

		users, err := rbacStore.ListUsers(user.OrgID)
		if err != nil {
			http.Error(w, `{"error":"failed to list users"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)

	case http.MethodPost:
		// Check create permission
		if !rbac.HasPermission(user.Role, rbac.ResourceUsers, rbac.ActionCreate) {
			http.Error(w, `{"error":"permission denied"}`, http.StatusForbidden)
			return
		}

		var req rbac.UserCreate
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Check if user can assign this role
		if !rbac.CanManageRole(user.Role, req.Role) {
			http.Error(w, `{"error":"cannot assign this role"}`, http.StatusForbidden)
			return
		}

		newUser, err := rbacAuth.CreateUser(user.OrgID, &req)
		if err != nil {
			http.Error(w, `{"error":"failed to create user"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(newUser)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleUser handles GET/PUT/DELETE for a specific user
func handleUser(w http.ResponseWriter, r *http.Request) {
	if rbacAuth == nil {
		http.Error(w, `{"error":"rbac not configured"}`, http.StatusServiceUnavailable)
		return
	}

	user := getUserFromRequest(r)
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Extract user ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/users/")
	parts := strings.Split(path, "/")
	targetUserID := parts[0]

	switch r.Method {
	case http.MethodGet:
		if !rbac.HasPermission(user.Role, rbac.ResourceUsers, rbac.ActionRead) && user.ID != targetUserID {
			http.Error(w, `{"error":"permission denied"}`, http.StatusForbidden)
			return
		}

		targetUser, err := rbacStore.GetUser(targetUserID)
		if err != nil || targetUser.OrgID != user.OrgID {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(targetUser)

	case http.MethodPut:
		if !rbac.HasPermission(user.Role, rbac.ResourceUsers, rbac.ActionUpdate) && user.ID != targetUserID {
			http.Error(w, `{"error":"permission denied"}`, http.StatusForbidden)
			return
		}

		targetUser, err := rbacStore.GetUser(targetUserID)
		if err != nil || targetUser.OrgID != user.OrgID {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}

		var req rbac.UserUpdate
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Only admins can change roles, and they can't promote above their level
		if req.Role != "" && req.Role != targetUser.Role {
			if !rbac.HasPermission(user.Role, rbac.ResourceUsers, rbac.ActionUpdate) {
				http.Error(w, `{"error":"cannot change role"}`, http.StatusForbidden)
				return
			}
			if !rbac.CanManageRole(user.Role, req.Role) {
				http.Error(w, `{"error":"cannot assign this role"}`, http.StatusForbidden)
				return
			}
		}

		// Apply updates
		if req.Name != "" {
			targetUser.Name = req.Name
		}
		if req.Email != "" {
			targetUser.Email = strings.ToLower(strings.TrimSpace(req.Email))
		}
		if req.Role != "" {
			targetUser.Role = req.Role
		}
		if req.Timezone != "" {
			targetUser.Timezone = req.Timezone
		}
		if req.IsActive != nil {
			targetUser.IsActive = *req.IsActive
		}
		if req.TeamIDs != nil {
			targetUser.TeamIDs = req.TeamIDs
		}

		if err := rbacStore.UpdateUser(targetUser); err != nil {
			http.Error(w, `{"error":"failed to update user"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(targetUser)

	case http.MethodDelete:
		if !rbac.HasPermission(user.Role, rbac.ResourceUsers, rbac.ActionDelete) {
			http.Error(w, `{"error":"permission denied"}`, http.StatusForbidden)
			return
		}

		// Can't delete yourself
		if targetUserID == user.ID {
			http.Error(w, `{"error":"cannot delete yourself"}`, http.StatusBadRequest)
			return
		}

		targetUser, err := rbacStore.GetUser(targetUserID)
		if err != nil || targetUser.OrgID != user.OrgID {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}

		// Can't delete users with higher roles
		if !rbac.CanManageRole(user.Role, targetUser.Role) {
			http.Error(w, `{"error":"cannot delete this user"}`, http.StatusForbidden)
			return
		}

		// Delete sessions first
		rbacStore.DeleteUserSessions(targetUserID)

		if err := rbacStore.DeleteUser(targetUserID); err != nil {
			http.Error(w, `{"error":"failed to delete user"}`, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleTeams handles GET/POST for teams
func handleTeams(w http.ResponseWriter, r *http.Request) {
	if rbacStore == nil {
		http.Error(w, `{"error":"rbac not configured"}`, http.StatusServiceUnavailable)
		return
	}

	user := getUserFromRequest(r)
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if !rbac.HasPermission(user.Role, rbac.ResourceTeams, rbac.ActionRead) {
			http.Error(w, `{"error":"permission denied"}`, http.StatusForbidden)
			return
		}

		teams, err := rbacStore.ListTeams(user.OrgID)
		if err != nil {
			http.Error(w, `{"error":"failed to list teams"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(teams)

	case http.MethodPost:
		if !rbac.HasPermission(user.Role, rbac.ResourceTeams, rbac.ActionCreate) {
			http.Error(w, `{"error":"permission denied"}`, http.StatusForbidden)
			return
		}

		var team rbac.Team
		if err := json.NewDecoder(r.Body).Decode(&team); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		team.OrgID = user.OrgID
		if err := rbacStore.CreateTeam(&team); err != nil {
			http.Error(w, `{"error":"failed to create team"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(team)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleTeam handles GET/PUT/DELETE for a specific team
func handleTeam(w http.ResponseWriter, r *http.Request) {
	if rbacStore == nil {
		http.Error(w, `{"error":"rbac not configured"}`, http.StatusServiceUnavailable)
		return
	}

	user := getUserFromRequest(r)
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Extract team ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/teams/")
	teamID := strings.Split(path, "/")[0]

	switch r.Method {
	case http.MethodGet:
		if !rbac.HasPermission(user.Role, rbac.ResourceTeams, rbac.ActionRead) {
			http.Error(w, `{"error":"permission denied"}`, http.StatusForbidden)
			return
		}

		team, err := rbacStore.GetTeam(teamID)
		if err != nil || team.OrgID != user.OrgID {
			http.Error(w, `{"error":"team not found"}`, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(team)

	case http.MethodPut:
		if !rbac.HasPermission(user.Role, rbac.ResourceTeams, rbac.ActionUpdate) {
			http.Error(w, `{"error":"permission denied"}`, http.StatusForbidden)
			return
		}

		team, err := rbacStore.GetTeam(teamID)
		if err != nil || team.OrgID != user.OrgID {
			http.Error(w, `{"error":"team not found"}`, http.StatusNotFound)
			return
		}

		var update rbac.Team
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		if update.Name != "" {
			team.Name = update.Name
		}
		if update.Description != "" {
			team.Description = update.Description
		}
		if update.MemberIDs != nil {
			team.MemberIDs = update.MemberIDs
		}

		if err := rbacStore.UpdateTeam(team); err != nil {
			http.Error(w, `{"error":"failed to update team"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(team)

	case http.MethodDelete:
		if !rbac.HasPermission(user.Role, rbac.ResourceTeams, rbac.ActionDelete) {
			http.Error(w, `{"error":"permission denied"}`, http.StatusForbidden)
			return
		}

		team, err := rbacStore.GetTeam(teamID)
		if err != nil || team.OrgID != user.OrgID {
			http.Error(w, `{"error":"team not found"}`, http.StatusNotFound)
			return
		}

		if err := rbacStore.DeleteTeam(teamID); err != nil {
			http.Error(w, `{"error":"failed to delete team"}`, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleAPIKeys handles GET/POST for API keys
func handleAPIKeys(w http.ResponseWriter, r *http.Request) {
	if rbacAuth == nil {
		http.Error(w, `{"error":"rbac not configured"}`, http.StatusServiceUnavailable)
		return
	}

	user := getUserFromRequest(r)
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if !rbac.HasPermission(user.Role, rbac.ResourceAPIKeys, rbac.ActionRead) {
			http.Error(w, `{"error":"permission denied"}`, http.StatusForbidden)
			return
		}

		keys, err := rbacStore.ListAPIKeys(user.OrgID)
		if err != nil {
			http.Error(w, `{"error":"failed to list api keys"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(keys)

	case http.MethodPost:
		if !rbac.HasPermission(user.Role, rbac.ResourceAPIKeys, rbac.ActionCreate) {
			http.Error(w, `{"error":"permission denied"}`, http.StatusForbidden)
			return
		}

		var req rbac.APIKeyCreate
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		keyCreated, err := rbacAuth.CreateAPIKey(user.OrgID, user.ID, req.Name, req.Permissions, req.ExpiresIn)
		if err != nil {
			http.Error(w, `{"error":"failed to create api key"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(keyCreated)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleAPIKey handles GET/DELETE for a specific API key
func handleAPIKey(w http.ResponseWriter, r *http.Request) {
	if rbacStore == nil {
		http.Error(w, `{"error":"rbac not configured"}`, http.StatusServiceUnavailable)
		return
	}

	user := getUserFromRequest(r)
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Extract key ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/apikeys/")
	keyID := strings.Split(path, "/")[0]

	switch r.Method {
	case http.MethodGet:
		if !rbac.HasPermission(user.Role, rbac.ResourceAPIKeys, rbac.ActionRead) {
			http.Error(w, `{"error":"permission denied"}`, http.StatusForbidden)
			return
		}

		key, err := rbacStore.GetAPIKey(keyID)
		if err != nil || key.OrgID != user.OrgID {
			http.Error(w, `{"error":"api key not found"}`, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(key)

	case http.MethodDelete:
		if !rbac.HasPermission(user.Role, rbac.ResourceAPIKeys, rbac.ActionDelete) {
			http.Error(w, `{"error":"permission denied"}`, http.StatusForbidden)
			return
		}

		key, err := rbacStore.GetAPIKey(keyID)
		if err != nil || key.OrgID != user.OrgID {
			http.Error(w, `{"error":"api key not found"}`, http.StatusNotFound)
			return
		}

		if err := rbacStore.DeleteAPIKey(keyID); err != nil {
			http.Error(w, `{"error":"failed to delete api key"}`, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleInvites handles GET/POST for invites
func handleInvites(w http.ResponseWriter, r *http.Request) {
	if rbacAuth == nil {
		http.Error(w, `{"error":"rbac not configured"}`, http.StatusServiceUnavailable)
		return
	}

	user := getUserFromRequest(r)
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if !rbac.HasPermission(user.Role, rbac.ResourceUsers, rbac.ActionRead) {
			http.Error(w, `{"error":"permission denied"}`, http.StatusForbidden)
			return
		}

		invites, err := rbacStore.ListInvites(user.OrgID)
		if err != nil {
			http.Error(w, `{"error":"failed to list invites"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(invites)

	case http.MethodPost:
		if !rbac.HasPermission(user.Role, rbac.ResourceUsers, rbac.ActionCreate) {
			http.Error(w, `{"error":"permission denied"}`, http.StatusForbidden)
			return
		}

		var req rbac.InviteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		// Check if user can invite this role
		if !rbac.CanManageRole(user.Role, req.Role) {
			http.Error(w, `{"error":"cannot invite this role"}`, http.StatusForbidden)
			return
		}

		invite, token, err := rbacAuth.CreateInvite(user.OrgID, req.Email, req.Role, user.ID)
		if err != nil {
			http.Error(w, `{"error":"failed to create invite"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"invite": invite,
			"token":  token,
		})

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleInvite handles DELETE for a specific invite
func handleInvite(w http.ResponseWriter, r *http.Request) {
	if rbacStore == nil {
		http.Error(w, `{"error":"rbac not configured"}`, http.StatusServiceUnavailable)
		return
	}

	user := getUserFromRequest(r)
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Extract invite ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/auth/invites/")
	inviteID := strings.Split(path, "/")[0]

	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	if !rbac.HasPermission(user.Role, rbac.ResourceUsers, rbac.ActionDelete) {
		http.Error(w, `{"error":"permission denied"}`, http.StatusForbidden)
		return
	}

	invite, err := rbacStore.GetInvite(inviteID)
	if err != nil || invite.OrgID != user.OrgID {
		http.Error(w, `{"error":"invite not found"}`, http.StatusNotFound)
		return
	}

	if err := rbacStore.DeleteInvite(inviteID); err != nil {
		http.Error(w, `{"error":"failed to delete invite"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleAcceptInvite handles accepting an invite (public)
func handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	if rbacAuth == nil {
		http.Error(w, `{"error":"rbac not configured"}`, http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	user, err := rbacAuth.AcceptInvite(req.Token, req.Password, req.Name)
	if err != nil {
		http.Error(w, `{"error":"invalid or expired invite"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

// handleOrg handles GET/PUT for the current organization
func handleOrg(w http.ResponseWriter, r *http.Request) {
	if rbacStore == nil {
		http.Error(w, `{"error":"rbac not configured"}`, http.StatusServiceUnavailable)
		return
	}

	user := getUserFromRequest(r)
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		org, err := rbacStore.GetOrganization(user.OrgID)
		if err != nil {
			http.Error(w, `{"error":"organization not found"}`, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(org)

	case http.MethodPut:
		if !rbac.HasPermission(user.Role, rbac.ResourceSettings, rbac.ActionUpdate) {
			http.Error(w, `{"error":"permission denied"}`, http.StatusForbidden)
			return
		}

		org, err := rbacStore.GetOrganization(user.OrgID)
		if err != nil {
			http.Error(w, `{"error":"organization not found"}`, http.StatusNotFound)
			return
		}

		var update rbac.Organization
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		if update.Name != "" {
			org.Name = update.Name
		}
		if update.Settings != nil {
			for k, v := range update.Settings {
				org.Settings[k] = v
			}
		}

		if err := rbacStore.UpdateOrganization(org); err != nil {
			http.Error(w, `{"error":"failed to update organization"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(org)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleSessions handles GET/DELETE for sessions
func handleSessions(w http.ResponseWriter, r *http.Request) {
	if rbacStore == nil {
		http.Error(w, `{"error":"rbac not configured"}`, http.StatusServiceUnavailable)
		return
	}

	user := getUserFromRequest(r)
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		sessions, err := rbacStore.ListUserSessions(user.ID)
		if err != nil {
			http.Error(w, `{"error":"failed to list sessions"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sessions)

	case http.MethodDelete:
		// Logout all sessions
		if err := rbacAuth.LogoutAll(user.ID); err != nil {
			http.Error(w, `{"error":"failed to logout"}`, http.StatusInternalServerError)
			return
		}

		// Clear cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "session_token",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "all sessions logged out"})

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handlePasswordChange handles password change
func handlePasswordChange(w http.ResponseWriter, r *http.Request) {
	if rbacAuth == nil {
		http.Error(w, `{"error":"rbac not configured"}`, http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	user := getUserFromRequest(r)
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if err := rbacAuth.ChangePassword(user.ID, req.OldPassword, req.NewPassword); err != nil {
		http.Error(w, `{"error":"failed to change password"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "password changed"})
}

// getUserFromRequest extracts and validates the user from request
func getUserFromRequest(r *http.Request) *rbac.User {
	// First try context (if middleware was applied)
	if user := rbac.GetUserFromContext(r.Context()); user != nil {
		return user
	}

	// Try cookie
	if cookie, err := r.Cookie("session_token"); err == nil && cookie.Value != "" {
		user, _, err := rbacAuth.ValidateSession(cookie.Value)
		if err == nil {
			return user
		}
	}

	// Try Authorization header
	if auth := r.Header.Get("Authorization"); auth != "" {
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 {
			scheme := strings.ToLower(parts[0])
			token := parts[1]

			switch scheme {
			case "bearer":
				user, _, err := rbacAuth.ValidateSession(token)
				if err == nil {
					return user
				}
			case "apikey":
				_, user, err := rbacAuth.ValidateAPIKey(token)
				if err == nil {
					return user
				}
			}
		}
	}

	// Try X-API-Key header
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		_, user, err := rbacAuth.ValidateAPIKey(apiKey)
		if err == nil {
			return user
		}
	}

	return nil
}
