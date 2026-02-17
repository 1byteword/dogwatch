package catalog

import (
	"path/filepath"
	"testing"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(filepath.Join(dir, "catalog.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// ---------- CreateService / GetService ----------

func TestCreateAndGetService(t *testing.T) {
	s := tempStore(t)

	svc := &Service{
		ID:          "svc-1",
		OrgID:       "org-1",
		Name:        "api-gateway",
		DisplayName: "API Gateway",
		Description: "Main entry point",
		Tier:        TierCritical,
		Lifecycle:   LifecycleActive,
		Health:      HealthHealthy,
		TeamID:      "team-1",
		TeamName:    "Platform",
		OwnerEmail:  "platform@example.com",
		Language:    "go",
		Framework:   "stdlib",
		Tags:        []string{"production", "core"},
		Metadata:    map[string]string{"region": "us-east-1", "version": "2.1"},
	}

	if err := s.CreateService(svc); err != nil {
		t.Fatalf("CreateService: %v", err)
	}

	got, err := s.GetService("svc-1")
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil service")
	}
	if got.ID != "svc-1" {
		t.Errorf("ID = %q, want %q", got.ID, "svc-1")
	}
	if got.OrgID != "org-1" {
		t.Errorf("OrgID = %q, want %q", got.OrgID, "org-1")
	}
	if got.Name != "api-gateway" {
		t.Errorf("Name = %q, want %q", got.Name, "api-gateway")
	}
	if got.DisplayName != "API Gateway" {
		t.Errorf("DisplayName = %q, want %q", got.DisplayName, "API Gateway")
	}
	if got.Description != "Main entry point" {
		t.Errorf("Description = %q, want %q", got.Description, "Main entry point")
	}
	if got.Tier != TierCritical {
		t.Errorf("Tier = %q, want %q", got.Tier, TierCritical)
	}
	if got.Lifecycle != LifecycleActive {
		t.Errorf("Lifecycle = %q, want %q", got.Lifecycle, LifecycleActive)
	}
	if got.Health != HealthHealthy {
		t.Errorf("Health = %q, want %q", got.Health, HealthHealthy)
	}
	if got.TeamID != "team-1" {
		t.Errorf("TeamID = %q, want %q", got.TeamID, "team-1")
	}
	if got.TeamName != "Platform" {
		t.Errorf("TeamName = %q, want %q", got.TeamName, "Platform")
	}
	if got.OwnerEmail != "platform@example.com" {
		t.Errorf("OwnerEmail = %q, want %q", got.OwnerEmail, "platform@example.com")
	}
	if got.Language != "go" {
		t.Errorf("Language = %q, want %q", got.Language, "go")
	}

	// Tags round-trip
	if len(got.Tags) != 2 {
		t.Fatalf("Tags len = %d, want 2", len(got.Tags))
	}
	if got.Tags[0] != "production" || got.Tags[1] != "core" {
		t.Errorf("Tags = %v, want [production core]", got.Tags)
	}

	// Metadata round-trip
	if len(got.Metadata) != 2 {
		t.Fatalf("Metadata len = %d, want 2", len(got.Metadata))
	}
	if got.Metadata["region"] != "us-east-1" {
		t.Errorf("Metadata[region] = %q, want %q", got.Metadata["region"], "us-east-1")
	}
	if got.Metadata["version"] != "2.1" {
		t.Errorf("Metadata[version] = %q, want %q", got.Metadata["version"], "2.1")
	}

	// Timestamps should be set
	if got.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}
}

func TestCreateServiceDefaults(t *testing.T) {
	s := tempStore(t)

	svc := &Service{
		ID:    "svc-defaults",
		OrgID: "org-1",
		Name:  "bare-service",
	}

	if err := s.CreateService(svc); err != nil {
		t.Fatalf("CreateService: %v", err)
	}

	got, err := s.GetService("svc-defaults")
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}
	if got.Tier != TierMedium {
		t.Errorf("default Tier = %q, want %q", got.Tier, TierMedium)
	}
	if got.Lifecycle != LifecycleActive {
		t.Errorf("default Lifecycle = %q, want %q", got.Lifecycle, LifecycleActive)
	}
	if got.Health != HealthUnknown {
		t.Errorf("default Health = %q, want %q", got.Health, HealthUnknown)
	}
}

