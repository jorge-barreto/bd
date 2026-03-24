package display

import (
	"sort"
	"strconv"
	"strings"

	"github.com/jorge-barreto/bd/internal/model"
)

// sortItems sorts items by priority, then by the trailing numeric segment of the ID.
func sortItems(items []model.Item) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Priority != items[j].Priority {
			return items[i].Priority < items[j].Priority
		}
		return trailingNum(items[i].ID) < trailingNum(items[j].ID)
	})
}

func trailingNum(id string) int {
	if idx := strings.LastIndex(id, "."); idx >= 0 {
		if n, err := strconv.Atoi(id[idx+1:]); err == nil {
			return n
		}
	}
	return 0
}
