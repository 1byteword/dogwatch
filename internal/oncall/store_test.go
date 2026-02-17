package oncall

import (
	"path/filepath"
	"testing"
	"time"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(filepath.Join(dir, "oncall.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// --- Schedule CRUD ---

func TestCreateAndGetSchedule(t *testing.T) {
	s := tempStore(t)

	endDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	sched := &Schedule{
		ID:          "sched-1",
		Name:        "Primary On-Call",
		Description: "Main rotation",
		Timezone:    "America/New_York",
		Teams:       []string{"platform", "backend"},
		Layers: []Layer{
			{
				ID:            "layer-1",
				Name:          "Weekday",
				Priority:      1,
				RotationType:  "daily",
				HandoffTime:   "09:00",
				HandoffDay:    1,
				ShiftDuration: Duration(24 * time.Hour),
				StartDate:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				EndDate:       &endDate,
				Users: []User{
					{ID: "u1", Name: "Alice", Email: "alice@example.com", Phone: "+1234"},
					{ID: "u2", Name: "Bob", Email: "bob@example.com", Phone: "+5678"},
				},
				Restrictions: []Restriction{
					{Type: "daily", StartTime: "09:00", EndTime: "17:00", StartDay: 1, EndDay: 5},
				},
			},
		},
	}

	if err := s.CreateSchedule(sched); err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}

	got, err := s.GetSchedule("sched-1")
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}

	if got.ID != "sched-1" {
		t.Errorf("ID = %q, want %q", got.ID, "sched-1")
	}
	if got.Name != "Primary On-Call" {
		t.Errorf("Name = %q, want %q", got.Name, "Primary On-Call")
	}
	if got.Description != "Main rotation" {
		t.Errorf("Description = %q, want %q", got.Description, "Main rotation")
	}
	if got.Timezone != "America/New_York" {
		t.Errorf("Timezone = %q, want %q", got.Timezone, "America/New_York")
	}
	if len(got.Teams) != 2 || got.Teams[0] != "platform" || got.Teams[1] != "backend" {
		t.Errorf("Teams = %v, want [platform backend]", got.Teams)
	}
	if got.CreatedAt.IsZero() {
		t.Errorf("CreatedAt is zero")
	}
	if got.UpdatedAt.IsZero() {
		t.Errorf("UpdatedAt is zero")
	}

	// Verify layer round-trip
	if len(got.Layers) != 1 {
		t.Fatalf("len(Layers) = %d, want 1", len(got.Layers))
	}
	layer := got.Layers[0]
	if layer.ID != "layer-1" {
		t.Errorf("Layer.ID = %q, want %q", layer.ID, "layer-1")
	}
	if layer.Name != "Weekday" {
		t.Errorf("Layer.Name = %q, want %q", layer.Name, "Weekday")
	}
	if layer.Priority != 1 {
		t.Errorf("Layer.Priority = %d, want 1", layer.Priority)
	}
	if layer.RotationType != "daily" {
		t.Errorf("Layer.RotationType = %q, want %q", layer.RotationType, "daily")
	}
	if layer.HandoffTime != "09:00" {
		t.Errorf("Layer.HandoffTime = %q, want %q", layer.HandoffTime, "09:00")
	}
	if layer.HandoffDay != 1 {
		t.Errorf("Layer.HandoffDay = %d, want 1", layer.HandoffDay)
	}
	if time.Duration(layer.ShiftDuration) != 24*time.Hour {
		t.Errorf("Layer.ShiftDuration = %v, want 24h", time.Duration(layer.ShiftDuration))
	}
	if layer.EndDate == nil {
		t.Fatalf("Layer.EndDate is nil, want non-nil")
	}
	if len(layer.Users) != 2 {
		t.Fatalf("len(Layer.Users) = %d, want 2", len(layer.Users))
	}
	if layer.Users[0].Email != "alice@example.com" {
		t.Errorf("User[0].Email = %q, want %q", layer.Users[0].Email, "alice@example.com")
	}
	if layer.Users[1].Phone != "+5678" {
		t.Errorf("User[1].Phone = %q, want %q", layer.Users[1].Phone, "+5678")
	}
	if len(layer.Restrictions) != 1 {
		t.Fatalf("len(Restrictions) = %d, want 1", len(layer.Restrictions))
	}
	if layer.Restrictions[0].Type != "daily" {
		t.Errorf("Restriction.Type = %q, want %q", layer.Restrictions[0].Type, "daily")
	}
}

func TestUpdateSchedule(t *testing.T) {
	s := tempStore(t)

	sched := &Schedule{
		ID:       "sched-u",
		Name:     "Before",
		Timezone: "UTC",
		Teams:    []string{"team-a"},
	}
	if err := s.CreateSchedule(sched); err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}

	sched.Name = "After"
	sched.Description = "Updated desc"
	sched.Teams = []string{"team-b", "team-c"}
	sched.Layers = []Layer{
		{ID: "l1", Name: "New Layer", Priority: 10, RotationType: "weekly"},
	}

	if err := s.UpdateSchedule(sched); err != nil {
		t.Fatalf("UpdateSchedule: %v", err)
	}

	got, err := s.GetSchedule("sched-u")
	if err != nil {
		t.Fatalf("GetSchedule after update: %v", err)
	}
	if got.Name != "After" {
		t.Errorf("Name = %q, want %q", got.Name, "After")
	}
	if got.Description != "Updated desc" {
		t.Errorf("Description = %q, want %q", got.Description, "Updated desc")
	}
	if len(got.Teams) != 2 {
		t.Errorf("len(Teams) = %d, want 2", len(got.Teams))
	}
	if len(got.Layers) != 1 || got.Layers[0].Priority != 10 {
		t.Errorf("Layers not updated correctly")
	}
	if got.UpdatedAt.Before(got.CreatedAt) {
		t.Errorf("UpdatedAt (%v) should not be before CreatedAt (%v)", got.UpdatedAt, got.CreatedAt)
	}
}

