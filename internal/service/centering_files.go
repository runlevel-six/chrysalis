// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

// Importing and exporting centering-line collections as YAML or JSON
// (docs/DESIGN.md §3.13). The format is documented in
// docs/reference/centering-line-files.md; keep the two in step.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/runlevel-six/chrysalis/internal/order"
	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/internal/store/db"
)

// MaxQuoteFileBytes bounds an import file. A thousand long entries fit in
// well under this.
const MaxQuoteFileBytes = 1 << 20

// MaxImportEntries bounds the entries in one import.
const MaxImportEntries = 2000

// QuoteFile is one collection in a file.
type QuoteFile struct {
	Collection string           `json:"collection,omitempty" yaml:"collection,omitempty"`
	Entries    []QuoteFileEntry `json:"entries" yaml:"entries"`
}

// QuoteFileEntry is one entry in a file. Only text is required.
type QuoteFileEntry struct {
	Text        string   `json:"text" yaml:"text"`
	Attribution string   `json:"attribution,omitempty" yaml:"attribution,omitempty"`
	Reference   string   `json:"reference,omitempty" yaml:"reference,omitempty"`
	Translation string   `json:"translation,omitempty" yaml:"translation,omitempty"`
	Tags        []string `json:"tags,omitempty" yaml:"tags,omitempty"`
}

var yamlTypeName = regexp.MustCompile(` in type \S+`)

func fileError(detail string) error {
	return &ValidationError{"That file couldn't be read as a centering line file: " + detail}
}

// ParseQuoteFile reads a collection file. format is "json", "yaml", or ""
// to tell from the content (JSON starts with "{").
func ParseQuoteFile(data []byte, format string) (QuoteFile, error) {
	var f QuoteFile
	if len(data) > MaxQuoteFileBytes {
		return f, &ValidationError{fmt.Sprintf("That file is too big to import (the limit is %d MB).", MaxQuoteFileBytes>>20)}
	}
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))) // and any byte order mark
	if len(trimmed) == 0 {
		return f, &ValidationError{"That file is empty."}
	}
	if format == "" {
		format = "yaml"
		if trimmed[0] == '{' {
			format = "json"
		}
	}
	switch format {
	case "json":
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&f); err != nil {
			var syn *json.SyntaxError
			if errors.As(err, &syn) {
				return f, fileError(fmt.Sprintf("the JSON isn't valid near byte %d.", syn.Offset))
			}
			return f, fileError(strings.TrimPrefix(err.Error(), "json: ") + ".")
		}
		if dec.More() {
			return f, fileError("there's more after the collection; a file holds one collection.")
		}
	case "yaml":
		dec := yaml.NewDecoder(bytes.NewReader(trimmed))
		dec.KnownFields(true)
		if err := dec.Decode(&f); err != nil {
			if errors.Is(err, io.EOF) {
				return f, &ValidationError{"That file is empty."}
			}
			msg := yamlTypeName.ReplaceAllString(err.Error(), "")
			msg = strings.TrimPrefix(strings.ReplaceAll(msg, "yaml: unmarshal errors:\n", ""), "yaml: ")
			return f, fileError(strings.Join(strings.Fields(msg), " ") + ".")
		}
	default:
		return f, &ValidationError{"format must be json or yaml"}
	}
	if len(f.Entries) > MaxImportEntries {
		return f, &ValidationError{fmt.Sprintf("A file can hold up to %d entries.", MaxImportEntries)}
	}
	return f, nil
}

// ImportTarget says which collection an import goes into: an existing one by
// ID, or one of the user's own by name (created if it doesn't exist). An
// empty name uses the file's own.
type ImportTarget struct {
	CollectionID string
	Name         string
}

// QuoteImportResult reports what an import did.
type QuoteImportResult struct {
	Collection QuoteCollection `json:"collection"`
	Created    bool            `json:"created"`  // the collection is new
	Added      int             `json:"added"`    // entries added
	Replaced   int64           `json:"replaced"` // entries moved to the trash by replace
}

