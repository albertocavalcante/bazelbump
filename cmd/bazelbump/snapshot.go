// Copyright 2025 bazelbump authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/albertocavalcante/bazelbump/pkg/track"
)

func runSnapshot(args []string) error {
	if len(args) < 1 {
		printSnapshotUsage()
		return fmt.Errorf("snapshot subcommand required")
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "list":
		return runSnapshotList(subargs)
	case "delete":
		return runSnapshotDelete(subargs)
	case "help", "-h", "--help":
		printSnapshotUsage()
		return nil
	default:
		printSnapshotUsage()
		return fmt.Errorf("unknown snapshot subcommand: %s", subcmd)
	}
}

func printSnapshotUsage() {
	fmt.Fprintf(os.Stderr, `Manage bazelbump snapshots.

Snapshots are file backups created when running recipes in non-git directories.
They allow undoing changes even without git version control.

%s
  bazelbump snapshot <subcommand> [flags]

%s
  list     List available snapshots
  delete   Delete a specific snapshot

%s
  bazelbump snapshot list
  bazelbump snapshot list --json
  bazelbump snapshot delete 20240115-143022

Use "bazelbump snapshot <subcommand> --help" for more information.
`, color(colorBold, "Usage:"),
		color(colorBold, "Subcommands:"),
		color(colorBold, "Examples:"))
}

func runSnapshotList(args []string) error {
	fs := flag.NewFlagSet("snapshot list", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `List available snapshots.

Shows all file-based snapshots that can be used to undo changes.
Snapshots are only available for non-git directories.

%s
  bazelbump snapshot list [flags]

%s
  --path   Path to project directory (default: current directory)
  --json   Output results as JSON
  --help   Show this help

%s
  bazelbump snapshot list
  bazelbump snapshot list --json
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"),
			color(colorBold, "Examples:"))
	}

	projectPath := fs.String("path", ".", "Path to project directory")
	jsonOutput := fs.Bool("json", false, "Output results as JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Check if we're in a git repo
	tracker, err := track.NewTracker(track.TrackerOptions{
		ProjectPath: *projectPath,
	})
	if err != nil {
		return fmt.Errorf("failed to create tracker: %w", err)
	}

	if tracker.Strategy() == "git" {
		fmt.Printf("%s This is a git repository. Snapshots are only used for non-git directories.\n",
			color(colorYellow, "[INFO]"))
		fmt.Printf("    For git repos, use 'bazelbump undo' which uses git checkout.\n")
		return nil
	}

	snapshots, err := tracker.ListSnapshots()
	if err != nil {
		return err
	}

	if *jsonOutput {
		return outputSnapshotListJSON(snapshots)
	}

	outputSnapshotListText(snapshots)
	return nil
}

func outputSnapshotListJSON(snapshots []track.SnapshotInfo) error {
	if snapshots == nil {
		snapshots = []track.SnapshotInfo{}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(snapshots)
}

func outputSnapshotListText(snapshots []track.SnapshotInfo) {
	if len(snapshots) == 0 {
		fmt.Printf("%s No snapshots found.\n", color(colorYellow, "[INFO]"))
		fmt.Printf("\nSnapshots are created when running recipes in non-git directories.\n")
		return
	}

	fmt.Printf("%s Available snapshots:\n\n", color(colorBlue, "==>"))

	// Header
	fmt.Printf("  %-20s  %-25s  %-8s  %s\n", "ID", "RECIPE", "FILES", "TIMESTAMP")
	fmt.Printf("  %s  %s  %s  %s\n",
		strings.Repeat("-", 20),
		strings.Repeat("-", 25),
		strings.Repeat("-", 8),
		strings.Repeat("-", 19))

	// Rows
	for _, s := range snapshots {
		recipeName := s.Recipe
		if len(recipeName) > 25 {
			recipeName = recipeName[:22] + "..."
		}
		fmt.Printf("  %-20s  %-25s  %-8d  %s\n",
			s.ID,
			recipeName,
			s.FileCount,
			s.Timestamp.Format("2006-01-02 15:04:05"))
	}

	fmt.Printf("\nTotal: %d snapshot(s)\n", len(snapshots))
	fmt.Printf("\nTo restore a snapshot:\n")
	fmt.Printf("  bazelbump undo --snapshot <ID>\n")
}

func runSnapshotDelete(args []string) error {
	fs := flag.NewFlagSet("snapshot delete", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Delete a specific snapshot.

Removes a snapshot from the .bazelbump/snapshots directory.
This action cannot be undone.

%s
  bazelbump snapshot delete [flags] <snapshot-id>

%s
  --path    Path to project directory (default: current directory)
  --force   Skip confirmation prompt
  --help    Show this help

%s
  bazelbump snapshot delete 20240115-143022
  bazelbump snapshot delete --force 20240115-143022
`, color(colorBold, "Usage:"),
			color(colorBold, "Flags:"),
			color(colorBold, "Examples:"))
	}

	projectPath := fs.String("path", ".", "Path to project directory")
	force := fs.Bool("force", false, "Skip confirmation prompt")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("snapshot ID required")
	}

	snapshotID := fs.Arg(0)

	// Confirm unless --force
	if !*force {
		fmt.Printf("Delete snapshot %s? This cannot be undone. [y/N]: ", snapshotID)
		var response string
		fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	if err := track.DeleteSnapshot(*projectPath, snapshotID); err != nil {
		return err
	}

	fmt.Printf("%s Deleted snapshot %s.\n", color(colorGreen, "[OK]"), snapshotID)
	return nil
}
