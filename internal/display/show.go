package display

import (
	"fmt"
	"strings"
	"time"

	"github.com/jorge-barreto/bd/internal/db"
	"github.com/jorge-barreto/bd/internal/model"
)

const separator = "────────────────────────────────────────────────────────"

func statusIcon(status string) string {
	switch status {
	case "closed":
		return "✓"
	case "in_progress":
		return "●"
	default:
		return "○"
	}
}

func formatDate(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return rfc3339
	}
	return t.Format("Jan 2, 2006")
}

func shortID(id string) string {
	// Strip prefix: "orc-4ho" -> "4ho", "orc-4ho.1" -> "4ho.1"
	if idx := strings.Index(id, "-"); idx >= 0 {
		return id[idx+1:]
	}
	return id
}

// Show renders the detail view for an item.
func Show(store *db.Store, item *model.Item, showAll bool) error {
	fmt.Println(item.Title)
	fmt.Println(separator)

	fmt.Printf("  ID:       %s  %s\n", shortID(item.ID), item.ID)
	fmt.Printf("  Type:     %s\n", item.IssueType)
	fmt.Printf("  Status:   %s %s\n", statusIcon(item.Status), item.Status)
	fmt.Printf("  Priority: %d\n", item.Priority)
	if item.Owner != "" {
		fmt.Printf("  Owner:    %s\n", item.Owner)
	}
	fmt.Printf("  Created:  %s\n", formatDate(item.CreatedAt))

	// Children
	children, _ := store.ListItems(db.ListFilters{ParentID: item.ID, All: true})
	var openChildren []model.Item
	var closedCount int
	for _, c := range children {
		if c.Status == "closed" {
			closedCount++
			if showAll {
				openChildren = append(openChildren, c)
			}
		} else {
			openChildren = append(openChildren, c)
		}
	}

	if len(openChildren) > 0 {
		fmt.Println()
		fmt.Println("  Children")
		for i, c := range openChildren {
			connector := "├──"
			if i == len(openChildren)-1 && (!showAll || closedCount == 0) {
				connector = "└──"
			}
			title := c.Title
			if len(title) > 40 {
				title = title[:37] + "..."
			}
			fmt.Printf("  %s %s %-8s %s\n", connector, statusIcon(c.Status), shortID(c.ID), title)
		}
		if closedCount > 0 && !showAll {
			fmt.Printf("  (%d closed children hidden, use --all to show)\n", closedCount)
		}
	}

	// Blocks (items this item blocks)
	deps, _ := store.GetDeps(item.ID)
	if len(deps) > 0 {
		fmt.Println()
		fmt.Println("  Blocks")
		for _, d := range deps {
			if blocked, err := store.GetItem(d.BlockedID); err == nil {
				fmt.Printf("  ──▶ %s %s (%s)\n", statusIcon(blocked.Status), blocked.Title, shortID(blocked.ID))
			}
		}
	}

	// Blocked By
	blockedBy, _ := store.GetBlockedBy(item.ID)
	if len(blockedBy) > 0 {
		fmt.Println()
		fmt.Println("  Blocked By")
		for _, d := range blockedBy {
			if blocker, err := store.GetItem(d.BlockerID); err == nil {
				fmt.Printf("  ◀── %s %s (%s)\n", statusIcon(blocker.Status), blocker.Title, shortID(blocker.ID))
			}
		}
	}

	// Relations
	rels, _ := store.GetRelations(item.ID)
	if len(rels) > 0 {
		fmt.Println()
		fmt.Println("  Relations")
		for _, r := range rels {
			otherID := r.ToID
			if otherID == item.ID {
				otherID = r.FromID
			}
			if other, err := store.GetItem(otherID); err == nil {
				fmt.Printf("  ─── %s %s (%s) [%s]\n", statusIcon(other.Status), other.Title, shortID(other.ID), r.RelType)
			}
		}
	}

	// Description
	if item.Description != "" {
		fmt.Println()
		fmt.Println("  Description")
		fmt.Printf("  %s\n", item.Description)
	}

	// Notes
	notes, _ := store.GetNotes(item.ID)
	if len(notes) > 0 {
		fmt.Println()
		fmt.Println("  Notes")
		for _, n := range notes {
			fmt.Printf("  [%s] %s\n", formatDate(n.CreatedAt), n.Content)
		}
	}

	return nil
}
