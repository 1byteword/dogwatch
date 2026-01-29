package rbac

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewAuth(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()

	auth := NewAuth(store)
	if auth == nil {
		t.Fatal("NewAuth returned nil")
	}

	if auth.sessionExpiry != 7*24*time.Hour {
		t.Errorf("expected default session expiry of 7 days, got %v", auth.sessionExpiry)
	}

	if auth.bcryptCost != 12 {
		t.Errorf("expected bcrypt cost of 12, got %d", auth.bcryptCost)
	}
}

func TestSetSessionExpiry(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()
	auth := NewAuth(store)

	auth.SetSessionExpiry(24 * time.Hour)
	if auth.sessionExpiry != 24*time.Hour {
		t.Errorf("expected session expiry of 24h, got %v", auth.sessionExpiry)
	}
}

func TestHashPassword(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()
	auth := NewAuth(store)

	tests := []struct {
		name     string
		password string
	}{
		{"simple password", "password123"},
		{"complex password", "P@ssw0rd!#$%^&*()"},
		{"empty password", ""},
		{"max length password", strings.Repeat("a", 72)}, // bcrypt has 72-byte limit
		{"unicode password", "пароль密码パスワード"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := auth.HashPassword(tt.password)
			if err != nil {
				t.Fatalf("HashPassword failed: %v", err)
			}

			if hash == "" {
				t.Error("expected non-empty hash")
			}

			if hash == tt.password {
				t.Error("hash should not equal password")
			}

			// Hash should start with bcrypt prefix
			if !strings.HasPrefix(hash, "$2") {
				t.Errorf("hash should start with bcrypt prefix, got %s", hash[:10])
			}
		})
	}
}

func TestCheckPassword(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()
	auth := NewAuth(store)

	password := "correctpassword"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	tests := []struct {
		name     string
		password string
		expected bool
	}{
		{"correct password", "correctpassword", true},
		{"wrong password", "wrongpassword", false},
		{"empty password", "", false},
		{"similar password", "correctpassword1", false},
		{"case sensitive", "CorrectPassword", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := auth.CheckPassword(tt.password, hash)
			if result != tt.expected {
				t.Errorf("CheckPassword(%q) = %v, want %v", tt.password, result, tt.expected)
			}
		})
	}
}

func TestGenerateToken(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()
	auth := NewAuth(store)

	t.Run("generates unique tokens", func(t *testing.T) {
		tokens := make(map[string]bool)
		for i := 0; i < 100; i++ {
			token, err := auth.GenerateToken()
			if err != nil {
				t.Fatalf("GenerateToken failed: %v", err)
			}

			if tokens[token] {
				t.Error("generated duplicate token")
			}
			tokens[token] = true
		}
	})

	t.Run("token length is consistent", func(t *testing.T) {
		token, _ := auth.GenerateToken()
		// Base64 encoded 32 bytes = 44 characters (with padding)
		if len(token) < 40 {
			t.Errorf("token too short: %d characters", len(token))
		}
	})
}

func TestHashToken(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()
	auth := NewAuth(store)

	t.Run("deterministic hashing", func(t *testing.T) {
		token := "test-token-12345"
		hash1 := auth.HashToken(token)
		hash2 := auth.HashToken(token)

		if hash1 != hash2 {
			t.Error("HashToken should be deterministic")
		}
	})

	t.Run("different tokens produce different hashes", func(t *testing.T) {
		hash1 := auth.HashToken("token1")
		hash2 := auth.HashToken("token2")

		if hash1 == hash2 {
			t.Error("different tokens should produce different hashes")
		}
	})

	t.Run("hash is hex encoded", func(t *testing.T) {
		hash := auth.HashToken("test")
		// SHA256 produces 32 bytes = 64 hex characters
		if len(hash) != 64 {
			t.Errorf("expected 64 character hash, got %d", len(hash))
		}

		for _, c := range hash {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("hash contains non-hex character: %c", c)
			}
		}
	})
}

func TestGenerateAPIKey(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()
	auth := NewAuth(store)

	t.Run("generates valid API key", func(t *testing.T) {
		key, prefix, err := auth.GenerateAPIKey()
		if err != nil {
			t.Fatalf("GenerateAPIKey failed: %v", err)
		}

		if !strings.HasPrefix(key, "dw_") {
			t.Errorf("key should start with 'dw_', got %s", key[:min(10, len(key))])
		}

		if !strings.HasPrefix(prefix, "dw_") {
			t.Errorf("prefix should start with 'dw_', got %s", prefix)
		}

		if len(prefix) != 11 {
			t.Errorf("prefix should be 11 characters, got %d", len(prefix))
		}
	})

	t.Run("generates unique keys", func(t *testing.T) {
		keys := make(map[string]bool)
		for i := 0; i < 100; i++ {
			key, _, err := auth.GenerateAPIKey()
			if err != nil {
				t.Fatalf("GenerateAPIKey failed: %v", err)
			}

			if keys[key] {
				t.Error("generated duplicate API key")
			}
			keys[key] = true
		}
	})
}

