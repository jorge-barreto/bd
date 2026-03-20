package db

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jorge-barreto/bd/internal/model"
	_ "modernc.org/sqlite"
)

// Store wraps a SQLite database connection.
type Store struct {
	db   *sql.DB
	Path string
}

const schema = `
CREATE TABLE IF NOT EXISTS items (
	id         TEXT PRIMARY KEY,
	title      TEXT NOT NULL,
	description TEXT,
	issue_type TEXT NOT NULL,
	status     TEXT NOT NULL DEFAULT 'open',
	priority   INTEGER DEFAULT 2,
	parent_id  TEXT REFERENCES items(id),
	owner      TEXT,
	created_at TEXT,
	updated_at TEXT
);

CREATE TABLE IF NOT EXISTS dependencies (
	blocked_id TEXT NOT NULL REFERENCES items(id),
	blocker_id TEXT NOT NULL REFERENCES items(id),
	PRIMARY KEY (blocked_id, blocker_id)
);

CREATE TABLE IF NOT EXISTS relations (
	from_id  TEXT NOT NULL REFERENCES items(id),
	to_id    TEXT NOT NULL REFERENCES items(id),
	rel_type TEXT NOT NULL,
	PRIMARY KEY (from_id, to_id)
);

CREATE TABLE IF NOT EXISTS notes (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	item_id    TEXT NOT NULL REFERENCES items(id),
	content    TEXT NOT NULL,
	created_at TEXT
);

CREATE TABLE IF NOT EXISTS config (
	key   TEXT PRIMARY KEY,
	value TEXT
);
`

// Init creates the .beads directory and database at the given root path.
func Init(root string) (*Store, error) {
	beadsDir := filepath.Join(root, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating .beads directory: %w", err)
	}

	dbPath := filepath.Join(beadsDir, "beads.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating schema: %w", err)
	}

	return &Store{db: db, Path: dbPath}, nil
}

// Open opens an existing database at the given path.
func Open(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	return &Store{db: db, Path: dbPath}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// FindDB locates the beads database by walking up from startDir.
// If BEADS_DIR is set, it uses that directory directly.
func FindDB(startDir string) (string, error) {
	if envDir := os.Getenv("BEADS_DIR"); envDir != "" {
		dbPath := filepath.Join(envDir, "beads.db")
		if _, err := os.Stat(dbPath); err == nil {
			return dbPath, nil
		}
		return "", fmt.Errorf("BEADS_DIR set to %q but no beads.db found there", envDir)
	}

	dir := startDir
	for {
		candidate := filepath.Join(dir, ".beads", "beads.db")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("no .beads/beads.db found (searched up from %s)", startDir)
}

const alphanum = "abcdefghijklmnopqrstuvwxyz0123456789"

func randomAlphanum(n int) string {
	b := make([]byte, n)
	for i := range b {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphanum))))
		b[i] = alphanum[idx.Int64()]
	}
	return string(b)
}

// GenerateID creates a new ID. If parentID is empty, generates a top-level ID
// using the configured prefix. Otherwise generates a child ID as parentID.{seq}.
func (s *Store) GenerateID(parentID string) (string, error) {
	if parentID == "" {
		prefix, _ := s.GetConfig("prefix")
		if prefix == "" {
			prefix = "orc"
		}
		return prefix + "-" + randomAlphanum(3), nil
	}

	// Find max existing child sequence number
	var maxSeq int
	rows, err := s.db.Query("SELECT id FROM items WHERE parent_id = ?", parentID)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var childID string
		rows.Scan(&childID)
		// Extract the last segment after the last "."
		parts := strings.Split(childID, ".")
		if len(parts) > 0 {
			if seq, err := strconv.Atoi(parts[len(parts)-1]); err == nil && seq > maxSeq {
				maxSeq = seq
			}
		}
	}

	return parentID + "." + strconv.Itoa(maxSeq+1), nil
}

// SetConfig sets a configuration value.
func (s *Store) SetConfig(key, value string) error {
	_, err := s.db.Exec(
		"INSERT INTO config (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = ?",
		key, value, value,
	)
	return err
}

