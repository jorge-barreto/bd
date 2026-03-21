package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// createOldSchema sets up a database with the old beads schema for testing migration.
func createOldSchema(t *testing.T, dir string) string {
	t.Helper()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(beadsDir, "beads.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Minimal old schema matching the real beads database
	_, err = db.Exec(`
		CREATE TABLE issues (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'open',
			priority INTEGER NOT NULL DEFAULT 2,
			issue_type TEXT NOT NULL DEFAULT 'task',
			owner TEXT DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at DATETIME
		);
		CREATE TABLE dependencies (
			issue_id TEXT NOT NULL,
			depends_on_id TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'blocks',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			created_by TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (issue_id, depends_on_id, type),
			FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE
		);
		CREATE TABLE config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Insert test data
	_, err = db.Exec(`
		INSERT INTO issues (id, title, description, status, priority, issue_type, owner, notes)
		VALUES
			('orc-aaa', 'Epic 1', 'An epic', 'open', 1, 'epic', 'alice@test.com', ''),
			('orc-aaa.1', 'Task 1', 'First task', 'open', 2, 'task', 'alice@test.com', 'Some notes here'),
			('orc-aaa.2', 'Task 2', '', 'closed', 2, 'task', '', ''),
			('orc-bbb', 'Epic 2', '', 'open', 1, 'epic', '', ''),
			('orc-del', 'Deleted', '', 'tombstone', 2, 'task', '', '');
		UPDATE issues SET deleted_at = '2026-01-01' WHERE id = 'orc-del';

		INSERT INTO dependencies (issue_id, depends_on_id, type, created_by)
		VALUES
			('orc-aaa.1', 'orc-aaa', 'parent-child', ''),
			('orc-aaa.2', 'orc-aaa', 'parent-child', ''),
			('orc-bbb', 'orc-aaa', 'blocks', ''),
			('orc-aaa.1', 'orc-aaa.2', 'relates-to', '');

		INSERT INTO config (key, value) VALUES ('prefix', 'orc');
	`)
	if err != nil {
		t.Fatal(err)
	}

	return dbPath
}

func TestNeedsMigration(t *testing.T) {
	dir := t.TempDir()

	// Old schema should need migration
	dbPath := createOldSchema(t, dir)
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if !store.NeedsMigration() {
		t.Fatal("expected NeedsMigration() = true for old schema")
	}

	// New schema should not need migration
	dir2 := t.TempDir()
	store2, err := Init(dir2)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()

	if store2.NeedsMigration() {
		t.Fatal("expected NeedsMigration() = false for new schema")
	}
}

func TestMigrateItems(t *testing.T) {
	dir := t.TempDir()
	dbPath := createOldSchema(t, dir)
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Should no longer need migration
	if store.NeedsMigration() {
		t.Fatal("should not need migration after migrating")
	}

	// Verify items migrated (excluding deleted)
	items, err := store.ListItems(ListFilters{All: true})
	if err != nil {
		t.Fatalf("ListItems failed: %v", err)
	}
	if len(items) != 4 { // orc-aaa, orc-aaa.1, orc-aaa.2, orc-bbb (not orc-del)
		t.Fatalf("expected 4 items, got %d", len(items))
	}

	// Verify parent_id was set correctly
	child, err := store.GetItem("orc-aaa.1")
	if err != nil {
		t.Fatalf("GetItem orc-aaa.1: %v", err)
	}
	if child.ParentID != "orc-aaa" {
		t.Errorf("parent_id = %q, want 'orc-aaa'", child.ParentID)
	}

	// Verify notes migrated
	notes, err := store.GetNotes("orc-aaa.1")
	if err != nil {
		t.Fatalf("GetNotes: %v", err)
	}
	if len(notes) != 1 || notes[0].Content != "Some notes here" {
		t.Errorf("expected 1 note with content 'Some notes here', got %v", notes)
	}

	// Verify blocking dep migrated
	blockedBy, err := store.GetBlockedBy("orc-bbb")
	if err != nil {
		t.Fatalf("GetBlockedBy: %v", err)
	}
	if len(blockedBy) != 1 || blockedBy[0].BlockerID != "orc-aaa" {
		t.Errorf("expected orc-bbb blocked by orc-aaa, got %v", blockedBy)
	}

	// Verify relation migrated
	rels, err := store.GetRelations("orc-aaa.1")
	if err != nil {
		t.Fatalf("GetRelations: %v", err)
	}
	if len(rels) != 1 {
		t.Errorf("expected 1 relation, got %d", len(rels))
	}

	// Verify config preserved
	prefix, _ := store.GetConfig("prefix")
	if prefix != "orc" {
		t.Errorf("config prefix = %q, want 'orc'", prefix)
	}

	// Verify tombstone/deleted items skipped
	_, err = store.GetItem("orc-del")
	if err == nil {
		t.Error("deleted item should not be migrated")
	}
}

func TestMigrateChildBeforeParent(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(beadsDir, "beads.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE issues (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'open',
			priority INTEGER NOT NULL DEFAULT 2,
			issue_type TEXT NOT NULL DEFAULT 'task',
			owner TEXT DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at DATETIME
		);
		CREATE TABLE dependencies (
			issue_id TEXT NOT NULL,
			depends_on_id TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'blocks',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			created_by TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (issue_id, depends_on_id, type),
			FOREIGN KEY (issue_id) REFERENCES issues(id) ON DELETE CASCADE
		);
		CREATE TABLE config (key TEXT PRIMARY KEY, value TEXT NOT NULL);
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Child ID sorts before parent ID alphabetically
	_, err = db.Exec(`
		INSERT INTO issues (id, title, issue_type) VALUES
			('aaa-child', 'Child Task', 'task'),
			('zzz-parent', 'Parent Epic', 'epic');
		INSERT INTO dependencies (issue_id, depends_on_id, type, created_by)
		VALUES ('aaa-child', 'zzz-parent', 'parent-child', '');
		INSERT INTO config (key, value) VALUES ('prefix', 'test');
	`)
	if err != nil {
		t.Fatal(err)
	}

	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed with child-before-parent ordering: %v", err)
	}

	child, err := store.GetItem("aaa-child")
	if err != nil {
		t.Fatalf("GetItem aaa-child: %v", err)
	}
	if child.ParentID != "zzz-parent" {
		t.Errorf("parent_id = %q, want 'zzz-parent'", child.ParentID)
	}
}

func TestMigrateBackup(t *testing.T) {
	dir := t.TempDir()
	dbPath := createOldSchema(t, dir)
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	store.Migrate()

	// Backup file should exist
	bakPath := dbPath + ".bak"
	if _, err := Open(bakPath); err != nil {
		t.Fatalf("backup file not accessible: %v", err)
	}
}
