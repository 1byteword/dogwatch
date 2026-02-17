package incidents

import (
	"os"
	"path/filepath"
	"testing"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(filepath.Join(dir, "incidents.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCreateAndGetIncident(t *testing.T) {
	s := tempStore(t)

	inc := &Incident{
		Title:    "CPU on fire",
		Severity: SeverityCritical,
		Service:  "api-server",
		Source:   "watch",
		SourceID: "rule-1",
		Tags:     map[string]string{"env": "prod"},
	}

	if err := s.CreateIncident(inc); err != nil {
		t.Fatalf("CreateIncident: %v", err)
	}

	if inc.ID == "" {
		t.Fatal("expected ID to be populated")
	}
	if inc.Status != StatusTriggered {
		t.Errorf("expected status triggered, got %s", inc.Status)
	}
	if inc.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	got, err := s.GetIncident(inc.ID)
	if err != nil {
		t.Fatalf("GetIncident: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil incident")
	}
	if got.Title != "CPU on fire" {
		t.Errorf("title = %q, want %q", got.Title, "CPU on fire")
	}
	if got.Severity != SeverityCritical {
		t.Errorf("severity = %q, want %q", got.Severity, SeverityCritical)
	}
	if got.Tags["env"] != "prod" {
		t.Errorf("tags[env] = %q, want %q", got.Tags["env"], "prod")
	}
	if len(got.Timeline) != 1 || got.Timeline[0].Type != "created" {
		t.Errorf("expected 1 'created' timeline event, got %d events", len(got.Timeline))
	}
}

func TestListIncidents(t *testing.T) {
	s := tempStore(t)

	// Create 3 incidents with different statuses
	s.CreateIncident(&Incident{Title: "inc-1", Severity: SeverityCritical})
	s.CreateIncident(&Incident{Title: "inc-2", Severity: SeverityHigh})
	s.CreateIncident(&Incident{Title: "inc-3", Severity: SeverityLow})

	all, err := s.ListIncidents("", 10)
	if err != nil {
		t.Fatalf("ListIncidents: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 incidents, got %d", len(all))
	}

	triggered, err := s.ListIncidents("triggered", 10)
	if err != nil {
		t.Fatalf("ListIncidents(triggered): %v", err)
	}
	if len(triggered) != 3 {
		t.Errorf("expected 3 triggered, got %d", len(triggered))
	}
}

func TestAcknowledgeIncident(t *testing.T) {
	s := tempStore(t)

	inc := &Incident{Title: "disk full", Severity: SeverityHigh}
	s.CreateIncident(inc)

	if err := s.AcknowledgeIncident(inc.ID, "alice"); err != nil {
		t.Fatalf("AcknowledgeIncident: %v", err)
	}

	got, _ := s.GetIncident(inc.ID)
	if got.Status != StatusAcknowledged {
		t.Errorf("status = %q, want %q", got.Status, StatusAcknowledged)
	}
	if got.AckedBy != "alice" {
		t.Errorf("acked_by = %q, want %q", got.AckedBy, "alice")
	}
	if got.AckedAt == nil {
		t.Error("expected acked_at to be set")
	}
	if got.AssignedTo != "alice" {
		t.Errorf("assigned_to = %q, want %q", got.AssignedTo, "alice")
	}

	// Check timeline
	found := false
	for _, ev := range got.Timeline {
		if ev.Type == "acknowledged" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'acknowledged' timeline event")
	}
}

func TestResolveIncident(t *testing.T) {
	s := tempStore(t)

	inc := &Incident{Title: "OOM killer", Severity: SeverityCritical}
	s.CreateIncident(inc)
	s.AcknowledgeIncident(inc.ID, "bob")
	s.ResolveIncident(inc.ID, "bob", "increased memory limit")

	got, _ := s.GetIncident(inc.ID)
	if got.Status != StatusResolved {
		t.Errorf("status = %q, want %q", got.Status, StatusResolved)
	}
	if got.ResolvedBy != "bob" {
		t.Errorf("resolved_by = %q, want %q", got.ResolvedBy, "bob")
	}
	if got.ResolvedAt == nil {
		t.Error("expected resolved_at to be set")
	}
}

func TestEscalateIncident(t *testing.T) {
	s := tempStore(t)

	inc := &Incident{Title: "latency spike", Severity: SeverityHigh}
	s.CreateIncident(inc)

	if err := s.EscalateIncident(inc.ID, 2, "senior-oncall"); err != nil {
		t.Fatalf("EscalateIncident: %v", err)
	}

	got, _ := s.GetIncident(inc.ID)
	if got.EscLevel != 2 {
		t.Errorf("escalation_level = %d, want 2", got.EscLevel)
	}
	if got.AssignedTo != "senior-oncall" {
		t.Errorf("assigned_to = %q, want %q", got.AssignedTo, "senior-oncall")
	}
}

func TestListActiveIncidents(t *testing.T) {
	s := tempStore(t)

	s.CreateIncident(&Incident{Title: "active-1", Severity: SeverityCritical})
	inc2 := &Incident{Title: "active-2", Severity: SeverityHigh}
	s.CreateIncident(inc2)
	s.ResolveIncident(inc2.ID, "admin", "false alarm")

	active, err := s.ListActiveIncidents()
	if err != nil {
		t.Fatalf("ListActiveIncidents: %v", err)
	}
	if len(active) != 1 {
		t.Errorf("expected 1 active incident, got %d", len(active))
	}
}

func TestAddNote(t *testing.T) {
	s := tempStore(t)

	inc := &Incident{Title: "investigation", Severity: SeverityMedium}
	s.CreateIncident(inc)

	if err := s.AddNote(inc.ID, "alice", "looks like a config issue"); err != nil {
		t.Fatalf("AddNote: %v", err)
	}

	got, _ := s.GetIncident(inc.ID)
	found := false
	for _, ev := range got.Timeline {
		if ev.Type == "note" && ev.Message == "looks like a config issue" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'note' timeline event")
	}
}

func TestCreateOrDedup_NewIncident(t *testing.T) {
	s := tempStore(t)

	inc := &Incident{
		Title:    "high CPU",
		DedupKey: "cpu-alert-prod",
		Severity: SeverityCritical,
	}

	result, existed, err := s.CreateOrDedup(inc)
	if err != nil {
		t.Fatalf("CreateOrDedup: %v", err)
	}
	if existed {
		t.Error("expected existed=false for new incident")
	}
	if result.ID == "" {
		t.Error("expected ID to be assigned")
	}
	if result.DedupKey != "cpu-alert-prod" {
		t.Errorf("dedup_key = %q, want %q", result.DedupKey, "cpu-alert-prod")
	}
}

func TestCreateOrDedup_Deduplicates(t *testing.T) {
	s := tempStore(t)

	// Create first incident
	first := &Incident{
		Title:    "high CPU",
		DedupKey: "cpu-alert-prod",
		Severity: SeverityCritical,
		Source:   "watch",
		SourceID: "rule-1",
	}
	result1, existed1, err := s.CreateOrDedup(first)
	if err != nil {
		t.Fatalf("CreateOrDedup first: %v", err)
	}
	if existed1 {
		t.Error("first call should not find existing")
	}

	// Second incident with same dedup key should merge
	second := &Incident{
		Title:    "high CPU (again)",
		DedupKey: "cpu-alert-prod",
		Severity: SeverityCritical,
		Source:   "watch",
		SourceID: "rule-1",
	}
	result2, existed2, err := s.CreateOrDedup(second)
	if err != nil {
		t.Fatalf("CreateOrDedup second: %v", err)
	}
	if !existed2 {
		t.Error("second call should find existing")
	}
	if result2.ID != result1.ID {
		t.Errorf("expected same ID %q, got %q", result1.ID, result2.ID)
	}

	// Verify dedup timeline event was added
	got, _ := s.GetIncident(result1.ID)
	foundDedup := false
	for _, ev := range got.Timeline {
		if ev.Type == "dedup" {
			foundDedup = true
		}
	}
	if !foundDedup {
		t.Error("expected 'dedup' timeline event")
	}
}

func TestCreateOrDedup_ResolvedAllowsNew(t *testing.T) {
	s := tempStore(t)

	// Create and resolve an incident
	first := &Incident{
		Title:    "disk full",
		DedupKey: "disk-prod",
		Severity: SeverityHigh,
	}
	s.CreateOrDedup(first)
	s.ResolveIncident(first.ID, "admin", "expanded disk")

	// New incident with same dedup key should create a new one
	second := &Incident{
		Title:    "disk full again",
		DedupKey: "disk-prod",
		Severity: SeverityHigh,
	}
	result, existed, err := s.CreateOrDedup(second)
	if err != nil {
		t.Fatalf("CreateOrDedup after resolve: %v", err)
	}
	if existed {
		t.Error("should create new incident after resolution")
	}
	if result.ID == first.ID {
		t.Error("should have new ID after resolution")
	}
}

func TestCreateOrDedup_NoDedupKeyCreatesAlways(t *testing.T) {
	s := tempStore(t)

	inc1 := &Incident{Title: "no-dedup-1", Severity: SeverityLow}
	inc2 := &Incident{Title: "no-dedup-2", Severity: SeverityLow}

	r1, _, _ := s.CreateOrDedup(inc1)
	r2, _, _ := s.CreateOrDedup(inc2)

	if r1.ID == r2.ID {
		t.Error("incidents without dedup key should have different IDs")
	}
}

func TestGetStats(t *testing.T) {
	s := tempStore(t)

	s.CreateIncident(&Incident{Title: "i1", Severity: SeverityCritical, Service: "api"})
	s.CreateIncident(&Incident{Title: "i2", Severity: SeverityHigh, Service: "api"})
	inc3 := &Incident{Title: "i3", Severity: SeverityLow, Service: "web"}
	s.CreateIncident(inc3)
	s.AcknowledgeIncident(inc3.ID, "alice")
	s.ResolveIncident(inc3.ID, "alice", "fixed")

	stats, err := s.GetStats()
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.TotalIncidents != 3 {
		t.Errorf("total = %d, want 3", stats.TotalIncidents)
	}
	if stats.ActiveIncidents != 2 {
		t.Errorf("active = %d, want 2", stats.ActiveIncidents)
	}
	if stats.TriggeredCount != 2 {
		t.Errorf("triggered = %d, want 2", stats.TriggeredCount)
	}
}

func TestScheduleCRUD(t *testing.T) {
	s := tempStore(t)

	sched := &OnCallSchedule{
		Name:     "primary",
		Timezone: "America/New_York",
		Rotations: []Rotation{
			{ID: "r1", Name: "weekly", Type: "weekly", Users: []string{"alice", "bob", "charlie"}},
		},
	}

	if err := s.CreateSchedule(sched); err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}
	if sched.ID == "" {
		t.Error("expected ID to be assigned")
	}

	got, err := s.GetSchedule(sched.ID)
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}
	if got.Name != "primary" {
		t.Errorf("name = %q, want %q", got.Name, "primary")
	}
	if len(got.Rotations) != 1 {
		t.Fatalf("expected 1 rotation, got %d", len(got.Rotations))
	}
	if len(got.Rotations[0].Users) != 3 {
		t.Errorf("expected 3 users, got %d", len(got.Rotations[0].Users))
	}

	all, err := s.ListSchedules()
	if err != nil {
		t.Fatalf("ListSchedules: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected 1 schedule, got %d", len(all))
	}

	if err := s.DeleteSchedule(sched.ID); err != nil {
		t.Fatalf("DeleteSchedule: %v", err)
	}
	deleted, _ := s.GetSchedule(sched.ID)
	if deleted != nil {
		t.Error("expected nil after delete")
	}
}

