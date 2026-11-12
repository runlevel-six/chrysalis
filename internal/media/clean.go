// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"net/http"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// Images bigger than this many pixels are refused before decoding: a small
// file can declare enormous dimensions and exhaust memory ("decompression
// bomb"). 50 MP covers any phone camera.
const MaxPixels = 50_000_000

// ThumbSize is the longest edge of a thumbnail.
const ThumbSize = 480

// Image is a cleaned image, ready to store.
type Image struct {
	Data          []byte // metadata removed; rotated upright if it needed it
	MIME          string
	Width, Height int
	Thumb         []byte // a small preview, JPEG (or PNG if transparent)
	ThumbMIME     string
}

// ErrNotImage means the bytes aren't an image type we can clean. The upload
// can still be stored, but only as a plain downloadable file.
var ErrNotImage = errors.New("media: not a supported image")

// SniffMIME returns the content type from the bytes themselves, never from
// what the client claimed (docs/DESIGN.md §7.3).
func SniffMIME(b []byte) string { return http.DetectContentType(b) }

// Clean strips metadata from an image, applies its EXIF orientation, scales
// it down if its longest edge exceeds maxEdge (0 = no limit), and makes a
// thumbnail.
func Clean(in []byte, maxEdge int) (Image, error) {
	mime := SniffMIME(in)
	switch mime {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
	default:
		return Image{}, ErrNotImage
	}

	cfg, _, err := decodeConfig(in, mime)
	if err != nil {
		return Image{}, fmt.Errorf("%w: %w", ErrNotImage, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > MaxPixels {
		return Image{}, fmt.Errorf("media: image is %dx%d, larger than allowed", cfg.Width, cfg.Height)
	}

	out := Image{MIME: mime}
	orientation := 0
	switch mime {
	case "image/jpeg":
		out.Data, orientation, err = StripJPEG(in)
	case "image/png":
		out.Data, err = StripPNG(in)
	case "image/webp":
		out.Data, err = StripWebP(in)
	case "image/gif":
		// GIFs carry no EXIF; animation would be lost by re-encoding.
		out.Data = in
	}
	if err != nil {
		return Image{}, fmt.Errorf("%w: %w", ErrNotImage, err)
	}

	img, err := decode(out.Data, mime)
	if err != nil {
		return Image{}, fmt.Errorf("%w: %w", ErrNotImage, err)
	}
	if orientation > 1 {
		img = orient(img, orientation)
	}
	b := img.Bounds()
	out.Width, out.Height = b.Dx(), b.Dy()

	// Re-encode only when we must: the picture was rotated, or it's bigger
	// than this use allows. WebP has no encoder in Go, so a WebP that needs
	// either becomes a JPEG/PNG.
	tooBig := maxEdge > 0 && max(out.Width, out.Height) > maxEdge
	if (orientation > 1 || tooBig) && mime != "image/gif" {
		if tooBig {
			img = fit(img, maxEdge)
			out.Width, out.Height = img.Bounds().Dx(), img.Bounds().Dy()
		}
		out.Data, out.MIME, err = encode(img, 90)
		if err != nil {
			return Image{}, err
		}
	}

	thumb := img
	if max(out.Width, out.Height) > ThumbSize {
		thumb = fit(img, ThumbSize)
	}
	out.Thumb, out.ThumbMIME, err = encode(thumb, 80)
	return out, err
}

func decodeConfig(b []byte, mime string) (image.Config, string, error) {
	if mime == "image/webp" {
		c, err := webp.DecodeConfig(bytes.NewReader(b))
		return c, "webp", err
	}
	return image.DecodeConfig(bytes.NewReader(b))
}

func decode(b []byte, mime string) (image.Image, error) {
	r := bytes.NewReader(b)
	switch mime {
	case "image/jpeg":
		return jpeg.Decode(r)
	case "image/png":
		return png.Decode(r)
	case "image/gif":
		return gif.Decode(r) // first frame
	case "image/webp":
		return webp.Decode(r)
	}
	return nil, ErrNotImage
}

func opaque(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return o.Opaque()
	}
	return false
}

// encode writes JPEG for opaque images and PNG when there's transparency.
func encode(img image.Image, quality int) ([]byte, string, error) {
	var b bytes.Buffer
	if opaque(img) {
		err := jpeg.Encode(&b, img, &jpeg.Options{Quality: quality})
		return b.Bytes(), "image/jpeg", err
	}
	err := png.Encode(&b, img)
	return b.Bytes(), "image/png", err
}

// fit scales img down so its longest edge is edge pixels.
func fit(img image.Image, edge int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w >= h {
		h = max(1, h*edge/w)
		w = edge
	} else {
		w = max(1, w*edge/h)
		h = edge
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

// orient applies an EXIF orientation (2-8) so the image displays upright
// once the tag is gone.
func orient(img image.Image, o int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range h {
		for x := range w {
			var dx, dy int
			switch o {
			case 2: // mirrored
				dx, dy = w-1-x, y
			case 3: // rotated 180
				dx, dy = w-1-x, h-1-y
			case 4: // mirrored vertically
				dx, dy = x, h-1-y
			case 5: // transposed
				dx, dy = y, x
			case 6: // rotated 90 clockwise
				dx, dy = h-1-y, x
			case 7: // transversed
				dx, dy = h-1-y, w-1-x
			case 8: // rotated 90 counter-clockwise
				dx, dy = y, w-1-x
			default:
				dx, dy = x, y
			}
			dst.Set(dx, dy, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

// StripPNG drops ancillary chunks that can hold personal data (text,
// timestamps, EXIF), keeping everything needed to display the image and its
// colors.
func StripPNG(in []byte) ([]byte, error) {
	const sig = "\x89PNG\r\n\x1a\n"
	if !bytes.HasPrefix(in, []byte(sig)) {
		return nil, errors.New("media: not a PNG")
	}
	drop := map[string]bool{"tEXt": true, "zTXt": true, "iTXt": true, "eXIf": true, "tIME": true}
	out := bytes.NewBuffer(make([]byte, 0, len(in)))
	out.WriteString(sig)
	for i := len(sig); i < len(in); {
		if i+12 > len(in) {
			return nil, errors.New("media: truncated PNG")
		}
		n := int(binary.BigEndian.Uint32(in[i:]))
		end := i + 12 + n
		if n < 0 || end > len(in) {
			return nil, errors.New("media: bad PNG chunk length")
		}
		typ := string(in[i+4 : i+8])
		// Verify the CRC so we never pass along a corrupt file.
		if crc32.ChecksumIEEE(in[i+4:i+8+n]) != binary.BigEndian.Uint32(in[i+8+n:]) {
			return nil, errors.New("media: PNG checksum mismatch")
		}
		if !drop[typ] {
			out.Write(in[i:end])
		}
		i = end
		if typ == "IEND" {
			break
		}
	}
	return out.Bytes(), nil
}

// StripWebP drops the EXIF and XMP chunks from a WebP (RIFF) file and clears
// the matching flags in its VP8X header.
func StripWebP(in []byte) ([]byte, error) {
	if len(in) < 12 || string(in[:4]) != "RIFF" || string(in[8:12]) != "WEBP" {
		return nil, errors.New("media: not a WebP")
	}
	var body bytes.Buffer
	for i := 12; i < len(in); {
		if i+8 > len(in) {
			return nil, errors.New("media: truncated WebP")
		}
		fourcc := string(in[i : i+4])
		n := int(binary.LittleEndian.Uint32(in[i+4:]))
		end := i + 8 + n + n%2 // chunks are padded to even length
		if n < 0 || end > len(in) {
			return nil, errors.New("media: bad WebP chunk length")
		}
		switch fourcc {
		case "EXIF", "XMP ":
			// dropped
		case "VP8X":
			chunk := append([]byte{}, in[i:end]...)
			if n >= 1 {
				chunk[8] &^= 0x08 | 0x04 // EXIF and XMP flags
			}
			body.Write(chunk)
		default:
			body.Write(in[i:end])
		}
		i = end
	}
	if body.Len() > math.MaxUint32-4 {
		return nil, errors.New("media: WebP too large")
	}
	out := make([]byte, 12, 12+body.Len())
	copy(out, in[:12])
	binary.LittleEndian.PutUint32(out[4:], uint32(4+body.Len())) //nolint:gosec // bounded just above
	return append(out, body.Bytes()...), nil
}