func TestGetServiceNotFound(t *testing.T) {
	s := tempStore(t)

	_, err := s.GetService("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent service")
	}
}

// ---------- GetServiceByName ----------

func TestGetServiceByName(t *testing.T) {
	s := tempStore(t)

	s.CreateService(&Service{ID: "svc-a", OrgID: "org-1", Name: "auth-service"})
	s.CreateService(&Service{ID: "svc-b", OrgID: "org-1", Name: "billing-service"})
	s.CreateService(&Service{ID: "svc-c", OrgID: "org-2", Name: "auth-service"})

	got, err := s.GetServiceByName("org-1", "auth-service")
	if err != nil {
		t.Fatalf("GetServiceByName: %v", err)
	}
	if got.ID != "svc-a" {
		t.Errorf("ID = %q, want %q", got.ID, "svc-a")
	}

	// Same name, different org
	got2, err := s.GetServiceByName("org-2", "auth-service")
	if err != nil {
		t.Fatalf("GetServiceByName org-2: %v", err)
	}
	if got2.ID != "svc-c" {
		t.Errorf("ID = %q, want %q", got2.ID, "svc-c")
	}

	// Not found
	_, err = s.GetServiceByName("org-1", "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent service name")
	}
}

// ---------- ListServices with ServiceFilters ----------

func TestListServicesNoFilters(t *testing.T) {
	s := tempStore(t)

	s.CreateService(&Service{ID: "svc-1", OrgID: "org-1", Name: "svc-a"})
	s.CreateService(&Service{ID: "svc-2", OrgID: "org-1", Name: "svc-b"})
	s.CreateService(&Service{ID: "svc-3", OrgID: "org-2", Name: "svc-c"})

	list, err := s.ListServices("org-1", ServiceFilters{})
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 services for org-1, got %d", len(list))
	}
}

func TestListServicesFilterByTeamID(t *testing.T) {
	s := tempStore(t)

	s.CreateService(&Service{ID: "svc-1", OrgID: "org-1", Name: "svc-a", TeamID: "team-1"})
	s.CreateService(&Service{ID: "svc-2", OrgID: "org-1", Name: "svc-b", TeamID: "team-2"})
	s.CreateService(&Service{ID: "svc-3", OrgID: "org-1", Name: "svc-c", TeamID: "team-1"})

	list, err := s.ListServices("org-1", ServiceFilters{TeamID: "team-1"})
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 services for team-1, got %d", len(list))
	}
}

func TestListServicesFilterByTier(t *testing.T) {
	s := tempStore(t)

	s.CreateService(&Service{ID: "svc-1", OrgID: "org-1", Name: "svc-a", Tier: TierCritical})
	s.CreateService(&Service{ID: "svc-2", OrgID: "org-1", Name: "svc-b", Tier: TierLow})
	s.CreateService(&Service{ID: "svc-3", OrgID: "org-1", Name: "svc-c", Tier: TierCritical})

	list, err := s.ListServices("org-1", ServiceFilters{Tier: TierCritical})
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 critical services, got %d", len(list))
	}
}

func TestListServicesFilterByLifecycle(t *testing.T) {
	s := tempStore(t)

	s.CreateService(&Service{ID: "svc-1", OrgID: "org-1", Name: "svc-a", Lifecycle: LifecycleActive})
	s.CreateService(&Service{ID: "svc-2", OrgID: "org-1", Name: "svc-b", Lifecycle: LifecycleDeprecated})

	list, err := s.ListServices("org-1", ServiceFilters{Lifecycle: LifecycleDeprecated})
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 deprecated service, got %d", len(list))
	}
	if list[0].ID != "svc-2" {
		t.Errorf("ID = %q, want %q", list[0].ID, "svc-2")
	}
}

