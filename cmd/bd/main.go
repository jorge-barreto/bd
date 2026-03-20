package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jorge-barreto/bd/internal/db"
	"github.com/jorge-barreto/bd/internal/display"
	"github.com/urfave/cli/v3"
)

func main() {
	app := &cli.Command{
		Name:  "bd",
		Usage: "A fast, minimal work item tracker",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "all", Usage: "show closed items"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return dashboardAction(ctx, cmd)
		},
		Commands: []*cli.Command{
			initCmd(),
			createCmd(),
			showCmd(),
			updateCmd(),
			closeCmd(),
			reopenCmd(),
			deleteCmd(),
			searchCmd(),
			listCmd(),
			readyCmd(),
			syncCmd(),
			depCmd(),
			depsCmd(),
			migrateCmd(),
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func openStore() (*db.Store, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	dbPath, err := db.FindDB(cwd)
	if err != nil {
		return nil, err
	}
	store, err := db.Open(dbPath)
	if err != nil {
		return nil, err
	}
	if store.NeedsMigration() {
		return nil, fmt.Errorf("database has old schema — run 'bd migrate' first")
	}
	return store, nil
}

func initCmd() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Initialize a new beads database",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cwd, _ := os.Getwd()
			store, err := db.Init(cwd)
			if err != nil {
				return err
			}
			defer store.Close()

			// Set default prefix from directory name
			prefix := filepath.Base(cwd)
			store.SetConfig("prefix", prefix)

			// Set default owner from git config
			if out, err := exec.Command("git", "config", "user.email").Output(); err == nil {
				email := strings.TrimSpace(string(out))
				if email != "" {
					store.SetConfig("owner", email)
				}
			}

			fmt.Printf("Initialized beads database at %s\n", store.Path)
			return nil
		},
	}
}

func createCmd() *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "Create a new work item",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "title", Aliases: []string{"t"}, Required: true},
			&cli.StringFlag{Name: "type", Value: "task"},
			&cli.IntFlag{Name: "priority", Aliases: []string{"p"}, Value: 2},
			&cli.StringFlag{Name: "parent"},
			&cli.StringFlag{Name: "description", Aliases: []string{"d"}},
			&cli.StringFlag{Name: "owner"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			parentID := cmd.String("parent")
			id, err := store.GenerateID(parentID)
			if err != nil {
				return err
			}

			owner := cmd.String("owner")
			if owner == "" {
				owner, _ = store.GetConfig("owner")
			}

			err = store.CreateItem(
				id,
				cmd.String("title"),
				cmd.String("description"),
				cmd.String("type"),
				int(cmd.Int("priority")),
				parentID,
				owner,
			)
			if err != nil {
				return err
			}

			fmt.Printf("✓ Created issue: %s\n", id)
			fmt.Printf("  Title: %s\n", cmd.String("title"))
			fmt.Printf("  Priority: P%d\n", cmd.Int("priority"))
			fmt.Printf("  Status: open\n")
			return nil
		},
	}
}

func showCmd() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show details of a work item",
		ArgsUsage: "<id>",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Usage: "output as JSON"},
			&cli.BoolFlag{Name: "all", Usage: "show closed children"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() < 1 {
				return fmt.Errorf("usage: bd show <id>")
			}
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			id := cmd.Args().First()
			item, err := store.GetItem(id)
			if err != nil {
				return fmt.Errorf("item %q not found", id)
			}

			if cmd.Bool("json") {
				deps, _ := store.GetDeps(id)
				blockedBy, _ := store.GetBlockedBy(id)

				type jsonItem struct {
					ID           string   `json:"id"`
					Title        string   `json:"title"`
					Description  string   `json:"description"`
					Status       string   `json:"status"`
					Priority     int      `json:"priority"`
					IssueType    string   `json:"issue_type"`
					Owner        string   `json:"owner"`
					CreatedAt    string   `json:"created_at"`
					UpdatedAt    string   `json:"updated_at"`
					Dependencies []string `json:"dependencies"`
					Dependents   []string `json:"dependents"`
				}

				ji := jsonItem{
					ID:          item.ID,
					Title:       item.Title,
					Description: item.Description,
					Status:      item.Status,
					Priority:    item.Priority,
					IssueType:   item.IssueType,
					Owner:       item.Owner,
					CreatedAt:   item.CreatedAt,
					UpdatedAt:   item.UpdatedAt,
				}

				ji.Dependencies = make([]string, 0)
				for _, d := range blockedBy {
					ji.Dependencies = append(ji.Dependencies, d.BlockerID)
				}
				ji.Dependents = make([]string, 0)
				for _, d := range deps {
					ji.Dependents = append(ji.Dependents, d.BlockedID)
				}

				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode([]jsonItem{ji})
			}

			return display.Show(store, item, cmd.Bool("all"))
		},
	}
}