// GetConfig gets a configuration value. Returns empty string if not found.
func (s *Store) GetConfig(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// CreateItem inserts a new item with the given fields.
func (s *Store) CreateItem(id, title, description, issueType string, priority int, parentID, owner string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT INTO items (id, title, description, issue_type, status, priority, parent_id, owner, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'open', ?, ?, ?, ?, ?)`,
		id, title, description, issueType, priority, nilIfEmpty(parentID), owner, now, now,
	)
	return err
}

// GetItem retrieves a single item by ID.
func (s *Store) GetItem(id string) (*model.Item, error) {
	item := &model.Item{}
	var parentID, description, owner sql.NullString
	err := s.db.QueryRow(
		`SELECT id, title, description, issue_type, status, priority, parent_id, owner, created_at, updated_at
		 FROM items WHERE id = ?`, id,
	).Scan(&item.ID, &item.Title, &description, &item.IssueType, &item.Status,
		&item.Priority, &parentID, &owner, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, err
	}
	item.ParentID = parentID.String
	item.Description = description.String
	item.Owner = owner.String
	return item, nil
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// UpdateItem updates the given fields on an item.
func (s *Store) UpdateItem(id string, fields map[string]string) error {
	allowed := map[string]bool{
		"title": true, "description": true, "issue_type": true,
		"status": true, "priority": true, "owner": true,
	}

	var sets []string
	var args []interface{}
	for k, v := range fields {
		if !allowed[k] {
			return fmt.Errorf("unknown field: %s", k)
		}
		sets = append(sets, k+" = ?")
		args = append(args, v)
	}
	if len(sets) == 0 {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	sets = append(sets, "updated_at = ?")
	args = append(args, now, id)

	query := "UPDATE items SET " + strings.Join(sets, ", ") + " WHERE id = ?"
	res, err := s.db.Exec(query, args...)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("item %q not found", id)
	}
	return nil
}

// CloseItem sets an item's status to closed.
func (s *Store) CloseItem(id string) error {
	return s.UpdateItem(id, map[string]string{"status": "closed"})
}

// ReopenItem sets an item's status to open.
func (s *Store) ReopenItem(id string) error {
	return s.UpdateItem(id, map[string]string{"status": "open"})
}

// DeleteItem permanently removes an item and its children, deps, relations, and notes.
func (s *Store) DeleteItem(id string) error {
	// Collect all IDs to delete (item + descendants)
	ids := []string{id}
	s.collectChildren(id, &ids)

	for _, itemID := range ids {
		s.db.Exec("DELETE FROM notes WHERE item_id = ?", itemID)
		s.db.Exec("DELETE FROM dependencies WHERE blocked_id = ? OR blocker_id = ?", itemID, itemID)
		s.db.Exec("DELETE FROM relations WHERE from_id = ? OR to_id = ?", itemID, itemID)
	}

	// Delete children first (leaf-to-root) to satisfy FK constraints
	for i := len(ids) - 1; i >= 0; i-- {
		s.db.Exec("DELETE FROM items WHERE id = ?", ids[i])
	}

	return nil
}

func (s *Store) collectChildren(parentID string, ids *[]string) {
	rows, err := s.db.Query("SELECT id FROM items WHERE parent_id = ?", parentID)
	if err != nil {
		return
	}
	defer rows.Close()

	var children []string
	for rows.Next() {
		var childID string
		rows.Scan(&childID)
		children = append(children, childID)
	}

	for _, child := range children {
		*ids = append(*ids, child)
		s.collectChildren(child, ids)
	}
}

// AddDep adds a dependency: blockedID is blocked by blockerID.
func (s *Store) AddDep(blockedID, blockerID string) error {
	_, err := s.db.Exec(
		"INSERT INTO dependencies (blocked_id, blocker_id) VALUES (?, ?)",
		blockedID, blockerID,
	)
	return err
}

// RemoveDep removes a dependency.
func (s *Store) RemoveDep(blockedID, blockerID string) error {
	_, err := s.db.Exec(
		"DELETE FROM dependencies WHERE blocked_id = ? AND blocker_id = ?",
		blockedID, blockerID,
	)
	return err
}

// GetBlockedBy returns items that block the given item.
func (s *Store) GetBlockedBy(itemID string) ([]model.Dependency, error) {
	rows, err := s.db.Query(
		"SELECT blocked_id, blocker_id FROM dependencies WHERE blocked_id = ?", itemID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []model.Dependency
	for rows.Next() {
		var d model.Dependency
		rows.Scan(&d.BlockedID, &d.BlockerID)
		deps = append(deps, d)
	}
	return deps, nil
}

// GetDeps returns items that the given item blocks.
func (s *Store) GetDeps(itemID string) ([]model.Dependency, error) {
	rows, err := s.db.Query(
		"SELECT blocked_id, blocker_id FROM dependencies WHERE blocker_id = ?", itemID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []model.Dependency
	for rows.Next() {
		var d model.Dependency
		rows.Scan(&d.BlockedID, &d.BlockerID)
		deps = append(deps, d)
	}
	return deps, nil
}

// AddRelation adds a relation between two items.
func (s *Store) AddRelation(fromID, toID, relType string) error {
	_, err := s.db.Exec(
		"INSERT INTO relations (from_id, to_id, rel_type) VALUES (?, ?, ?)",
		fromID, toID, relType,
	)
	return err
}

// GetRelations returns all relations involving the given item.
func (s *Store) GetRelations(itemID string) ([]model.Relation, error) {
	rows, err := s.db.Query(
		"SELECT from_id, to_id, rel_type FROM relations WHERE from_id = ? OR to_id = ?",
		itemID, itemID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rels []model.Relation
	for rows.Next() {
		var r model.Relation
		rows.Scan(&r.FromID, &r.ToID, &r.RelType)
		rels = append(rels, r)
	}
	return rels, nil
}

// AddNote appends a note to an item.
func (s *Store) AddNote(itemID, content string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		"INSERT INTO notes (item_id, content, created_at) VALUES (?, ?, ?)",
		itemID, content, now,
	)
	return err
}

// GetNotes returns all notes for an item in chronological order.
func (s *Store) GetNotes(itemID string) ([]model.Note, error) {
	rows, err := s.db.Query(
		"SELECT id, item_id, content, created_at FROM notes WHERE item_id = ? ORDER BY created_at ASC",
		itemID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []model.Note
	for rows.Next() {
		var n model.Note
		rows.Scan(&n.ID, &n.ItemID, &n.Content, &n.CreatedAt)
		notes = append(notes, n)
	}
	return notes, nil
}

// ListFilters specifies optional filters for ListItems.
type ListFilters struct {
	Status   string
	Type     string
	ParentID string
	All      bool // include closed items
}

// ListItems returns items matching the given filters.
func (s *Store) ListItems(f ListFilters) ([]model.Item, error) {
	query := "SELECT id, title, description, issue_type, status, priority, parent_id, owner, created_at, updated_at FROM items"
	var conditions []string
	var args []interface{}

	if f.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, f.Status)
	} else if !f.All {
		conditions = append(conditions, "status != 'closed'")
	}
	if f.Type != "" {
		conditions = append(conditions, "issue_type = ?")
		args = append(args, f.Type)
	}
	if f.ParentID != "" {
		conditions = append(conditions, "parent_id = ?")
		args = append(args, f.ParentID)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY priority ASC, created_at ASC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanItems(rows)
}

// SearchItems does a full-text search across title and description.
func (s *Store) SearchItems(query string) ([]model.Item, error) {
	pattern := "%" + query + "%"
	rows, err := s.db.Query(
		`SELECT id, title, description, issue_type, status, priority, parent_id, owner, created_at, updated_at
		 FROM items WHERE title LIKE ? OR description LIKE ?
		 ORDER BY priority ASC, created_at ASC`,
		pattern, pattern,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanItems(rows)
}

func scanItems(rows *sql.Rows) ([]model.Item, error) {
	var items []model.Item
	for rows.Next() {
		var item model.Item
		var parentID, description, owner sql.NullString
		err := rows.Scan(&item.ID, &item.Title, &description, &item.IssueType, &item.Status,
			&item.Priority, &parentID, &owner, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			return nil, err
		}
		item.ParentID = parentID.String
		item.Description = description.String
		item.Owner = owner.String
		items = append(items, item)
	}
	return items, nil
}

// ReadyItems returns items that are ready to work on.
// An item is ready if: status is open or in_progress, and all its blockers are closed.
// If parentID is non-empty, only items with that parent are returned.
func (s *Store) ReadyItems(parentID string) ([]model.Item, error) {
	query := `
		SELECT i.id, i.title, i.description, i.issue_type, i.status, i.priority,
		       i.parent_id, i.owner, i.created_at, i.updated_at
		FROM items i
		WHERE i.status IN ('open', 'in_progress')
		  AND NOT EXISTS (
		    SELECT 1 FROM dependencies d
		    JOIN items blocker ON blocker.id = d.blocker_id
		    WHERE d.blocked_id = i.id AND blocker.status != 'closed'
		  )`

	var args []interface{}
	if parentID != "" {
		query += " AND i.parent_id = ?"
		args = append(args, parentID)
	}

	query += " ORDER BY i.priority ASC, i.created_at ASC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanItems(rows)
}
