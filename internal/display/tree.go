package display

import (
	"fmt"

	"github.com/jorge-barreto/bd/internal/db"
	"github.com/jorge-barreto/bd/internal/model"
)

// Tree renders a recursive tree view of an item and its descendants.
func Tree(store *db.Store, root *model.Item, maxDepth int, showAll bool) error {
	fmt.Printf("%s %s (%s)\n", statusIcon(root.Status), root.Title, root.ID)
	printChildren(store, root.ID, "", maxDepth, 1, showAll)
	return nil
}

func printChildren(store *db.Store, parentID string, prefix string, maxDepth int, depth int, showAll bool) {
	if depth > maxDepth {
		return
	}

	children, _ := store.ListItems(db.ListFilters{ParentID: parentID, All: true})

	var visible []model.Item
	closedCount := 0
	for _, c := range children {
		if c.Status == "closed" {
			closedCount++
			if showAll {
				visible = append(visible, c)
			}
		} else {
			visible = append(visible, c)
		}
	}

	for i, c := range visible {
		isLast := i == len(visible)-1 && (showAll || closedCount == 0)
		connector := "├── "
		childPrefix := prefix + "│   "
		if isLast {
			connector = "└── "
			childPrefix = prefix + "    "
		}

		title := c.Title
		if len(title) > 120 {
			title = title[:117] + "..."
		}

		fmt.Printf("%s%s%s %-8s %s\n", prefix, connector, statusIcon(c.Status), shortID(c.ID), title)
		printChildren(store, c.ID, childPrefix, maxDepth, depth+1, showAll)
	}

	if closedCount > 0 && !showAll {
		connector := "└── "
		fmt.Printf("%s%s(%d closed, use --all to show)\n", prefix, connector, closedCount)
	}
}
