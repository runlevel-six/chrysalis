// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package media

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// exifBlock builds an APP1 EXIF payload with an orientation tag and a fake
// GPS string, in the given byte order.
func exifBlock(bo binary.ByteOrder, orientation uint16) []byte {
	var t bytes.Buffer
	if bo == binary.LittleEndian {
		t.WriteString("II")
	} else {
		t.WriteString("MM")
	}
	binary.Write(&t, bo, uint16(42))
	binary.Write(&t, bo, uint32(8)) // IFD0 offset
	binary.Write(&t, bo, uint16(1)) // one entry
	binary.Write(&t, bo, uint16(0x0112))
	binary.Write(&t, bo, uint16(3)) // SHORT
	binary.Write(&t, bo, uint32(1))
	binary.Write(&t, bo, orientation)
	binary.Write(&t, bo, uint16(0))
	binary.Write(&t, bo, uint32(0))        // no next IFD
	t.WriteString("GPS 40.7128N 74.0060W") // stands in for location data
	return append([]byte("Exif\x00\x00"), t.Bytes()...)
}

func segment(marker byte, payload []byte) []byte {
	out := []byte{0xFF, marker, 0, 0}
	binary.BigEndian.PutUint16(out[2:], uint16(len(payload)+2))
	return append(out, payload...)
}

// photo makes a real JPEG and splices metadata segments in after SOI.
func photo(t *testing.T, extra ...[]byte) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 8))
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			img.Set(x, y, color.RGBA{R: 200, A: 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	raw := b.Bytes()
	out := append([]byte{}, raw[:2]...)
	for _, e := range extra {
		out = append(out, e...)
	}
	return append(out, raw[2:]...)
}

func TestStripJPEGRemovesMetadataKeepsImage(t *testing.T) {
	for _, bo := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		in := photo(t,
			segment(markerAPP1, exifBlock(bo, 6)),
			segment(markerAPP1, []byte("http://ns.adobe.com/xap/1.0/\x00<x:xmpmeta>GPS</x:xmpmeta>")),
			segment(0xED, []byte("Photoshop 3.0\x00IPTC city")), // APP13
			segment(markerCOM, []byte("shot at home")),
			segment(0xE2, []byte("ICC_PROFILE\x00\x01\x01profile")),
		)
		out, orientation, err := StripJPEG(in)
		if err != nil {
			t.Fatal(err)
		}
		if orientation != 6 {
			t.Errorf("orientation = %d, want 6", orientation)
		}
		for _, leak := range []string{"GPS", "Exif", "IPTC", "shot at home", "xmpmeta"} {
			if bytes.Contains(out, []byte(leak)) {
				t.Errorf("%q survived stripping", leak)
			}
		}
		if !bytes.Contains(out, []byte("ICC_PROFILE")) {
			t.Error("ICC profile should be kept (colors)")
		}
		img, err := jpeg.Decode(bytes.NewReader(out))
		if err != nil {
			t.Fatalf("stripped JPEG doesn't decode: %v", err)
		}
		if img.Bounds().Dx() != 16 || img.Bounds().Dy() != 8 {
			t.Errorf("bounds = %v", img.Bounds())
		}
	}
}

func TestStripJPEGNoMetadata(t *testing.T) {
	in := photo(t)
	out, orientation, err := StripJPEG(in)
	if err != nil || orientation != 0 || !bytes.Equal(out, in) {
		t.Fatalf("plain JPEG changed: orientation=%d err=%v equal=%v", orientation, err, bytes.Equal(out, in))
	}
}

func TestStripJPEGRejectsGarbage(t *testing.T) {
	for _, in := range [][]byte{
		nil,
		[]byte("not a jpeg"),
		{0xFF, 0xD8, 0xFF, 0xE1, 0xFF, 0xFF}, // segment length past the end
		{0xFF, 0xD8, 0x00},                   // not a marker
		{0xFF, 0xD8},                         // no image data
	} {
		if _, _, err := StripJPEG(in); err == nil {
			t.Errorf("StripJPEG(%q) succeeded", in)
		}
	}
}

func TestExifOrientationMalformed(t *testing.T) {
	for _, tiff := range [][]byte{
		nil,
		[]byte("XX\x00\x2a\x00\x00\x00\x08"),
		[]byte("II\x2a\x00\xff\xff\x00\x00"), // IFD offset past the end
	} {
		if o := exifOrientation(tiff); o != 0 {
			t.Errorf("exifOrientation(%q) = %d", tiff, o)
		}
	}
}
