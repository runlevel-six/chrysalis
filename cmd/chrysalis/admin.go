// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/runlevel-six/chrysalis/internal/backup"
	"github.com/runlevel-six/chrysalis/internal/config"
)

// adminEnv is what every admin command gets: the loaded configuration and
// somewhere to write its results.
type adminEnv struct {
	cfg *config.Config
	out io.Writer
	now func() time.Time
}

// adminCommand is one `chrysalis admin <name>` subcommand. To add one, write
// its run function and add a line to adminCommands.
type adminCommand struct {
	name    string
	args    string // shown in usage, such as "<backup> [--force]"
	summary string
	run     func(ctx context.Context, env adminEnv, args []string) error
}

var adminCommands = []adminCommand{
	{"backup", "", "Back up the database and attachments now, then prune old backups", adminBackup},
	{"list-backups", "", "List backups, newest first", adminListBackups},
	{"restore", "<backup> [--force]", "Restore a backup into the data directory (stop the server first)", adminRestore},
	{"quotes", "import --user <email> [--collection <name>] [--replace] <file>", "Import centering-line entries from a YAML or JSON file", adminQuotes},
}

func adminUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: chrysalis admin <command> [arguments]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "commands:")
	for _, c := range adminCommands {
		fmt.Fprintf(w, "  %-36s %s\n", strings.TrimSpace(c.name+" "+c.args), c.summary)
	}
}

func runAdmin(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		adminUsage(stderr)
		if len(args) == 0 {
			return errors.New("admin: missing command")
		}
		return nil
	}
	for _, c := range adminCommands {
		if c.name != args[0] {
			continue
		}
		cfg, err := config.Load(os.Getenv)
		if err != nil {
			return fmt.Errorf("configuration:\n%w", err)
		}
		cfg.Version = version
		return c.run(ctx, adminEnv{cfg: cfg, out: stdout, now: time.Now}, args[1:])
	}
	adminUsage(stderr)
	return fmt.Errorf("admin: unknown command %q", args[0])
}

// parseArgs parses flags anywhere among the arguments (Go's flag package
// stops at the first non-flag) and returns the positional ones.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func adminBackup(ctx context.Context, env adminEnv, args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	if pos, err := parseArgs(fs, args); err != nil {
		return err
	} else if len(pos) > 0 {
		return errors.New("usage: chrysalis admin backup")
	}
	db, err := backup.OpenSource(env.cfg.DBPath())
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	b, removed, err := backup.Run(ctx, backup.Source{
		DB: db, AttachmentsDir: env.cfg.AttachmentsDir(), Version: env.cfg.Version,
	}, backupPolicy(env.cfg), env.now())
	if b.Path != "" {
		fmt.Fprintln(env.out, "backed up to", b.Path)
	}
	for _, r := range removed {
		fmt.Fprintln(env.out, "pruned", r.Path)
	}
	return err
}

func adminListBackups(_ context.Context, env adminEnv, args []string) error {
	if len(args) > 0 {
		return errors.New("usage: chrysalis admin list-backups")
	}
	all, err := backup.List(env.cfg.Backup.Dir)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		fmt.Fprintln(env.out, "no backups in", env.cfg.Backup.Dir)
	}
	for _, b := range all {
		fmt.Fprintln(env.out, b.Name)
	}
	return nil
}

func adminRestore(ctx context.Context, env adminEnv, args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	force := fs.Bool("force", false, "replace existing data (it's moved aside, not deleted)")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: chrysalis admin restore <backup> [--force]\n\n" +
			"<backup> is a backup's name (see list-backups), its full path, or \"latest\"")
	}
	dir, err := resolveBackup(env.cfg.Backup.Dir, pos[0])
	if err != nil {
		return err
	}
	r, err := backup.Restore(ctx, dir, backup.Target{
		DBPath: env.cfg.DBPath(), AttachmentsDir: env.cfg.AttachmentsDir(),
	}, *force, env.now())
	if errors.Is(err, backup.ErrTargetNotEmpty) {
		return fmt.Errorf("%w in %s; add --force to move it aside and restore anyway", err, env.cfg.DataDir)
	}
	if err != nil {
		return err
	}
	if r.MovedAside != "" {
		fmt.Fprintln(env.out, "moved the previous data to", r.MovedAside)
	}
	fmt.Fprintf(env.out, "restored %s (%d attachment files) into %s\n", dir, r.Attachments, env.cfg.DataDir)
	return nil
}

// resolveBackup turns the restore argument into a backup directory: "latest",
// a name in the backup directory, or a path.
func resolveBackup(backupDir, arg string) (string, error) {
	if arg == "latest" {
		all, err := backup.List(backupDir)
		if err != nil {
			return "", err
		}
		if len(all) == 0 {
			return "", fmt.Errorf("no backups in %s", backupDir)
		}
		return all[0].Path, nil
	}
	if !strings.ContainsRune(arg, filepath.Separator) {
		if p := filepath.Join(backupDir, arg); exists(p) {
			return p, nil
		}
	}
	if exists(arg) {
		return arg, nil
	}
	return "", fmt.Errorf("no backup named %q in %s", arg, backupDir)
}

func exists(p string) bool {
	_, err := os.Stat(p) // #nosec G703 -- the operator names the backup on the command line
	return err == nil
}

func backupPolicy(cfg *config.Config) backup.Policy {
	return backup.Policy{Dir: cfg.Backup.Dir, KeepDaily: cfg.Backup.KeepDaily, KeepWeekly: cfg.Backup.KeepWeekly}
}