func TestListServicesFilterByHealth(t *testing.T) {
	s := tempStore(t)

	s.CreateService(&Service{ID: "svc-1", OrgID: "org-1", Name: "svc-a", Health: HealthHealthy})
	s.CreateService(&Service{ID: "svc-2", OrgID: "org-1", Name: "svc-b", Health: HealthDegraded})
	s.CreateService(&Service{ID: "svc-3", OrgID: "org-1", Name: "svc-c", Health: HealthUnhealthy})

	list, err := s.ListServices("org-1", ServiceFilters{Health: HealthDegraded})
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 degraded service, got %d", len(list))
	}
	if list[0].ID != "svc-2" {
		t.Errorf("ID = %q, want %q", list[0].ID, "svc-2")
	}
}

// ---------- UpdateService ----------

func TestUpdateService(t *testing.T) {
	s := tempStore(t)

	svc := &Service{
		ID:          "svc-1",
		OrgID:       "org-1",
		Name:        "api",
		DisplayName: "API",
		Tier:        TierMedium,
		Health:      HealthUnknown,
		Tags:        []string{"v1"},
		Metadata:    map[string]string{"env": "staging"},
	}
	if err := s.CreateService(svc); err != nil {
		t.Fatalf("CreateService: %v", err)
	}

	// Modify fields
	svc.DisplayName = "API v2"
	svc.Tier = TierCritical
	svc.Health = HealthHealthy
	svc.Tags = []string{"v2", "production"}
	svc.Metadata = map[string]string{"env": "production", "region": "eu-west-1"}
	svc.OwnerEmail = "new-owner@example.com"

	if err := s.UpdateService(svc); err != nil {
		t.Fatalf("UpdateService: %v", err)
	}

	got, err := s.GetService("svc-1")
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}
	if got.DisplayName != "API v2" {
		t.Errorf("DisplayName = %q, want %q", got.DisplayName, "API v2")
	}
	if got.Tier != TierCritical {
		t.Errorf("Tier = %q, want %q", got.Tier, TierCritical)
	}
	if got.Health != HealthHealthy {
		t.Errorf("Health = %q, want %q", got.Health, HealthHealthy)
	}
	if got.OwnerEmail != "new-owner@example.com" {
		t.Errorf("OwnerEmail = %q, want %q", got.OwnerEmail, "new-owner@example.com")
	}
	if len(got.Tags) != 2 || got.Tags[0] != "v2" {
		t.Errorf("Tags = %v, want [v2 production]", got.Tags)
	}
	if got.Metadata["region"] != "eu-west-1" {
		t.Errorf("Metadata[region] = %q, want %q", got.Metadata["region"], "eu-west-1")
	}
}

// ---------- DeleteService ----------

func TestDeleteService(t *testing.T) {
	s := tempStore(t)

	svc := &Service{ID: "svc-del", OrgID: "org-1", Name: "to-delete"}
	s.CreateService(svc)

	// Add a dependency involving this service
	s.AddDependency(&Dependency{
		ID:            "dep-1",
		SourceService: "svc-del",
		TargetService: "svc-other",
	})

	// Add a runbook for this service
	s.CreateRunbook(&Runbook{
		ID:        "rb-1",
		ServiceID: "svc-del",
		Title:     "Restart Procedure",
		Content:   "Step 1: restart",
	})

	if err := s.DeleteService("svc-del"); err != nil {
		t.Fatalf("DeleteService: %v", err)
	}

	// Service should be gone
	_, err := s.GetService("svc-del")
	if err == nil {
		t.Error("expected error after deleting service")
	}

	// Dependencies should be cascaded
	upstream, downstream, err := s.GetDependencies("svc-del")
	if err != nil {
		t.Fatalf("GetDependencies: %v", err)
	}
	if len(upstream) != 0 {
		t.Errorf("expected 0 upstream deps after delete, got %d", len(upstream))
	}
	if len(downstream) != 0 {
		t.Errorf("expected 0 downstream deps after delete, got %d", len(downstream))
	}

	// Runbooks should be cascaded
	runbooks, err := s.GetRunbooksForService("svc-del")
	if err != nil {
		t.Fatalf("GetRunbooksForService: %v", err)
	}
	if len(runbooks) != 0 {
		t.Errorf("expected 0 runbooks after delete, got %d", len(runbooks))
	}
}

