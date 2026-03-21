package db

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

	id, err := store.GenerateID("", "Test Item", "", "")
	if err != nil {
		t.Fatalf("GenerateID failed: %v", err)
	}

	// Should match pattern: orc-{3-8 alphanum}
	matched, _ := regexp.MatchString(`^orc-[a-z0-9]{3,8}$`, id)
	if !matched {
		t.Errorf("ID %q does not match pattern orc-{3-8 alphanum}", id)
	}
}

func TestGenerateIDChild(t *testing.T) {
	store := testStore(t)
	store.SetConfig("prefix", "orc")

	// Create a parent item first
	parentID, _ := store.GenerateID("", "Parent", "", "")
	store.CreateItem(parentID, "Parent", "", "epic", 2, "", "")

	// First child should be parentID.1
	childID, err := store.GenerateID(parentID, "Child 1", "", "")
	if err != nil {
		t.Fatalf("GenerateID child failed: %v", err)
	}
	if childID != parentID+".1" {
		t.Errorf("first child ID = %q, want %q", childID, parentID+".1")
	}

	// Create the child, then next should be .2
	store.CreateItem(childID, "Child 1", "", "task", 2, parentID, "")
	childID2, _ := store.GenerateID(parentID, "Child 2", "", "")
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

func TestListItemsAll(t *testing.T) {
	store := testStore(t)
	store.CreateItem("orc-aaa", "A", "", "task", 2, "", "")
	store.CreateItem("orc-bbb", "B", "", "bug", 1, "", "")
	store.CreateItem("orc-ccc", "C", "", "epic", 0, "", "")

	items, err := store.ListItems(ListFilters{})
	if err != nil {
		t.Fatalf("ListItems failed: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("ListItems returned %d items, want 3", len(items))
	}
}

func TestListItemsDefaultHidesClosed(t *testing.T) {
	store := testStore(t)
	store.CreateItem("orc-aaa", "Open", "", "task", 2, "", "")
	store.CreateItem("orc-bbb", "Closed", "", "task", 2, "", "")
	store.CloseItem("orc-bbb")

	// Default (All=false) should hide closed
	items, _ := store.ListItems(ListFilters{})
	if len(items) != 1 {
		t.Errorf("default list should hide closed, got %d items", len(items))
	}

	// All=true should show everything
	items, _ = store.ListItems(ListFilters{All: true})
	if len(items) != 2 {
		t.Errorf("list --all should show 2 items, got %d", len(items))
	}
}

func TestListItemsFilterByStatus(t *testing.T) {
	store := testStore(t)
	store.CreateItem("orc-aaa", "Open", "", "task", 2, "", "")
	store.CreateItem("orc-bbb", "Closed", "", "task", 2, "", "")
	store.CloseItem("orc-bbb")

	items, _ := store.ListItems(ListFilters{Status: "open"})
	if len(items) != 1 {
		t.Errorf("expected 1 open item, got %d", len(items))
	}
	if items[0].ID != "orc-aaa" {
		t.Errorf("expected orc-aaa, got %s", items[0].ID)
	}
}

func TestListItemsFilterByType(t *testing.T) {
	store := testStore(t)
	store.CreateItem("orc-aaa", "Task", "", "task", 2, "", "")
	store.CreateItem("orc-bbb", "Epic", "", "epic", 2, "", "")

	items, _ := store.ListItems(ListFilters{Type: "epic"})
	if len(items) != 1 {
		t.Errorf("expected 1 epic, got %d", len(items))
	}
	if items[0].ID != "orc-bbb" {
		t.Errorf("expected orc-bbb, got %s", items[0].ID)
	}
}

func TestListItemsFilterByParent(t *testing.T) {
	store := testStore(t)
	store.CreateItem("orc-aaa", "Parent", "", "epic", 2, "", "")
	store.CreateItem("orc-aaa.1", "Child", "", "task", 2, "orc-aaa", "")
	store.CreateItem("orc-bbb", "Other", "", "task", 2, "", "")

	items, _ := store.ListItems(ListFilters{ParentID: "orc-aaa"})
	if len(items) != 1 {
		t.Errorf("expected 1 child, got %d", len(items))
	}
	if items[0].ID != "orc-aaa.1" {
		t.Errorf("expected orc-aaa.1, got %s", items[0].ID)
	}
}

func TestSearchItems(t *testing.T) {
	store := testStore(t)
	store.CreateItem("orc-aaa", "Fix login bug", "Auth fails on Safari", "bug", 1, "", "")
	store.CreateItem("orc-bbb", "Add dashboard", "New analytics page", "feature", 2, "", "")
	store.CreateItem("orc-ccc", "Refactor auth", "", "chore", 3, "", "")

	// Search by title
	items, _ := store.SearchItems("login")
	if len(items) != 1 || items[0].ID != "orc-aaa" {
		t.Errorf("search 'login' expected orc-aaa, got %v", items)
	}

	// Search by description
	items, _ = store.SearchItems("analytics")
	if len(items) != 1 || items[0].ID != "orc-bbb" {
		t.Errorf("search 'analytics' expected orc-bbb, got %v", items)
	}

	// Search matching multiple
	items, _ = store.SearchItems("auth")
	if len(items) != 2 {
		t.Errorf("search 'auth' expected 2 results, got %d", len(items))
	}
}

func TestAddAndRemoveDep(t *testing.T) {
	store := testStore(t)
	store.CreateItem("orc-aaa", "A", "", "task", 2, "", "")
	store.CreateItem("orc-bbb", "B", "", "task", 2, "", "")

	// B is blocked by A
	err := store.AddDep("orc-bbb", "orc-aaa")
	if err != nil {
		t.Fatalf("AddDep failed: %v", err)
	}

	blockers, _ := store.GetBlockedBy("orc-bbb")
	if len(blockers) != 1 || blockers[0].BlockerID != "orc-aaa" {
		t.Errorf("expected A to block B, got %v", blockers)
	}

	dependents, _ := store.GetDeps("orc-aaa")
	if len(dependents) != 1 || dependents[0].BlockedID != "orc-bbb" {
		t.Errorf("expected A to have dependent B, got %v", dependents)
	}

	// Remove dep
	store.RemoveDep("orc-bbb", "orc-aaa")
	blockers, _ = store.GetBlockedBy("orc-bbb")
	if len(blockers) != 0 {
		t.Errorf("expected 0 blockers after remove, got %d", len(blockers))
	}
}

func TestAddRelation(t *testing.T) {
	store := testStore(t)
	store.CreateItem("orc-aaa", "A", "", "task", 2, "", "")
	store.CreateItem("orc-bbb", "B", "", "task", 2, "", "")

	err := store.AddRelation("orc-aaa", "orc-bbb", "relates_to")
	if err != nil {
		t.Fatalf("AddRelation failed: %v", err)
	}

	rels, _ := store.GetRelations("orc-aaa")
	if len(rels) != 1 {
		t.Fatalf("expected 1 relation, got %d", len(rels))
	}
	if rels[0].ToID != "orc-bbb" || rels[0].RelType != "relates_to" {
		t.Errorf("unexpected relation: %+v", rels[0])
	}
}

func TestNotes(t *testing.T) {
	store := testStore(t)
	store.CreateItem("orc-aaa", "A", "", "task", 2, "", "")

	store.AddNote("orc-aaa", "first note")
	store.AddNote("orc-aaa", "second note")

	notes, err := store.GetNotes("orc-aaa")
	if err != nil {
		t.Fatalf("GetNotes failed: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(notes))
	}
	if notes[0].Content != "first note" || notes[1].Content != "second note" {
		t.Errorf("notes out of order: %v", notes)
	}
}

func TestCreateItemValidatesParentExists(t *testing.T) {
	store := testStore(t)
	err := store.CreateItem("t-aaa", "Orphan", "", "task", 2, "nonexistent", "")
	if err == nil {
		t.Fatal("expected error for nonexistent parent")
	}
	if !strings.Contains(err.Error(), "parent") {
		t.Errorf("error should mention parent, got: %v", err)
	}
}

func TestGenerateIDNoCollision(t *testing.T) {
	store := testStore(t)
	store.SetConfig("prefix", "t")

	// Generate many IDs, ensure no duplicates
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		title := fmt.Sprintf("Item %d", i)
		id, err := store.GenerateID("", title, "", "")
		if err != nil {
			t.Fatalf("GenerateID failed on attempt %d: %v", i, err)
		}
		if seen[id] {
			t.Fatalf("duplicate ID generated: %s", id)
		}
		seen[id] = true
		store.CreateItem(id, title, "", "task", 2, "", "")
	}
}

func TestCreateItemValidatesType(t *testing.T) {
	store := testStore(t)
	err := store.CreateItem("t-aaa", "Bad", "", "invalid_type", 2, "", "")
	if err == nil {
		t.Fatal("expected error for invalid issue_type")
	}
}

func TestCreateItemValidatesPriority(t *testing.T) {
	store := testStore(t)
	err := store.CreateItem("t-aaa", "Bad", "", "task", 99, "", "")
	if err == nil {
		t.Fatal("expected error for priority out of range")
	}
	err = store.CreateItem("t-bbb", "Bad", "", "task", -1, "", "")
	if err == nil {
		t.Fatal("expected error for negative priority")
	}
}

func TestUpdateItemValidatesStatus(t *testing.T) {
	store := testStore(t)
	store.CreateItem("t-aaa", "A", "", "task", 2, "", "")

	err := store.UpdateItem("t-aaa", map[string]string{"status": "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid status")
	}

	// Valid statuses should work
	for _, s := range []string{"open", "in_progress", "closed"} {
		if err := store.UpdateItem("t-aaa", map[string]string{"status": s}); err != nil {
			t.Errorf("status %q should be valid: %v", s, err)
		}
	}
}

func TestUpdateItemValidatesType(t *testing.T) {
	store := testStore(t)
	store.CreateItem("t-aaa", "A", "", "task", 2, "", "")

	err := store.UpdateItem("t-aaa", map[string]string{"issue_type": "nope"})
	if err == nil {
		t.Fatal("expected error for invalid issue_type")
	}
}

func TestReadyItemsNoBlockers(t *testing.T) {
	store := testStore(t)
	store.CreateItem("orc-aaa", "A", "", "task", 2, "", "")

	items, err := store.ReadyItems("")
	if err != nil {
		t.Fatalf("ReadyItems failed: %v", err)
	}
	if len(items) != 1 || items[0].ID != "orc-aaa" {
		t.Errorf("expected [orc-aaa], got %v", items)
	}
}

func TestReadyItemsWithOpenBlocker(t *testing.T) {
	store := testStore(t)
	store.CreateItem("orc-aaa", "Blocker", "", "task", 2, "", "")
	store.CreateItem("orc-bbb", "Blocked", "", "task", 2, "", "")
	store.AddDep("orc-bbb", "orc-aaa")

	items, _ := store.ReadyItems("")
	// Only A should be ready, B is blocked
	if len(items) != 1 || items[0].ID != "orc-aaa" {
		t.Errorf("expected [orc-aaa], got %v", items)
	}
}

func TestReadyItemsBlockerClosed(t *testing.T) {
	store := testStore(t)
	store.CreateItem("orc-aaa", "Blocker", "", "task", 2, "", "")
	store.CreateItem("orc-bbb", "Blocked", "", "task", 2, "", "")
	store.AddDep("orc-bbb", "orc-aaa")
	store.CloseItem("orc-aaa")

	items, _ := store.ReadyItems("")
	// B should now be ready (A is closed), A should not appear (closed)
	if len(items) != 1 || items[0].ID != "orc-bbb" {
		t.Errorf("expected [orc-bbb], got %v", items)
	}
}

func TestReadyItemsScopedToParent(t *testing.T) {
	store := testStore(t)
	store.CreateItem("orc-aaa", "Epic", "", "epic", 2, "", "")
	store.CreateItem("orc-aaa.1", "Child 1", "", "task", 2, "orc-aaa", "")
	store.CreateItem("orc-bbb", "Other", "", "task", 2, "", "")

	items, _ := store.ReadyItems("orc-aaa")
	if len(items) != 1 || items[0].ID != "orc-aaa.1" {
		t.Errorf("expected [orc-aaa.1], got %v", items)
	}
}

func TestReadyItemsSortOrder(t *testing.T) {
	store := testStore(t)
	// Create items with different priorities (lower = more important)
	store.CreateItem("orc-aaa", "Low priority", "", "task", 3, "", "")
	store.CreateItem("orc-bbb", "High priority", "", "task", 0, "", "")
	store.CreateItem("orc-ccc", "Medium priority", "", "task", 1, "", "")

	items, _ := store.ReadyItems("")
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if items[0].ID != "orc-bbb" || items[1].ID != "orc-ccc" || items[2].ID != "orc-aaa" {
		t.Errorf("wrong sort order: %s, %s, %s", items[0].ID, items[1].ID, items[2].ID)
	}
}

func TestReadyItemsIncludesInProgress(t *testing.T) {
	store := testStore(t)
	store.CreateItem("orc-aaa", "In Progress", "", "task", 2, "", "")
	store.UpdateItem("orc-aaa", map[string]string{"status": "in_progress"})

	items, _ := store.ReadyItems("")
	if len(items) != 1 || items[0].ID != "orc-aaa" {
		t.Errorf("expected in_progress item to be ready, got %v", items)
	}
}
