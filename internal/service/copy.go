// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"strings"

	"github.com/oklog/ulid/v2"

	"github.com/runlevel-six/chrysalis/internal/order"
	"github.com/runlevel-six/chrysalis/internal/sse"
	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/internal/store/db"
)

// Copying cards (docs/DESIGN.md §4, Phase 2): "Copy card", saving a card as
// a template, and starting a card from a template are all one operation with
// different choices. Attached files are copied too, so deleting one card can
// never take another's file with it.

// CopyOptions says what a copy brings along and where it goes.
type CopyOptions struct {
	Title    string `json:"title"`
	ColumnID string `json:"columnId"` // "" = the same column
	// Template makes the copy a template (kept out of the columns).
	Template bool `json:"template"`
	// FreshChecklists unticks every item in the copy.
	FreshChecklists bool        `json:"freshChecklists"`
	Include         CopyInclude `json:"include"`
}

// CopyInclude chooses the parts to copy; the title always comes along.
type CopyInclude struct {
	Checklists  bool `json:"checklists"`
	Labels      bool `json:"labels"`
	Description bool `json:"description"`
	Dates       bool `json:"dates"`
	Fields      bool `json:"fields"`
	Attachments bool `json:"attachments"`
}

// CopyEverything is the default for templates.
var CopyEverything = CopyInclude{Checklists: true, Labels: true, Description: true, Dates: true, Fields: true, Attachments: true}

// maxCopyBytes bounds the files copied in one go.
const maxCopyBytes = 200 << 20

