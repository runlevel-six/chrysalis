// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package media

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"golang.org/x/image/webp"
)

func TestCleanJPEGRotatesAndStrips(t *testing.T) {
	// 16x8, red on the left half. Orientation 6 = "rotate 90° clockwise to
	// display", so upright it's 8x16 with red on the top half.
	in := photo(t, segment(markerAPP1, exifBlock(binary.BigEndian, 6)))
	img, err := Clean(in, 0)
	if err != nil {
		t.Fatal(err)
	}
	if img.Width != 8 || img.Height != 16 {
		t.Fatalf("size = %dx%d, want 8x16 after rotation", img.Width, img.Height)
	}
	if bytes.Contains(img.Data, []byte("GPS")) || bytes.Contains(img.Data, []byte("Exif")) {
		t.Error("metadata survived")
	}
	dec, err := jpeg.Decode(bytes.NewReader(img.Data))
	if err != nil {
		t.Fatal(err)
	}
	top, bottom := dec.At(4, 2), dec.At(4, 13)
	if r, _, _, _ := top.RGBA(); r>>8 < 150 {
		t.Errorf("top should be red after rotation, got %v", top)
	}
	if r, _, _, _ := bottom.RGBA(); r>>8 > 100 {
		t.Errorf("bottom should be dark after rotation, got %v", bottom)
	}
	if img.ThumbMIME != "image/jpeg" || len(img.Thumb) == 0 {
		t.Errorf("thumb = %s, %d bytes", img.ThumbMIME, len(img.Thumb))
	}
}

func TestCleanJPEGWithoutRotationIsNotReencoded(t *testing.T) {
	in := photo(t, segment(markerAPP1, exifBlock(binary.LittleEndian, 1)))
	img, err := Clean(in, 0)
	if err != nil {
		t.Fatal(err)
	}
	plain := photo(t)
	if !bytes.Equal(img.Data, plain) {
		t.Error("a JPEG that needs no rotation should only lose its metadata, not be re-encoded")
	}
}

func pngWithText(t *testing.T, w, h int, transparent bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	if transparent {
		img.SetNRGBA(0, 0, color.NRGBA{A: 0})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	raw := b.Bytes()
	// Insert a tEXt chunk right after IHDR (8-byte signature + 25-byte IHDR).
	data := []byte("Comment\x00GPS 40.7N 74.0W")
	chunk := make([]byte, 8, 12+len(data))
	binary.BigEndian.PutUint32(chunk, uint32(len(data)))
	copy(chunk[4:], "tEXt")
	chunk = append(chunk, data...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(chunk[4:]))
	out := append([]byte{}, raw[:33]...)
	out = append(out, chunk...)
	return append(out, raw[33:]...)
}

func TestCleanPNG(t *testing.T) {
	img, err := Clean(pngWithText(t, 20, 10, true), 0)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(img.Data, []byte("GPS")) || bytes.Contains(img.Data, []byte("tEXt")) {
		t.Error("text chunk survived")
	}
	if _, err := png.Decode(bytes.NewReader(img.Data)); err != nil {
		t.Fatalf("cleaned PNG doesn't decode: %v", err)
	}
	if img.MIME != "image/png" || img.ThumbMIME != "image/png" {
		t.Errorf("transparent image should stay PNG: %s / %s", img.MIME, img.ThumbMIME)
	}
}

func TestCleanScalesLargeImages(t *testing.T) {
	img, err := Clean(pngWithText(t, 3000, 100, false), 2560)
	if err != nil {
		t.Fatal(err)
	}
	if img.Width != 2560 || img.Height != 85 {
		t.Errorf("size = %dx%d, want 2560x85", img.Width, img.Height)
	}
	th, _, err := image.DecodeConfig(bytes.NewReader(img.Thumb))
	if err != nil || th.Width != ThumbSize {
		t.Errorf("thumb width = %d (%v), want %d", th.Width, err, ThumbSize)
	}
}

func TestCleanRefusesDecompressionBombs(t *testing.T) {
	in := pngWithText(t, 10, 10, false)
	// Claim 100000x100000 in IHDR and fix up its CRC.
	binary.BigEndian.PutUint32(in[16:], 100000)
	binary.BigEndian.PutUint32(in[20:], 100000)
	binary.BigEndian.PutUint32(in[29:], crc32.ChecksumIEEE(in[12:29]))
	if _, err := Clean(in, 0); err == nil || errors.Is(err, ErrNotImage) {
		t.Fatalf("err = %v, want a size refusal", err)
	}
}

func TestCleanNotImages(t *testing.T) {
	for name, in := range map[string][]byte{
		"text": []byte("hello"),
		"svg":  []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"pdf":  []byte("%PDF-1.7\n..."),
	} {
		if _, err := Clean(in, 0); !errors.Is(err, ErrNotImage) {
			t.Errorf("%s: err = %v, want ErrNotImage", name, err)
		}
	}
}

// A 1x1 lossless WebP, wrapped in an extended (VP8X) container with an EXIF
// chunk, the way cameras and phones write them.
func webpWithEXIF(t *testing.T) []byte {
	t.Helper()
	simple, err := base64.StdEncoding.DecodeString("UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA==")
	if err != nil {
		t.Fatal(err)
	}
	vp8l := simple[12:]                                                              // the VP8L chunk
	vp8x := []byte{'V', 'P', '8', 'X', 10, 0, 0, 0, 0x08, 0, 0, 0, 0, 0, 0, 0, 0, 0} // EXIF flag; 1x1 canvas
	exif := append([]byte{'E', 'X', 'I', 'F', 0, 0, 0, 0}, []byte("GPS 40.7N")...)
	binary.LittleEndian.PutUint32(exif[4:], uint32(len(exif)-8))
	if len(exif)%2 == 1 {
		exif = append(exif, 0)
	}
	body := append(append(append([]byte{}, vp8x...), vp8l...), exif...)
	out := append([]byte("RIFF\x00\x00\x00\x00WEBP"), body...)
	binary.LittleEndian.PutUint32(out[4:], uint32(4+len(body)))
	return out
}

func TestCleanWebP(t *testing.T) {
	in := webpWithEXIF(t)
	if _, err := webp.DecodeConfig(bytes.NewReader(in)); err != nil {
		t.Fatalf("test fixture isn't a valid WebP: %v", err)
	}
	img, err := Clean(in, 0)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(img.Data, []byte("GPS")) || bytes.Contains(img.Data, []byte("EXIF")) {
		t.Error("EXIF chunk survived")
	}
	if img.Data[20]&0x08 != 0 {
		t.Error("EXIF flag still set in VP8X")
	}
	if _, err := webp.Decode(bytes.NewReader(img.Data)); err != nil {
		t.Fatalf("cleaned WebP doesn't decode: %v", err)
	}
}
