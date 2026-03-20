package model

// Item represents a work item (task, bug, feature, chore, epic).
type Item struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	IssueType   string `json:"issue_type"`
	Status      string `json:"status"`
	Priority    int    `json:"priority"`
	ParentID    string `json:"parent_id,omitempty"`
	Owner       string `json:"owner,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// Dependency represents a blocking relationship: BlockedID is blocked by BlockerID.
type Dependency struct {
	BlockedID string `json:"blocked_id"`
	BlockerID string `json:"blocker_id"`
}

// Relation represents a non-blocking relationship between two items.
type Relation struct {
	FromID  string `json:"from_id"`
	ToID    string `json:"to_id"`
	RelType string `json:"rel_type"`
}

// Note is an append-only comment on an item.
type Note struct {
	ID        int    `json:"id"`
	ItemID    string `json:"item_id"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}