// ---------- UpdateServiceHealth ----------

func TestUpdateServiceHealth(t *testing.T) {
	s := tempStore(t)

	svc := &Service{ID: "svc-health", OrgID: "org-1", Name: "health-test", Health: HealthUnknown}
	s.CreateService(svc)

	if err := s.UpdateServiceHealth("svc-health", HealthDegraded); err != nil {
		t.Fatalf("UpdateServiceHealth: %v", err)
	}

	got, err := s.GetService("svc-health")
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}
	if got.Health != HealthDegraded {
		t.Errorf("Health = %q, want %q", got.Health, HealthDegraded)
	}

	// Update again to unhealthy
	s.UpdateServiceHealth("svc-health", HealthUnhealthy)
	got, _ = s.GetService("svc-health")
	if got.Health != HealthUnhealthy {
		t.Errorf("Health = %q, want %q", got.Health, HealthUnhealthy)
	}
}

// ---------- AddDependency / GetDependencies ----------

func TestAddAndGetDependencies(t *testing.T) {
	s := tempStore(t)

	s.CreateService(&Service{ID: "svc-a", OrgID: "org-1", Name: "frontend"})
	s.CreateService(&Service{ID: "svc-b", OrgID: "org-1", Name: "backend"})
	s.CreateService(&Service{ID: "svc-c", OrgID: "org-1", Name: "database"})

	// frontend -> backend (sync)
	dep1 := &Dependency{
		ID:             "dep-1",
		SourceService:  "svc-a",
		TargetService:  "svc-b",
		DependencyType: DepTypeSync,
		IsAutoDetected: false,
		Confidence:     1.0,
	}
	if err := s.AddDependency(dep1); err != nil {
		t.Fatalf("AddDependency dep-1: %v", err)
	}

	// backend -> database (database)
	dep2 := &Dependency{
		ID:             "dep-2",
		SourceService:  "svc-b",
		TargetService:  "svc-c",
		DependencyType: DepTypeDatabase,
		IsAutoDetected: true,
		Confidence:     0.85,
	}
	if err := s.AddDependency(dep2); err != nil {
		t.Fatalf("AddDependency dep-2: %v", err)
	}

	// Check backend's dependencies
	upstream, downstream, err := s.GetDependencies("svc-b")
	if err != nil {
		t.Fatalf("GetDependencies: %v", err)
	}

	// backend depends on database (upstream from backend's perspective)
	if len(upstream) != 1 {
		t.Fatalf("expected 1 upstream dep for backend, got %d", len(upstream))
	}
	if upstream[0].TargetService != "svc-c" {
		t.Errorf("upstream target = %q, want %q", upstream[0].TargetService, "svc-c")
	}
	if upstream[0].DependencyType != DepTypeDatabase {
		t.Errorf("upstream type = %q, want %q", upstream[0].DependencyType, DepTypeDatabase)
	}
	if upstream[0].IsAutoDetected != true {
		t.Errorf("upstream IsAutoDetected = %v, want true", upstream[0].IsAutoDetected)
	}
	if upstream[0].Confidence != 0.85 {
		t.Errorf("upstream Confidence = %f, want 0.85", upstream[0].Confidence)
	}

	// frontend depends on backend (downstream from backend's perspective)
	if len(downstream) != 1 {
		t.Fatalf("expected 1 downstream dep for backend, got %d", len(downstream))
	}
	if downstream[0].SourceService != "svc-a" {
		t.Errorf("downstream source = %q, want %q", downstream[0].SourceService, "svc-a")
	}

	// Timestamps should be set
	if upstream[0].CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set on dependency")
	}
	if upstream[0].LastSeenAt.IsZero() {
		t.Error("expected LastSeenAt to be set on dependency")
	}
}