// Import adds a file's entries to a collection, after the ones already
// there. With replace, the collection's current entries go to the trash
// first (so they can still be restored).
func (c *Centering) Import(ctx context.Context, userID string, target ImportTarget, f QuoteFile, replace bool) (QuoteImportResult, error) {
	var out QuoteImportResult
	entries := make([]cleanEntry, 0, len(f.Entries))
	for i, e := range f.Entries {
		ce, err := validateEntry(e.Text, e.Attribution, e.Reference, e.Translation, e.Tags)
		if err != nil {
			var verr *ValidationError
			if errors.As(err, &verr) {
				return out, &ValidationError{fmt.Sprintf("Entry %d: %s", i+1, verr.Msg)}
			}
			return out, err
		}
		entries = append(entries, ce)
	}
	if len(entries) == 0 && !replace {
		return out, &ValidationError{"That file has no entries."}
	}
	err := c.b.tx(ctx, func(q *db.Queries) error {
		var col db.QuoteCollection
		role := RoleOwner
		if target.CollectionID != "" {
			r, err := collectionRole(ctx, q, userID, target.CollectionID, true)
			if err != nil {
				return err
			}
			role = r
			if col, err = q.GetQuoteCollection(ctx, target.CollectionID); err != nil {
				return err
			}
		} else {
			name := target.Name
			if strings.TrimSpace(name) == "" {
				name = f.Collection
			}
			if strings.TrimSpace(name) == "" {
				return &ValidationError{"Name the collection to import into."}
			}
			name, err := cleanName(name, "collection", MaxCollectionNameLen)
			if err != nil {
				return err
			}
			col, err = q.FindQuoteCollectionByName(ctx, db.FindQuoteCollectionByNameParams{OwnerID: userID, Name: name})
			if err != nil && !isNoRows(err) {
				return err
			}
			if isNoRows(err) {
				created, err := c.createCollection(ctx, q, userID, name)
				if err != nil {
					return err
				}
				out.Created = true
				if col, err = q.GetQuoteCollection(ctx, created.ID); err != nil {
					return err
				}
			}
		}
		now := store.FormatTime(c.b.now())
		if replace {
			n, err := q.TrashAllQuotes(ctx, db.TrashAllQuotesParams{CollectionID: col.ID, DeletedAt: sql.NullString{String: now, Valid: true}, UpdatedAt: now})
			if err != nil {
				return err
			}
			out.Replaced = n
		}
		existing, err := q.CountQuotes(ctx, col.ID)
		if err != nil {
			return err
		}
		if existing+int64(len(entries)) > MaxEntriesPerCollection {
			return &ValidationError{fmt.Sprintf("A collection can hold up to %d entries; this import would make %d.", MaxEntriesPerCollection, existing+int64(len(entries)))}
		}
		last, err := q.LastQuotePosition(ctx, col.ID)
		if err != nil && !isNoRows(err) {
			return err
		}
		for _, e := range entries {
			if last, err = order.KeyBetween(last, ""); err != nil {
				return err
			}
			if _, err := insertQuote(ctx, q, col.ID, e, last, now); err != nil {
				return err
			}
		}
		out.Added = len(entries)
		n, err := q.CountQuotes(ctx, col.ID)
		out.Collection = collectionFromRow(col, string(role), n)
		return err
	})
	if err == nil {
		c.notifyMembers(ctx, out.Collection.ID)
	}
	return out, err
}

// Export writes a collection's live entries as "yaml" or "json". It returns
// the file and a suggested file name.
func (c *Centering) Export(ctx context.Context, userID, collectionID, format string) ([]byte, string, error) {
	if format == "" {
		format = "yaml"
	}
	if format != "yaml" && format != "json" {
		return nil, "", &ValidationError{"format must be yaml or json"}
	}
	if _, err := collectionRole(ctx, c.st.Queries, userID, collectionID, false); err != nil {
		return nil, "", err
	}
	col, err := c.st.GetQuoteCollection(ctx, collectionID)
	if err != nil {
		return nil, "", err
	}
	rows, err := c.st.ListQuotes(ctx, collectionID)
	if err != nil {
		return nil, "", err
	}
	f := QuoteFile{Collection: col.Name, Entries: make([]QuoteFileEntry, 0, len(rows))}
	for _, r := range rows {
		e := quoteFromRow(r)
		fe := QuoteFileEntry{Text: e.Text, Attribution: e.Attribution, Reference: e.Reference, Translation: e.Translation}
		if len(e.Tags) > 0 {
			fe.Tags = e.Tags
		}
		f.Entries = append(f.Entries, fe)
	}
	var buf bytes.Buffer
	if format == "json" {
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		err = enc.Encode(f)
	} else {
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		err = enc.Encode(f)
		if err == nil {
			err = enc.Close()
		}
	}
	if err != nil {
		return nil, "", err
	}
	return buf.Bytes(), exportFileName(col.Name) + "." + format, nil
}

// exportFileName makes a plain file name from a collection name.
func exportFileName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
				b.WriteByte('-')
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "centering-line"
	}
	return s
}

func isNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }
