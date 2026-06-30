package regextra

import (
	"reflect"
	"strings"
	"testing"
)

// Issue #113: pin the two tag-grammar forward-compatibility rules that the
// package doc (regextra.go "Tag grammar") declares part of the v1 contract:
//
//   - Unknown key=value pairs are preserved, not rejected, so a future minor
//     release can recognize new option keys without a parser change.
//   - Lone tokens (no `=`) are silently ignored, reserving that slot for
//     future flag-style options (e.g. `required`).
//
// Both branches in parseFieldTag (the empty-piece skip and the lone-token
// drop) were previously uncovered; these tests lock the behavior in so the
// contract can't regress silently.

// fieldWithTag builds a reflect.StructField carrying the given regex tag.
// parseFieldTag only reads field.Tag, so this is enough to exercise it
// directly without constructing a whole regexp/Unmarshal round trip.
func fieldWithTag(tag string) reflect.StructField {
	return reflect.StructField{
		Name: "F",
		Type: reflect.TypeOf(""),
		Tag:  reflect.StructTag(`regex:"` + tag + `"`),
	}
}

func TestParseFieldTag(t *testing.T) {
	tests := []struct {
		name     string
		tag      string
		wantName string
		wantOpts map[string]string
		wantSkip bool
	}{
		{
			name:     "empty tag falls back to field name",
			tag:      "",
			wantName: "",
			wantOpts: nil,
			wantSkip: false,
		},
		{
			name:     "bare dash excludes",
			tag:      "-",
			wantName: "",
			wantOpts: nil,
			wantSkip: true,
		},
		{
			name:     "lone name only",
			tag:      "name",
			wantName: "name",
			wantOpts: nil,
			wantSkip: false,
		},
		{
			name:     "recognized key=value",
			tag:      "name,default=x",
			wantName: "name",
			wantOpts: map[string]string{"default": "x"},
			wantSkip: false,
		},
		{
			// Forward-compat rule 1: an unknown key is stored verbatim, not
			// rejected, so a later minor can give it meaning.
			name:     "unknown key=value preserved",
			tag:      "name,future=42",
			wantName: "name",
			wantOpts: map[string]string{"future": "42"},
			wantSkip: false,
		},
		{
			// Forward-compat rule 2: a lone token (no '=') is dropped today.
			// opts is allocated because there is a second piece, but stays
			// empty — the lone token leaves nothing behind.
			name:     "lone token is ignored",
			tag:      "name,required",
			wantName: "name",
			wantOpts: map[string]string{},
			wantSkip: false,
		},
		{
			// A lone token and a recognized key=value in the same tag: the
			// lone token is dropped, the key=value survives.
			name:     "lone token dropped while a key=value is kept",
			tag:      "name,a,b=2",
			wantName: "name",
			wantOpts: map[string]string{"b": "2"},
			wantSkip: false,
		},
		{
			// Empty pieces between commas are skipped without affecting opts.
			name:     "empty piece is skipped",
			tag:      "name,,default=x",
			wantName: "name",
			wantOpts: map[string]string{"default": "x"},
			wantSkip: false,
		},
		{
			// Whitespace inside a key=value is trimmed on both sides of '='.
			name:     "whitespace around key and value is trimmed",
			tag:      " name , default = x ",
			wantName: "name",
			wantOpts: map[string]string{"default": "x"},
			wantSkip: false,
		},
		{
			// Both rules together plus surrounding whitespace: lone token
			// dropped, empty piece skipped, key=value kept and trimmed.
			name:     "lone token and empty piece alongside a real option",
			tag:      "name, required ,, layout=2006 ",
			wantName: "name",
			wantOpts: map[string]string{"layout": "2006"},
			wantSkip: false,
		},
		{
			// Leading "-" with options is NOT an exclude: "-" parses as the
			// group name (matches no group), per the documented boundary.
			name:     "dash with options is a group name, not exclude",
			tag:      "-,default=x",
			wantName: "-",
			wantOpts: map[string]string{"default": "x"},
			wantSkip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, opts, skip := parseFieldTag(fieldWithTag(tt.tag))
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
			if skip != tt.wantSkip {
				t.Errorf("skip = %v, want %v", skip, tt.wantSkip)
			}
			if !reflect.DeepEqual(opts, tt.wantOpts) {
				t.Errorf("opts = %#v, want %#v", opts, tt.wantOpts)
			}
		})
	}
}

// FuzzParseFieldTag drives parseFieldTag with arbitrary tag bodies and asserts
// the structural invariants the v1 contract rests on, rather than re-deriving
// the expected output (which would just duplicate the parser).
func FuzzParseFieldTag(f *testing.F) {
	for _, s := range []string{
		"", "-", "name", "name,default=x", "name,required",
		"name,,default=x", "-,default=x", " name , a=b ", "=", ",", "a=b=c",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, rawTag string) {
		field := fieldWithTag(rawTag)
		// Embedding rawTag in a struct-tag literal does not round-trip when it
		// contains quotes or backslashes, so assert against the value the
		// parser actually reads — the same Tag.Get call parseFieldTag makes.
		tag := field.Tag.Get("regex")

		name, opts, skip := parseFieldTag(field)

		// Only the bare "-" excludes; nothing else does.
		if skip != (tag == "-") {
			t.Fatalf("skip = %v for tag %q, want %v", skip, tag, tag == "-")
		}
		// An excluded field carries no name and no options.
		if skip {
			if name != "" || opts != nil {
				t.Fatalf("excluded tag %q yielded name=%q opts=%#v, want empty", tag, name, opts)
			}
			return
		}
		// The name is always the trimmed first comma-piece.
		if want := strings.TrimSpace(strings.Split(tag, ",")[0]); name != want {
			t.Fatalf("name = %q for tag %q, want %q", name, tag, want)
		}
		// opts is allocated iff there is at least one option piece, regardless
		// of whether any piece survived parsing (lone tokens / empties leave an
		// allocated-but-empty map). A single-piece tag yields a nil opts.
		single := !strings.Contains(tag, ",")
		if single && opts != nil {
			t.Fatalf("single-piece tag %q yielded non-nil opts %#v", tag, opts)
		}
		if !single && opts == nil {
			t.Fatalf("multi-piece tag %q yielded nil opts", tag)
		}
		// Every stored key=value came from a piece containing '='; a lone
		// token (no '=') must never appear as a key.
		for k := range opts {
			if k == "" {
				continue // "=value" cuts to an empty key; allowed, just not a lone token
			}
			found := false
			for _, p := range strings.Split(tag, ",")[1:] {
				p = strings.TrimSpace(p)
				ck, _, ok := strings.Cut(p, "=")
				if ok && strings.TrimSpace(ck) == k {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("opts key %q in tag %q did not come from a key=value piece", k, tag)
			}
		}
	})
}