func TestAddDependencyDefaultType(t *testing.T) {
	s := tempStore(t)

	dep := &Dependency{
		ID:            "dep-default",
		SourceService: "svc-x",
		TargetService: "svc-y",
		// DependencyType intentionally omitted
	}
	if err := s.AddDependency(dep); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}

	upstream, _, err := s.GetDependencies("svc-x")
	if err != nil {
		t.Fatalf("GetDependencies: %v", err)
	}
	if len(upstream) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(upstream))
	}
	if upstream[0].DependencyType != DepTypeSync {
		t.Errorf("DependencyType = %q, want %q (default)", upstream[0].DependencyType, DepTypeSync)
	}
}

// ---------- AddDependency upsert ----------

func TestAddDependencyUpsert(t *testing.T) {
	s := tempStore(t)

	// First insert with confidence 0.5
	dep1 := &Dependency{
		ID:             "dep-first",
		SourceService:  "svc-a",
		TargetService:  "svc-b",
		DependencyType: DepTypeSync,
		Confidence:     0.5,
	}
	if err := s.AddDependency(dep1); err != nil {
		t.Fatalf("AddDependency first: %v", err)
	}

	// Second insert with same source+target but higher confidence
	dep2 := &Dependency{
		ID:             "dep-second",
		SourceService:  "svc-a",
		TargetService:  "svc-b",
		DependencyType: DepTypeAsync,
		Confidence:     0.9,
	}
	if err := s.AddDependency(dep2); err != nil {
		t.Fatalf("AddDependency upsert: %v", err)
	}

	// Should still be one dependency (upserted, not duplicated)
	upstream, _, err := s.GetDependencies("svc-a")
	if err != nil {
		t.Fatalf("GetDependencies: %v", err)
	}
	if len(upstream) != 1 {
		t.Fatalf("expected 1 dep after upsert, got %d", len(upstream))
	}

	// Confidence should be MAX(0.5, 0.9) = 0.9
	if upstream[0].Confidence != 0.9 {
		t.Errorf("Confidence = %f, want 0.9 (MAX of old and new)", upstream[0].Confidence)
	}
}

func TestAddDependencyUpsertKeepsHigherConfidence(t *testing.T) {
	s := tempStore(t)

	// First insert with high confidence
	s.AddDependency(&Dependency{
		ID:            "dep-high",
		SourceService: "svc-x",
		TargetService: "svc-y",
		Confidence:    0.95,
	})

	// Upsert with lower confidence should keep the higher one
	s.AddDependency(&Dependency{
		ID:            "dep-low",
		SourceService: "svc-x",
		TargetService: "svc-y",
		Confidence:    0.6,
	})

	upstream, _, _ := s.GetDependencies("svc-x")
	if len(upstream) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(upstream))
	}
	if upstream[0].Confidence != 0.95 {
		t.Errorf("Confidence = %f, want 0.95 (should keep higher)", upstream[0].Confidence)
	}
}

// ---------- CreateTeam / GetTeam / ListTeams ----------

