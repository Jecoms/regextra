package regextra_test

import (
	"regexp"
	"testing"

	rx "github.com/jecoms/regextra"
)

// Issue #113: pin the two tag-grammar forward-compatibility rules that the
// package doc (regextra.go "Tag grammar") declares part of the v1 contract,
// exercised through the public Unmarshal/Decoder surface rather than the
// unexported parser:
//
//   - Unknown key=value pairs are accepted, not rejected, so a future minor
//     release can recognize new option keys without a parser change.
//   - Lone tokens (no '=') are silently ignored, reserving that slot for
//     future flag-style options (e.g. `required`).
//
// Each rule is observable only as a no-op today: the extra option carries no
// meaning, so the field resolves exactly as if it were absent. Pairing the
// forward-compat piece with a `default` on an absent group makes that visible
// — the default still applies, proving the parser neither errored on nor was
// derailed by the extra piece. (Whether an unknown key is retained in the
// internal option map is not observable through the public API, so the tests
// pin only that such keys are accepted, not that they are stored.)

func TestUnmarshal_unknownOptionKeyIsAccepted(t *testing.T) {
	// Forward-compat rule 1: an unrecognized key=value must not be rejected.
	// `future` has no meaning today, so the field matches its group exactly as
	// if the option were absent.
	type Person struct {
		Name string `regex:"name,future=42"`
	}
	re := regexp.MustCompile(`(?P<name>\w+)`)

	var p Person
	if err := rx.Unmarshal(re, "Alice", &p); err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil (unknown option keys must be accepted, not rejected)", err)
	}
	if p.Name != "Alice" {
		t.Errorf("Name = %q, want %q", p.Name, "Alice")
	}
}

func TestUnmarshal_loneTokenIsIgnored(t *testing.T) {
	// Forward-compat rule 2: a lone token (no '=') is silently ignored today,
	// reserving the slot for future flag-style options. `required` has no
	// meaning yet, so it neither errors nor blocks the `default` that follows
	// it — the "role" group is absent, so the field takes the default.
	type Person struct {
		Role string `regex:"role,required,default=guest"`
	}
	re := regexp.MustCompile(`(?P<name>\w+)`) // no "role" group

	var p Person
	if err := rx.Unmarshal(re, "Alice", &p); err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil (a lone token must be ignored)", err)
	}
	if p.Role != "guest" {
		t.Errorf("Role = %q, want %q (lone token ignored; default still applies)", p.Role, "guest")
	}
}

func TestUnmarshal_emptyOptionPieceIsSkipped(t *testing.T) {
	// An empty option piece is skipped without disturbing the options around
	// it: the `default` still resolves when the group is absent. The skip
	// applies the same way wherever the empty piece falls, so these positional
	// variants share subtests (struct tags are compile-time literals, so each
	// needs its own struct rather than a value table). This pins the observable
	// behavior, not a specific parser branch — an empty piece is inert whether
	// the dedicated skip handles it or it falls through the lone-token path.
	re := regexp.MustCompile(`(?P<name>\w+)`) // no "role" group; default fires

	t.Run("doubled comma", func(t *testing.T) {
		type Person struct {
			Role string `regex:"role,,default=guest"`
		}
		var p Person
		if err := rx.Unmarshal(re, "Alice", &p); err != nil {
			t.Fatalf("Unmarshal() error = %v, want nil", err)
		}
		if p.Role != "guest" {
			t.Errorf("Role = %q, want %q (empty piece skipped; default still applies)", p.Role, "guest")
		}
	})

	t.Run("trailing comma", func(t *testing.T) {
		type Person struct {
			Role string `regex:"role,default=guest,"`
		}
		var p Person
		if err := rx.Unmarshal(re, "Alice", &p); err != nil {
			t.Fatalf("Unmarshal() error = %v, want nil", err)
		}
		if p.Role != "guest" {
			t.Errorf("Role = %q, want %q (trailing empty piece skipped)", p.Role, "guest")
		}
	})

	t.Run("whitespace-only piece", func(t *testing.T) {
		type Person struct {
			Role string `regex:"role, ,default=guest"`
		}
		var p Person
		if err := rx.Unmarshal(re, "Alice", &p); err != nil {
			t.Fatalf("Unmarshal() error = %v, want nil", err)
		}
		if p.Role != "guest" {
			t.Errorf("Role = %q, want %q (whitespace-only piece skipped)", p.Role, "guest")
		}
	})
}

// parseFieldTag feeds the Decoder/Compile path too, so the same forward-compat
// no-ops must hold there. Parsing happens once at compile time and One/All/Iter
// share that result, so a single One probe is sufficient parity coverage.
func TestDecoder_tagForwardCompatRules(t *testing.T) {
	type Person struct {
		Name string `regex:"name,future=42"`              // unknown key: accepted, not rejected
		Role string `regex:"role,required,default=guest"` // lone token: ignored; default applies
	}
	d, err := rx.Compile[Person](`(?P<name>\w+)`) // no "role" group; default fires
	if err != nil {
		t.Fatalf("Compile() error = %v, want nil (forward-compat options must not break compilation)", err)
	}

	p, err := d.One("Alice")
	if err != nil {
		t.Fatalf("One() error = %v", err)
	}
	if p.Name != "Alice" {
		t.Errorf("Name = %q, want %q", p.Name, "Alice")
	}
	if p.Role != "guest" {
		t.Errorf("Role = %q, want %q (lone token ignored; default applies on the Decoder path)", p.Role, "guest")
	}
}