// CopyCard copies a card. A plain copy lands just after the original; a
// card made from a template goes at the bottom of its column.
func (a *Attachments) CopyCard(ctx context.Context, userID, cardID string, opt CopyOptions) (Card, error) {
	s := a.b
	src, err := liveCard(ctx, s.st.Queries, userID, cardID, true)
	if err != nil {
		return Card{}, err
	}
	title := opt.Title
	if strings.TrimSpace(title) == "" {
		title = src.Title
	}
	if title, err = cleanTitle(title); err != nil {
		return Card{}, err
	}
	columnID := opt.ColumnID
	if columnID == "" {
		columnID = src.ColumnID
	}

	// Files first, outside the transaction; removed again if it fails.
	type copied struct {
		row      db.Attachment
		oldID    string
		key, thb string
	}
	var files []copied
	var made []string
	cleanup := func() {
		for _, k := range made {
			_ = a.files.Delete(ctx, k)
		}
	}
	if opt.Include.Attachments {
		atts, err := s.st.ListCardAttachments(ctx, sql.NullString{String: src.ID, Valid: true})
		if err != nil {
			return Card{}, err
		}
		var total int64
		for _, att := range atts {
			total += att.SizeBytes
		}
		if total > maxCopyBytes {
			return Card{}, &ValidationError{"This card's attachments are too large to copy together; copy it without them."}
		}
		for _, att := range atts {
			c := copied{row: att, oldID: att.ID}
			newID := ulid.Make().String()
			if att.StorageKey != "" {
				c.key = newID
				if err := a.copyBlob(ctx, att.StorageKey, c.key); err != nil {
					cleanup()
					return Card{}, err
				}
				made = append(made, c.key)
			}
			if att.ThumbKey != "" {
				c.thb = newID + ".thumb"
				if err := a.copyBlob(ctx, att.ThumbKey, c.thb); err != nil {
					cleanup()
					return Card{}, err
				}
				made = append(made, c.thb)
			}
			c.row.ID = newID
			files = append(files, c)
		}
	}

	var out Card
	err = s.tx(ctx, func(q *db.Queries) error {
		col, err := q.GetColumn(ctx, columnID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && (col.DeletedAt.Valid || col.BoardID != src.BoardID)) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		pos, err := copyPosition(ctx, q, src, columnID, opt)
		if err != nil {
			return err
		}
		now := store.FormatTime(s.now())
		id := ulid.Make().String()
		row := db.CopyCardRowParams{
			ID: id, BoardID: src.BoardID, ColumnID: columnID, Title: title, Position: pos,
			CoverType: "none", CreatedAt: now, UpdatedAt: now,
		}
		if opt.Template {
			row.IsTemplate = 1
		}
		if opt.Include.Dates {
			row.StartAt, row.DueAt, row.DueHasTime = src.StartAt, src.DueAt, src.DueHasTime
		}
		// Old attachment ids to new, for the cover and inline images.
		idMap := map[string]string{}
		for _, f := range files {
			idMap[f.oldID] = f.row.ID
		}
		switch {
		case src.CoverType == "color":
			row.CoverType, row.CoverValue = src.CoverType, src.CoverValue
		case src.CoverType == "image" && idMap[src.CoverValue] != "":
			row.CoverType, row.CoverValue = "image", idMap[src.CoverValue]
		}
		if err := q.CopyCardRow(ctx, row); err != nil {
			return err
		}

		for _, f := range files {
			r := f.row
			if _, err := q.CreateAttachment(ctx, db.CreateAttachmentParams{
				ID: r.ID, BoardID: src.BoardID, CardID: sql.NullString{String: id, Valid: true}, UploaderID: userID,
				Kind: r.Kind, Filename: r.Filename, Mime: r.Mime, SizeBytes: r.SizeBytes, StorageKey: f.key, ThumbKey: f.thb,
				Url: r.Url, Width: r.Width, Height: r.Height, CreatedAt: now,
			}); err != nil {
				return err
			}
		}
		if opt.Include.Labels {
			ids, err := q.ListLabelIDsForCard(ctx, src.ID)
			if err != nil {
				return err
			}
			for _, l := range ids {
				if err := q.AddCardLabel(ctx, db.AddCardLabelParams{CardID: id, LabelID: l}); err != nil {
					return err
				}
			}
		}
		if opt.Include.Checklists {
			lists, err := q.ListChecklists(ctx, src.ID)
			if err != nil {
				return err
			}
			all, err := q.ListChecklistItemsForCard(ctx, src.ID)
			if err != nil {
				return err
			}
			for _, cl := range lists {
				clID := ulid.Make().String()
				if err := q.ImportChecklist(ctx, db.ImportChecklistParams{
					ID: clID, CardID: id, Title: cl.Title, Position: cl.Position, CreatedAt: now, UpdatedAt: now,
				}); err != nil {
					return err
				}
				for _, it := range all {
					if it.ChecklistID != cl.ID {
						continue
					}
					done := it.DoneAt
					if opt.FreshChecklists || opt.Template {
						done = sql.NullString{}
					}
					if err := q.ImportChecklistItem(ctx, db.ImportChecklistItemParams{
						ID: ulid.Make().String(), ChecklistID: clID, Text: it.Text, DoneAt: done,
						Position: it.Position, CreatedAt: now, UpdatedAt: now,
					}); err != nil {
						return err
					}
				}
			}
		}
		if opt.Include.Description {
			d, err := q.GetCardDescription(ctx, src.ID)
			switch {
			case errors.Is(err, sql.ErrNoRows):
			case err != nil:
				return err
			case d.DocJson != "":
				doc := d.DocJson
				for old, nw := range idMap {
					doc = strings.ReplaceAll(doc, "attachment:"+old, "attachment:"+nw)
				}
				if _, err := q.UpsertCardDescription(ctx, db.UpsertCardDescriptionParams{
					CardID: id, DocJson: doc, Version: 1, UpdatedAt: now, UpdatedBy: userID,
				}); err != nil {
					return err
				}
			}
		}
		if opt.Include.Fields {
			vals, err := q.ListCardFieldValues(ctx, src.ID)
			if err != nil {
				return err
			}
			for _, v := range vals {
				if err := q.UpsertFieldValue(ctx, db.UpsertFieldValueParams{CardID: id, FieldID: v.FieldID, ValueJson: v.ValueJson, UpdatedAt: now}); err != nil {
					return err
				}
			}
		}
		c, err := q.GetCard(ctx, id)
		if err != nil {
			return err
		}
		out, err = withLabels(ctx, q, cardFromRow(c))
		return err
	})
	if err != nil {
		cleanup()
		return Card{}, err
	}
	s.emit(ctx, sse.Event{BoardID: out.BoardID, Kind: "card", EntityID: out.ID, CardID: out.ID})
	return out, nil
}

// copyPosition: just after the original for a plain copy in its own column;
// otherwise at the bottom of the target column.
func copyPosition(ctx context.Context, q *db.Queries, src db.Card, columnID string, opt CopyOptions) (string, error) {
	rows, err := q.ListColumnCardPositions(ctx, columnID)
	if err != nil {
		return "", err
	}
	if !opt.Template && src.IsTemplate == 0 && columnID == src.ColumnID {
		for i, r := range rows {
			if r.ID == src.ID {
				next := ""
				if i+1 < len(rows) {
					next = rows[i+1].Position
				}
				return order.KeyBetween(src.Position, next)
			}
		}
	}
	if len(rows) == 0 {
		return order.First(), nil
	}
	return order.KeyBetween(rows[len(rows)-1].Position, "")
}

func (a *Attachments) copyBlob(ctx context.Context, from, to string) error {
	r, err := a.files.Open(ctx, from)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	data, err := io.ReadAll(io.LimitReader(r, maxCopyBytes+1))
	if err != nil {
		return err
	}
	return a.files.Put(ctx, to, data)
}

// Templates lists a board's card templates.
func (s *Boards) templates(ctx context.Context, q *db.Queries, boardID string) ([]Card, error) {
	rows, err := q.ListTemplates(ctx, boardID)
	if err != nil {
		return nil, err
	}
	out := make([]Card, 0, len(rows))
	for _, r := range rows {
		c, err := withLabels(ctx, q, cardFromRow(r))
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}
