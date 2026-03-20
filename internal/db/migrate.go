package db

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// NeedsMigration checks if the database has the old schema (issues table, no items table).
func (s *Store) NeedsMigration() bool {
	var name string
	err := s.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='issues'").Scan(&name)
	if err != nil {
		return false
	}
	// Check that new schema doesn't exist yet
	err = s.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='items'").Scan(&name)
	return err == sql.ErrNoRows
}

// Migrate converts an old beads database (issues table) to the new schema (items table).
// It backs up the database first and performs the migration in a transaction.
func (s *Store) Migrate() error {
	if !s.NeedsMigration() {
		return fmt.Errorf("database does not need migration (no issues table or items table already exists)")
	}

	// Flush WAL to main database file before backup
	s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")

	// Back up the database
	bakPath := s.Path + ".bak"
	if err := copyFile(s.Path, bakPath); err != nil {
		return fmt.Errorf("backing up database: %w", err)
	}
	fmt.Printf("Backed up database to %s\n", bakPath)

	// Everything happens in a transaction so failure is atomic
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Rename old tables that conflict with new schema (inside transaction)
	tx.Exec("ALTER TABLE dependencies RENAME TO old_dependencies")
	tx.Exec("ALTER TABLE config RENAME TO old_config")

	// Create new tables
	if _, err := tx.Exec(schema); err != nil {
		return fmt.Errorf("creating new schema: %w", err)
	}

	// Build parent map from old dependencies table
	parentMap := map[string]string{}
	rows, err := tx.Query("SELECT issue_id, depends_on_id FROM old_dependencies WHERE type='parent-child'")
	if err == nil {
		for rows.Next() {
			var childID, parentID string
			rows.Scan(&childID, &parentID)
			parentMap[childID] = parentID
		}
		rows.Close()
	}

	// Migrate issues → items
	issueRows, err := tx.Query(`
		SELECT id, title, description, issue_type, status, priority, owner, created_at, updated_at, notes
		FROM issues WHERE deleted_at IS NULL`)
	if err != nil {
		return fmt.Errorf("reading issues: %w", err)
	}

	var count int
	type issueRow struct {
		id, title, description, issueType, status string
		priority                                  int
		owner, createdAt, updatedAt, notes        string
	}
	var issues []issueRow

	for issueRows.Next() {
		var r issueRow
		var owner, createdAt, updatedAt, notes sql.NullString
		if err := issueRows.Scan(&r.id, &r.title, &r.description, &r.issueType, &r.status,
			&r.priority, &owner, &createdAt, &updatedAt, &notes); err != nil {
			return fmt.Errorf("scanning issue: %w", err)
		}
		r.owner = owner.String
		r.createdAt = normalizeTimestamp(createdAt.String)
		r.updatedAt = normalizeTimestamp(updatedAt.String)
		r.notes = notes.String

		// Map old statuses to new
		switch r.status {
		case "tombstone":
			continue // skip deleted
		case "blocked", "deferred":
			r.status = "open"
		}

		// Clamp priority
		if r.priority < 0 {
			r.priority = 0
		}
		if r.priority > 4 {
			r.priority = 4
		}

		// Validate issue_type, default to "task" if unknown
		if !validTypes[r.issueType] {
			r.issueType = "task"
		}

		issues = append(issues, r)
	}
	issueRows.Close()

	// Build set of migrated IDs to detect orphaned parents
	migratedIDs := map[string]bool{}
	for _, r := range issues {
		migratedIDs[r.id] = true
	}

	for _, r := range issues {
		parentID := parentMap[r.id]
		// Clear parent if it wasn't migrated (deleted/tombstone)
		if parentID != "" && !migratedIDs[parentID] {
			fmt.Printf("  Warning: clearing parent %s for %s (parent not migrated)\n", parentID, r.id)
			parentID = ""
		}

		_, err := tx.Exec(
			`INSERT INTO items (id, title, description, issue_type, status, priority, parent_id, owner, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.id, r.title, r.description, r.issueType, r.status, r.priority,
			nilIfEmpty(parentID), r.owner, r.createdAt, r.updatedAt,
		)
		if err != nil {
			return fmt.Errorf("inserting item %s: %w", r.id, err)
		}
		count++

		// Migrate notes (inline text → notes table)
		if r.notes != "" {
			_, err := tx.Exec(
				"INSERT INTO notes (item_id, content, created_at) VALUES (?, ?, ?)",
				r.id, r.notes, r.createdAt,
			)
			if err != nil {
				return fmt.Errorf("inserting note for %s: %w", r.id, err)
			}
		}
	}

	// Migrate blocking dependencies
	depRows, err := tx.Query("SELECT issue_id, depends_on_id FROM old_dependencies WHERE type='blocks'")
	if err == nil {
		var deps []struct{ blocked, blocker string }
		for depRows.Next() {
			var blocked, blocker string
			depRows.Scan(&blocked, &blocker)
			deps = append(deps, struct{ blocked, blocker string }{blocked, blocker})
		}
		depRows.Close()

		for _, d := range deps {
			tx.Exec(
				"INSERT OR IGNORE INTO dependencies (blocked_id, blocker_id) VALUES (?, ?)",
				d.blocked, d.blocker,
			)
		}
	}

	// Migrate relations
	relRows, err := tx.Query("SELECT issue_id, depends_on_id FROM old_dependencies WHERE type='relates-to'")
	if err == nil {
		var rels []struct{ from, to string }
		for relRows.Next() {
			var from, to string
			relRows.Scan(&from, &to)
			rels = append(rels, struct{ from, to string }{from, to})
		}
		relRows.Close()

		for _, r := range rels {
			tx.Exec(
				"INSERT OR IGNORE INTO relations (from_id, to_id, rel_type) VALUES (?, ?, 'relates_to')",
				r.from, r.to,
			)
		}
	}

	// Copy config
	cfgRows, err := tx.Query("SELECT key, value FROM old_config")
	if err == nil {
		for cfgRows.Next() {
			var k, v string
			cfgRows.Scan(&k, &v)
			tx.Exec("INSERT OR IGNORE INTO config (key, value) VALUES (?, ?)", k, v)
		}
		cfgRows.Close()
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing migration: %w", err)
	}

	fmt.Printf("Migrated %d items\n", count)
	return nil
}

func normalizeTimestamp(ts string) string {
	if ts == "" {
		return time.Now().UTC().Format(time.RFC3339)
	}
	// Try parsing common SQLite datetime formats
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.999999999Z07:00",
	} {
		if t, err := time.Parse(layout, strings.TrimSpace(ts)); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	// If unparseable, use as-is
	return ts
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
