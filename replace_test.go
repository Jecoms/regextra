package regextra_test

import (
	"fmt"
	rx "github.com/jecoms/regextra/v2"
	"regexp"
	"strings"
	"testing"
)

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
		{
			// A named group that does not participate in the match (the
			// optional group is absent) is silently skipped; the rest of the
			// match still substitutes.
			name:    "non-participating optional group is skipped",
			pattern: `(?P<num>\d+)(?P<sign>%)?`,
			target:  "value 50 end",
			repl:    map[string]string{"num": "N", "sign": "S"},
			want:    "value N end",
		},
		{
			// Unnamed groups mixed with named groups: only the named group is
			// substituted; the unnamed group's text passes through untouched.
			name:    "unnamed group mixed with named is left untouched",
			pattern: `(\w+)=(?P<val>\d+)`,
			target:  "a=1 b=2",
			repl:    map[string]string{"val": "?"},
			want:    "a=? b=?",
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

func TestReplaceFirst(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		target  string
		repl    map[string]string
		want    string
	}{
		{
			name:    "first match substituted, later matches untouched",
			pattern: `(?P<user>\w+)@(?P<domain>[\w.]+)`,
			target:  "alice@example.com bob@other.org",
			repl:    map[string]string{"domain": "redacted"},
			want:    "alice@redacted bob@other.org",
		},
		{
			name:    "single match behaves like Replace",
			pattern: `(?P<user>\w+)@(?P<domain>[\w.]+)`,
			target:  "alice@example.com",
			repl:    map[string]string{"user": "bob", "domain": "redacted"},
			want:    "bob@redacted",
		},
		{
			name:    "text before and after the first match passes through",
			pattern: `(?P<num>\d+)`,
			target:  "a 1 b 2 c 3 d",
			repl:    map[string]string{"num": "*"},
			want:    "a * b 2 c 3 d",
		},
		{
			name:    "key not in map passes through, only first match considered",
			pattern: `(?P<key>\w+)=(?P<val>\d+)`,
			target:  "a=1 b=2",
			repl:    map[string]string{"val": "?"},
			want:    "a=? b=2",
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
			// Overlap/nesting within the first match follows Replace's rule: the
			// outermost span (same start, larger end) wins; the inner group inside
			// the already-replaced span is not substituted. Only the first match is
			// processed.
			name:    "nested groups within first match, outermost wins",
			pattern: `(?P<outer>(?P<inner>\w+)@[\w.]+)`,
			target:  "alice@example.com bob@other.org",
			repl:    map[string]string{"outer": "REDACTED", "inner": "X"},
			want:    "REDACTED bob@other.org",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.pattern)
			got := rx.ReplaceFirst(re, tt.target, tt.repl)
			if got != tt.want {
				t.Errorf("ReplaceFirst = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestReplaceFirstRemainderByteIdentical asserts that everything from the end of
// the first match onward is copied through byte-for-byte, not re-scanned.
func TestReplaceFirstRemainderByteIdentical(t *testing.T) {
	re := regexp.MustCompile(`(?P<word>\w+)`)
	target := "alpha beta gamma delta"
	got := rx.ReplaceFirst(re, target, map[string]string{"word": "X"})
	const want = "X beta gamma delta"
	if got != want {
		t.Fatalf("ReplaceFirst = %q, want %q", got, want)
	}
	// The tail after the first match must equal the original target's tail.
	firstLoc := re.FindStringIndex(target)
	if tail := got[len("X"):]; tail != target[firstLoc[1]:] {
		t.Errorf("tail after first match = %q, want %q", tail, target[firstLoc[1]:])
	}
}

func ExampleReplaceFirst() {
	re := regexp.MustCompile(`(?P<user>\w+)@(?P<domain>[\w.]+)`)
	out := rx.ReplaceFirst(re, "alice@example.com bob@other.org", map[string]string{
		"domain": "redacted",
	})
	fmt.Println(out)
	// Output: alice@redacted bob@other.org
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

func TestReplaceFuncFirst(t *testing.T) {
	upper := func(_, m string) string { return strings.ToUpper(m) }

	tests := []struct {
		name    string
		pattern string
		target  string
		fn      func(group, match string) string
		want    string
	}{
		{
			name:    "first match substituted, later matches untouched",
			pattern: `(?P<card>\d{12,19})`,
			target:  "4111111111111111 then 4242424242424242",
			fn: func(_, m string) string {
				return strings.Repeat("*", len(m)-4) + m[len(m)-4:]
			},
			want: "************1111 then 4242424242424242",
		},
		{
			name:    "single match behaves like ReplaceFunc",
			pattern: `(?P<user>\w+)@(?P<domain>[\w.]+)`,
			target:  "alice@example.com",
			fn:      upper,
			want:    "ALICE@EXAMPLE.COM",
		},
		{
			name:    "text before and after the first match passes through",
			pattern: `(?P<num>\d+)`,
			target:  "a 1 b 2 c 3 d",
			fn:      func(_, m string) string { return "<" + m + ">" },
			want:    "a <1> b 2 c 3 d",
		},
		{
			name:    "fn receives the group name",
			pattern: `(?P<key>\w+)=(?P<val>\d+)`,
			target:  "a=1 b=2",
			fn:      func(group, m string) string { return group + ":" + m },
			want:    "key:a=val:1 b=2",
		},
		{
			name:    "return match verbatim leaves the group unchanged",
			pattern: `(?P<user>\w+)@(?P<domain>[\w.]+)`,
			target:  "alice@example.com bob@other.org",
			fn: func(group, m string) string {
				if group == "domain" {
					return "redacted"
				}
				return m // leave user unchanged
			},
			want: "alice@redacted bob@other.org",
		},
		{
			name:    "no match returns target unchanged",
			pattern: `(?P<word>[A-Z]+)`,
			target:  "no matches here",
			fn:      upper,
			want:    "no matches here",
		},
		{
			// Overlap/nesting within the first match follows ReplaceFunc's rule:
			// the outermost span (same start, larger end) wins and the inner group
			// inside the already-substituted span never reaches fn. Only the first
			// match is processed.
			name:    "nested groups within first match, outermost wins",
			pattern: `(?P<outer>(?P<inner>\w+)@[\w.]+)`,
			target:  "alice@example.com bob@other.org",
			fn:      upper,
			want:    "ALICE@EXAMPLE.COM bob@other.org",
		},
		{
			// Non-participating optional group is skipped, so fn never sees it.
			name:    "optional non-participating group skipped",
			pattern: `(?P<word>\w+)(?P<bang>!)?`,
			target:  "hi there",
			fn:      func(_, m string) string { return "[" + m + "]" },
			want:    "[hi] there",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.pattern)
			got := rx.ReplaceFuncFirst(re, tt.target, tt.fn)
			if got != tt.want {
				t.Errorf("ReplaceFuncFirst = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestReplaceFuncFirstRemainderByteIdentical asserts that everything from the
// end of the first match onward is copied through byte-for-byte, not re-scanned.
func TestReplaceFuncFirstRemainderByteIdentical(t *testing.T) {
	re := regexp.MustCompile(`(?P<word>\w+)`)
	target := "alpha beta gamma delta"
	got := rx.ReplaceFuncFirst(re, target, func(_, _ string) string { return "X" })
	const want = "X beta gamma delta"
	if got != want {
		t.Fatalf("ReplaceFuncFirst = %q, want %q", got, want)
	}
	// The tail after the first match must equal the original target's tail.
	firstLoc := re.FindStringIndex(target)
	if tail := got[len("X"):]; tail != target[firstLoc[1]:] {
		t.Errorf("tail after first match = %q, want %q", tail, target[firstLoc[1]:])
	}
}

func TestReplaceFuncFirstInnerGroupNotInvoked(t *testing.T) {
	// fn must never be called for an inner group suppressed by an outermost
	// span — assert the callback is not invoked for the "inner" name.
	re := regexp.MustCompile(`(?P<outer>(?P<inner>\w+)@[\w.]+)`)
	var seen []string
	out := rx.ReplaceFuncFirst(re, "alice@example.com bob@other.org", func(group, m string) string {
		seen = append(seen, group)
		return strings.ToUpper(m)
	})
	if out != "ALICE@EXAMPLE.COM bob@other.org" {
		t.Errorf("ReplaceFuncFirst = %q, want %q", out, "ALICE@EXAMPLE.COM bob@other.org")
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

func TestReplaceFuncFirstNoMatchNeverCallsFn(t *testing.T) {
	re := regexp.MustCompile(`(?P<word>[A-Z]+)`)
	called := false
	out := rx.ReplaceFuncFirst(re, "no matches here", func(_, m string) string {
		called = true
		return m
	})
	if called {
		t.Error("fn was called despite no match")
	}
	if out != "no matches here" {
		t.Errorf("ReplaceFuncFirst = %q, want target unchanged", out)
	}
}

func TestReplaceFuncFirstNilFn(t *testing.T) {
	// A nil fn is a programmer error. It panics on the first substituted match,
	// mirroring regexp.Regexp.ReplaceAllStringFunc, but never panics when there
	// is nothing to substitute (no match returns target before fn is reached).
	t.Run("panics on first match", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("ReplaceFuncFirst with nil fn did not panic on a match")
			}
		}()
		re := regexp.MustCompile(`(?P<word>[A-Z]+)`)
		rx.ReplaceFuncFirst(re, "HELLO", nil)
	})

	t.Run("no panic on no match", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ReplaceFuncFirst with nil fn panicked on no match: %v", r)
			}
		}()
		re := regexp.MustCompile(`(?P<word>[A-Z]+)`)
		if out := rx.ReplaceFuncFirst(re, "no matches here", nil); out != "no matches here" {
			t.Errorf("ReplaceFuncFirst = %q, want target unchanged", out)
		}
	})
}

func ExampleReplaceFuncFirst() {
	// Mask only the first captured card number; later matches pass through.
	re := regexp.MustCompile(`(?P<card>\d{12,19})`)
	out := rx.ReplaceFuncFirst(re, "4111111111111111 then 4242424242424242", func(_, match string) string {
		return strings.Repeat("*", len(match)-4) + match[len(match)-4:]
	})
	fmt.Println(out)
	// Output: ************1111 then 4242424242424242
}
