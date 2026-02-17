package dashboard

import (
	"path/filepath"
	"testing"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(filepath.Join(dir, "dashboards.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCreateAndGetDashboard(t *testing.T) {
	s := tempStore(t)

	layout := []WidgetPosition{
		{ID: "cpu", X: 0, Y: 0, Width: 6, Height: 4},
		{ID: "mem", X: 6, Y: 0, Width: 6, Height: 4},
	}
	config := map[string]WidgetConfig{
		"cpu": {Service: "api", Since: "1h"},
	}

	dash, err := s.Create("Ops Dashboard", layout, config, false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if dash.ID == "" {
		t.Error("expected ID to be assigned")
	}
	if dash.Name != "Ops Dashboard" {
		t.Errorf("name = %q, want %q", dash.Name, "Ops Dashboard")
	}

	got, err := s.Get(dash.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil dashboard")
	}
	if len(got.Layout) != 2 {
		t.Errorf("layout len = %d, want 2", len(got.Layout))
	}
	if got.Layout[0].ID != "cpu" {
		t.Errorf("layout[0].ID = %q, want %q", got.Layout[0].ID, "cpu")
	}
	if got.WidgetConfig["cpu"].Service != "api" {
		t.Errorf("widget_config[cpu].service = %q, want %q", got.WidgetConfig["cpu"].Service, "api")
	}
}

func TestListDashboards(t *testing.T) {
	s := tempStore(t)

	s.Create("Dashboard A", []WidgetPosition{{ID: "w1", X: 0, Y: 0, Width: 12, Height: 6}}, nil, false)
	s.Create("Dashboard B", []WidgetPosition{{ID: "w2", X: 0, Y: 0, Width: 12, Height: 6}}, nil, false)

	all, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 dashboards, got %d", len(all))
	}
}

func TestSetDefault(t *testing.T) {
	s := tempStore(t)

	d1, _ := s.Create("First", []WidgetPosition{{ID: "w1"}}, nil, true)
	d2, _ := s.Create("Second", []WidgetPosition{{ID: "w2"}}, nil, false)

	// d1 should be default
	def, err := s.GetDefault()
	if err != nil {
		t.Fatalf("GetDefault: %v", err)
	}
	if def == nil || def.ID != d1.ID {
		t.Errorf("expected d1 as default")
	}

	// Switch default to d2
	if err := s.SetDefault(d2.ID); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}

	def, _ = s.GetDefault()
	if def == nil || def.ID != d2.ID {
		t.Errorf("expected d2 as default after switch")
	}

	// d1 should no longer be default
	d1Got, _ := s.Get(d1.ID)
	if d1Got.IsDefault {
		t.Error("d1 should no longer be default")
	}
}

func TestUpdateDashboard(t *testing.T) {
	s := tempStore(t)

	dash, _ := s.Create("Initial", []WidgetPosition{{ID: "w1", X: 0, Y: 0, Width: 6, Height: 4}}, nil, false)

	newLayout := []WidgetPosition{
		{ID: "w1", X: 0, Y: 0, Width: 12, Height: 4},
		{ID: "w2", X: 0, Y: 4, Width: 12, Height: 4},
	}
	newConfig := map[string]WidgetConfig{"w1": {Since: "24h"}}

	if err := s.Update(dash.ID, "Updated Name", newLayout, newConfig); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := s.Get(dash.ID)
	if got.Name != "Updated Name" {
		t.Errorf("name = %q, want %q", got.Name, "Updated Name")
	}
	if len(got.Layout) != 2 {
		t.Errorf("layout len = %d, want 2", len(got.Layout))
	}
}

func TestDeleteDashboard(t *testing.T) {
	s := tempStore(t)

	dash, _ := s.Create("ToDelete", []WidgetPosition{{ID: "w1"}}, nil, false)

	if err := s.Delete(dash.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, _ := s.Get(dash.ID)
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestFolderCRUD(t *testing.T) {
	s := tempStore(t)

	folder, err := s.CreateFolder("Production", nil)
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if folder.ID == "" {
		t.Error("expected ID")
	}
	if folder.Name != "Production" {
		t.Errorf("name = %q, want %q", folder.Name, "Production")
	}

	got, err := s.GetFolder(folder.ID)
	if err != nil {
		t.Fatalf("GetFolder: %v", err)
	}
	if got.Name != "Production" {
		t.Errorf("name = %q, want %q", got.Name, "Production")
	}

	// Nested folder
	child, err := s.CreateFolder("API Services", &folder.ID)
	if err != nil {
		t.Fatalf("CreateFolder child: %v", err)
	}
	if child.ParentID == nil || *child.ParentID != folder.ID {
		t.Error("expected child parent_id to match parent")
	}

	folders, err := s.ListFolders()
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if len(folders) != 2 {
		t.Errorf("expected 2 folders, got %d", len(folders))
	}
}

func TestMoveDashboardToFolder(t *testing.T) {
	s := tempStore(t)

	folder, _ := s.CreateFolder("Staging", nil)
	dash, _ := s.Create("StageDash", []WidgetPosition{{ID: "w1"}}, nil, false)

	if err := s.MoveDashboard(dash.ID, &folder.ID); err != nil {
		t.Fatalf("MoveDashboard: %v", err)
	}

	got, _ := s.Get(dash.ID)
	if got.FolderID == nil || *got.FolderID != folder.ID {
		t.Error("expected dashboard to be in folder")
	}

	// Move back to root
	if err := s.MoveDashboard(dash.ID, nil); err != nil {
		t.Fatalf("MoveDashboard to root: %v", err)
	}

	got, _ = s.Get(dash.ID)
	if got.FolderID != nil {
		t.Error("expected dashboard to be at root")
	}
}

func TestGetNonexistentDashboard(t *testing.T) {
	s := tempStore(t)

	got, err := s.Get("nonexistent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent")
	}
}

func TestFolderTree(t *testing.T) {
	s := tempStore(t)

	parent, _ := s.CreateFolder("Infrastructure", nil)
	s.CreateFolder("Networking", &parent.ID)

	tree, err := s.GetFolderTree()
	if err != nil {
		t.Fatalf("GetFolderTree: %v", err)
	}
	if len(tree) != 1 {
		t.Fatalf("expected 1 root folder, got %d", len(tree))
	}
	if tree[0].Name != "Infrastructure" {
		t.Errorf("root name = %q, want %q", tree[0].Name, "Infrastructure")
	}
	if len(tree[0].Children) != 1 {
		t.Errorf("expected 1 child, got %d", len(tree[0].Children))
	}
}
