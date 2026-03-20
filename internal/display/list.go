package display

import (
	"fmt"

	"github.com/jorge-barreto/bd/internal/model"
)

// List renders a list of items as a table.
func List(items []model.Item) {
	if len(items) == 0 {
		fmt.Println("No items found.")
		return
	}

	for _, item := range items {
		title := item.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		fmt.Printf("%s %-12s P%d  %-12s %s\n",
			statusIcon(item.Status),
			item.ID,
			item.Priority,
			item.IssueType,
			title,
		)
	}
}