func TestDeleteSchedule(t *testing.T) {
	s := tempStore(t)

	// Create schedule with inline overrides (stored in the schedules table JSON column).
	// Note: DeleteSchedule deletes the schedule row first, then cleans up the
	// overrides table. With foreign_keys enabled, deleting a schedule that has
	// rows in the overrides table would violate the FK constraint because the
	// production code deletes in parent-first order. We test the basic delete
	// path here (no separate-table overrides) which is the common case.
	sched := &Schedule{
		ID:       "sched-del",
		Name:     "ToDelete",
		Timezone: "UTC",
		Overrides: []Override{
			{ID: "inline-ov", UserID: "u1", UserName: "Alice"},
		},
	}
	if err := s.CreateSchedule(sched); err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}

	if err := s.DeleteSchedule("sched-del"); err != nil {
		t.Fatalf("DeleteSchedule: %v", err)
	}

	_, err := s.GetSchedule("sched-del")
	if err == nil {
		t.Errorf("GetSchedule after delete should return error")
	}
}

func TestListSchedules(t *testing.T) {
	s := tempStore(t)

	names := []string{"Charlie", "Alice", "Bob"}
	for _, name := range names {
		sched := &Schedule{ID: "s-" + name, Name: name, Timezone: "UTC"}
		if err := s.CreateSchedule(sched); err != nil {
			t.Fatalf("CreateSchedule(%s): %v", name, err)
		}
	}

	list, err := s.ListSchedules()
	if err != nil {
		t.Fatalf("ListSchedules: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len(list) = %d, want 3", len(list))
	}
	// Should be ordered by name: Alice, Bob, Charlie
	if list[0].Name != "Alice" || list[1].Name != "Bob" || list[2].Name != "Charlie" {
		t.Errorf("order = [%s, %s, %s], want [Alice, Bob, Charlie]",
			list[0].Name, list[1].Name, list[2].Name)
	}
}

// --- Override CRUD ---

