// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/runlevel-six/chrysalis/internal/service"
	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/internal/store/db"
)

func TestAdminQuotesImport(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	setEnv(t, data, t.TempDir())
	file := filepath.Join(t.TempDir(), "morning.yaml")
	if err := os.WriteFile(file, []byte("collection: Morning\nentries:\n  - text: Be still, and breathe.\n    attribution: A gardener\n  - text: Rest is part of the work.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := admin(t, "quotes", "import", "--user", "alice@example.com", file); err == nil || !strings.Contains(err.Error(), "no database") {
		t.Fatalf("no database: %v", err)
	}
	st, err := store.Open(ctx, filepath.Join(data, "chrysalis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := admin(t, "quotes", "import", "--user", "alice@example.com", file); err == nil || !strings.Contains(err.Error(), "sign in once") {
		t.Fatalf("unknown user: %v", err)
	}
	alice, err := st.UpsertUser(ctx, db.UpsertUserParams{
		ID: "01JALICE000000000000000000", OidcSub: "alice", Email: "Alice@Example.com", DisplayName: "Alice",
		CreatedAt: "2026-01-01T00:00:00.000Z", LastSeenAt: "2026-01-01T00:00:00.000Z",
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := admin(t, "quotes", "import", "--user", "alice@example.com", file)
	if err != nil || !strings.Contains(out, `created collection "Morning"`) || !strings.Contains(out, "2 entries added") {
		t.Fatalf("import: %q, %v", out, err)
	}
	// Additive, with flags after the file too; then --replace.
	out, err = admin(t, "quotes", "import", file, "--user", "alice@example.com", "--collection", "morning")
	if err != nil || !strings.Contains(out, `added to collection "Morning"`) || !strings.Contains(out, "(4 in the collection now)") {
		t.Fatalf("second import: %q, %v", out, err)
	}
	out, err = admin(t, "quotes", "import", "--user", "alice@example.com", "--replace", file)
	if err != nil || !strings.Contains(out, "4 moved to the trash") || !strings.Contains(out, "(2 in the collection now)") {
		t.Fatalf("replace: %q, %v", out, err)
	}
	// "-" reads standard input, for kubectl exec -i.
	in, err := os.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	stdin := os.Stdin
	os.Stdin = in
	out, err = admin(t, "quotes", "import", "--user", "alice@example.com", "--collection", "Evening", "-")
	os.Stdin = stdin
	if err != nil || !strings.Contains(out, `created collection "Evening"`) || !strings.Contains(out, "2 entries added") {
		t.Fatalf("from stdin: %q, %v", out, err)
	}

	c := service.NewCentering(service.NewBoards(st, time.Now))
	cols, err := c.Collections(ctx, alice.ID)
	if err != nil || len(cols) != 2 || cols[0].EntryCount != 2 {
		t.Fatalf("collections = %+v, %v", cols, err)
	}
	// It feeds Alice's line once she turns it on.
	if s, _ := c.Settings(ctx, alice.ID); len(s.CollectionIDs) != 2 || s.Enabled {
		t.Fatalf("settings = %+v", s)
	}

	bad := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(bad, []byte("entries:\n  - txt: oops\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := admin(t, "quotes", "import", "--user", "alice@example.com", "--collection", "X", bad); err == nil || !strings.Contains(err.Error(), "txt") {
		t.Fatalf("bad file: %v", err)
	}
	for _, args := range [][]string{{"quotes"}, {"quotes", "export"}, {"quotes", "import", file}, {"quotes", "import", "--user", "a@example.com"}} {
		if _, err := admin(t, args...); err == nil || !strings.Contains(err.Error(), "usage:") {
			t.Errorf("%v: %v", args, err)
		}
	}
}
