package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

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
