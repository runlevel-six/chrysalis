// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

// Package order generates fractional-index ordering keys (docs/DESIGN.md §5.4).
//
// A key is a string; rows sort by comparing keys as plain strings (SQLite's
// default BINARY collation). KeyBetween(a, b) returns a key that sorts
// strictly between a and b, so moving an item updates only that item's row.
//
// This is a Go implementation of the algorithm in the CC0-licensed JavaScript
// package "fractional-indexing" (github.com/rocicorp/fractional-indexing),
// which the frontend uses, so keys made on either side interoperate.
//
// Key shape: an "integer part" whose first character encodes its length
// ('a'..'z' = 2..27 characters for non-negative values, 'Z'..'A' for
// negative), followed by an optional fraction with no trailing zero digit.
package order

import (
	"errors"
	"fmt"
	"strings"
)

// digits are the base-62 digits in ASCII (and therefore sort) order.
const digits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

const (
	integerZero     = "a0"
	smallestInteger = "A00000000000000000000000000"
)

var ErrInvalidKey = errors.New("invalid order key")

// First is the key for the first item in an empty list.
func First() string { return integerZero }

// KeyBetween returns a key strictly between a and b. An empty a means "before
// the first item"; an empty b means "after the last item".
func KeyBetween(a, b string) (string, error) {
	if a != "" {
		if err := validate(a); err != nil {
			return "", err
		}
	}
	if b != "" {
		if err := validate(b); err != nil {
			return "", err
		}
	}
	if a != "" && b != "" && a >= b {
		return "", fmt.Errorf("order: %q is not before %q", a, b)
	}

	switch {
	case a == "" && b == "":
		return integerZero, nil

	case a == "":
		ib := integerPart(b)
		fb := b[len(ib):]
		if ib == smallestInteger {
			m, err := midpoint("", fb)
			return ib + m, err
		}
		if ib < b {
			return ib, nil
		}
		res, ok := decrementInteger(ib)
		if !ok {
			return "", errors.New("order: cannot go below the smallest key")
		}
		return res, nil

	case b == "":
		ia := integerPart(a)
		fa := a[len(ia):]
		if i, ok := incrementInteger(ia); ok {
			return i, nil
		}
		m, err := midpoint(fa, "")
		return ia + m, err
	}

	ia := integerPart(a)
	fa := a[len(ia):]
	ib := integerPart(b)
	fb := b[len(ib):]
	if ia == ib {
		m, err := midpoint(fa, fb)
		return ia + m, err
	}
	i, ok := incrementInteger(ia)
	if !ok {
		return "", errors.New("order: cannot increment past the largest key")
	}
	if i < b {
		return i, nil
	}
	m, err := midpoint(fa, "")
	return ia + m, err
}

// midpoint returns a fraction strictly between fractions a and b, where an
// empty b means "1" (the top of the range).
func midpoint(a, b string) (string, error) {
	zero := digits[0]
	if b != "" && a >= b {
		return "", fmt.Errorf("order: fraction %q is not before %q", a, b)
	}
	if (a != "" && a[len(a)-1] == zero) || (b != "" && b[len(b)-1] == zero) {
		return "", ErrInvalidKey
	}
	if b != "" {
		// Skip the common prefix (treating a as zero-padded).
		n := 0
		for {
			ca := zero
			if n < len(a) {
				ca = a[n]
			}
			if n >= len(b) || ca != b[n] {
				break
			}
			n++
		}
		if n > 0 {
			rest := ""
			if n < len(a) {
				rest = a[n:]
			}
			m, err := midpoint(rest, b[n:])
			return b[:n] + m, err
		}
	}

	digitA := 0
	if a != "" {
		digitA = strings.IndexByte(digits, a[0])
	}
	digitB := len(digits)
	if b != "" {
		digitB = strings.IndexByte(digits, b[0])
	}
	if digitB-digitA > 1 {
		// Round half up, matching JavaScript's Math.round.
		return string(digits[(digitA+digitB+1)/2]), nil
	}
	if len(b) > 1 {
		return b[:1], nil
	}
	rest := ""
	if len(a) > 1 {
		rest = a[1:]
	}
	m, err := midpoint(rest, "")
	return string(digits[digitA]) + m, err
}

func integerLength(head byte) (int, error) {
	switch {
	case head >= 'a' && head <= 'z':
		return int(head-'a') + 2, nil
	case head >= 'A' && head <= 'Z':
		return int('Z'-head) + 2, nil
	}
	return 0, ErrInvalidKey
}

// integerPart assumes key has already been validated.
func integerPart(key string) string {
	n, _ := integerLength(key[0])
	return key[:n]
}

func validate(key string) error {
	if key == smallestInteger {
		return ErrInvalidKey
	}
	n, err := integerLength(key[0])
	if err != nil || n > len(key) {
		return ErrInvalidKey
	}
	for i := 1; i < len(key); i++ {
		if strings.IndexByte(digits, key[i]) < 0 {
			return ErrInvalidKey
		}
	}
	if f := key[n:]; f != "" && f[len(f)-1] == digits[0] {
		return ErrInvalidKey
	}
	return nil
}

// incrementInteger adds one to an integer part. ok is false at the maximum.
func incrementInteger(x string) (string, bool) {
	head, digs := x[0], []byte(x[1:])
	carry := true
	for i := len(digs) - 1; carry && i >= 0; i-- {
		d := strings.IndexByte(digits, digs[i]) + 1
		if d == len(digits) {
			digs[i] = digits[0]
		} else {
			digs[i] = digits[d]
			carry = false
		}
	}
	if !carry {
		return string(head) + string(digs), true
	}
	switch head {
	case 'Z':
		return "a" + string(digits[0]), true
	case 'z':
		return "", false
	}
	h := head + 1
	if h > 'a' {
		digs = append(digs, digits[0])
	} else {
		digs = digs[:len(digs)-1]
	}
	return string(h) + string(digs), true
}

// decrementInteger subtracts one from an integer part. ok is false at the minimum.
func decrementInteger(x string) (string, bool) {
	head, digs := x[0], []byte(x[1:])
	last := digits[len(digits)-1]
	borrow := true
	for i := len(digs) - 1; borrow && i >= 0; i-- {
		d := strings.IndexByte(digits, digs[i]) - 1
		if d == -1 {
			digs[i] = last
		} else {
			digs[i] = digits[d]
			borrow = false
		}
	}
	if !borrow {
		return string(head) + string(digs), true
	}
	switch head {
	case 'a':
		return "Z" + string(last), true
	case 'A':
		return "", false
	}
	h := head - 1
	if h < 'Z' {
		digs = append(digs, last)
	} else {
		digs = digs[:len(digs)-1]
	}
	return string(h) + string(digs), true
}
