package display

import (
	"fmt"

	"github.com/jorge-barreto/bd/internal/db"
	"github.com/jorge-barreto/bd/internal/model"
)

// Dashboard renders the main dashboard view with epics and their children.
func Dashboard(store *db.Store, showAll bool) error {
	allItems, err := store.ListItems(db.ListFilters{})
	if err != nil {
		return err
	}

	if len(allItems) == 0 {
		fmt.Println("No items. Run 'bd create' to get started.")
		return nil
	}

	// Separate epics and orphans
	var epics []model.Item
	var orphans []model.Item
	childrenByParent := map[string][]model.Item{}

	for _, item := range allItems {
		if item.ParentID != "" {
			childrenByParent[item.ParentID] = append(childrenByParent[item.ParentID], item)
		} else if item.IssueType == "epic" {
			epics = append(epics, item)
		} else {
			orphans = append(orphans, item)
		}
	}

	if len(epics) > 0 {
		fmt.Printf("%-52s %s\n", "EPICS", "status")
		fmt.Println(separator)

		for _, epic := range epics {
			children := childrenByParent[epic.ID]
			openCount := 0
			totalCount := len(children)
			for _, c := range children {
				if c.Status != "closed" {
					openCount++
				}
			}

			fmt.Printf("%s %s  %d/%d open\n", statusIcon(epic.Status), epic.Title, openCount, totalCount)

			// Show children
			var visibleChildren []model.Item
			for _, c := range children {
				if c.Status != "closed" || showAll {
					visibleChildren = append(visibleChildren, c)
				}
			}

			for i, c := range visibleChildren {
				connector := "├──"
				if i == len(visibleChildren)-1 {
					connector = "└──"
				}
				title := c.Title
				if len(title) > 45 {
					title = title[:42] + "..."
				}
				fmt.Printf("  %s %s %-8s %s\n", connector, statusIcon(c.Status), shortID(c.ID), title)
			}

			// Show blocking relationships
			deps, _ := store.GetDeps(epic.ID)
			for _, d := range deps {
				if blocked, err := store.GetItem(d.BlockedID); err == nil {
					fmt.Printf("  ──▶ blocks: %s\n", blocked.Title)
				}
			}
			blockedBy, _ := store.GetBlockedBy(epic.ID)
			for _, d := range blockedBy {
				if blocker, err := store.GetItem(d.BlockerID); err == nil {
					fmt.Printf("  ◀── blocked by: %s\n", blocker.Title)
				}
			}
		}
	}

	if len(orphans) > 0 {
		if len(epics) > 0 {
			fmt.Println()
		}
		fmt.Println("ORPHANS")
		fmt.Println(separator)
		for _, item := range orphans {
			if item.Status == "closed" && !showAll {
				continue
			}
			title := item.Title
			if len(title) > 50 {
				title = title[:47] + "..."
			}
			fmt.Printf("%s %-12s %s\n", statusIcon(item.Status), shortID(item.ID), title)
		}
	}

	return nil
}

// Deps renders the dependency chain DAG across epics.
func Deps(store *db.Store) error {
	epics, err := store.ListItems(db.ListFilters{Type: "epic"})
	if err != nil {
		return err
	}

	if len(epics) == 0 {
		fmt.Println("No epics with dependencies found.")
		return nil
	}

	fmt.Println("DEPENDENCY CHAIN")
	fmt.Println(separator)

	// Find root epics (not blocked by any other epic)
	blockedSet := map[string]bool{}
	for _, epic := range epics {
		blockedBy, _ := store.GetBlockedBy(epic.ID)
		for _, d := range blockedBy {
			blockedSet[epic.ID] = true
			_ = d
		}
	}

	for _, epic := range epics {
		if blockedSet[epic.ID] {
			continue
		}
		printDepChain(store, epic, 0)
	}

	return nil
}

func printDepChain(store *db.Store, item model.Item, depth int) {
	indent := ""
	for i := 0; i < depth; i++ {
		indent += "  "
	}

	children, _ := store.ListItems(db.ListFilters{ParentID: item.ID})
	openCount := 0
	for _, c := range children {
		if c.Status != "closed" {
			openCount++
		}
	}

	fmt.Printf("%s%s %-8s %s  %d/%d open\n",
		indent, statusIcon(item.Status), shortID(item.ID), item.Title, openCount, len(children))

	// Follow dependency chain
	deps, _ := store.GetDeps(item.ID)
	for _, d := range deps {
		if blocked, err := store.GetItem(d.BlockedID); err == nil {
			fmt.Printf("%s└──▶\n", indent)
			printDepChain(store, *blocked, depth+1)
		}
	}
}
