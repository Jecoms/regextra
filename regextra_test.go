package regextra_test

import (
	"fmt"
	rx "github.com/jecoms/regextra"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestFindNamed(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		target    string
		groupName string
		want      string
		wantFound bool
	}{
		{
			name:      "found first group",
			pattern:   `(?P<first>one) (?P<second>two) (?P<second>again) three`,
			target:    "one two again three",
			groupName: "first",
			want:      "one",
			wantFound: true,
		},
		{
			// Two `second` groups, both participating: the last occurrence
			// wins, consistent with NamedGroups (see TestNamedGroups,
			// "duplicate names return last match"). FindNamed used to return
			// the first occurrence ("two") via re.SubexpIndex; that disagreed
			// with every other name-based reader and is the issue-105 bug.
			name:      "found second group (duplicate name: last participating wins)",
			pattern:   `(?P<first>one) (?P<second>two) (?P<second>again) three`,
			target:    "one two again three",
			groupName: "second",
			want:      "again",
			wantFound: true,
		},
		{
			name:      "group not found",
			pattern:   `(?P<first>one) (?P<second>two) (?P<second>again) three`,
			target:    "one two again three",
			groupName: "third",
			want:      "",
			wantFound: false,
		},
		{
			name:      "extract price",
			pattern:   `(?P<price>\$\d+(,\d{3})*(\.\d{1,2})?)`,
			target:    "The price is $1,234.56",
			groupName: "price",
			want:      "$1,234.56",
			wantFound: true,
		},
		{
			name:      "no match returns empty",
			pattern:   `(?P<name>[a-z]+)`,
			target:    "123",
			groupName: "name",
			want:      "",
			wantFound: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.pattern)
			got, found := rx.FindNamed(re, tt.target, tt.groupName)
			if got != tt.want {
				t.Errorf("FindNamed() got = %v, want %v", got, tt.want)
			}
			if found != tt.wantFound {
				t.Errorf("FindNamed() found = %v, wantFound %v", found, tt.wantFound)
			}
		})
	}
}

func TestNamedGroups(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		target  string
		want    map[string]string
	}{
		{
			name:    "found multiple named groups",
			pattern: `(?P<first>one) (?P<second>two) (?P<second>again) three`,
			target:  "one two again three",
			want: map[string]string{
				"first":  "one",
				"second": "again", // duplicate names return last match
			},
		},
		{
			name:    "no match returns empty map",
			pattern: `(?P<first>one) (?P<second>two) (?P<second>again) three`,
			target:  "one two three",
			want:    map[string]string{},
		},
		{
			name:    "single named group",
			pattern: `(?P<word>\w+)`,
			target:  "hello",
			want: map[string]string{
				"word": "hello",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.pattern)
			if got := rx.NamedGroups(re, tt.target); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NamedGroups() = %v, want %v", got, tt.want)
			}
		})
	}
}

func ExampleFindNamed() {
	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
	name, ok := rx.FindNamed(re, "Alice 30", "name")
	fmt.Printf("%s: %v\n", name, ok)
	// Output: Alice: true
}

func TestFindAllNamed(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		target  string
		group   string
		want    []string
	}{
		{
			name:    "multiple matches collects all values",
			pattern: `(?P<word>\S+)`,
			target:  "alpha beta gamma",
			group:   "word",
			want:    []string{"alpha", "beta", "gamma"},
		},
		{
			name:    "two-group pattern, picks the requested group",
			pattern: `(?P<key>\w+)=(?P<val>\d+)`,
			target:  "a=1 b=2 c=3",
			group:   "val",
			want:    []string{"1", "2", "3"},
		},
		{
			name:    "no matches returns empty slice (not nil)",
			pattern: `(?P<word>[A-Z]+)`,
			target:  "all lowercase",
			group:   "word",
			want:    []string{},
		},
		{
			name:    "undeclared group returns nil",
			pattern: `(?P<word>\S+)`,
			target:  "anything",
			group:   "missing",
			want:    nil,
		},
		{
			name:    "empty target returns empty slice when group declared",
			pattern: `(?P<word>\S+)`,
			target:  "",
			group:   "word",
			want:    []string{},
		},
		{
			// The requested group is optional and does not participate in every
			// match; non-participating occurrences yield "" (matching what
			// FindStringSubmatch reports for an unmatched group).
			name:    "optional group, non-participating matches yield empty string",
			pattern: `(?P<a>\w+)(?P<b>!)?`,
			target:  "x! y z!",
			group:   "b",
			want:    []string{"!", "", "!"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.pattern)
			got := rx.FindAllNamed(re, tt.target, tt.group)
			if tt.want == nil {
				if got != nil {
					t.Errorf("FindAllNamed = %v, want nil", got)
				}
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FindAllNamed = %v, want %v", got, tt.want)
			}
		})
	}
}