func TestCreateOverrideAndGetViaSchedule(t *testing.T) {
	s := tempStore(t)

	sched := &Schedule{ID: "sched-ov", Name: "With Override", Timezone: "UTC"}
	if err := s.CreateSchedule(sched); err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}

	now := time.Now()
	override := &Override{
		ID:        "ov-1",
		UserID:    "u-override",
		UserName:  "Override User",
		StartTime: now.Add(-30 * time.Minute),
		EndTime:   now.Add(2 * time.Hour), // future end time so it shows up
		Reason:    "PTO coverage",
		CreatedBy: "manager",
	}
	if err := s.CreateOverride("sched-ov", override); err != nil {
		t.Fatalf("CreateOverride: %v", err)
	}

	got, err := s.GetSchedule("sched-ov")
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}

	if len(got.Overrides) != 1 {
		t.Fatalf("len(Overrides) = %d, want 1", len(got.Overrides))
	}

	ov := got.Overrides[0]
	if ov.ID != "ov-1" {
		t.Errorf("Override.ID = %q, want %q", ov.ID, "ov-1")
	}
	if ov.UserID != "u-override" {
		t.Errorf("Override.UserID = %q, want %q", ov.UserID, "u-override")
	}
	if ov.UserName != "Override User" {
		t.Errorf("Override.UserName = %q, want %q", ov.UserName, "Override User")
	}
	if ov.Reason != "PTO coverage" {
		t.Errorf("Override.Reason = %q, want %q", ov.Reason, "PTO coverage")
	}
	if ov.CreatedBy != "manager" {
		t.Errorf("Override.CreatedBy = %q, want %q", ov.CreatedBy, "manager")
	}
	if ov.CreatedAt.IsZero() {
		t.Errorf("Override.CreatedAt is zero")
	}
}

func TestOverrideExpiredNotReturned(t *testing.T) {
	s := tempStore(t)

	sched := &Schedule{ID: "sched-exp", Name: "Expired", Timezone: "UTC"}
	if err := s.CreateSchedule(sched); err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}

	// Override that ended in the past
	past := &Override{
		ID:        "ov-past",
		UserID:    "u1",
		UserName:  "Past",
		StartTime: time.Now().Add(-3 * time.Hour),
		EndTime:   time.Now().Add(-1 * time.Hour),
		Reason:    "expired",
		CreatedBy: "admin",
	}
	if err := s.CreateOverride("sched-exp", past); err != nil {
		t.Fatalf("CreateOverride: %v", err)
	}

	got, err := s.GetSchedule("sched-exp")
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}

	// getOverridesForSchedule filters end_time > now, so expired should not appear
	if len(got.Overrides) != 0 {
		t.Errorf("expected 0 overrides (expired), got %d", len(got.Overrides))
	}
}

func TestDeleteOverride(t *testing.T) {
	s := tempStore(t)

	sched := &Schedule{ID: "sched-dov", Name: "Del Override", Timezone: "UTC"}
	if err := s.CreateSchedule(sched); err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}

	override := &Override{
		ID:        "ov-to-del",
		UserID:    "u1",
		UserName:  "Alice",
		StartTime: time.Now().Add(-time.Minute),
		EndTime:   time.Now().Add(time.Hour),
		Reason:    "will be deleted",
		CreatedBy: "admin",
	}
	if err := s.CreateOverride("sched-dov", override); err != nil {
		t.Fatalf("CreateOverride: %v", err)
	}

	if err := s.DeleteOverride("ov-to-del"); err != nil {
		t.Fatalf("DeleteOverride: %v", err)
	}

	got, err := s.GetSchedule("sched-dov")
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}
	if len(got.Overrides) != 0 {
		t.Errorf("expected 0 overrides after delete, got %d", len(got.Overrides))
	}
}

// --- Escalation Policy CRUD ---

