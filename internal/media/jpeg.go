// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Package media cleans and resizes uploaded images (docs/DESIGN.md §7.3).
//
// Photos from phones carry EXIF metadata, often including GPS coordinates.
// Every image we keep has its metadata removed. Where possible that's done
// without re-encoding, so quality is untouched.
package media

import (
	"bytes"
	"encoding/binary"
	"errors"
)

var errNotJPEG = errors.New("media: not a JPEG")

// JPEG markers we care about. A JPEG is a sequence of segments: 0xFF, a
// marker byte, then (for most markers) a 2-byte big-endian length that
// includes itself.
const (
	markerSOI  = 0xD8 // start of image
	markerSOS  = 0xDA // start of scan: compressed image data follows
	markerAPP1 = 0xE1 // EXIF or XMP
	markerAPP0 = 0xE0 // JFIF (harmless, kept)
	markerCOM  = 0xFE // comment
)

// keepJPEGSegment reports whether a metadata segment is safe to keep.
// Everything that can carry personal data goes: EXIF/XMP (APP1), Photoshop
// IPTC (APP13), comments, and other application segments. JFIF (APP0),
// ICC color profiles (APP2) and Adobe color-transform info (APP14) stay,
// because dropping them changes how colors render.
func keepJPEGSegment(marker byte, payload []byte) bool {
	switch {
	case marker == markerAPP0:
		return true
	case marker == 0xE2: // APP2: ICC profile
		return bytes.HasPrefix(payload, []byte("ICC_PROFILE\x00"))
	case marker == 0xEE: // APP14: Adobe
		return bytes.HasPrefix(payload, []byte("Adobe"))
	case marker >= 0xE0 && marker <= 0xEF, marker == markerCOM:
		return false
	}
	return true // tables, frame headers: needed to decode the image
}

// StripJPEG removes metadata segments from a JPEG without touching the
// compressed image data. It also returns the EXIF orientation (1-8; 1 means
// "as stored", 0 if absent), which the caller must apply before the
// orientation tag is gone, or rotated phone photos would display sideways.
func StripJPEG(in []byte) (out []byte, orientation int, err error) {
	if len(in) < 4 || in[0] != 0xFF || in[1] != markerSOI {
		return nil, 0, errNotJPEG
	}
	var buf bytes.Buffer
	buf.Grow(len(in))
	buf.Write(in[:2])
	i := 2
	for i < len(in) {
		if in[i] != 0xFF {
			return nil, 0, errors.New("media: malformed JPEG segment")
		}
		// Fill bytes (extra 0xFF) are allowed between segments.
		for i < len(in) && in[i] == 0xFF {
			i++
		}
		if i >= len(in) {
			return nil, 0, errors.New("media: truncated JPEG")
		}
		marker := in[i]
		i++
		// Markers without a length field.
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			buf.Write([]byte{0xFF, marker})
			continue
		}
		if i+2 > len(in) {
			return nil, 0, errors.New("media: truncated JPEG")
		}
		n := int(binary.BigEndian.Uint16(in[i:]))
		if n < 2 || i+n > len(in) {
			return nil, 0, errors.New("media: bad JPEG segment length")
		}
		payload := in[i+2 : i+n]
		if marker == markerAPP1 && orientation == 0 && bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
			orientation = exifOrientation(payload[6:])
		}
		if marker == markerSOS {
			// Everything from here is entropy-coded image data (and any
			// trailing segments are rare); copy the rest as-is.
			buf.Write([]byte{0xFF, marker})
			buf.Write(in[i:])
			return buf.Bytes(), orientation, nil
		}
		if keepJPEGSegment(marker, payload) {
			buf.Write([]byte{0xFF, marker})
			buf.Write(in[i : i+n])
		}
		i += n
	}
	return nil, 0, errors.New("media: JPEG has no image data")
}

// exifOrientation reads tag 0x0112 from a TIFF-structured EXIF block.
// Returns 0 if it isn't there or the block is malformed.
func exifOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 0
	}
	var bo binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}
	ifd := int(bo.Uint32(tiff[4:]))
	if ifd < 8 || ifd+2 > len(tiff) {
		return 0
	}
	count := int(bo.Uint16(tiff[ifd:]))
	for e := range count {
		off := ifd + 2 + e*12
		if off+12 > len(tiff) {
			return 0
		}
		if bo.Uint16(tiff[off:]) == 0x0112 {
			v := int(bo.Uint16(tiff[off+8:]))
			if v >= 1 && v <= 8 {
				return v
			}
			return 0
		}
	}
	return 0
}
