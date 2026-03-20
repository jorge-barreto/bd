package db

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestInitCreatesDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, ".beads", "beads.db")

	store, err := Init(dir)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer store.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("expected database file to exist")
	}
}

func TestInitCreatesTables(t *testing.T) {
	dir := t.TempDir()
	store, err := Init(dir)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer store.Close()

	// Verify all tables exist by querying sqlite_master
	tables := []string{"items", "dependencies", "relations", "notes", "config"}
	for _, table := range tables {
		var name string
		err := store.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("expected table %q to exist: %v", table, err)
		}
	}
}

func TestFindDBWalksUp(t *testing.T) {
	// Create a directory structure: root/.beads/beads.db
	root := t.TempDir()
	beadsDir := filepath.Join(root, ".beads")
	os.MkdirAll(beadsDir, 0o755)
	os.WriteFile(filepath.Join(beadsDir, "beads.db"), []byte{}, 0o644)

	// Create a nested child directory
	child := filepath.Join(root, "a", "b", "c")
	os.MkdirAll(child, 0o755)

	// FindDB from the child should find root/.beads/beads.db
	found, err := FindDB(child)
	if err != nil {
		t.Fatalf("FindDB failed: %v", err)
	}
	expected := filepath.Join(beadsDir, "beads.db")
	if found != expected {
		t.Errorf("FindDB = %q, want %q", found, expected)
	}
}

func TestFindDBRespectsEnvVar(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, "custom")
	os.MkdirAll(beadsDir, 0o755)
	os.WriteFile(filepath.Join(beadsDir, "beads.db"), []byte{}, 0o644)

	t.Setenv("BEADS_DIR", beadsDir)

	found, err := FindDB("/some/random/path")
	if err != nil {
		t.Fatalf("FindDB failed: %v", err)
	}
	expected := filepath.Join(beadsDir, "beads.db")
	if found != expected {
		t.Errorf("FindDB = %q, want %q", found, expected)
	}
}

func TestFindDBErrorsWhenNotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BEADS_DIR", "")

	_, err := FindDB(dir)
	if err == nil {
		t.Fatal("expected error when no .beads directory found")
	}
}

// helper to create a fresh store for tests
func testStore(t *testing.T) *Store {
	t.Helper()
	store, err := Init(t.TempDir())
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestGenerateIDTopLevel(t *testing.T) {
	store := testStore(t)
	store.SetConfig("prefix", "orc")

	id, err := store.GenerateID("")
	if err != nil {
		t.Fatalf("GenerateID failed: %v", err)
	}

	// Should match pattern: orc-{3 alphanum}
	matched, _ := regexp.MatchString(`^orc-[a-z0-9]{3}$`, id)
	if !matched {
		t.Errorf("ID %q does not match pattern orc-XXX", id)
	}
}

func TestGenerateIDChild(t *testing.T) {
	store := testStore(t)
	store.SetConfig("prefix", "orc")

	// Create a parent item first
	parentID, _ := store.GenerateID("")
	store.CreateItem(parentID, "Parent", "", "epic", 2, "", "")

	// First child should be parentID.1
	childID, err := store.GenerateID(parentID)
	if err != nil {
		t.Fatalf("GenerateID child failed: %v", err)
	}
	if childID != parentID+".1" {
		t.Errorf("first child ID = %q, want %q", childID, parentID+".1")
	}

	// Create the child, then next should be .2
	store.CreateItem(childID, "Child 1", "", "task", 2, parentID, "")
	childID2, _ := store.GenerateID(parentID)
	if childID2 != parentID+".2" {
		t.Errorf("second child ID = %q, want %q", childID2, parentID+".2")
	}
}

func TestCreateAndGetItem(t *testing.T) {
	store := testStore(t)

	err := store.CreateItem("orc-abc", "Test Task", "A description", "task", 1, "", "alice@example.com")
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}

	item, err := store.GetItem("orc-abc")
	if err != nil {
		t.Fatalf("GetItem failed: %v", err)
	}

	if item.ID != "orc-abc" {
		t.Errorf("ID = %q, want %q", item.ID, "orc-abc")
	}
	if item.Title != "Test Task" {
		t.Errorf("Title = %q, want %q", item.Title, "Test Task")
	}
	if item.Description != "A description" {
		t.Errorf("Description = %q, want %q", item.Description, "A description")
	}
	if item.IssueType != "task" {
		t.Errorf("IssueType = %q, want %q", item.IssueType, "task")
	}
	if item.Status != "open" {
		t.Errorf("Status = %q, want %q", item.Status, "open")
	}
	if item.Priority != 1 {
		t.Errorf("Priority = %d, want %d", item.Priority, 1)
	}
	if item.Owner != "alice@example.com" {
		t.Errorf("Owner = %q, want %q", item.Owner, "alice@example.com")
	}
	if item.CreatedAt == "" {
		t.Error("CreatedAt should not be empty")
	}
	if item.UpdatedAt == "" {
		t.Error("UpdatedAt should not be empty")
	}
}

