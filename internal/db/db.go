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