func TestCreateUser(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()
	auth := NewAuth(store)

	// Create default org
	org, _ := store.EnsureDefaultOrg("test", "Test Org")

	t.Run("creates user successfully", func(t *testing.T) {
		user, err := auth.CreateUser(org.ID, &UserCreate{
			Email:    "test@example.com",
			Password: "password123",
			Name:     "Test User",
			Role:     RoleEditor,
		})
		if err != nil {
			t.Fatalf("CreateUser failed: %v", err)
		}

		if user.ID == "" {
			t.Error("user ID should not be empty")
		}

		if user.Email != "test@example.com" {
			t.Errorf("email = %s, want test@example.com", user.Email)
		}

		if user.PasswordHash == "password123" {
			t.Error("password should be hashed")
		}

		if user.Role != RoleEditor {
			t.Errorf("role = %s, want editor", user.Role)
		}

		if !user.IsActive {
			t.Error("new user should be active")
		}
	})

	t.Run("normalizes email", func(t *testing.T) {
		user, err := auth.CreateUser(org.ID, &UserCreate{
			Email:    "  TEST2@EXAMPLE.COM  ",
			Password: "password123",
			Name:     "Test User 2",
			Role:     RoleViewer,
		})
		if err != nil {
			t.Fatalf("CreateUser failed: %v", err)
		}

		if user.Email != "test2@example.com" {
			t.Errorf("email should be normalized, got %s", user.Email)
		}
	})
}

