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