func TestPolicyCRUD(t *testing.T) {
	s := tempStore(t)

	policy := &EscalationPolicy{
		Name: "default",
		Rules: []EscalationRule{
			{Level: 1, DelayMinutes: 5, Targets: []Target{{Type: "schedule", ID: "primary"}}},
			{Level: 2, DelayMinutes: 15, Targets: []Target{{Type: "user", ID: "manager"}}},
		},
		RepeatAfter: 30,
	}

	if err := s.CreatePolicy(policy); err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}

	got, err := s.GetPolicy(policy.ID)
	if err != nil {
		t.Fatalf("GetPolicy: %v", err)
	}
	if got.Name != "default" {
		t.Errorf("name = %q, want %q", got.Name, "default")
	}
	if len(got.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(got.Rules))
	}
	if got.RepeatAfter != 30 {
		t.Errorf("repeat_after = %d, want 30", got.RepeatAfter)
	}

	all, err := s.ListPolicies()
	if err != nil {
		t.Fatalf("ListPolicies: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected 1 policy, got %d", len(all))
	}
}

func TestNotificationLog(t *testing.T) {
	s := tempStore(t)

	inc := &Incident{Title: "test", Severity: SeverityLow}
	s.CreateIncident(inc)

	notif := &NotificationLog{
		IncidentID: inc.ID,
		Channel:    "slack",
		Target:     "#oncall",
		Status:     "sent",
		Message:    "Incident: test",
	}

	if err := s.LogNotification(notif); err != nil {
		t.Fatalf("LogNotification: %v", err)
	}

	logs, err := s.GetNotificationLogs(inc.ID)
	if err != nil {
		t.Fatalf("GetNotificationLogs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].Channel != "slack" {
		t.Errorf("channel = %q, want %q", logs[0].Channel, "slack")
	}
}

func TestGetNonexistentIncident(t *testing.T) {
	s := tempStore(t)

	got, err := s.GetIncident("nonexistent")
	if err != nil {
		t.Fatalf("GetIncident: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent incident")
	}
}

func TestStoreRequiresValidPath(t *testing.T) {
	_, err := NewStore(filepath.Join(os.DevNull, "impossible", "path.db"))
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestAssignIncident(t *testing.T) {
	s := tempStore(t)

	inc := &Incident{Title: "assign-test", Severity: SeverityMedium}
	s.CreateIncident(inc)

	if err := s.AssignIncident(inc.ID, "manager", "alice"); err != nil {
		t.Fatalf("AssignIncident: %v", err)
	}

	got, _ := s.GetIncident(inc.ID)
	if got.AssignedTo != "alice" {
		t.Errorf("assigned_to = %q, want %q", got.AssignedTo, "alice")
	}
}
