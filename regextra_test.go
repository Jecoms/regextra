package regextra

import (
	"fmt"
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
			name:      "found second group",
			pattern:   `(?P<first>one) (?P<second>two) (?P<second>again) three`,
			target:    "one two again three",
			groupName: "second",
			want:      "two",
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
			got, found := FindNamed(re, tt.target, tt.groupName)
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
			if got := NamedGroups(re, tt.target); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NamedGroups() = %v, want %v", got, tt.want)
			}
		})
	}
}

func ExampleFindNamed() {
	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
	name, ok := FindNamed(re, "Alice 30", "name")
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.pattern)
			got := FindAllNamed(re, tt.target, tt.group)
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
	words := FindAllNamed(re, "alpha beta gamma", "word")
	fmt.Println(words)
	// Output: [alpha beta gamma]
}

func ExampleNamedGroups() {
	re := regexp.MustCompile(`(?P<year>\d{4})-(?P<month>\d{2})-(?P<day>\d{2})`)
	groups := NamedGroups(re, "Date: 2025-10-04")

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
			got := AllNamedGroups(re, tt.target)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AllNamedGroups() = %v, want %v", got, tt.want)
			}
		})
	}
}

func ExampleAllNamedGroups() {
	re := regexp.MustCompile(`(?P<word>\w+) (?P<word>\w+) (?P<word>\w+)`)
	allGroups := AllNamedGroups(re, "one two three")

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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.pattern)
			got := Replace(re, tt.target, tt.repl)
			if got != tt.want {
				t.Errorf("Replace = %q, want %q", got, tt.want)
			}
		})
	}
}

func ExampleReplace() {
	re := regexp.MustCompile(`(?P<user>\w+)@(?P<domain>[\w.]+)`)
	out := Replace(re, "alice@example.com bob@other.org", map[string]string{
		"domain": "redacted",
	})
	fmt.Println(out)
	// Output: alice@redacted bob@redacted
}

func TestValidate(t *testing.T) {
	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)

	t.Run("all declared returns nil", func(t *testing.T) {
		if err := Validate(re, "name", "age"); err != nil {
			t.Errorf("Validate returned %v, want nil", err)
		}
	})

	t.Run("subset of declared returns nil", func(t *testing.T) {
		if err := Validate(re, "name"); err != nil {
			t.Errorf("Validate returned %v, want nil", err)
		}
	})

	t.Run("empty required returns nil", func(t *testing.T) {
		if err := Validate(re); err != nil {
			t.Errorf("Validate returned %v, want nil", err)
		}
	})

	t.Run("single missing reports it", func(t *testing.T) {
		err := Validate(re, "name", "ssn")
		if err == nil {
			t.Fatal("Validate returned nil, want error")
		}
		want := "regextra.Validate: missing named groups: ssn"
		if err.Error() != want {
			t.Errorf("Validate error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("multiple missing preserves request order", func(t *testing.T) {
		err := Validate(re, "ssn", "age", "email", "name", "phone")
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
		err := Validate(bare, "name")
		if err == nil {
			t.Fatal("Validate returned nil, want error")
		}
	})
}

// status is a custom type whose pointer satisfies RegexUnmarshaler;
// used by TestUnmarshalRegexUnmarshaler.
func ExampleValidate() {
	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
	if err := Validate(re, "name", "age", "ssn"); err != nil {
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
		got, ok := FindNamed(re, target, "word")
		if !ok && got != "" {
			t.Fatalf("FindNamed contract violated: ok=false but got=%q (want empty)", got)
		}
		// Group name that doesn't exist must always return ("", false).
		miss, missOk := FindNamed(re, target, "definitelyNotThere")
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
		groups := NamedGroups(re, target)
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