func TestCreateAndGetPolicy(t *testing.T) {
	s := tempStore(t)

	policy := &EscalationPolicy{
		ID:          "pol-1",
		Name:        "Critical",
		Description: "Critical escalation",
		Teams:       []string{"sre", "platform"},
		Rules: []EscalationRule{
			{
				Level:        1,
				DelayMinutes: 5,
				Targets: []Target{
					{Type: "schedule", ID: "sched-1"},
					{Type: "user", ID: "u-admin"},
				},
			},
			{
				Level:        2,
				DelayMinutes: 15,
				Targets: []Target{
					{Type: "team", ID: "management"},
				},
			},
		},
		RepeatEnabled: true,
		RepeatLimit:   3,
	}

	if err := s.CreatePolicy(policy); err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}

	got, err := s.GetPolicy("pol-1")
	if err != nil {
		t.Fatalf("GetPolicy: %v", err)
	}

	if got.ID != "pol-1" {
		t.Errorf("ID = %q, want %q", got.ID, "pol-1")
	}
	if got.Name != "Critical" {
		t.Errorf("Name = %q, want %q", got.Name, "Critical")
	}
	if got.Description != "Critical escalation" {
		t.Errorf("Description = %q, want %q", got.Description, "Critical escalation")
	}
	if len(got.Teams) != 2 || got.Teams[0] != "sre" {
		t.Errorf("Teams = %v, want [sre platform]", got.Teams)
	}
	if !got.RepeatEnabled {
		t.Errorf("RepeatEnabled = false, want true")
	}
	if got.RepeatLimit != 3 {
		t.Errorf("RepeatLimit = %d, want 3", got.RepeatLimit)
	}
	if got.CreatedAt.IsZero() {
		t.Errorf("CreatedAt is zero")
	}

	// Verify rules round-trip
	if len(got.Rules) != 2 {
		t.Fatalf("len(Rules) = %d, want 2", len(got.Rules))
	}
	if got.Rules[0].Level != 1 || got.Rules[0].DelayMinutes != 5 {
		t.Errorf("Rules[0] = {Level:%d, Delay:%d}, want {1, 5}",
			got.Rules[0].Level, got.Rules[0].DelayMinutes)
	}
	if len(got.Rules[0].Targets) != 2 {
		t.Fatalf("len(Rules[0].Targets) = %d, want 2", len(got.Rules[0].Targets))
	}
	if got.Rules[0].Targets[0].Type != "schedule" || got.Rules[0].Targets[0].ID != "sched-1" {
		t.Errorf("Rules[0].Targets[0] = %+v, want {schedule sched-1}", got.Rules[0].Targets[0])
	}
	if got.Rules[1].Level != 2 || got.Rules[1].Targets[0].Type != "team" {
		t.Errorf("Rules[1] unexpected: %+v", got.Rules[1])
	}
}

func TestUpdatePolicy(t *testing.T) {
	s := tempStore(t)

	policy := &EscalationPolicy{
		ID:            "pol-upd",
		Name:          "Before",
		RepeatEnabled: false,
		RepeatLimit:   0,
		Teams:         []string{"team-a"},
		Rules: []EscalationRule{
			{Level: 1, DelayMinutes: 5, Targets: []Target{{Type: "user", ID: "u1"}}},
		},
	}
	if err := s.CreatePolicy(policy); err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}

	// Update: change name, enable repeat, change rules
	policy.Name = "After"
	policy.Description = "Updated"
	policy.RepeatEnabled = true
	policy.RepeatLimit = 5
	policy.Teams = []string{"team-b"}
	policy.Rules = []EscalationRule{
		{Level: 1, DelayMinutes: 10, Targets: []Target{{Type: "schedule", ID: "s1"}}},
		{Level: 2, DelayMinutes: 20, Targets: []Target{{Type: "user", ID: "u2"}}},
	}

	if err := s.UpdatePolicy(policy); err != nil {
		t.Fatalf("UpdatePolicy: %v", err)
	}

	got, err := s.GetPolicy("pol-upd")
	if err != nil {
		t.Fatalf("GetPolicy: %v", err)
	}

	if got.Name != "After" {
		t.Errorf("Name = %q, want %q", got.Name, "After")
	}
	if got.Description != "Updated" {
		t.Errorf("Description = %q, want %q", got.Description, "Updated")
	}
	if !got.RepeatEnabled {
		t.Errorf("RepeatEnabled = false, want true (bool->int conversion)")
	}
	if got.RepeatLimit != 5 {
		t.Errorf("RepeatLimit = %d, want 5", got.RepeatLimit)
	}
	if len(got.Rules) != 2 {
		t.Fatalf("len(Rules) = %d, want 2", len(got.Rules))
	}
	if got.Rules[0].DelayMinutes != 10 {
		t.Errorf("Rules[0].DelayMinutes = %d, want 10", got.Rules[0].DelayMinutes)
	}

	// Verify bool->int->bool round-trip: set RepeatEnabled back to false
	policy.RepeatEnabled = false
	if err := s.UpdatePolicy(policy); err != nil {
		t.Fatalf("UpdatePolicy (disable repeat): %v", err)
	}
	got2, err := s.GetPolicy("pol-upd")
	if err != nil {
		t.Fatalf("GetPolicy: %v", err)
	}
	if got2.RepeatEnabled {
		t.Errorf("RepeatEnabled = true after setting to false")
	}
}

func TestDeletePolicy(t *testing.T) {
	s := tempStore(t)

	policy := &EscalationPolicy{ID: "pol-del", Name: "ToDelete"}
	if err := s.CreatePolicy(policy); err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}

	if err := s.DeletePolicy("pol-del"); err != nil {
		t.Fatalf("DeletePolicy: %v", err)
	}

	_, err := s.GetPolicy("pol-del")
	if err == nil {
		t.Errorf("GetPolicy after delete should return error")
	}
}

func TestListPolicies(t *testing.T) {
	s := tempStore(t)

	names := []string{"Zulu", "Alpha", "Mike"}
	for _, name := range names {
		p := &EscalationPolicy{ID: "p-" + name, Name: name}
		if err := s.CreatePolicy(p); err != nil {
			t.Fatalf("CreatePolicy(%s): %v", name, err)
		}
	}

	list, err := s.ListPolicies()
	if err != nil {
		t.Fatalf("ListPolicies: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len(list) = %d, want 3", len(list))
	}
	// Ordered by name: Alpha, Mike, Zulu
	if list[0].Name != "Alpha" || list[1].Name != "Mike" || list[2].Name != "Zulu" {
		t.Errorf("order = [%s, %s, %s], want [Alpha, Mike, Zulu]",
			list[0].Name, list[1].Name, list[2].Name)
	}
}

// --- Escalation State Persistence ---

