package db

import (
	"os"
	"path/filepath"
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
