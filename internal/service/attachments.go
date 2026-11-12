// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"

	"github.com/runlevel-six/chrysalis/internal/media"
	"github.com/runlevel-six/chrysalis/internal/storage"
	"github.com/runlevel-six/chrysalis/internal/store"
	"github.com/runlevel-six/chrysalis/internal/store/db"
)

// Attachment is a file, image or link on a card.
type Attachment struct {
	ID        string `json:"id"`
	CardID    string `json:"cardId,omitempty"`
	Kind      string `json:"kind"` // file | image | link
	Filename  string `json:"filename"`
	MIME      string `json:"mime,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	URL       string `json:"url,omitempty"` // links only
	Width     int64  `json:"width,omitempty"`
	Height    int64  `json:"height,omitempty"`
	HasThumb  bool   `json:"hasThumb"`
	CreatedAt string `json:"createdAt"`
}

func attachmentFromRow(a db.Attachment) Attachment {
	return Attachment{
		ID: a.ID, CardID: a.CardID.String, Kind: a.Kind, Filename: a.Filename, MIME: a.Mime,
		SizeBytes: a.SizeBytes, URL: a.Url, Width: a.Width, Height: a.Height,
		HasThumb: a.ThumbKey != "", CreatedAt: a.CreatedAt,
	}
}

// Attachments handles uploads and downloads. It's separate from Boards
// because it owns the file store.
type Attachments struct {
	b     *Boards
	files storage.Store
}

func NewAttachments(b *Boards, files storage.Store) *Attachments {
	return &Attachments{b: b, files: files}
}

// cleanFilename keeps a filename for display: the last path element, no
// control characters, bounded length. It's never used to build a path.
func cleanFilename(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == "/" {
		name = "file"
	}
	for utf8.RuneCountInString(name) > 200 {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	return name
}

// Upload stores a file on a card. Images are cleaned first: metadata (GPS!)
// stripped, rotated upright, thumbnailed. Anything that isn't an image we
// can decode is kept as a plain file, served only as a download.
func (a *Attachments) Upload(ctx context.Context, userID, cardID, filename string, data []byte) (Attachment, error) {
	card, err := liveCard(ctx, a.b.st.Queries, userID, cardID, true)
	if err != nil {
		return Attachment{}, err
	}
	if len(data) == 0 {
		return Attachment{}, &ValidationError{"That file is empty."}
	}

	id := ulid.Make().String()
	row := db.CreateAttachmentParams{
		ID: id, BoardID: card.BoardID, CardID: sql.NullString{String: cardID, Valid: true}, UploaderID: userID,
		Kind: "file", Filename: cleanFilename(filename), Mime: media.SniffMIME(data),
		CreatedAt: store.FormatTime(a.b.now()),
	}
	body := data
	img, err := media.Clean(data, 0)
	switch {
	case err == nil:
		row.Kind, row.Mime, body = "image", img.MIME, img.Data
		row.Width, row.Height = int64(img.Width), int64(img.Height)
		row.ThumbKey = id + ".thumb"
		if err := a.files.Put(ctx, row.ThumbKey, img.Thumb); err != nil {
			return Attachment{}, err
		}
	case errors.Is(err, media.ErrNotImage):
		// Stored as a plain file.
	default:
		return Attachment{}, &ValidationError{"That image is too large to use."}
	}
	row.SizeBytes = int64(len(body))
	row.StorageKey = id
	if err := a.files.Put(ctx, row.StorageKey, body); err != nil {
		_ = a.files.Delete(ctx, row.ThumbKey)
		return Attachment{}, err
	}
	created, err := a.b.st.CreateAttachment(ctx, row)
	if err != nil {
		_ = a.files.Delete(ctx, row.StorageKey)
		_ = a.files.Delete(ctx, row.ThumbKey)
		return Attachment{}, err
	}
	return attachmentFromRow(created), nil
}

// AddLink attaches a web link to a card.
func (a *Attachments) AddLink(ctx context.Context, userID, cardID, rawURL, title string) (Attachment, error) {
	card, err := liveCard(ctx, a.b.st.Queries, userID, cardID, true)
	if err != nil {
		return Attachment{}, err
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(rawURL) > 2048 {
		return Attachment{}, &ValidationError{"Links must be full web addresses starting with http:// or https://."}
	}
	name := cleanFilename(title)
	if strings.TrimSpace(title) == "" {
		name = u.Host
	}
	created, err := a.b.st.CreateAttachment(ctx, db.CreateAttachmentParams{
		ID: ulid.Make().String(), BoardID: card.BoardID, CardID: sql.NullString{String: cardID, Valid: true},
		UploaderID: userID, Kind: "link", Filename: name, Url: u.String(), CreatedAt: store.FormatTime(a.b.now()),
	})
	if err != nil {
		return Attachment{}, err
	}
	return attachmentFromRow(created), nil
}

func (a *Attachments) List(ctx context.Context, userID, cardID string) ([]Attachment, error) {
	if _, err := liveCard(ctx, a.b.st.Queries, userID, cardID, false); err != nil {
		return nil, err
	}
	rows, err := a.b.st.ListCardAttachments(ctx, sql.NullString{String: cardID, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]Attachment, 0, len(rows))
	for _, r := range rows {
		out = append(out, attachmentFromRow(r))
	}
	return out, nil
}

// visible loads an attachment the user may see (via its board).
func (a *Attachments) visible(ctx context.Context, userID, id string, write bool) (db.Attachment, error) {
	row, err := a.b.st.GetAttachment(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, err
	}
	_, err = role(ctx, a.b.st.Queries, userID, row.BoardID, write)
	return row, err
}

// Open returns an attachment's content (or its thumbnail) for download.
// Trashed attachments stay downloadable by members until purged, so the
// trash can show what it holds.
func (a *Attachments) Open(ctx context.Context, userID, id string, thumb bool) (Attachment, io.ReadSeekCloser, error) {
	row, err := a.visible(ctx, userID, id, false)
	if err != nil {
		return Attachment{}, nil, err
	}
	key := row.StorageKey
	if thumb {
		key = row.ThumbKey
	}
	if row.Kind == "link" || key == "" {
		return Attachment{}, nil, ErrNotFound
	}
	f, err := a.files.Open(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return Attachment{}, nil, ErrNotFound
	}
	if err != nil {
		return Attachment{}, nil, err
	}
	att := attachmentFromRow(row)
	if thumb {
		att.MIME = media.SniffMIME(peek(f))
	}
	return att, f, nil
}

func peek(f io.ReadSeeker) []byte {
	b := make([]byte, 512)
	n, _ := io.ReadFull(f, b)
	_, _ = f.Seek(0, io.SeekStart)
	return b[:n]
}

// SetTrashed moves an attachment to the trash or back.
func (a *Attachments) SetTrashed(ctx context.Context, userID, id string, trashed bool) (Attachment, error) {
	row, err := a.visible(ctx, userID, id, true)
	if err != nil {
		return Attachment{}, err
	}
	del := sql.NullString{}
	if trashed {
		del = sql.NullString{String: store.FormatTime(a.b.now()), Valid: true}
	}
	row, err = a.b.st.SetAttachmentDeleted(ctx, db.SetAttachmentDeletedParams{ID: row.ID, DeletedAt: del})
	if err != nil {
		return Attachment{}, err
	}
	return attachmentFromRow(row), nil
}

// PurgeFiles deletes the stored files (and rows) of attachments that are
// past the trash cutoff, directly or because their card, column or board
// is. Run it before PurgeTrash, which removes those parents.
func PurgeFiles(ctx context.Context, st *store.Store, files storage.Store, cutoff time.Time) (int, error) {
	rows, err := st.ListPurgeableAttachments(ctx, sql.NullString{String: store.FormatTime(cutoff), Valid: true})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		for _, key := range []string{r.StorageKey, r.ThumbKey} {
			if key == "" {
				continue
			}
			if err := files.Delete(ctx, key); err != nil {
				return n, fmt.Errorf("delete %s: %w", key, err)
			}
		}
		if err := st.DeleteAttachment(ctx, r.ID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
