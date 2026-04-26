package regextra

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
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
type status int

const (
	statusUnknown status = iota
	statusOpen
	statusClosed
)

func (s *status) UnmarshalRegex(value string) error {
	switch value {
	case "open":
		*s = statusOpen
	case "closed":
		*s = statusClosed
	default:
		return fmt.Errorf("unknown status %q", value)
	}
	return nil
}

// valueReceiver is here to confirm the value-receiver implements path is hit.
// Note: a value receiver can't actually mutate the field, so this is only
// useful for read-only side effects in tests.
type valueReceiver struct{}

func (v valueReceiver) UnmarshalRegex(value string) error {
	if value == "fail" {
		return fmt.Errorf("valueReceiver rejected: %s", value)
	}
	return nil
}

func TestUnmarshalRegexUnmarshaler(t *testing.T) {
	t.Run("pointer-receiver custom type populates field", func(t *testing.T) {
		type Issue struct {
			ID    int    `regex:"id"`
			State status `regex:"state"`
		}
		re := regexp.MustCompile(`#(?P<id>\d+) \[(?P<state>\w+)\]`)
		var issue Issue
		if err := Unmarshal(re, "#42 [open]", &issue); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		if issue.ID != 42 {
			t.Errorf("ID = %d, want 42", issue.ID)
		}
		if issue.State != statusOpen {
			t.Errorf("State = %d, want %d (statusOpen)", issue.State, statusOpen)
		}
	})

	t.Run("RegexUnmarshaler error propagates", func(t *testing.T) {
		type Issue struct {
			State status `regex:"state"`
		}
		re := regexp.MustCompile(`\[(?P<state>\w+)\]`)
		var issue Issue
		err := Unmarshal(re, "[bogus]", &issue)
		if err == nil {
			t.Fatal("Unmarshal returned nil, want error")
		}
		if !strings.Contains(err.Error(), `unknown status "bogus"`) {
			t.Errorf("error = %q, want it to mention 'unknown status \"bogus\"'", err.Error())
		}
	})

	t.Run("RegexUnmarshaler intercepts before built-in int conversion", func(t *testing.T) {
		// status's underlying type is int, so without the interface check
		// setFieldValue would try strconv.ParseInt on "open" and fail.
		type Issue struct {
			State status `regex:"state"`
		}
		re := regexp.MustCompile(`\[(?P<state>\w+)\]`)
		var issue Issue
		if err := Unmarshal(re, "[open]", &issue); err != nil {
			t.Fatalf("Unmarshal returned %v — interface check was not hit before int conversion", err)
		}
	})

	t.Run("value-receiver implementation is consulted", func(t *testing.T) {
		type Holder struct {
			V valueReceiver `regex:"v"`
		}
		re := regexp.MustCompile(`(?P<v>\w+)`)
		var h Holder
		if err := Unmarshal(re, "ok", &h); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
	})
}

