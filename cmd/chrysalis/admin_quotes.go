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
	"strings"

	"github.com/runlevel-six/chrysalis/internal/service"
	"github.com/runlevel-six/chrysalis/internal/store"
)

const quotesUsage = "usage: chrysalis admin quotes import --user <email> [--collection <name>] [--replace] <file.yaml|file.json|->\n\n" +
	"\"-\" reads the file from standard input (the container image has no shell to copy files into)"

// adminQuotes imports centering-line entries for a person (docs/DESIGN.md
// §3.13), for example through kubectl exec. It's safe while the server runs:
// SQLite takes one writer at a time, and open pages show the new entries on
// their next load.
func adminQuotes(ctx context.Context, env adminEnv, args []string) error {
	if len(args) == 0 || args[0] != "import" {
		return errors.New(quotesUsage)
	}
	fs := flag.NewFlagSet("quotes import", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	email := fs.String("user", "", "the email address of the person to import for")
	collection := fs.String("collection", "", "the collection to add to (created if needed; defaults to the name in the file)")
	replace := fs.Bool("replace", false, "move the collection's current entries to the trash first")
	pos, err := parseArgs(fs, args[1:])
	if err != nil {
		return fmt.Errorf("%w\n%s", err, quotesUsage)
	}
	if len(pos) != 1 || strings.TrimSpace(*email) == "" {
		return errors.New(quotesUsage)
	}
	data, err := readLimited(pos[0], service.MaxQuoteFileBytes)
	if err != nil {
		return err
	}
	format := ""
	switch lower := strings.ToLower(pos[0]); {
	case strings.HasSuffix(lower, ".json"):
		format = "json"
	case strings.HasSuffix(lower, ".yaml"), strings.HasSuffix(lower, ".yml"):
		format = "yaml"
	}
	file, err := service.ParseQuoteFile(data, format)
	if err != nil {
		return err
	}

	if !exists(env.cfg.DBPath()) {
		return fmt.Errorf("no database at %s; check CHRYSALIS_DATA_DIR", env.cfg.DBPath())
	}
	st, err := store.Open(ctx, env.cfg.DBPath())
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	users, err := st.ListUsersByEmail(ctx, strings.TrimSpace(*email))
	if err != nil {
		return err
	}
	switch len(users) {
	case 0:
		return fmt.Errorf("no user with the email %s (they need to sign in once first)", *email)
	case 1:
	default:
		return fmt.Errorf("%d users share the email %s; this command can't tell them apart", len(users), *email)
	}
	c := service.NewCentering(service.NewBoards(st, env.now))
	res, err := c.Import(ctx, users[0].ID, service.ImportTarget{Name: *collection}, file, *replace)
	if err != nil {
		return err
	}
	verb := "added to"
	if res.Created {
		verb = "created"
	}
	fmt.Fprintf(env.out, "%s collection %q for %s: %d entries added", verb, res.Collection.Name, *email, res.Added)
	if *replace {
		fmt.Fprintf(env.out, ", %d moved to the trash", res.Replaced)
	}
	fmt.Fprintf(env.out, " (%d in the collection now)\n", res.Collection.EntryCount)
	return nil
}

// readLimited reads a file the operator named ("-" for standard input),
// refusing one over limit.
func readLimited(path string, limit int64) ([]byte, error) {
	var r io.Reader = os.Stdin
	if path != "-" {
		f, err := os.Open(path) // #nosec G304 G703 -- the operator names the file on the command line
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s is too big to import (the limit is %d MB)", path, limit>>20)
	}
	return data, nil
}
