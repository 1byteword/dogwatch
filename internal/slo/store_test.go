package slo

import (
	"path/filepath"
	"testing"
	"time"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(filepath.Join(dir, "slo.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func makeSLO(name string) *SLO {
	return &SLO{
		ID:          "slo-" + name,
		Name:        name,
		Description: "desc for " + name,
		ServiceID:   "svc-1",
		Type:        SLOAvailability,
		Target:      99.9,
		Window:      Window30Days,
		Source: DataSource{
			Type:       "metric",
			ID:         "http_requests_total",
			Field:      "error_rate",
			Percentile: 99,
		},
		Threshold: 200.0,
		Enabled:   true,
	}
}

func TestCreateAndGetSLO(t *testing.T) {
	s := tempStore(t)
	in := makeSLO("api-availability")

	if err := s.CreateSLO(in); err != nil {
		t.Fatalf("CreateSLO: %v", err)
	}

	got, err := s.GetSLO(in.ID)
	if err != nil {
		t.Fatalf("GetSLO: %v", err)
	}
	if got == nil {
		t.Fatal("GetSLO returned nil")
	}
	if got.ID != in.ID {
		t.Errorf("ID = %q, want %q", got.ID, in.ID)
	}
	if got.Name != in.Name {
		t.Errorf("Name = %q, want %q", got.Name, in.Name)
	}
	if got.Description != in.Description {
		t.Errorf("Description = %q, want %q", got.Description, in.Description)
	}
	if got.ServiceID != in.ServiceID {
		t.Errorf("ServiceID = %q, want %q", got.ServiceID, in.ServiceID)
	}
	if got.Type != in.Type {
		t.Errorf("Type = %q, want %q", got.Type, in.Type)
	}
	if got.Target != in.Target {
		t.Errorf("Target = %f, want %f", got.Target, in.Target)
	}
	if got.Window != in.Window {
		t.Errorf("Window = %q, want %q", got.Window, in.Window)
	}
	if got.Source.Type != in.Source.Type {
		t.Errorf("Source.Type = %q, want %q", got.Source.Type, in.Source.Type)
	}
	if got.Source.ID != in.Source.ID {
		t.Errorf("Source.ID = %q, want %q", got.Source.ID, in.Source.ID)
	}
	if got.Source.Field != in.Source.Field {
		t.Errorf("Source.Field = %q, want %q", got.Source.Field, in.Source.Field)
	}
	if got.Source.Percentile != in.Source.Percentile {
		t.Errorf("Source.Percentile = %d, want %d", got.Source.Percentile, in.Source.Percentile)
	}
	if got.Threshold != in.Threshold {
		t.Errorf("Threshold = %f, want %f", got.Threshold, in.Threshold)
	}
	if !got.Enabled {
		t.Errorf("Enabled = false, want true")
	}
	if got.Created.IsZero() {
		t.Errorf("Created is zero")
	}
	if got.Updated.IsZero() {
		t.Errorf("Updated is zero")
	}
}

func TestCreateSLO_AutoGeneratesID(t *testing.T) {
	s := tempStore(t)
	slo := makeSLO("auto-id")
	slo.ID = ""

	if err := s.CreateSLO(slo); err != nil {
		t.Fatalf("CreateSLO: %v", err)
	}
	if slo.ID == "" {
		t.Fatal("expected auto-generated ID, got empty")
	}
	if len(slo.ID) < 32 {
		t.Errorf("ID %q looks too short for UUID", slo.ID)
	}

	got, err := s.GetSLO(slo.ID)
	if err != nil {
		t.Fatalf("GetSLO: %v", err)
	}
	if got == nil {
		t.Fatal("GetSLO returned nil for auto-generated ID")
	}
}

func TestCreateSLO_DefaultWindow(t *testing.T) {
	s := tempStore(t)
	slo := makeSLO("default-window")
	slo.Window = ""

	if err := s.CreateSLO(slo); err != nil {
		t.Fatalf("CreateSLO: %v", err)
	}
	if slo.Window != Window30Days {
		t.Errorf("Window = %q, want %q", slo.Window, Window30Days)
	}

	got, err := s.GetSLO(slo.ID)
	if err != nil {
		t.Fatalf("GetSLO: %v", err)
	}
	if got.Window != Window30Days {
		t.Errorf("persisted Window = %q, want %q", got.Window, Window30Days)
	}
}

func TestListSLOs(t *testing.T) {
	s := tempStore(t)
	names := []string{"charlie", "alpha", "bravo"}
	for _, n := range names {
		if err := s.CreateSLO(makeSLO(n)); err != nil {
			t.Fatalf("CreateSLO(%s): %v", n, err)
		}
	}

	list, err := s.ListSLOs()
	if err != nil {
		t.Fatalf("ListSLOs: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("ListSLOs returned %d, want 3", len(list))
	}
	// Should be ordered by name
	want := []string{"alpha", "bravo", "charlie"}
	for i, w := range want {
		if list[i].Name != w {
			t.Errorf("list[%d].Name = %q, want %q", i, list[i].Name, w)
		}
	}
}

func TestListSLOsByService(t *testing.T) {
	s := tempStore(t)

	slo1 := makeSLO("svc1-slo")
	slo1.ServiceID = "svc-a"
	slo2 := makeSLO("svc2-slo")
	slo2.ServiceID = "svc-b"
	slo3 := makeSLO("svc1-other")
	slo3.ServiceID = "svc-a"

	for _, sl := range []*SLO{slo1, slo2, slo3} {
		if err := s.CreateSLO(sl); err != nil {
			t.Fatalf("CreateSLO: %v", err)
		}
	}

	list, err := s.ListSLOsByService("svc-a")
	if err != nil {
		t.Fatalf("ListSLOsByService: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d SLOs, want 2", len(list))
	}
	for _, sl := range list {
		if sl.ServiceID != "svc-a" {
			t.Errorf("ServiceID = %q, want svc-a", sl.ServiceID)
		}
	}

	empty, err := s.ListSLOsByService("nonexistent")
	if err != nil {
		t.Fatalf("ListSLOsByService(nonexistent): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected empty list, got %d", len(empty))
	}
}

func TestListEnabledSLOs(t *testing.T) {
	s := tempStore(t)

	enabled := makeSLO("enabled-slo")
	enabled.Enabled = true
	disabled := makeSLO("disabled-slo")
	disabled.Enabled = false

	for _, sl := range []*SLO{enabled, disabled} {
		if err := s.CreateSLO(sl); err != nil {
			t.Fatalf("CreateSLO: %v", err)
		}
	}

	list, err := s.ListEnabledSLOs()
	if err != nil {
		t.Fatalf("ListEnabledSLOs: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d, want 1", len(list))
	}
	if list[0].Name != "enabled-slo" {
		t.Errorf("Name = %q, want enabled-slo", list[0].Name)
	}
}

func TestUpdateSLO(t *testing.T) {
	s := tempStore(t)
	slo := makeSLO("to-update")
	if err := s.CreateSLO(slo); err != nil {
		t.Fatalf("CreateSLO: %v", err)
	}
	originalUpdated := slo.Updated

	// Allow a small time gap so Updated changes
	time.Sleep(10 * time.Millisecond)

	slo.Name = "updated-name"
	slo.Description = "new desc"
	slo.Target = 99.99
	slo.Type = SLOLatency
	slo.Threshold = 500.0
	slo.Enabled = false
	slo.Window = Window7Days
	slo.Source = DataSource{Type: "synthetics", ID: "check-1", Field: "latency_p95", Percentile: 95}

	if err := s.UpdateSLO(slo); err != nil {
		t.Fatalf("UpdateSLO: %v", err)
	}

	got, err := s.GetSLO(slo.ID)
	if err != nil {
		t.Fatalf("GetSLO: %v", err)
	}
	if got.Name != "updated-name" {
		t.Errorf("Name = %q, want updated-name", got.Name)
	}
	if got.Description != "new desc" {
		t.Errorf("Description = %q, want new desc", got.Description)
	}
	if got.Target != 99.99 {
		t.Errorf("Target = %f, want 99.99", got.Target)
	}
	if got.Type != SLOLatency {
		t.Errorf("Type = %q, want %q", got.Type, SLOLatency)
	}
	if got.Threshold != 500.0 {
		t.Errorf("Threshold = %f, want 500", got.Threshold)
	}
	if got.Enabled {
		t.Errorf("Enabled = true, want false")
	}
	if got.Window != Window7Days {
		t.Errorf("Window = %q, want %q", got.Window, Window7Days)
	}
	if got.Source.Type != "synthetics" {
		t.Errorf("Source.Type = %q, want synthetics", got.Source.Type)
	}
	if got.Source.Percentile != 95 {
		t.Errorf("Source.Percentile = %d, want 95", got.Source.Percentile)
	}
	if !got.Updated.After(originalUpdated) {
		t.Errorf("Updated %v should be after original %v", got.Updated, originalUpdated)
	}
}

func TestDeleteSLO(t *testing.T) {
	s := tempStore(t)
	slo := makeSLO("to-delete")
	if err := s.CreateSLO(slo); err != nil {
		t.Fatalf("CreateSLO: %v", err)
	}

	if err := s.DeleteSLO(slo.ID); err != nil {
		t.Fatalf("DeleteSLO: %v", err)
	}

	got, err := s.GetSLO(slo.ID)
	if err != nil {
		t.Fatalf("GetSLO after delete: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}

func TestRecordAndGetSnapshots(t *testing.T) {
	s := tempStore(t)
	slo := makeSLO("snap-slo")
	if err := s.CreateSLO(slo); err != nil {
		t.Fatalf("CreateSLO: %v", err)
	}

	now := time.Now()
	snaps := []SLOSnapshot{
		{SLOID: slo.ID, Timestamp: now.Add(-2 * time.Hour), CurrentValue: 99.8, BudgetRemaining: 0.5, Status: StatusMet},
		{SLOID: slo.ID, Timestamp: now.Add(-1 * time.Hour), CurrentValue: 99.5, BudgetRemaining: 0.3, Status: StatusAtRisk},
		{SLOID: slo.ID, Timestamp: now, CurrentValue: 98.0, BudgetRemaining: -0.1, Status: StatusBreached},
	}
	for i := range snaps {
		if err := s.RecordSnapshot(&snaps[i]); err != nil {
			t.Fatalf("RecordSnapshot[%d]: %v", i, err)
		}
		if snaps[i].ID == "" {
			t.Errorf("snapshot[%d] ID not auto-generated", i)
		}
	}

	got, err := s.GetSnapshots(slo.ID, 24*time.Hour, 100)
	if err != nil {
		t.Fatalf("GetSnapshots: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d snapshots, want 3", len(got))
	}
	// Ordered by timestamp DESC
	if got[0].Status != StatusBreached {
		t.Errorf("first snapshot status = %q, want %q", got[0].Status, StatusBreached)
	}
	if got[2].Status != StatusMet {
		t.Errorf("last snapshot status = %q, want %q", got[2].Status, StatusMet)
	}
}

func TestGetSnapshots_Limit(t *testing.T) {
	s := tempStore(t)
	slo := makeSLO("limit-slo")
	if err := s.CreateSLO(slo); err != nil {
		t.Fatalf("CreateSLO: %v", err)
	}

	now := time.Now()
	for i := 0; i < 10; i++ {
		snap := &SLOSnapshot{
			SLOID:        slo.ID,
			Timestamp:    now.Add(-time.Duration(i) * time.Minute),
			CurrentValue: float64(100 - i),
			Status:       StatusMet,
		}
		if err := s.RecordSnapshot(snap); err != nil {
			t.Fatalf("RecordSnapshot: %v", err)
		}
	}

	got, err := s.GetSnapshots(slo.ID, 24*time.Hour, 3)
	if err != nil {
		t.Fatalf("GetSnapshots: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("got %d snapshots, want 3", len(got))
	}
}

func TestGetSnapshots_Since(t *testing.T) {
	s := tempStore(t)
	slo := makeSLO("since-slo")
	if err := s.CreateSLO(slo); err != nil {
		t.Fatalf("CreateSLO: %v", err)
	}

	now := time.Now()
	// One recent, one old
	recent := &SLOSnapshot{SLOID: slo.ID, Timestamp: now.Add(-30 * time.Minute), CurrentValue: 99.9, Status: StatusMet}
	old := &SLOSnapshot{SLOID: slo.ID, Timestamp: now.Add(-3 * time.Hour), CurrentValue: 99.0, Status: StatusAtRisk}

	for _, snap := range []*SLOSnapshot{recent, old} {
		if err := s.RecordSnapshot(snap); err != nil {
			t.Fatalf("RecordSnapshot: %v", err)
		}
	}

	got, err := s.GetSnapshots(slo.ID, 1*time.Hour, 100)
	if err != nil {
		t.Fatalf("GetSnapshots: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d snapshots, want 1 (only recent)", len(got))
	}
	if got[0].CurrentValue != 99.9 {
		t.Errorf("CurrentValue = %f, want 99.9", got[0].CurrentValue)
	}
}

func TestCleanupSnapshots(t *testing.T) {
	s := tempStore(t)
	slo := makeSLO("cleanup-slo")
	if err := s.CreateSLO(slo); err != nil {
		t.Fatalf("CreateSLO: %v", err)
	}

	now := time.Now()
	// 2 old snapshots, 1 recent
	for _, ts := range []time.Time{
		now.Add(-48 * time.Hour),
		now.Add(-49 * time.Hour),
		now.Add(-1 * time.Hour),
	} {
		snap := &SLOSnapshot{SLOID: slo.ID, Timestamp: ts, CurrentValue: 99.0, Status: StatusMet}
		if err := s.RecordSnapshot(snap); err != nil {
			t.Fatalf("RecordSnapshot: %v", err)
		}
	}

	removed, err := s.CleanupSnapshots(24 * time.Hour)
	if err != nil {
		t.Fatalf("CleanupSnapshots: %v", err)
	}
	if removed != 2 {
		t.Errorf("removed %d, want 2", removed)
	}

	remaining, err := s.GetSnapshots(slo.ID, 72*time.Hour, 100)
	if err != nil {
		t.Fatalf("GetSnapshots: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("remaining %d, want 1", len(remaining))
	}
}

func TestGetWindowDuration(t *testing.T) {
	tests := []struct {
		window TimeWindow
		want   time.Duration
	}{
		{Window7Days, 7 * 24 * time.Hour},
		{Window30Days, 30 * 24 * time.Hour},
		{Window90Days, 90 * 24 * time.Hour},
		{TimeWindow("unknown"), 30 * 24 * time.Hour},
		{TimeWindow(""), 30 * 24 * time.Hour},
	}
	for _, tt := range tests {
		got := GetWindowDuration(tt.window)
		if got != tt.want {
			t.Errorf("GetWindowDuration(%q) = %v, want %v", tt.window, got, tt.want)
		}
	}
}