func updateCmd() *cli.Command {
	return &cli.Command{
		Name:      "update",
		Usage:     "Update a work item",
		ArgsUsage: "<id>",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "status"},
			&cli.StringFlag{Name: "title"},
			&cli.StringFlag{Name: "type"},
			&cli.StringFlag{Name: "priority"},
			&cli.StringFlag{Name: "owner"},
			&cli.StringFlag{Name: "append-notes"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() < 1 {
				return fmt.Errorf("usage: bd update <id>")
			}
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			id := cmd.Args().First()

			// Handle append-notes
			if notes := cmd.String("append-notes"); notes != "" {
				if err := store.AddNote(id, notes); err != nil {
					return err
				}
				fmt.Printf("✓ Added note to %s\n", id)
			}

			// Collect field updates
			fields := map[string]string{}
			if v := cmd.String("status"); v != "" {
				fields["status"] = v
			}
			if v := cmd.String("title"); v != "" {
				fields["title"] = v
			}
			if v := cmd.String("type"); v != "" {
				fields["issue_type"] = v
			}
			if v := cmd.String("priority"); v != "" {
				fields["priority"] = v
			}
			if v := cmd.String("owner"); v != "" {
				fields["owner"] = v
			}

			if len(fields) > 0 {
				if err := store.UpdateItem(id, fields); err != nil {
					return err
				}
				fmt.Printf("✓ Updated %s\n", id)
			}

			return nil
		},
	}
}

func closeCmd() *cli.Command {
	return &cli.Command{
		Name:      "close",
		Usage:     "Close a work item",
		ArgsUsage: "<id>",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() < 1 {
				return fmt.Errorf("usage: bd close <id>")
			}
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			id := cmd.Args().First()
			if err := store.CloseItem(id); err != nil {
				return err
			}
			fmt.Printf("✓ Closed %s\n", id)
			return nil
		},
	}
}

func reopenCmd() *cli.Command {
	return &cli.Command{
		Name:      "reopen",
		Usage:     "Reopen a closed work item",
		ArgsUsage: "<id>",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() < 1 {
				return fmt.Errorf("usage: bd reopen <id>")
			}
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			id := cmd.Args().First()
			if err := store.ReopenItem(id); err != nil {
				return err
			}
			fmt.Printf("✓ Reopened %s\n", id)
			return nil
		},
	}
}

func deleteCmd() *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Usage:     "Permanently delete a work item",
		ArgsUsage: "<id>",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() < 1 {
				return fmt.Errorf("usage: bd delete <id>")
			}
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			id := cmd.Args().First()
			if err := store.DeleteItem(id); err != nil {
				return err
			}
			fmt.Printf("✓ Deleted %s\n", id)
			return nil
		},
	}
}

func searchCmd() *cli.Command {
	return &cli.Command{
		Name:      "search",
		Usage:     "Search items by title and description",
		ArgsUsage: "<query>",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() < 1 {
				return fmt.Errorf("usage: bd search <query>")
			}
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			items, err := store.SearchItems(cmd.Args().First())
			if err != nil {
				return err
			}

			display.List(items)
			return nil
		},
	}
}

func listCmd() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List work items",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "all", Usage: "include closed items"},
			&cli.StringFlag{Name: "status"},
			&cli.StringFlag{Name: "type"},
			&cli.StringFlag{Name: "parent"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			filters := db.ListFilters{
				Status:   cmd.String("status"),
				Type:     cmd.String("type"),
				ParentID: cmd.String("parent"),
				All:      cmd.Bool("all"),
			}

			items, err := store.ListItems(filters)
			if err != nil {
				return err
			}

			display.List(items)
			return nil
		},
	}
}