func TestUnmarshalTimeTypes(t *testing.T) {
	t.Run("time.Time RFC3339", func(t *testing.T) {
		type Event struct {
			TS time.Time `regex:"ts"`
		}
		re := regexp.MustCompile(`(?P<ts>\S+)`)
		var ev Event
		if err := Unmarshal(re, "2026-04-26T12:34:56Z", &ev); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		want, _ := time.Parse(time.RFC3339, "2026-04-26T12:34:56Z")
		if !ev.TS.Equal(want) {
			t.Errorf("TS = %v, want %v", ev.TS, want)
		}
	})

	t.Run("time.Time DateTime fallback", func(t *testing.T) {
		type Event struct {
			TS time.Time `regex:"ts"`
		}
		re := regexp.MustCompile(`(?P<ts>.+)`)
		var ev Event
		if err := Unmarshal(re, "2026-04-26 12:34:56", &ev); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		if ev.TS.Year() != 2026 || ev.TS.Month() != time.April || ev.TS.Day() != 26 {
			t.Errorf("TS = %v, want 2026-04-26 ...", ev.TS)
		}
	})

	t.Run("time.Time DateOnly fallback", func(t *testing.T) {
		type Event struct {
			TS time.Time `regex:"ts"`
		}
		re := regexp.MustCompile(`(?P<ts>\S+)`)
		var ev Event
		if err := Unmarshal(re, "2026-04-26", &ev); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		if ev.TS.Year() != 2026 {
			t.Errorf("Year = %d, want 2026", ev.TS.Year())
		}
	})

	t.Run("time.Time unparseable returns error", func(t *testing.T) {
		type Event struct {
			TS time.Time `regex:"ts"`
		}
		re := regexp.MustCompile(`(?P<ts>.+)`)
		var ev Event
		err := Unmarshal(re, "definitely not a time", &ev)
		if err == nil {
			t.Fatal("Unmarshal returned nil, want error")
		}
		if !strings.Contains(err.Error(), "cannot convert") || !strings.Contains(err.Error(), "time.Time") {
			t.Errorf("error = %q, want it to mention time.Time conversion failure", err.Error())
		}
	})

	t.Run("time.Duration", func(t *testing.T) {
		type Span struct {
			D time.Duration `regex:"d"`
		}
		re := regexp.MustCompile(`(?P<d>\S+)`)
		var sp Span
		if err := Unmarshal(re, "1h30m", &sp); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		want := 90 * time.Minute
		if sp.D != want {
			t.Errorf("D = %v, want %v", sp.D, want)
		}
	})

	t.Run("time.Duration unparseable returns error", func(t *testing.T) {
		type Span struct {
			D time.Duration `regex:"d"`
		}
		re := regexp.MustCompile(`(?P<d>\S+)`)
		var sp Span
		err := Unmarshal(re, "notaduration", &sp)
		if err == nil {
			t.Fatal("Unmarshal returned nil, want error")
		}
		if !strings.Contains(err.Error(), "cannot convert") || !strings.Contains(err.Error(), "time.Duration") {
			t.Errorf("error = %q, want it to mention time.Duration conversion failure", err.Error())
		}
	})

	t.Run("time.Duration is matched by Type, not by Int64 kind", func(t *testing.T) {
		// time.Duration's underlying type is int64. Without the type-based
		// match before the kind switch, "1h30m" would fall into reflect.Int64
		// and strconv.ParseInt would reject it.
		type Span struct {
			D time.Duration `regex:"d"`
		}
		re := regexp.MustCompile(`(?P<d>\S+)`)
		var sp Span
		if err := Unmarshal(re, "5s", &sp); err != nil {
			t.Fatalf("Unmarshal returned %v — Type-based match did not pre-empt Int64 kind", err)
		}
	})
}

func ExampleValidate() {
	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
	if err := Validate(re, "name", "age", "ssn"); err != nil {
		fmt.Println(err)
	}
	// Output: regextra.Validate: missing named groups: ssn
}

func ExampleRegexUnmarshaler() {
	type Severity int
	const (
		_ Severity = iota
		Low
		Medium
		High
	)
	// In real code this would be a type defined in the same package as
	// the call to Unmarshal, with `func (s *Severity) UnmarshalRegex(...) error`.
	// Compile-time check elided here for example brevity.
	_ = Low
	_ = Medium
	_ = High
	fmt.Println("see TestUnmarshalRegexUnmarshaler for a runnable demo")
	// Output: see TestUnmarshalRegexUnmarshaler for a runnable demo
}

func ExampleUnmarshal_timeTypes() {
	type Event struct {
		Started time.Time     `regex:"start"`
		Took    time.Duration `regex:"took"`
	}
	re := regexp.MustCompile(`(?P<start>\S+)\s+\((?P<took>\S+)\)`)
	var ev Event
	_ = Unmarshal(re, "2026-04-26T12:34:56Z (1h30m)", &ev)
	fmt.Printf("%s for %s\n", ev.Started.Format(time.RFC3339), ev.Took)
	// Output: 2026-04-26T12:34:56Z for 1h30m0s
}