func TestLogin(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()
	auth := NewAuth(store)

	// Create org and user
	org, _ := store.EnsureDefaultOrg("test", "Test Org")
	_, err := auth.CreateUser(org.ID, &UserCreate{
		Email:    "user@example.com",
		Password: "password123",
		Name:     "Test User",
		Role:     RoleEditor,
	})
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	t.Run("successful login", func(t *testing.T) {
		resp, err := auth.Login(org.ID, "user@example.com", "password123", "Test Browser", "127.0.0.1")
		if err != nil {
			t.Fatalf("Login failed: %v", err)
		}

		if resp.Token == "" {
			t.Error("token should not be empty")
		}

		if resp.User.Email != "user@example.com" {
			t.Errorf("email = %s, want user@example.com", resp.User.Email)
		}

		if resp.ExpiresAt.Before(time.Now()) {
			t.Error("session should not be expired")
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		_, err := auth.Login(org.ID, "user@example.com", "wrongpassword", "Test Browser", "127.0.0.1")
		if err != ErrInvalidCredentials {
			t.Errorf("expected ErrInvalidCredentials, got %v", err)
		}
	})

	t.Run("non-existent user", func(t *testing.T) {
		_, err := auth.Login(org.ID, "nonexistent@example.com", "password123", "Test Browser", "127.0.0.1")
		if err != ErrInvalidCredentials {
			t.Errorf("expected ErrInvalidCredentials, got %v", err)
		}
	})
}

func TestValidateSession(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()
	auth := NewAuth(store)

	// Create org and user, then login
	org, _ := store.EnsureDefaultOrg("test", "Test Org")
	auth.CreateUser(org.ID, &UserCreate{
		Email:    "user@example.com",
		Password: "password123",
		Name:     "Test User",
		Role:     RoleEditor,
	})
	resp, _ := auth.Login(org.ID, "user@example.com", "password123", "Test Browser", "127.0.0.1")

	t.Run("valid session", func(t *testing.T) {
		user, session, err := auth.ValidateSession(resp.Token)
		if err != nil {
			t.Fatalf("ValidateSession failed: %v", err)
		}

		if user == nil {
			t.Fatal("user should not be nil")
		}

		if user.Email != "user@example.com" {
			t.Errorf("email = %s, want user@example.com", user.Email)
		}

		if session == nil {
			t.Fatal("session should not be nil")
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		_, _, err := auth.ValidateSession("invalid-token")
		if err != ErrInvalidToken {
			t.Errorf("expected ErrInvalidToken, got %v", err)
		}
	})
}

func TestLogout(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()
	auth := NewAuth(store)

	// Create org, user, and login
	org, _ := store.EnsureDefaultOrg("test", "Test Org")
	auth.CreateUser(org.ID, &UserCreate{
		Email:    "user@example.com",
		Password: "password123",
		Name:     "Test User",
		Role:     RoleEditor,
	})
	resp, _ := auth.Login(org.ID, "user@example.com", "password123", "Test Browser", "127.0.0.1")

	t.Run("logout invalidates session", func(t *testing.T) {
		err := auth.Logout(resp.Token)
		if err != nil {
			t.Fatalf("Logout failed: %v", err)
		}

		// Session should no longer be valid
		_, _, err = auth.ValidateSession(resp.Token)
		if err != ErrInvalidToken {
			t.Errorf("expected ErrInvalidToken after logout, got %v", err)
		}
	})
}

func TestChangePassword(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()
	auth := NewAuth(store)

	// Create org and user
	org, _ := store.EnsureDefaultOrg("test", "Test Org")
	user, _ := auth.CreateUser(org.ID, &UserCreate{
		Email:    "user@example.com",
		Password: "oldpassword",
		Name:     "Test User",
		Role:     RoleEditor,
	})

	t.Run("successful password change", func(t *testing.T) {
		err := auth.ChangePassword(user.ID, "oldpassword", "newpassword")
		if err != nil {
			t.Fatalf("ChangePassword failed: %v", err)
		}

		// Should be able to login with new password
		_, err = auth.Login(org.ID, "user@example.com", "newpassword", "Browser", "127.0.0.1")
		if err != nil {
			t.Errorf("login with new password failed: %v", err)
		}

		// Old password should not work
		_, err = auth.Login(org.ID, "user@example.com", "oldpassword", "Browser", "127.0.0.1")
		if err != ErrInvalidCredentials {
			t.Errorf("expected ErrInvalidCredentials with old password, got %v", err)
		}
	})

	t.Run("wrong old password", func(t *testing.T) {
		err := auth.ChangePassword(user.ID, "wrongpassword", "newpassword2")
		if err != ErrInvalidCredentials {
			t.Errorf("expected ErrInvalidCredentials, got %v", err)
		}
	})
}

func TestCreateAPIKey(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()
	auth := NewAuth(store)

	org, _ := store.EnsureDefaultOrg("test", "Test Org")
	user, _ := auth.CreateUser(org.ID, &UserCreate{
		Email:    "user@example.com",
		Password: "password",
		Name:     "Test User",
		Role:     RoleEditor,
	})

	t.Run("creates API key successfully", func(t *testing.T) {
		result, err := auth.CreateAPIKey(org.ID, user.ID, "Test Key", []Permission{
			{Resource: ResourceMetrics, Action: ActionRead},
		}, "30d")
		if err != nil {
			t.Fatalf("CreateAPIKey failed: %v", err)
		}

		if result.Key == "" {
			t.Error("key should not be empty")
		}

		if !strings.HasPrefix(result.Key, "dw_") {
			t.Errorf("key should start with 'dw_', got %s", result.Key[:min(10, len(result.Key))])
		}

		if result.Name != "Test Key" {
			t.Errorf("name = %s, want Test Key", result.Name)
		}

		if result.ExpiresAt == nil {
			t.Error("expires_at should be set")
		}
	})

	t.Run("creates non-expiring API key", func(t *testing.T) {
		result, err := auth.CreateAPIKey(org.ID, user.ID, "Permanent Key", nil, "never")
		if err != nil {
			t.Fatalf("CreateAPIKey failed: %v", err)
		}

		if result.ExpiresAt != nil {
			t.Error("expires_at should be nil for non-expiring key")
		}
	})
}

func TestValidateAPIKey(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()
	auth := NewAuth(store)

	org, _ := store.EnsureDefaultOrg("test", "Test Org")
	user, _ := auth.CreateUser(org.ID, &UserCreate{
		Email:    "user@example.com",
		Password: "password",
		Name:     "Test User",
		Role:     RoleEditor,
	})
	keyResult, _ := auth.CreateAPIKey(org.ID, user.ID, "Test Key", nil, "never")

	t.Run("valid API key", func(t *testing.T) {
		apiKey, returnedUser, err := auth.ValidateAPIKey(keyResult.Key)
		if err != nil {
			t.Fatalf("ValidateAPIKey failed: %v", err)
		}

		if apiKey.Name != "Test Key" {
			t.Errorf("name = %s, want Test Key", apiKey.Name)
		}

		if returnedUser.Email != "user@example.com" {
			t.Errorf("email = %s, want user@example.com", returnedUser.Email)
		}
	})

	t.Run("invalid API key", func(t *testing.T) {
		_, _, err := auth.ValidateAPIKey("dw_invalid_key")
		if err != ErrInvalidToken {
			t.Errorf("expected ErrInvalidToken, got %v", err)
		}
	})
}

func TestCheckPermission(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()
	auth := NewAuth(store)

	tests := []struct {
		name     string
		role     Role
		resource string
		action   string
		expected bool
	}{
		// Owner has all permissions
		{"owner can do anything", RoleOwner, ResourceDashboards, ActionDelete, true},
		{"owner can manage users", RoleOwner, ResourceUsers, ActionDelete, true},

		// Admin has most permissions
		{"admin can delete dashboards", RoleAdmin, ResourceDashboards, ActionDelete, true},
		{"admin can manage users", RoleAdmin, ResourceUsers, ActionDelete, true},
		{"admin cannot delete settings", RoleAdmin, ResourceSettings, ActionDelete, false},

		// Editor has limited permissions
		{"editor can create dashboards", RoleEditor, ResourceDashboards, ActionCreate, true},
		{"editor can read users", RoleEditor, ResourceUsers, ActionRead, true},
		{"editor cannot delete users", RoleEditor, ResourceUsers, ActionDelete, false},

		// Viewer has read-only permissions
		{"viewer can read dashboards", RoleViewer, ResourceDashboards, ActionRead, true},
		{"viewer cannot create dashboards", RoleViewer, ResourceDashboards, ActionCreate, false},
		{"viewer cannot delete anything", RoleViewer, ResourceAlerts, ActionDelete, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{Role: tt.role}
			result := auth.CheckPermission(user, tt.resource, tt.action)
			if result != tt.expected {
				t.Errorf("CheckPermission(%s, %s, %s) = %v, want %v",
					tt.role, tt.resource, tt.action, result, tt.expected)
			}
		})
	}
}

func TestCheckAPIKeyPermission(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()
	auth := NewAuth(store)

	tests := []struct {
		name        string
		permissions []Permission
		resource    string
		action      string
		expected    bool
	}{
		{
			name: "exact match",
			permissions: []Permission{
				{Resource: ResourceMetrics, Action: ActionRead},
			},
			resource: ResourceMetrics,
			action:   ActionRead,
			expected: true,
		},
		{
			name: "no match",
			permissions: []Permission{
				{Resource: ResourceMetrics, Action: ActionRead},
			},
			resource: ResourceMetrics,
			action:   ActionCreate, // ActionWrite doesn't exist, use ActionCreate
			expected: false,
		},
		{
			name: "wildcard resource",
			permissions: []Permission{
				{Resource: ResourceAll, Action: ActionRead},
			},
			resource: ResourceDashboards,
			action:   ActionRead,
			expected: true,
		},
		{
			name: "wildcard action",
			permissions: []Permission{
				{Resource: ResourceMetrics, Action: ActionAll},
			},
			resource: ResourceMetrics,
			action:   ActionDelete,
			expected: true,
		},
		{
			name: "full wildcard",
			permissions: []Permission{
				{Resource: ResourceAll, Action: ActionAll},
			},
			resource: ResourceUsers,
			action:   ActionDelete,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := &APIKey{Permissions: tt.permissions}
			result := auth.CheckAPIKeyPermission(key, tt.resource, tt.action)
			if result != tt.expected {
				t.Errorf("CheckAPIKeyPermission() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSecureCompare(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b     string
		expected bool
	}{
		{"same", "same", true},
		{"different", "other", false},
		{"", "", true},
		{"a", "aa", false},
		{"abc", "abd", false},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			result := SecureCompare(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("SecureCompare(%q, %q) = %v, want %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected time.Duration
		hasError bool
	}{
		{"30s", 30 * time.Second, false},
		{"5m", 5 * time.Minute, false},
		{"2h", 2 * time.Hour, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"2w", 2 * 7 * 24 * time.Hour, false},
		{"1y", 365 * 24 * time.Hour, false},
		{"x", 0, true},
		{"", 0, true},
		{"abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := parseDuration(tt.input)
			if tt.hasError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// Helper function to set up a test store
func setupTestStore(t *testing.T) *Store {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "rbac_test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	return store
}

// Benchmarks

func BenchmarkHashPassword(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")
	store, _ := NewStore(dbPath)
	defer store.Close()
	auth := NewAuth(store)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		auth.HashPassword("password123")
	}
}

func BenchmarkCheckPassword(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")
	store, _ := NewStore(dbPath)
	defer store.Close()
	auth := NewAuth(store)
	hash, _ := auth.HashPassword("password123")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		auth.CheckPassword("password123", hash)
	}
}

func BenchmarkGenerateToken(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")
	store, _ := NewStore(dbPath)
	defer store.Close()
	auth := NewAuth(store)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		auth.GenerateToken()
	}
}