func TestCreateAndGetTeam(t *testing.T) {
	s := tempStore(t)

	team := &Team{
		ID:           "team-1",
		OrgID:        "org-1",
		Name:         "Platform Engineering",
		Description:  "Core platform team",
		SlackChannel: "#platform",
		Email:        "platform@example.com",
		Members:      []string{"alice", "bob", "charlie"},
	}

	if err := s.CreateTeam(team); err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}

	got, err := s.GetTeam("team-1")
	if err != nil {
		t.Fatalf("GetTeam: %v", err)
	}
	if got.Name != "Platform Engineering" {
		t.Errorf("Name = %q, want %q", got.Name, "Platform Engineering")
	}
	if got.Description != "Core platform team" {
		t.Errorf("Description = %q, want %q", got.Description, "Core platform team")
	}
	if got.SlackChannel != "#platform" {
		t.Errorf("SlackChannel = %q, want %q", got.SlackChannel, "#platform")
	}
	if got.Email != "platform@example.com" {
		t.Errorf("Email = %q, want %q", got.Email, "platform@example.com")
	}

	// Members JSON round-trip
	if len(got.Members) != 3 {
		t.Fatalf("Members len = %d, want 3", len(got.Members))
	}
	if got.Members[0] != "alice" || got.Members[1] != "bob" || got.Members[2] != "charlie" {
		t.Errorf("Members = %v, want [alice bob charlie]", got.Members)
	}

	if got.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestGetTeamNotFound(t *testing.T) {
	s := tempStore(t)

	_, err := s.GetTeam("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent team")
	}
}

func TestListTeams(t *testing.T) {
	s := tempStore(t)

	s.CreateTeam(&Team{ID: "team-1", OrgID: "org-1", Name: "Backend"})
	s.CreateTeam(&Team{ID: "team-2", OrgID: "org-1", Name: "Frontend"})
	s.CreateTeam(&Team{ID: "team-3", OrgID: "org-2", Name: "DevOps"})

	teams, err := s.ListTeams("org-1")
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
	if len(teams) != 2 {
		t.Errorf("expected 2 teams for org-1, got %d", len(teams))
	}

	// Should be ordered by name
	if teams[0].Name != "Backend" {
		t.Errorf("first team = %q, want %q (ordered by name)", teams[0].Name, "Backend")
	}
	if teams[1].Name != "Frontend" {
		t.Errorf("second team = %q, want %q (ordered by name)", teams[1].Name, "Frontend")
	}
}

// ---------- CreateRunbook / GetRunbooksForService ----------

func TestCreateAndGetRunbooks(t *testing.T) {
	s := tempStore(t)

	s.CreateService(&Service{ID: "svc-1", OrgID: "org-1", Name: "api"})

	rb := &Runbook{
		ID:        "rb-1",
		ServiceID: "svc-1",
		Title:     "Emergency Restart",
		Content:   "# Step 1\nRestart the service",
		AlertTags: []string{"high-cpu", "oom"},
	}
	if err := s.CreateRunbook(rb); err != nil {
		t.Fatalf("CreateRunbook: %v", err)
	}

	rb2 := &Runbook{
		ID:        "rb-2",
		ServiceID: "svc-1",
		Title:     "Database Recovery",
		Content:   "# DB Recovery\nCheck replication",
		AlertTags: []string{"db-down"},
	}
	s.CreateRunbook(rb2)

	runbooks, err := s.GetRunbooksForService("svc-1")
	if err != nil {
		t.Fatalf("GetRunbooksForService: %v", err)
	}
	if len(runbooks) != 2 {
		t.Fatalf("expected 2 runbooks, got %d", len(runbooks))
	}

	// Ordered by title
	if runbooks[0].Title != "Database Recovery" {
		t.Errorf("first runbook = %q, want %q (ordered by title)", runbooks[0].Title, "Database Recovery")
	}
	if runbooks[1].Title != "Emergency Restart" {
		t.Errorf("second runbook = %q, want %q", runbooks[1].Title, "Emergency Restart")
	}

	// Verify content and alert tags
	if runbooks[1].Content != "# Step 1\nRestart the service" {
		t.Errorf("Content = %q, want markdown content", runbooks[1].Content)
	}
	if len(runbooks[1].AlertTags) != 2 || runbooks[1].AlertTags[0] != "high-cpu" {
		t.Errorf("AlertTags = %v, want [high-cpu oom]", runbooks[1].AlertTags)
	}
	if runbooks[1].CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set on runbook")
	}
}