func TestUnmarshal(t *testing.T) {
	t.Run("basic string fields", func(t *testing.T) {
		type Person struct {
			Name string
			Age  string
		}
		re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
		var person Person
		err := Unmarshal(re, "Alice is 30", &person)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if person.Name != "Alice" {
			t.Errorf("Name = %q, want %q", person.Name, "Alice")
		}
		if person.Age != "30" {
			t.Errorf("Age = %q, want %q", person.Age, "30")
		}
	})

	t.Run("int type conversion", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}
		re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
		var person Person
		err := Unmarshal(re, "Bob is 25", &person)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if person.Name != "Bob" {
			t.Errorf("Name = %q, want %q", person.Name, "Bob")
		}
		if person.Age != 25 {
			t.Errorf("Age = %d, want %d", person.Age, 25)
		}
	})

	t.Run("float type conversion", func(t *testing.T) {
		type Product struct {
			Name  string
			Price float64
		}
		re := regexp.MustCompile(`(?P<name>\w+) costs \$(?P<price>[\d.]+)`)
		var product Product
		err := Unmarshal(re, "Widget costs $19.99", &product)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if product.Name != "Widget" {
			t.Errorf("Name = %q, want %q", product.Name, "Widget")
		}
		if product.Price != 19.99 {
			t.Errorf("Price = %f, want %f", product.Price, 19.99)
		}
	})

	t.Run("struct tags for custom mapping", func(t *testing.T) {
		type Email struct {
			Username string `regex:"user"`
			Domain   string `regex:"domain"`
		}
		re := regexp.MustCompile(`(?P<user>\w+)@(?P<domain>[\w.]+)`)
		var email Email
		err := Unmarshal(re, "alice@example.com", &email)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if email.Username != "alice" {
			t.Errorf("Username = %q, want %q", email.Username, "alice")
		}
		if email.Domain != "example.com" {
			t.Errorf("Domain = %q, want %q", email.Domain, "example.com")
		}
	})

	t.Run("case insensitive field matching", func(t *testing.T) {
		type Data struct {
			UserName string
			Age      int
		}
		re := regexp.MustCompile(`(?P<username>\w+) (?P<age>\d+)`)
		var data Data
		err := Unmarshal(re, "john 42", &data)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if data.UserName != "john" {
			t.Errorf("UserName = %q, want %q", data.UserName, "john")
		}
		if data.Age != 42 {
			t.Errorf("Age = %d, want %d", data.Age, 42)
		}
	})

	t.Run("no match returns no error", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>[a-z]+)`) // Only lowercase letters
		var person Person
		err := Unmarshal(re, "123", &person)
		if err != nil {
			t.Errorf("Unmarshal() error = %v, want nil", err)
		}
		if person.Name != "" {
			t.Errorf("Name = %q, want empty string", person.Name)
		}
	})

	t.Run("error on non-pointer", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var person Person
		err := Unmarshal(re, "Alice", person) // Not a pointer
		if err == nil {
			t.Error("Unmarshal() expected error for non-pointer, got nil")
		}
	})

	t.Run("error on nil pointer", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var person *Person
		err := Unmarshal(re, "Alice", person) // Nil pointer
		if err == nil {
			t.Error("Unmarshal() expected error for nil pointer, got nil")
		}
	})

	t.Run("error on pointer to non-struct", func(t *testing.T) {
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var name string
		err := Unmarshal(re, "Alice", &name) // Pointer to string, not struct
		if err == nil {
			t.Error("Unmarshal() expected error for pointer to non-struct, got nil")
		}
	})

	t.Run("bool type conversion", func(t *testing.T) {
		type Config struct {
			Enabled string
			Debug   bool
		}
		re := regexp.MustCompile(`enabled=(?P<enabled>\w+) debug=(?P<debug>\w+)`)
		var config Config
		err := Unmarshal(re, "enabled=yes debug=true", &config)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if config.Enabled != "yes" {
			t.Errorf("Enabled = %q, want %q", config.Enabled, "yes")
		}
		if config.Debug != true {
			t.Errorf("Debug = %v, want %v", config.Debug, true)
		}
	})

	t.Run("skip unexported fields", func(t *testing.T) {
		type Data struct {
			Public  string
			private string
		}
		re := regexp.MustCompile(`(?P<public>\w+) (?P<private>\w+)`)
		var data Data
		err := Unmarshal(re, "hello world", &data)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if data.Public != "hello" {
			t.Errorf("Public = %q, want %q", data.Public, "hello")
		}
		if data.private != "" {
			t.Errorf("private should be empty, got %q", data.private)
		}
	})

	t.Run("struct tag takes precedence over field name", func(t *testing.T) {
		type Data struct {
			// Field name is "Value" but tag maps to "count"
			Value string `regex:"count"`
		}
		// Pattern has both "value" and "count" groups
		re := regexp.MustCompile(`value=(?P<value>\w+) count=(?P<count>\d+)`)
		var data Data
		err := Unmarshal(re, "value=hello count=42", &data)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		// Should get "42" from "count" group (via tag), not "hello" from "value" group (field name)
		if data.Value != "42" {
			t.Errorf("Value = %q, want %q (should use tag mapping)", data.Value, "42")
		}
	})
}

func ExampleUnmarshal() {
	type Person struct {
		Name string
		Age  int
	}

	re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+) years old`)
	var person Person
	err := Unmarshal(re, "Alice is 30 years old", &person)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("%s is %d\n", person.Name, person.Age)
	// Output: Alice is 30
}