func TestGetItemNotFound(t *testing.T) {
	store := testStore(t)

	_, err := store.GetItem("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent item")
	}
}

func TestConfigGetSet(t *testing.T) {
	store := testStore(t)

	store.SetConfig("prefix", "test")
	val, err := store.GetConfig("prefix")
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}
	if val != "test" {
		t.Errorf("GetConfig = %q, want %q", val, "test")
	}

	// Update existing key
	store.SetConfig("prefix", "updated")
	val, _ = store.GetConfig("prefix")
	if val != "updated" {
		t.Errorf("after update, GetConfig = %q, want %q", val, "updated")
	}
}

func TestGetConfigDefault(t *testing.T) {
	store := testStore(t)

	val, err := store.GetConfig("nonexistent")
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}
	if val != "" {
		t.Errorf("GetConfig for missing key = %q, want empty", val)
	}
}

func TestUpdateItemStatus(t *testing.T) {
	store := testStore(t)
	store.CreateItem("orc-abc", "Test", "", "task", 2, "", "")

	err := store.UpdateItem("orc-abc", map[string]string{"status": "in_progress"})
	if err != nil {
		t.Fatalf("UpdateItem failed: %v", err)
	}

	item, _ := store.GetItem("orc-abc")
	if item.Status != "in_progress" {
		t.Errorf("Status = %q, want %q", item.Status, "in_progress")
	}
}

func TestCloseItem(t *testing.T) {
	store := testStore(t)
	store.CreateItem("orc-abc", "Test", "", "task", 2, "", "")

	err := store.CloseItem("orc-abc")
	if err != nil {
		t.Fatalf("CloseItem failed: %v", err)
	}

	item, _ := store.GetItem("orc-abc")
	if item.Status != "closed" {
		t.Errorf("Status = %q, want %q", item.Status, "closed")
	}
}

func TestReopenItem(t *testing.T) {
	store := testStore(t)
	store.CreateItem("orc-abc", "Test", "", "task", 2, "", "")
	store.CloseItem("orc-abc")

	err := store.ReopenItem("orc-abc")
	if err != nil {
		t.Fatalf("ReopenItem failed: %v", err)
	}

	item, _ := store.GetItem("orc-abc")
	if item.Status != "open" {
		t.Errorf("Status = %q, want %q", item.Status, "open")
	}
}

func TestDeleteItem(t *testing.T) {
	store := testStore(t)
	store.CreateItem("orc-abc", "Parent", "", "epic", 2, "", "")
	store.CreateItem("orc-abc.1", "Child", "", "task", 2, "orc-abc", "")
	store.AddNote("orc-abc", "a note")

	err := store.DeleteItem("orc-abc")
	if err != nil {
		t.Fatalf("DeleteItem failed: %v", err)
	}

	_, err = store.GetItem("orc-abc")
	if err == nil {
		t.Error("expected error getting deleted item")
	}

	// Child should also be deleted
	_, err = store.GetItem("orc-abc.1")
	if err == nil {
		t.Error("expected child to be deleted with parent")
	}
}

func TestDeleteItemWithDeps(t *testing.T) {
	store := testStore(t)
	store.CreateItem("orc-aaa", "A", "", "task", 2, "", "")
	store.CreateItem("orc-bbb", "B", "", "task", 2, "", "")
	store.AddDep("orc-bbb", "orc-aaa")

	err := store.DeleteItem("orc-aaa")
	if err != nil {
		t.Fatalf("DeleteItem failed: %v", err)
	}

	// Dependency should be cleaned up
	deps, _ := store.GetBlockedBy("orc-bbb")
	if len(deps) != 0 {
		t.Errorf("expected 0 blockers after delete, got %d", len(deps))
	}
}