func ExampleFindAllNamed() {
	re := regexp.MustCompile(`(?P<word>\S+)`)
	words := rx.FindAllNamed(re, "alpha beta gamma", "word")
	fmt.Println(words)
	// Output: [alpha beta gamma]
}

func ExampleNamedGroups() {
	re := regexp.MustCompile(`(?P<year>\d{4})-(?P<month>\d{2})-(?P<day>\d{2})`)
	groups := rx.NamedGroups(re, "Date: 2025-10-04")

	// Note: map iteration order is not guaranteed, so we print sorted
	keys := []string{"year", "month", "day"}
	for _, key := range keys {
		fmt.Printf("%s=%s ", key, groups[key])
	}
	// Output: year=2025 month=10 day=04
}

func TestAllNamedGroups(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		target  string
		want    map[string][]string
	}{
		{
			name:    "duplicate group names in same match",
			pattern: `(?P<word>\w+) (?P<word>\w+)`,
			target:  "hello world",
			want: map[string][]string{
				"word": {"hello", "world"},
			},
		},
		{
			name:    "multiple different groups",
			pattern: `(?P<name>\w+) (?P<age>\d+)`,
			target:  "Alice 30",
			want: map[string][]string{
				"name": {"Alice"},
				"age":  {"30"},
			},
		},
		{
			name:    "three duplicate group names",
			pattern: `(?P<item>\w+) (?P<item>\w+) (?P<item>\w+)`,
			target:  "one two three",
			want: map[string][]string{
				"item": {"one", "two", "three"},
			},
		},
		{
			name:    "mixed duplicate and unique groups",
			pattern: `(?P<word>\w+) (?P<num>\d+) (?P<word>\w+)`,
			target:  "hello 123 world",
			want: map[string][]string{
				"word": {"hello", "world"},
				"num":  {"123"},
			},
		},
		{
			name:    "no match returns empty map",
			pattern: `(?P<digit>\d+)`,
			target:  "abc",
			want:    map[string][]string{},
		},
		{
			name:    "single group single match",
			pattern: `(?P<price>\$\d+\.\d{2})`,
			target:  "Total: $19.99",
			want: map[string][]string{
				"price": {"$19.99"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.pattern)
			got := rx.AllNamedGroups(re, tt.target)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AllNamedGroups() = %v, want %v", got, tt.want)
			}
		})
	}
}

func ExampleAllNamedGroups() {
	re := regexp.MustCompile(`(?P<word>\w+) (?P<word>\w+) (?P<word>\w+)`)
	allGroups := rx.AllNamedGroups(re, "one two three")

	fmt.Printf("word: %v\n", allGroups["word"])
	// Output: word: [one two three]
}

func TestReplace(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		target  string
		repl    map[string]string
		want    string
	}{
		{
			name:    "single match, single substitution",
			pattern: `(?P<user>\w+)@(?P<domain>[\w.]+)`,
			target:  "alice@example.com",
			repl:    map[string]string{"domain": "redacted"},
			want:    "alice@redacted",
		},
		{
			name:    "single match, all groups substituted",
			pattern: `(?P<user>\w+)@(?P<domain>[\w.]+)`,
			target:  "alice@example.com",
			repl:    map[string]string{"user": "bob", "domain": "redacted"},
			want:    "bob@redacted",
		},
		{
			name:    "multiple matches, every match substituted",
			pattern: `(?P<word>\w+)`,
			target:  "alpha beta gamma",
			repl:    map[string]string{"word": "X"},
			want:    "X X X",
		},
		{
			name:    "multiple matches, key not in map passes through",
			pattern: `(?P<key>\w+)=(?P<val>\d+)`,
			target:  "a=1 b=2",
			repl:    map[string]string{"val": "?"},
			want:    "a=? b=?",
		},
		{
			name:    "no match returns target unchanged",
			pattern: `(?P<word>[A-Z]+)`,
			target:  "no matches here",
			repl:    map[string]string{"word": "X"},
			want:    "no matches here",
		},
		{
			name:    "empty replacements returns target unchanged",
			pattern: `(?P<word>\w+)`,
			target:  "alpha beta",
			repl:    map[string]string{},
			want:    "alpha beta",
		},
		{
			name:    "unknown group name in map is ignored",
			pattern: `(?P<word>\w+)`,
			target:  "hello",
			repl:    map[string]string{"missing": "X"},
			want:    "hello",
		},
		{
			name:    "preserves non-matching text between matches",
			pattern: `(?P<num>\d+)`,
			target:  "a 1 b 2 c 3 d",
			repl:    map[string]string{"num": "*"},
			want:    "a * b * c * d",
		},
		{
			// Issue #107: nested named groups share a start offset, so the span
			// sort must be deterministic. The outermost span (same start, larger
			// end) wins; the inner group inside the already-replaced span is not
			// substituted. The previous unstable sort.Slice made this flaky.
			name:    "nested groups, outermost wins",
			pattern: `(?P<outer>(?P<inner>\w+)@[\w.]+)`,
			target:  "alice@example.com",
			repl:    map[string]string{"outer": "REDACTED", "inner": "X"},
			want:    "REDACTED",
		},
		{
			name:    "nested groups, only inner group in map is substituted",
			pattern: `(?P<outer>(?P<inner>\w+)@[\w.]+)`,
			target:  "alice@example.com",
			repl:    map[string]string{"inner": "X"},
			want:    "X@example.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.pattern)
			got := rx.Replace(re, tt.target, tt.repl)
			if got != tt.want {
				t.Errorf("Replace = %q, want %q", got, tt.want)
			}
		})
	}
}

func ExampleReplace() {
	re := regexp.MustCompile(`(?P<user>\w+)@(?P<domain>[\w.]+)`)
	out := rx.Replace(re, "alice@example.com bob@other.org", map[string]string{
		"domain": "redacted",
	})
	fmt.Println(out)
	// Output: alice@redacted bob@redacted
}