func ExampleUnmarshal_structTags() {
	type Email struct {
		Username string `regex:"user"`
		Domain   string `regex:"domain"`
	}

	re := regexp.MustCompile(`(?P<user>\w+)@(?P<domain>[\w.]+)`)
	var email Email
	err := Unmarshal(re, "alice@example.com", &email)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("User: %s, Domain: %s\n", email.Username, email.Domain)
	// Output: User: alice, Domain: example.com
}

func TestUnmarshalAll(t *testing.T) {
	t.Run("multiple matches", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}
		re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
		var people []Person
		err := UnmarshalAll(re, "Alice is 30 and Bob is 25", &people)
		if err != nil {
			t.Fatalf("UnmarshalAll() error = %v", err)
		}
		if len(people) != 2 {
			t.Fatalf("len(people) = %d, want 2", len(people))
		}
		if people[0].Name != "Alice" || people[0].Age != 30 {
			t.Errorf("people[0] = %+v, want {Name:Alice Age:30}", people[0])
		}
		if people[1].Name != "Bob" || people[1].Age != 25 {
			t.Errorf("people[1] = %+v, want {Name:Bob Age:25}", people[1])
		}
	})

	t.Run("no matches clears slice", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>[a-z]+)`)
		people := []Person{{Name: "existing"}}
		err := UnmarshalAll(re, "123", &people)
		if err != nil {
			t.Fatalf("UnmarshalAll() error = %v", err)
		}
		if len(people) != 0 {
			t.Errorf("len(people) = %d, want 0 (should clear slice)", len(people))
		}
	})

	t.Run("single match", func(t *testing.T) {
		type Email struct {
			User   string `regex:"user"`
			Domain string `regex:"domain"`
		}
		re := regexp.MustCompile(`(?P<user>\w+)@(?P<domain>[\w.]+)`)
		var emails []Email
		err := UnmarshalAll(re, "Contact: alice@example.com", &emails)
		if err != nil {
			t.Fatalf("UnmarshalAll() error = %v", err)
		}
		if len(emails) != 1 {
			t.Fatalf("len(emails) = %d, want 1", len(emails))
		}
		if emails[0].User != "alice" || emails[0].Domain != "example.com" {
			t.Errorf("emails[0] = %+v, want {User:alice Domain:example.com}", emails[0])
		}
	})

	t.Run("three matches with different types", func(t *testing.T) {
		type Product struct {
			Name  string
			Price float64
		}
		re := regexp.MustCompile(`(?P<name>\w+):\$(?P<price>[\d.]+)`)
		var products []Product
		err := UnmarshalAll(re, "Items: Apple:$1.50 Banana:$0.75 Orange:$2.00", &products)
		if err != nil {
			t.Fatalf("UnmarshalAll() error = %v", err)
		}
		if len(products) != 3 {
			t.Fatalf("len(products) = %d, want 3", len(products))
		}
		want := []Product{
			{Name: "Apple", Price: 1.50},
			{Name: "Banana", Price: 0.75},
			{Name: "Orange", Price: 2.00},
		}
		if !reflect.DeepEqual(products, want) {
			t.Errorf("products = %+v, want %+v", products, want)
		}
	})

	t.Run("error on non-pointer", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var people []Person
		err := UnmarshalAll(re, "Alice", people) // Not a pointer
		if err == nil {
			t.Error("UnmarshalAll() expected error for non-pointer, got nil")
		}
	})

	t.Run("error on nil pointer", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var people *[]Person
		err := UnmarshalAll(re, "Alice", people) // Nil pointer
		if err == nil {
			t.Error("UnmarshalAll() expected error for nil pointer, got nil")
		}
	})

	t.Run("error on pointer to non-slice", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var person Person
		err := UnmarshalAll(re, "Alice", &person) // Pointer to struct, not slice
		if err == nil {
			t.Error("UnmarshalAll() expected error for pointer to non-slice, got nil")
		}
	})

	t.Run("error on slice of non-structs", func(t *testing.T) {
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var names []string
		err := UnmarshalAll(re, "Alice", &names) // Slice of strings, not structs
		if err == nil {
			t.Error("UnmarshalAll() expected error for slice of non-structs, got nil")
		}
	})
}

func ExampleUnmarshalAll() {
	type Person struct {
		Name string
		Age  int
	}

	re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
	var people []Person
	err := UnmarshalAll(re, "Alice is 30 and Bob is 25", &people)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	for _, person := range people {
		fmt.Printf("%s: %d\n", person.Name, person.Age)
	}
	// Output: Alice: 30
	// Bob: 25
}
