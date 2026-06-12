package regextra

import (
	"regexp"
	"testing"
)

// Tests for https://github.com/Jecoms/regextra/issues/110: the Unmarshal and
// Decoder paths previously used two different case-folding implementations
// (Unicode-aware strings.ToLower vs a hand-rolled ASCII lower), which could
// disagree on non-ASCII field names. Both now use strings.EqualFold.
//
// Note: Go's regexp rejects non-ASCII capture group names, so divergence was
// only possible via the *field* name side of the comparison.

func TestCaseInsensitiveFallback_parity(t *testing.T) {
	// The field name "KELVIN" below starts with U+212A (KELVIN SIGN), an
	// uppercase Unicode letter that folds to ASCII 'k' under Unicode case
	// folding but is untouched by ASCII-only lowering. Before the fix,
	// Unmarshal matched it to the "kelvin" group and the Decoder did not.
	const pattern = `(?P<kelvin>\d+)`
	type reading struct {
		KELVIN int
	}

	re := regexp.MustCompile(pattern)
	var fromUnmarshal reading
	if err := Unmarshal(re, "273", &fromUnmarshal); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	dec, err := Compile[reading](pattern)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	fromDecoder, err := dec.One("273")
	if err != nil {
		t.Fatalf("Decoder.One: %v", err)
	}

	if fromUnmarshal.KELVIN != 273 {
		t.Errorf("Unmarshal: KELVIN = %d, want 273", fromUnmarshal.KELVIN)
	}
	if fromDecoder.KELVIN != 273 {
		t.Errorf("Decoder.One: KELVIN = %d, want 273", fromDecoder.KELVIN)
	}
}

func TestCaseInsensitiveFallback_ascii(t *testing.T) {
	// Plain ASCII case-insensitive fallback keeps working in both paths.
	const pattern = `(?P<username>\w+)`
	type rec struct {
		UserName string
	}

	re := regexp.MustCompile(pattern)
	var u rec
	if err := Unmarshal(re, "alice", &u); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if u.UserName != "alice" {
		t.Errorf("Unmarshal: UserName = %q, want %q", u.UserName, "alice")
	}

	dec := MustCompile[rec](pattern)
	got, err := dec.One("alice")
	if err != nil {
		t.Fatalf("Decoder.One: %v", err)
	}
	if got.UserName != "alice" {
		t.Errorf("Decoder.One: UserName = %q, want %q", got.UserName, "alice")
	}
}
