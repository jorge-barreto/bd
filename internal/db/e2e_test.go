package db

import (
	"encoding/json"
	"testing"

	"github.com/jorge-barreto/bd/internal/model"
)

// TestE2ERoundTrip verifies the full lifecycle: create → show → update → close → reopen → delete
func TestE2ERoundTrip(t *testing.T) {
	store := testStore(t)
	store.SetConfig("prefix", "test")

	// Create
	id, err := store.GenerateID("", "Round trip task", "A test item", "alice@test.com")
	if err != nil {
		t.Fatalf("GenerateID: %v", err)
	}
	if err := store.CreateItem(id, "Round trip task", "A test item", "task", 2, "", "alice@test.com"); err != nil {
		t.Fatalf("CreateItem: %v", err)
	}

	// Show (get)
	item, err := store.GetItem(id)
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if item.Title != "Round trip task" || item.Status != "open" {
		t.Fatalf("unexpected item state: %+v", item)
	}

	// Update
	if err := store.UpdateItem(id, map[string]string{"status": "in_progress"}); err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}
	item, _ = store.GetItem(id)
	if item.Status != "in_progress" {
		t.Fatalf("status = %q after update, want in_progress", item.Status)
	}

	// Append note
	if err := store.AddNote(id, "work started"); err != nil {
		t.Fatalf("AddNote: %v", err)
	}
	notes, _ := store.GetNotes(id)
	if len(notes) != 1 || notes[0].Content != "work started" {
		t.Fatalf("unexpected notes: %v", notes)
	}

	// Close
	if err := store.CloseItem(id); err != nil {
		t.Fatalf("CloseItem: %v", err)
	}
	item, _ = store.GetItem(id)
	if item.Status != "closed" {
		t.Fatalf("status = %q after close, want closed", item.Status)
	}

	// Reopen
	if err := store.ReopenItem(id); err != nil {
		t.Fatalf("ReopenItem: %v", err)
	}
	item, _ = store.GetItem(id)
	if item.Status != "open" {
		t.Fatalf("status = %q after reopen, want open", item.Status)
	}

	// Delete
	if err := store.DeleteItem(id); err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}
	if _, err := store.GetItem(id); err == nil {
		t.Fatal("expected error after delete, item still exists")
	}
}

// TestE2EDependencyChain verifies: create A, create B, dep add B A,
// ready shows only A, close A, ready shows B
func TestE2EDependencyChain(t *testing.T) {
	store := testStore(t)

	store.CreateItem("t-aaa", "Task A", "", "task", 2, "", "")
	store.CreateItem("t-bbb", "Task B", "", "task", 2, "", "")
	store.AddDep("t-bbb", "t-aaa") // B blocked by A

	// Only A should be ready
	ready, _ := store.ReadyItems("")
	if len(ready) != 1 || ready[0].ID != "t-aaa" {
		t.Fatalf("expected only t-aaa ready, got %v", ids(ready))
	}

	// Close A → B becomes ready
	store.CloseItem("t-aaa")
	ready, _ = store.ReadyItems("")
	if len(ready) != 1 || ready[0].ID != "t-bbb" {
		t.Fatalf("expected only t-bbb ready after closing A, got %v", ids(ready))
	}
}

// TestE2EDependencyDirection verifies GetBlockedBy returns blockers and GetDeps returns dependents
func TestE2EDependencyDirection(t *testing.T) {
	store := testStore(t)

	store.CreateItem("t-aaa", "Task A", "", "task", 2, "", "")
	store.CreateItem("t-bbb", "Task B", "", "task", 2, "", "")
	store.AddDep("t-bbb", "t-aaa") // B is blocked by A

	// GetBlockedBy(B) should return A as blocker (B's dependencies)
	blockers, _ := store.GetBlockedBy("t-bbb")
	if len(blockers) != 1 || blockers[0].BlockerID != "t-aaa" {
		t.Errorf("B's blockers (dependencies) should be [A], got %v", blockers)
	}

	// GetDeps(A) should return B as dependent (A blocks B)
	dependents, _ := store.GetDeps("t-aaa")
	if len(dependents) != 1 || dependents[0].BlockedID != "t-bbb" {
		t.Errorf("A's dependents should be [B], got %v", dependents)
	}
}

// TestE2EParentChildReady verifies ready scoped to a parent
func TestE2EParentChildReady(t *testing.T) {
	store := testStore(t)

	store.CreateItem("t-epic", "Epic", "", "epic", 1, "", "")
	store.CreateItem("t-epic.1", "Child 1", "", "task", 2, "t-epic", "")
	store.CreateItem("t-epic.2", "Child 2", "", "task", 3, "t-epic", "")
	store.CreateItem("t-other", "Orphan", "", "task", 0, "", "")

	// Scoped ready should only return epic's children
	ready, _ := store.ReadyItems("t-epic")
	if len(ready) != 2 {
		t.Fatalf("expected 2 ready children, got %d", len(ready))
	}
	if ready[0].ID != "t-epic.1" || ready[1].ID != "t-epic.2" {
		t.Fatalf("wrong order or items: %v", ids(ready))
	}
}

// TestE2EReadyJSON verifies the JSON output structure matches spec
func TestE2EReadyJSON(t *testing.T) {
	store := testStore(t)

	store.CreateItem("t-epic", "Epic", "", "epic", 1, "", "")
	store.CreateItem("t-epic.1", "Task 1", "", "task", 2, "t-epic", "")

	items, _ := store.ReadyItems("t-epic")

	// Simulate the JSON encoding that the CLI does
	type readyItem struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Status    string `json:"status"`
		Priority  int    `json:"priority"`
		IssueType string `json:"issue_type"`
		ParentID  string `json:"parent_id,omitempty"`
	}
	out := struct {
		Total int         `json:"total"`
		Items []readyItem `json:"items"`
	}{
		Total: len(items),
		Items: make([]readyItem, len(items)),
	}
	for i, item := range items {
		out.Items[i] = readyItem{
			ID: item.ID, Title: item.Title, Status: item.Status,
			Priority: item.Priority, IssueType: item.IssueType, ParentID: item.ParentID,
		}
	}

	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}

	// Verify structure by unmarshaling into a map
	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)

	if parsed["total"].(float64) != 1 {
		t.Errorf("total = %v, want 1", parsed["total"])
	}
	arr := parsed["items"].([]interface{})
	first := arr[0].(map[string]interface{})
	if first["id"] != "t-epic.1" {
		t.Errorf("first item id = %v, want t-epic.1", first["id"])
	}
	if first["parent_id"] != "t-epic" {
		t.Errorf("parent_id = %v, want t-epic", first["parent_id"])
	}
}

// TestE2ECascadeDelete verifies deleting a parent removes children, deps, and notes
func TestE2ECascadeDelete(t *testing.T) {
	store := testStore(t)

	store.CreateItem("t-epic", "Epic", "", "epic", 1, "", "")
	store.CreateItem("t-epic.1", "Child", "", "task", 2, "t-epic", "")
	store.CreateItem("t-other", "Other", "", "task", 2, "", "")
	store.AddNote("t-epic", "epic note")
	store.AddNote("t-epic.1", "child note")
	store.AddDep("t-other", "t-epic")           // other blocked by epic
	store.AddRelation("t-epic", "t-other", "relates_to")

	store.DeleteItem("t-epic")

	// Parent gone
	if _, err := store.GetItem("t-epic"); err == nil {
		t.Error("parent should be deleted")
	}
	// Child gone
	if _, err := store.GetItem("t-epic.1"); err == nil {
		t.Error("child should be cascade deleted")
	}
	// Dep cleaned up
	deps, _ := store.GetBlockedBy("t-other")
	if len(deps) != 0 {
		t.Errorf("expected 0 deps after cascade delete, got %d", len(deps))
	}
	// Other item still exists
	if _, err := store.GetItem("t-other"); err != nil {
		t.Error("unrelated item should not be deleted")
	}
}

func ids(items []model.Item) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.ID
	}
	return out
}