func TestGetRunbooksForServiceEmpty(t *testing.T) {
	s := tempStore(t)

	runbooks, err := s.GetRunbooksForService("nonexistent")
	if err != nil {
		t.Fatalf("GetRunbooksForService: %v", err)
	}
	if len(runbooks) != 0 {
		t.Errorf("expected 0 runbooks for nonexistent service, got %d", len(runbooks))
	}
}

// ---------- FindRunbookByAlertTags ----------

func TestFindRunbookByAlertTags(t *testing.T) {
	s := tempStore(t)

	// Create services and runbooks in org-1
	s.CreateService(&Service{ID: "svc-1", OrgID: "org-1", Name: "api"})
	s.CreateService(&Service{ID: "svc-2", OrgID: "org-1", Name: "worker"})

	s.CreateRunbook(&Runbook{
		ID:        "rb-1",
		ServiceID: "svc-1",
		Title:     "CPU Runbook",
		Content:   "fix cpu",
		AlertTags: []string{"high-cpu", "throttling"},
	})
	s.CreateRunbook(&Runbook{
		ID:        "rb-2",
		ServiceID: "svc-1",
		Title:     "Memory Runbook",
		Content:   "fix memory",
		AlertTags: []string{"oom", "memory-pressure"},
	})
	s.CreateRunbook(&Runbook{
		ID:        "rb-3",
		ServiceID: "svc-2",
		Title:     "Queue Runbook",
		Content:   "fix queue",
		AlertTags: []string{"queue-lag", "high-cpu"},
	})

	// Search for "high-cpu" should match rb-1 and rb-3
	matches, err := s.FindRunbookByAlertTags("org-1", []string{"high-cpu"})
	if err != nil {
		t.Fatalf("FindRunbookByAlertTags: %v", err)
	}
	if len(matches) != 2 {
		t.Errorf("expected 2 matches for high-cpu, got %d", len(matches))
	}

	// Search for "oom" should match only rb-2
	matches, err = s.FindRunbookByAlertTags("org-1", []string{"oom"})
	if err != nil {
		t.Fatalf("FindRunbookByAlertTags: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for oom, got %d", len(matches))
	}
	if matches[0].Title != "Memory Runbook" {
		t.Errorf("match title = %q, want %q", matches[0].Title, "Memory Runbook")
	}

	// Search for nonexistent tag should return empty
	matches, err = s.FindRunbookByAlertTags("org-1", []string{"nonexistent-tag"})
	if err != nil {
		t.Fatalf("FindRunbookByAlertTags: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for nonexistent tag, got %d", len(matches))
	}

	// Search across multiple tags should match any
	matches, err = s.FindRunbookByAlertTags("org-1", []string{"oom", "queue-lag"})
	if err != nil {
		t.Fatalf("FindRunbookByAlertTags: %v", err)
	}
	if len(matches) != 2 {
		t.Errorf("expected 2 matches for oom+queue-lag, got %d", len(matches))
	}
}

// ---------- GetServiceGraph ----------

func TestGetServiceGraph(t *testing.T) {
	s := tempStore(t)

	s.CreateService(&Service{ID: "svc-1", OrgID: "org-1", Name: "frontend"})
	s.CreateService(&Service{ID: "svc-2", OrgID: "org-1", Name: "backend"})
	s.CreateService(&Service{ID: "svc-3", OrgID: "org-1", Name: "db"})

	s.AddDependency(&Dependency{ID: "dep-1", SourceService: "svc-1", TargetService: "svc-2", DependencyType: DepTypeSync})
	s.AddDependency(&Dependency{ID: "dep-2", SourceService: "svc-2", TargetService: "svc-3", DependencyType: DepTypeDatabase})

	graph, err := s.GetServiceGraph("org-1")
	if err != nil {
		t.Fatalf("GetServiceGraph: %v", err)
	}
	if len(graph.Services) != 3 {
		t.Errorf("expected 3 services in graph, got %d", len(graph.Services))
	}
	if len(graph.Dependencies) != 2 {
		t.Errorf("expected 2 dependencies in graph, got %d", len(graph.Dependencies))
	}
}