func readyCmd() *cli.Command {
	return &cli.Command{
		Name:      "ready",
		Usage:     "Show items ready to work on",
		ArgsUsage: "[parent-id]",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Usage: "output as JSON"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			parentID := ""
			if cmd.NArg() > 0 {
				parentID = cmd.Args().First()
			}

			items, err := store.ReadyItems(parentID)
			if err != nil {
				return err
			}

			if cmd.Bool("json") {
				type readyItem struct {
					ID        string `json:"id"`
					Title     string `json:"title"`
					Status    string `json:"status"`
					Priority  int    `json:"priority"`
					IssueType string `json:"issue_type"`
					ParentID  string `json:"parent_id,omitempty"`
				}

				out := struct {
					Total int         `json:"total"`
					Items []readyItem `json:"items"`
				}{
					Total: len(items),
					Items: make([]readyItem, len(items)),
				}

				for i, item := range items {
					out.Items[i] = readyItem{
						ID:        item.ID,
						Title:     item.Title,
						Status:    item.Status,
						Priority:  item.Priority,
						IssueType: item.IssueType,
						ParentID:  item.ParentID,
					}
				}

				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}

			display.List(items)
			return nil
		},
	}
}

func syncCmd() *cli.Command {
	return &cli.Command{
		Name:  "sync",
		Usage: "Sync (no-op)",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			fmt.Println("nothing to sync")
			return nil
		},
	}
}

func depCmd() *cli.Command {
	return &cli.Command{
		Name:  "dep",
		Usage: "Manage dependencies",
		Commands: []*cli.Command{
			{
				Name:      "add",
				Usage:     "Add a dependency (blocked blocker)",
				ArgsUsage: "<blocked> <blocker>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.NArg() < 2 {
						return fmt.Errorf("usage: bd dep add <blocked> <blocker>")
					}
					store, err := openStore()
					if err != nil {
						return err
					}
					defer store.Close()

					blocked := cmd.Args().Get(0)
					blocker := cmd.Args().Get(1)
					if err := store.AddDep(blocked, blocker); err != nil {
						return err
					}
					fmt.Printf("✓ %s is now blocked by %s\n", blocked, blocker)
					return nil
				},
			},
			{
				Name:      "remove",
				Usage:     "Remove a dependency",
				ArgsUsage: "<blocked> <blocker>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.NArg() < 2 {
						return fmt.Errorf("usage: bd dep remove <blocked> <blocker>")
					}
					store, err := openStore()
					if err != nil {
						return err
					}
					defer store.Close()

					blocked := cmd.Args().Get(0)
					blocker := cmd.Args().Get(1)
					if err := store.RemoveDep(blocked, blocker); err != nil {
						return err
					}
					fmt.Printf("✓ Removed dependency: %s blocked by %s\n", blocked, blocker)
					return nil
				},
			},
			{
				Name:      "relate",
				Usage:     "Add a relation between two items",
				ArgsUsage: "<a> <b>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.NArg() < 2 {
						return fmt.Errorf("usage: bd dep relate <a> <b>")
					}
					store, err := openStore()
					if err != nil {
						return err
					}
					defer store.Close()

					a := cmd.Args().Get(0)
					b := cmd.Args().Get(1)
					if err := store.AddRelation(a, b, "relates_to"); err != nil {
						return err
					}
					fmt.Printf("✓ %s relates to %s\n", a, b)
					return nil
				},
			},
		},
	}
}

func depsCmd() *cli.Command {
	return &cli.Command{
		Name:  "deps",
		Usage: "Show dependency chain DAG",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			return display.Deps(store)
		},
	}
}

func migrateCmd() *cli.Command {
	return &cli.Command{
		Name:  "migrate",
		Usage: "Migrate an old beads database to the new schema",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			dbPath, err := db.FindDB(cwd)
			if err != nil {
				return err
			}
			store, err := db.Open(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()

			return store.Migrate()
		},
	}
}

func dashboardAction(ctx context.Context, cmd *cli.Command) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	return display.Dashboard(store, cmd.Bool("all"))
}