func TestReplaceFunc(t *testing.T) {
	upper := func(_, m string) string { return strings.ToUpper(m) }

	tests := []struct {
		name    string
		pattern string
		target  string
		fn      func(group, match string) string
		want    string
	}{
		{
			name:    "redaction: mask all but last four digits",
			pattern: `(?P<card>\d{12,19})`,
			target:  "card 4111111111111111 ok",
			fn: func(_, m string) string {
				return strings.Repeat("*", len(m)-4) + m[len(m)-4:]
			},
			want: "card ************1111 ok",
		},
		{
			name:    "normalization: lowercase captured host across matches",
			pattern: `https?://(?P<host>[\w.]+)`,
			target:  "http://Example.COM and https://API.Example.com",
			fn:      func(_, m string) string { return strings.ToLower(m) },
			want:    "http://example.com and https://api.example.com",
		},
		{
			name:    "fn receives the group name",
			pattern: `(?P<key>\w+)=(?P<val>\d+)`,
			target:  "a=1 b=2",
			fn:      func(group, m string) string { return group + ":" + m },
			want:    "key:a=val:1 key:b=val:2",
		},
		{
			name:    "return match verbatim leaves the group unchanged",
			pattern: `(?P<user>\w+)@(?P<domain>[\w.]+)`,
			target:  "alice@example.com",
			fn: func(group, m string) string {
				if group == "domain" {
					return "redacted"
				}
				return m // leave user unchanged
			},
			want: "alice@redacted",
		},
		{
			name:    "no match returns target unchanged",
			pattern: `(?P<word>[A-Z]+)`,
			target:  "no matches here",
			fn:      upper,
			want:    "no matches here",
		},
		{
			name:    "preserves non-matching text between matches",
			pattern: `(?P<num>\d+)`,
			target:  "a 1 b 2 c 3 d",
			fn:      func(_, m string) string { return "<" + m + ">" },
			want:    "a <1> b <2> c <3> d",
		},
		{
			// Inner group inside an already-substituted outermost span is not
			// reached by fn — mirrors Replace's overlap rule.
			name:    "nested groups, outermost wins and inner fn not invoked",
			pattern: `(?P<outer>(?P<inner>\w+)@[\w.]+)`,
			target:  "alice@example.com",
			fn:      upper,
			want:    "ALICE@EXAMPLE.COM",
		},
		{
			// Non-participating optional group is skipped, so fn never sees it.
			name:    "optional non-participating group skipped",
			pattern: `(?P<word>\w+)(?P<bang>!)?`,
			target:  "hi there",
			fn:      func(_, m string) string { return "[" + m + "]" },
			want:    "[hi] [there]",
		},
		{
			// Duplicate group name: fn runs for each participating occurrence.
			name:    "duplicate group name, fn runs per occurrence",
			pattern: `(?P<w>\w+) (?P<w>\w+)`,
			target:  "hello world",
			fn:      upper,
			want:    "HELLO WORLD",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.pattern)
			got := rx.ReplaceFunc(re, tt.target, tt.fn)
			if got != tt.want {
				t.Errorf("ReplaceFunc = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReplaceFuncInnerGroupNotInvoked(t *testing.T) {
	// fn must never be called for an inner group suppressed by an outermost
	// span — assert the callback is not invoked for the "inner" name.
	re := regexp.MustCompile(`(?P<outer>(?P<inner>\w+)@[\w.]+)`)
	var seen []string
	out := rx.ReplaceFunc(re, "alice@example.com", func(group, m string) string {
		seen = append(seen, group)
		return strings.ToUpper(m)
	})
	if out != "ALICE@EXAMPLE.COM" {
		t.Errorf("ReplaceFunc = %q, want %q", out, "ALICE@EXAMPLE.COM")
	}
	for _, g := range seen {
		if g == "inner" {
			t.Errorf("fn invoked for suppressed inner group; groups seen = %v", seen)
		}
	}
	if len(seen) != 1 || seen[0] != "outer" {
		t.Errorf("groups seen = %v, want [outer]", seen)
	}
}

func TestReplaceFuncNoMatchNeverCallsFn(t *testing.T) {
	re := regexp.MustCompile(`(?P<word>[A-Z]+)`)
	called := false
	out := rx.ReplaceFunc(re, "no matches here", func(_, m string) string {
		called = true
		return m
	})
	if called {
		t.Error("fn was called despite no match")
	}
	if out != "no matches here" {
		t.Errorf("ReplaceFunc = %q, want target unchanged", out)
	}
}

func TestReplaceFuncNilFn(t *testing.T) {
	// A nil fn is a programmer error. It panics on the first substituted match,
	// mirroring regexp.Regexp.ReplaceAllStringFunc, but never panics when there
	// is nothing to substitute (no match returns target before fn is reached).
	t.Run("panics on first match", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("ReplaceFunc with nil fn did not panic on a match")
			}
		}()
		re := regexp.MustCompile(`(?P<word>[A-Z]+)`)
		rx.ReplaceFunc(re, "HELLO", nil)
	})

	t.Run("no panic on no match", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ReplaceFunc with nil fn panicked on no match: %v", r)
			}
		}()
		re := regexp.MustCompile(`(?P<word>[A-Z]+)`)
		if out := rx.ReplaceFunc(re, "no matches here", nil); out != "no matches here" {
			t.Errorf("ReplaceFunc = %q, want target unchanged", out)
		}
	})
}