func TestGetServiceGraphEmpty(t *testing.T) {
	s := tempStore(t)

	graph, err := s.GetServiceGraph("org-empty")
	if err != nil {
		t.Fatalf("GetServiceGraph: %v", err)
	}
	if len(graph.Services) != 0 {
		t.Errorf("expected 0 services, got %d", len(graph.Services))
	}
	if len(graph.Dependencies) != 0 {
		t.Errorf("expected 0 dependencies, got %d", len(graph.Dependencies))
	}
}

// ---------- GetServiceStats ----------

func TestGetServiceStats(t *testing.T) {
	s := tempStore(t)

	s.CreateService(&Service{ID: "svc-1", OrgID: "org-1", Name: "a", Tier: TierCritical, Health: HealthHealthy})
	s.CreateService(&Service{ID: "svc-2", OrgID: "org-1", Name: "b", Tier: TierCritical, Health: HealthDegraded})
	s.CreateService(&Service{ID: "svc-3", OrgID: "org-1", Name: "c", Tier: TierHigh, Health: HealthHealthy})
	s.CreateService(&Service{ID: "svc-4", OrgID: "org-1", Name: "d", Tier: TierMedium, Health: HealthUnhealthy})
	s.CreateService(&Service{ID: "svc-5", OrgID: "org-1", Name: "e", Tier: TierLow, Health: HealthHealthy})

	// Service in different org should not be counted
	s.CreateService(&Service{ID: "svc-6", OrgID: "org-2", Name: "f", Tier: TierCritical, Health: HealthHealthy})

	stats, err := s.GetServiceStats("org-1")
	if err != nil {
		t.Fatalf("GetServiceStats: %v", err)
	}

	if stats.Total != 5 {
		t.Errorf("Total = %d, want 5", stats.Total)
	}
	if stats.Critical != 2 {
		t.Errorf("Critical = %d, want 2", stats.Critical)
	}
	if stats.High != 1 {
		t.Errorf("High = %d, want 1", stats.High)
	}
	if stats.Medium != 1 {
		t.Errorf("Medium = %d, want 1", stats.Medium)
	}
	if stats.Low != 1 {
		t.Errorf("Low = %d, want 1", stats.Low)
	}
	if stats.Healthy != 3 {
		t.Errorf("Healthy = %d, want 3", stats.Healthy)
	}
	if stats.Degraded != 1 {
		t.Errorf("Degraded = %d, want 1", stats.Degraded)
	}
	if stats.Unhealthy != 1 {
		t.Errorf("Unhealthy = %d, want 1", stats.Unhealthy)
	}
}

func TestGetServiceStatsEmpty(t *testing.T) {
	s := tempStore(t)

	stats, err := s.GetServiceStats("org-empty")
	if err != nil {
		t.Fatalf("GetServiceStats: %v", err)
	}
	if stats.Total != 0 {
		t.Errorf("Total = %d, want 0", stats.Total)
	}
}

// ---------- formatServiceName ----------

func TestFormatServiceName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"my-service", "My Service"},
		{"my_service", "My Service"},
		{"api-gateway-v2", "Api Gateway V2"},
		{"single", "Single"},
		{"ALLCAPS", "Allcaps"},
		{"mixed-with_both", "Mixed With Both"},
		{"", ""},
	}

	for _, tt := range tests {
		got := formatServiceName(tt.input)
		if got != tt.want {
			t.Errorf("formatServiceName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
