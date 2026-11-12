// Copyright 2026 Jason Hall
// SPDX-License-Identifier: Apache-2.0

package order

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// Expected values computed with the JavaScript fractional-indexing package's
// generateKeyBetween, so the two implementations stay interchangeable.
func TestKeyBetweenKnownValues(t *testing.T) {
	tests := []struct{ a, b, want string }{
		{"", "", "a0"},
		{"a0", "", "a1"},
		{"a1", "", "a2"},
		{"", "a0", "Zz"},
		{"", "Zz", "Zy"},
		{"a0", "a1", "a0V"},
		{"a1", "a2", "a1V"},
		{"a0V", "a1", "a0l"},
		{"Zz", "a0", "ZzV"},
		{"Zz", "a1", "a0"},
		{"a0", "a0V", "a0G"},
		{"a0", "a0G", "a08"},
		{"az", "", "b00"},
		{"b0z", "", "b10"},
		{"", "b00", "az"},
		{"a1", "a1V", "a1G"},
		{"zzzzzzzzzzzzzzzzzzzzzzzzzzz", "", "zzzzzzzzzzzzzzzzzzzzzzzzzzzV"},
		{"", "A00000000000000000000000001", "A00000000000000000000000000"},
	}
	for _, tt := range tests {
		got, err := KeyBetween(tt.a, tt.b)
		if err != nil || got != tt.want {
			t.Errorf("KeyBetween(%q, %q) = %q, %v; want %q", tt.a, tt.b, got, err, tt.want)
		}
	}
}

func TestKeyBetweenErrors(t *testing.T) {
	for _, tt := range []struct{ a, b string }{
		{"a1", "a0"}, // reversed
		{"a0", "a0"}, // equal
		{"a00", ""},  // trailing zero
		{"a", ""},    // too short for its head
		{"!0", ""},   // bad head
		{"a0!", ""},  // bad digit
		{"", smallestInteger},
	} {
		if got, err := KeyBetween(tt.a, tt.b); err == nil {
			t.Errorf("KeyBetween(%q, %q) = %q, want error", tt.a, tt.b, got)
		}
	}
}

// Random inserts into a list must always produce a key strictly between its
// neighbors, so the list stays sorted.
func TestKeyBetweenRandomInserts(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	keys := []string{}
	for range 5000 {
		i := r.IntN(len(keys) + 1)
		var a, b string
		if i > 0 {
			a = keys[i-1]
		}
		if i < len(keys) {
			b = keys[i]
		}
		k, err := KeyBetween(a, b)
		if err != nil {
			t.Fatalf("KeyBetween(%q, %q): %v", a, b, err)
		}
		if (a != "" && k <= a) || (b != "" && k >= b) {
			t.Fatalf("KeyBetween(%q, %q) = %q, not between", a, b, k)
		}
		if err := validate(k); err != nil {
			t.Fatalf("generated invalid key %q", k)
		}
		keys = slices.Insert(keys, i, k)
	}
	if !slices.IsSorted(keys) {
		t.Fatal("keys not sorted")
	}
}

func TestAppendStaysShort(t *testing.T) {
	k := First()
	for range 10000 {
		var err error
		if k, err = KeyBetween(k, ""); err != nil {
			t.Fatal(err)
		}
	}
	if len(k) > 4 {
		t.Errorf("10000 appends grew the key to %q", k)
	}
}
