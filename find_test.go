package regextra_test

import (
	"fmt"
	rx "github.com/jecoms/regextra/v2"
	"reflect"
	"regexp"
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

func TestNamedGroupOccurrences(t *testing.T) {
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
			got := rx.NamedGroupOccurrences(re, tt.target)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NamedGroupOccurrences() = %v, want %v", got, tt.want)
			}
		})
	}
}

func ExampleNamedGroupOccurrences() {
	re := regexp.MustCompile(`(?P<word>\w+) (?P<word>\w+) (?P<word>\w+)`)
	occurrences := rx.NamedGroupOccurrences(re, "one two three")

	fmt.Printf("word: %v\n", occurrences["word"])
	// Output: word: [one two three]
}

func TestAllNamedGroups_deprecatedAlias(t *testing.T) {
	re := regexp.MustCompile(`(?:x(?P<word>a)|y(?P<word>b))?(?P<tail>\w*)`)
	for _, target := range []string{"xa", "yb", "plain", ""} {
		got := rx.AllNamedGroups(re, target) //nolint:staticcheck // deliberate coverage of the deprecated alias
		want := rx.NamedGroupOccurrences(re, target)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("AllNamedGroups(re, %q) = %v, want NamedGroupOccurrences result %v", target, got, want)
		}
	}
}

func TestNamedGroupsPerMatch(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		target  string
		want    []map[string]string
	}{
		{
			name:    "multiple matches, one map per match",
			pattern: `(?P<key>\w+)=(?P<value>\w+)`,
			target:  "a=1 b=2 c=3",
			want: []map[string]string{
				{"key": "a", "value": "1"},
				{"key": "b", "value": "2"},
				{"key": "c", "value": "3"},
			},
		},
		{
			name:    "single match",
			pattern: `(?P<name>\w+) (?P<age>\d+)`,
			target:  "Alice 30",
			want: []map[string]string{
				{"name": "Alice", "age": "30"},
			},
		},
		{
			name:    "no match returns empty non-nil slice",
			pattern: `(?P<word>[A-Z]+)`,
			target:  "all lowercase",
			want:    []map[string]string{},
		},
		{
			// Duplicate name in one match: last participating occurrence wins,
			// mirroring NamedGroups per-match semantics.
			name:    "duplicate group name, last participating wins per match",
			pattern: `(?P<word>\w+) (?P<word>\w+)`,
			target:  "hello world foo bar",
			want: []map[string]string{
				{"word": "world"},
				{"word": "bar"},
			},
		},
		{
			// Optional group that does not participate in a given match is still
			// present, mapped to "" (includeNonParticipating=true).
			name:    "non-participating optional group present as empty string",
			pattern: `(?P<a>\w+)(?P<b>!)?`,
			target:  "x! y",
			want: []map[string]string{
				{"a": "x", "b": "!"},
				{"a": "y", "b": ""},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.pattern)
			got := rx.NamedGroupsPerMatch(re, tt.target)
			if got == nil {
				t.Fatalf("NamedGroupsPerMatch() = nil, want non-nil")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NamedGroupsPerMatch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNamedGroupsPerMatchSeq(t *testing.T) {
	t.Run("yields one map per match in order", func(t *testing.T) {
		re := regexp.MustCompile(`(?P<key>\w+)=(?P<value>\w+)`)
		want := []map[string]string{
			{"key": "a", "value": "1"},
			{"key": "b", "value": "2"},
		}
		var got []map[string]string
		for m := range rx.NamedGroupsPerMatchSeq(re, "a=1 b=2") {
			got = append(got, m)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("NamedGroupsPerMatchSeq() = %v, want %v", got, want)
		}
	})

	t.Run("no match yields zero times", func(t *testing.T) {
		re := regexp.MustCompile(`(?P<word>[A-Z]+)`)
		count := 0
		for range rx.NamedGroupsPerMatchSeq(re, "all lowercase") {
			count++
		}
		if count != 0 {
			t.Errorf("iterations = %d, want 0", count)
		}
	})

	t.Run("break stops iteration early", func(t *testing.T) {
		re := regexp.MustCompile(`(?P<word>\w+)`)
		var got []string
		for m := range rx.NamedGroupsPerMatchSeq(re, "alpha beta gamma") {
			got = append(got, m["word"])
			if len(got) == 2 {
				break
			}
		}
		want := []string{"alpha", "beta"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("after break, got = %v, want %v", got, want)
		}
	})

	t.Run("matches slice form", func(t *testing.T) {
		re := regexp.MustCompile(`(?P<a>\w+)(?P<b>!)?`)
		target := "x! y z!"
		var seq []map[string]string
		for m := range rx.NamedGroupsPerMatchSeq(re, target) {
			seq = append(seq, m)
		}
		if !reflect.DeepEqual(seq, rx.NamedGroupsPerMatch(re, target)) {
			t.Errorf("Seq form = %v, want parity with slice form %v", seq, rx.NamedGroupsPerMatch(re, target))
		}
	})
}

func ExampleNamedGroupsPerMatch() {
	re := regexp.MustCompile(`(?P<key>\w+)=(?P<value>\w+)`)
	all := rx.NamedGroupsPerMatch(re, "a=1 b=2")
	for _, m := range all {
		fmt.Printf("%s=%s\n", m["key"], m["value"])
	}
	// Output:
	// a=1
	// b=2
}

func ExampleNamedGroupsPerMatchSeq() {
	re := regexp.MustCompile(`(?P<key>\w+)=(?P<value>\w+)`)
	for m := range rx.NamedGroupsPerMatchSeq(re, "a=1 b=2") {
		fmt.Printf("%s=%s\n", m["key"], m["value"])
	}
	// Output:
	// a=1
	// b=2
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

	t.Run("NamedGroupOccurrences still preserves every occurrence", func(t *testing.T) {
		got := rx.NamedGroupOccurrences(re, "xa")
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

// More than four occurrences of one group name exercises the spill path of
// FindAllNamed's stack-backed occurrence buffer — behavior must be identical
// to the small-count case.
func TestFindAllNamed_manyDuplicateNamesSpill(t *testing.T) {
	re := regexp.MustCompile(`(?:v(?P<word>1)|w(?P<word>2)|x(?P<word>3)|y(?P<word>4)|z(?P<word>5))`)
	got := rx.FindAllNamed(re, "v1 w2 x3 y4 z5", "word")
	want := []string{"1", "2", "3", "4", "5"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FindAllNamed with 5 duplicate occurrences = %q, want %q", got, want)
	}
	if got := rx.FindAllNamed(re, "v1", "missing"); got != nil {
		t.Errorf("FindAllNamed undeclared group = %q, want nil", got)
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