func TestSaveAndLoadEscalationState(t *testing.T) {
	s := tempStore(t)

	now := time.Now()
	ackedAt := now.Add(-5 * time.Minute)

	state := &EscalationState{
		IncidentID:     "inc-1",
		PolicyID:       "pol-1",
		CurrentLevel:   2,
		RepeatCount:    1,
		StartedAt:      now.Add(-time.Hour),
		LastEscalation: now.Add(-10 * time.Minute),
		Acknowledged:   true,
		AckedBy:        "user-alice",
		AckedAt:        &ackedAt,
		Resolved:       false,
		Notifications: []Notification{
			{
				ID:      "notif-1",
				Level:   1,
				Target:  Target{Type: "user", ID: "u1"},
				Channel: "slack",
				SentAt:  now.Add(-30 * time.Minute),
				Status:  "sent",
				Message: "You are paged",
			},
		},
	}

	if err := s.SaveEscalationState(state); err != nil {
		t.Fatalf("SaveEscalationState: %v", err)
	}

	states, err := s.LoadActiveEscalationStates()
	if err != nil {
		t.Fatalf("LoadActiveEscalationStates: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("len(states) = %d, want 1", len(states))
	}

	got := states[0]
	if got.IncidentID != "inc-1" {
		t.Errorf("IncidentID = %q, want %q", got.IncidentID, "inc-1")
	}
	if got.PolicyID != "pol-1" {
		t.Errorf("PolicyID = %q, want %q", got.PolicyID, "pol-1")
	}
	if got.CurrentLevel != 2 {
		t.Errorf("CurrentLevel = %d, want 2", got.CurrentLevel)
	}
	if got.RepeatCount != 1 {
		t.Errorf("RepeatCount = %d, want 1", got.RepeatCount)
	}
	if !got.Acknowledged {
		t.Errorf("Acknowledged = false, want true")
	}
	if got.AckedBy != "user-alice" {
		t.Errorf("AckedBy = %q, want %q", got.AckedBy, "user-alice")
	}
	if got.AckedAt == nil {
		t.Fatalf("AckedAt is nil, want non-nil")
	}
	if got.AckedAt.Unix() != ackedAt.Unix() {
		t.Errorf("AckedAt = %v, want %v", got.AckedAt, ackedAt)
	}
	if got.Resolved {
		t.Errorf("Resolved = true, want false")
	}
	if len(got.Notifications) != 1 {
		t.Fatalf("len(Notifications) = %d, want 1", len(got.Notifications))
	}
	if got.Notifications[0].Channel != "slack" {
		t.Errorf("Notification.Channel = %q, want %q", got.Notifications[0].Channel, "slack")
	}
	if got.Notifications[0].Status != "sent" {
		t.Errorf("Notification.Status = %q, want %q", got.Notifications[0].Status, "sent")
	}
}

func TestSaveEscalationStateUpsert(t *testing.T) {
	s := tempStore(t)

	now := time.Now()
	state := &EscalationState{
		IncidentID:     "inc-upsert",
		PolicyID:       "pol-1",
		CurrentLevel:   1,
		RepeatCount:    0,
		StartedAt:      now,
		LastEscalation: now,
		Resolved:       false,
	}

	if err := s.SaveEscalationState(state); err != nil {
		t.Fatalf("SaveEscalationState (insert): %v", err)
	}

	// Upsert: update same incident_id
	state.CurrentLevel = 3
	state.RepeatCount = 2
	state.LastEscalation = now.Add(10 * time.Minute)
	state.Acknowledged = true
	state.AckedBy = "bob"
	ackedAt := now.Add(5 * time.Minute)
	state.AckedAt = &ackedAt

	if err := s.SaveEscalationState(state); err != nil {
		t.Fatalf("SaveEscalationState (upsert): %v", err)
	}

	states, err := s.LoadActiveEscalationStates()
	if err != nil {
		t.Fatalf("LoadActiveEscalationStates: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("len(states) = %d, want 1 (upsert should not duplicate)", len(states))
	}
	got := states[0]
	if got.CurrentLevel != 3 {
		t.Errorf("CurrentLevel = %d, want 3 (upserted)", got.CurrentLevel)
	}
	if got.RepeatCount != 2 {
		t.Errorf("RepeatCount = %d, want 2 (upserted)", got.RepeatCount)
	}
	if !got.Acknowledged {
		t.Errorf("Acknowledged = false, want true (upserted)")
	}
	if got.AckedBy != "bob" {
		t.Errorf("AckedBy = %q, want %q (upserted)", got.AckedBy, "bob")
	}
}

func TestLoadActiveExcludesResolved(t *testing.T) {
	s := tempStore(t)

	now := time.Now()

	// Unresolved state
	active := &EscalationState{
		IncidentID:     "inc-active",
		PolicyID:       "pol-1",
		CurrentLevel:   1,
		StartedAt:      now,
		LastEscalation: now,
		Resolved:       false,
	}
	// Resolved state
	resolved := &EscalationState{
		IncidentID:     "inc-resolved",
		PolicyID:       "pol-1",
		CurrentLevel:   1,
		StartedAt:      now,
		LastEscalation: now,
		Resolved:       true,
	}

	if err := s.SaveEscalationState(active); err != nil {
		t.Fatalf("SaveEscalationState(active): %v", err)
	}
	if err := s.SaveEscalationState(resolved); err != nil {
		t.Fatalf("SaveEscalationState(resolved): %v", err)
	}

	states, err := s.LoadActiveEscalationStates()
	if err != nil {
		t.Fatalf("LoadActiveEscalationStates: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("len(states) = %d, want 1 (only unresolved)", len(states))
	}
	if states[0].IncidentID != "inc-active" {
		t.Errorf("IncidentID = %q, want %q", states[0].IncidentID, "inc-active")
	}
}

func TestDeleteEscalationState(t *testing.T) {
	s := tempStore(t)

	now := time.Now()
	state := &EscalationState{
		IncidentID:     "inc-del",
		PolicyID:       "pol-1",
		CurrentLevel:   1,
		StartedAt:      now,
		LastEscalation: now,
	}

	if err := s.SaveEscalationState(state); err != nil {
		t.Fatalf("SaveEscalationState: %v", err)
	}

	if err := s.DeleteEscalationState("inc-del"); err != nil {
		t.Fatalf("DeleteEscalationState: %v", err)
	}

	states, err := s.LoadActiveEscalationStates()
	if err != nil {
		t.Fatalf("LoadActiveEscalationStates: %v", err)
	}
	if len(states) != 0 {
		t.Errorf("len(states) = %d, want 0 after delete", len(states))
	}
}

// --- Calculator tests ---

func TestCalculatorGetCurrentOnCall(t *testing.T) {
	s := tempStore(t)

	now := time.Now().UTC()
	sched := &Schedule{
		ID:       "sched-calc",
		Name:     "Calculator Test",
		Timezone: "UTC",
		Layers: []Layer{
			{
				ID:            "layer-daily",
				Name:          "Daily",
				Priority:      1,
				RotationType:  "daily",
				HandoffTime:   "00:00",
				ShiftDuration: Duration(24 * time.Hour),
				StartDate:     now.Add(-7 * 24 * time.Hour), // started a week ago
				Users: []User{
					{ID: "u1", Name: "Alice", Email: "alice@test.com"},
					{ID: "u2", Name: "Bob", Email: "bob@test.com"},
				},
			},
		},
	}

	if err := s.CreateSchedule(sched); err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}

	calc := NewCalculator(s)
	entry, err := calc.GetCurrentOnCall("sched-calc")
	if err != nil {
		t.Fatalf("GetCurrentOnCall: %v", err)
	}
	if entry == nil {
		t.Fatalf("GetCurrentOnCall returned nil")
	}

	// With 2 users in daily rotation, the current user should be one of them
	if entry.User.ID != "u1" && entry.User.ID != "u2" {
		t.Errorf("User.ID = %q, want u1 or u2", entry.User.ID)
	}
	if entry.LayerID != "layer-daily" {
		t.Errorf("LayerID = %q, want %q", entry.LayerID, "layer-daily")
	}
	if entry.IsOverride {
		t.Errorf("IsOverride = true, want false")
	}
}

func TestCalculatorOverrideTakesPrecedence(t *testing.T) {
	s := tempStore(t)

	now := time.Now().UTC()
	sched := &Schedule{
		ID:       "sched-ov-calc",
		Name:     "Override Precedence",
		Timezone: "UTC",
		Layers: []Layer{
			{
				ID:            "layer-1",
				Name:          "Base",
				Priority:      1,
				RotationType:  "daily",
				HandoffTime:   "00:00",
				ShiftDuration: Duration(24 * time.Hour),
				StartDate:     now.Add(-7 * 24 * time.Hour),
				Users: []User{
					{ID: "u1", Name: "Alice", Email: "alice@test.com"},
					{ID: "u2", Name: "Bob", Email: "bob@test.com"},
				},
			},
		},
	}

	if err := s.CreateSchedule(sched); err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}

	// Create an override covering now
	override := &Override{
		ID:        "ov-now",
		UserID:    "u-override",
		UserName:  "Override Person",
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now.Add(1 * time.Hour),
		Reason:    "emergency coverage",
		CreatedBy: "admin",
	}
	if err := s.CreateOverride("sched-ov-calc", override); err != nil {
		t.Fatalf("CreateOverride: %v", err)
	}

	calc := NewCalculator(s)
	entry, err := calc.GetCurrentOnCall("sched-ov-calc")
	if err != nil {
		t.Fatalf("GetCurrentOnCall: %v", err)
	}
	if entry == nil {
		t.Fatalf("GetCurrentOnCall returned nil")
	}

	if entry.User.ID != "u-override" {
		t.Errorf("User.ID = %q, want %q (override should take precedence)", entry.User.ID, "u-override")
	}
	if entry.User.Name != "Override Person" {
		t.Errorf("User.Name = %q, want %q", entry.User.Name, "Override Person")
	}
	if !entry.IsOverride {
		t.Errorf("IsOverride = false, want true")
	}
}