func ExampleReplaceFunc() {
	// Mask all but the last four digits of a captured card number.
	re := regexp.MustCompile(`(?P<card>\d{12,19})`)
	out := rx.ReplaceFunc(re, "card 4111111111111111 ok", func(_, match string) string {
		return strings.Repeat("*", len(match)-4) + match[len(match)-4:]
	})
	fmt.Println(out)
	// Output: card ************1111 ok
}

func TestValidate(t *testing.T) {
	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)

	t.Run("all declared returns nil", func(t *testing.T) {
		if err := rx.Validate(re, "name", "age"); err != nil {
			t.Errorf("Validate returned %v, want nil", err)
		}
	})

	t.Run("subset of declared returns nil", func(t *testing.T) {
		if err := rx.Validate(re, "name"); err != nil {
			t.Errorf("Validate returned %v, want nil", err)
		}
	})

	t.Run("empty required returns nil", func(t *testing.T) {
		if err := rx.Validate(re); err != nil {
			t.Errorf("Validate returned %v, want nil", err)
		}
	})

	t.Run("single missing reports it", func(t *testing.T) {
		err := rx.Validate(re, "name", "ssn")
		if err == nil {
			t.Fatal("Validate returned nil, want error")
		}
		want := "regextra.Validate: missing named groups: ssn"
		if err.Error() != want {
			t.Errorf("Validate error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("multiple missing preserves request order", func(t *testing.T) {
		err := rx.Validate(re, "ssn", "age", "email", "name", "phone")
		if err == nil {
			t.Fatal("Validate returned nil, want error")
		}
		want := "regextra.Validate: missing named groups: ssn, email, phone"
		if err.Error() != want {
			t.Errorf("Validate error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("regex with no named groups, all required missing", func(t *testing.T) {
		bare := regexp.MustCompile(`\w+`)
		err := rx.Validate(bare, "name")
		if err == nil {
			t.Fatal("Validate returned nil, want error")
		}
	})
}

// status is a custom type whose pointer satisfies RegexUnmarshaler;
// used by TestUnmarshalRegexUnmarshaler.
func ExampleValidate() {
	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
	if err := rx.Validate(re, "name", "age", "ssn"); err != nil {
		fmt.Println(err)
	}
	// Output: regextra.Validate: missing named groups: ssn
}

func FuzzFindNamed(f *testing.F) {
	seeds := []string{
		"",
		"Alice 30",
		"Bob 25 ignored",
		"  spaces  42  ",
		"unicode: αβγ 99",
		"emoji: 🦀 7",
		"only digits: 12345",
		"multiline\nrow 1\nrow 2",
		"\x00null\x00 0",
		"name 99999999999999999999",
		"x \t1",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	re := regexp.MustCompile(`(?P<word>\S+)\s+(?P<num>\d+)`)
	f.Fuzz(func(t *testing.T, target string) {
		// Should not panic for any input.
		got, ok := rx.FindNamed(re, target, "word")
		if !ok && got != "" {
			t.Fatalf("FindNamed contract violated: ok=false but got=%q (want empty)", got)
		}
		// Group name that doesn't exist must always return ("", false).
		miss, missOk := rx.FindNamed(re, target, "definitelyNotThere")
		if miss != "" || missOk {
			t.Fatalf("missing group name should return (\"\", false), got (%q, %v)", miss, missOk)
		}
	})
}

// FuzzNamedGroups feeds arbitrary inputs to NamedGroups. Asserts contract:
// returned map is non-nil; every key, when present, corresponds to a
// declared group on the regex.
func FuzzNamedGroups(f *testing.F) {
	for _, s := range []string{"", "a 1", "abc 12 def 34", "\xff bad utf8"} {
		f.Add(s)
	}

	re := regexp.MustCompile(`(?P<word>\S+)\s+(?P<num>\d+)`)
	declared := map[string]bool{"word": true, "num": true}

	f.Fuzz(func(t *testing.T, target string) {
		groups := rx.NamedGroups(re, target)
		if groups == nil {
			t.Fatalf("NamedGroups returned nil; contract requires non-nil map")
		}
		for k := range groups {
			if !declared[k] {
				t.Fatalf("NamedGroups returned undeclared key %q", k)
			}
		}
	})
}

// FuzzUnmarshalInt drives the int-conversion branch of setFieldValue with
// arbitrary group values. Pattern is one named group; target is constructed
// from the fuzz input. Failure modes: panic, or success on a value that
// strconv.ParseInt(value, 10, 64) would reject.

// Regression tests for https://github.com/Jecoms/regextra/issues/105:
// patterns that reuse a group name (legal in Go's regexp, e.g. across
// alternation branches) were mishandled in both decode paths — the map
// builders let a non-participating later occurrence clobber the real value
// with "", and the Decoder read only SubexpIndex's first occurrence.

func TestNamedGroups_duplicateNames(t *testing.T) {
	re := regexp.MustCompile(`(?:x(?P<word>a)|y(?P<word>b))`)

	t.Run("first alternation branch participates", func(t *testing.T) {
		got := rx.NamedGroups(re, "xa")
		if got["word"] != "a" {
			t.Errorf(`got word=%q, want "a" (non-participating duplicate must not clobber)`, got["word"])
		}
	})

	t.Run("second alternation branch participates", func(t *testing.T) {
		got := rx.NamedGroups(re, "yb")
		if got["word"] != "b" {
			t.Errorf(`got word=%q, want "b"`, got["word"])
		}
	})

	t.Run("sequential duplicates keep last-wins", func(t *testing.T) {
		seq := regexp.MustCompile(`(?P<word>\w+) (?P<word>\w+)`)
		got := rx.NamedGroups(seq, "hello world")
		if got["word"] != "world" {
			t.Errorf(`got word=%q, want "world" (last participating occurrence wins)`, got["word"])
		}
	})

	t.Run("AllNamedGroups still preserves every occurrence", func(t *testing.T) {
		got := rx.AllNamedGroups(re, "xa")
		want := []string{"a", ""}
		if len(got["word"]) != 2 || got["word"][0] != want[0] || got["word"][1] != want[1] {
			t.Errorf("got word=%q, want %q", got["word"], want)
		}
	})
}

func TestFindNamed_duplicateNames(t *testing.T) {
	re := regexp.MustCompile(`(?:x(?P<word>a)|y(?P<word>b))`)

	if got, ok := rx.FindNamed(re, "yb", "word"); got != "b" || !ok {
		t.Errorf(`FindNamed("yb","word") = (%q,%v), want ("b",true) — must read the participating branch, not SubexpIndex's first`, got, ok)
	}
	if got, ok := rx.FindNamed(re, "xa", "word"); got != "a" || !ok {
		t.Errorf(`FindNamed("xa","word") = (%q,%v), want ("a",true)`, got, ok)
	}
	if got, ok := rx.FindNamed(re, "zz", "word"); ok {
		t.Errorf(`FindNamed("zz","word") = (%q,%v), want ("",false) on no match`, got, ok)
	}
	if got, ok := rx.FindNamed(re, "yb", "missing"); ok {
		t.Errorf(`FindNamed("yb","missing") = (%q,%v), want ("",false) for an undeclared group`, got, ok)
	}
}

func TestFindAllNamed_duplicateNames(t *testing.T) {
	re := regexp.MustCompile(`(?:x(?P<word>a)|y(?P<word>b))`)
	got := rx.FindAllNamed(re, "xa yb", "word")
	want := []string{"a", "b"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("FindAllNamed(%q) = %q, want %q (each match reads its participating branch)", "xa yb", got, want)
	}
}

// NamedGroups still surfaces a declared-but-non-participating group as "" —
// only the Unmarshal path omits it (the includeNonParticipating split).
func TestNamedGroups_nonParticipatingStillPresent(t *testing.T) {
	re := regexp.MustCompile(`(?:x(?P<a>1)|y(?P<b>2))`)
	got := rx.NamedGroups(re, "y2")
	if v, ok := got["a"]; !ok || v != "" {
		t.Errorf(`NamedGroups("y2")["a"] = (%q,%v), want ("",true) — declared but did not participate`, v, ok)
	}
	if got["b"] != "2" {
		t.Errorf(`NamedGroups("y2")["b"] = %q, want "2"`, got["b"])
	}
}
