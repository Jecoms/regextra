package regextra_test

import (
	"regexp"
	"testing"

	rx "github.com/jecoms/regextra"
)

// Issue #106: `regex:"-"` must exclude a field entirely, even when a declared
// group shares the field's name (the trap the old name-fallback created). An
// absent tag (`regex:""`) must still fall back to the field name — the guard
// against over-skipping.

func TestUnmarshal_dashExcludesField(t *testing.T) {
	type Person struct {
		Name string `regex:"name"`
		Age  string `regex:"-"` // excluded, despite a declared "age" group
	}
	re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)

	var p Person
	if err := rx.Unmarshal(re, "Alice is 30", &p); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if p.Name != "Alice" {
		t.Errorf("Name = %q, want %q", p.Name, "Alice")
	}
	if p.Age != "" {
		t.Errorf("Age = %q, want it left at zero value (excluded by regex:\"-\")", p.Age)
	}
}

func TestUnmarshalAll_dashExcludesField(t *testing.T) {
	type Person struct {
		Name string `regex:"name"`
		Age  string `regex:"-"`
	}
	re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)

	var people []Person
	if err := rx.UnmarshalAll(re, "Alice is 30 Bob is 25", &people); err != nil {
		t.Fatalf("UnmarshalAll() error = %v", err)
	}
	if len(people) != 2 {
		t.Fatalf("got %d people, want 2", len(people))
	}
	for i, p := range people {
		if p.Age != "" {
			t.Errorf("people[%d].Age = %q, want it left at zero value", i, p.Age)
		}
	}
}

func TestUnmarshal_emptyTagStillFallsBackToFieldName(t *testing.T) {
	// Guard against over-skipping: an absent tag must still match by field name.
	type Person struct {
		Name string
		Age  string `regex:""`
	}
	re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)

	var p Person
	if err := rx.Unmarshal(re, "Alice is 30", &p); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if p.Age != "30" {
		t.Errorf("Age = %q, want %q (regex:\"\" should fall back to field name)", p.Age, "30")
	}
}

func TestDecoder_dashExcludesField(t *testing.T) {
	type Person struct {
		Name string `regex:"name"`
		Age  string `regex:"-"`
	}
	d, err := rx.Compile[Person](`(?P<name>\w+) is (?P<age>\d+)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	t.Run("One", func(t *testing.T) {
		p, err := d.One("Alice is 30")
		if err != nil {
			t.Fatalf("One() error = %v", err)
		}
		if p.Name != "Alice" {
			t.Errorf("Name = %q, want %q", p.Name, "Alice")
		}
		if p.Age != "" {
			t.Errorf("Age = %q, want it left at zero value (excluded by regex:\"-\")", p.Age)
		}
	})

	t.Run("All", func(t *testing.T) {
		ps, err := d.All("Alice is 30 Bob is 25")
		if err != nil {
			t.Fatalf("All() error = %v", err)
		}
		if len(ps) != 2 {
			t.Fatalf("got %d people, want 2", len(ps))
		}
		for i, p := range ps {
			if p.Age != "" {
				t.Errorf("ps[%d].Age = %q, want it left at zero value", i, p.Age)
			}
		}
	})
}

// A field tagged regex:"-" must not trigger Compile's undeclared-group error
// even when no group of that name exists — it is excluded before resolution.
func TestCompile_dashFieldNeedsNoGroup(t *testing.T) {
	type Person struct {
		Name     string `regex:"name"`
		Internal string `regex:"-"` // no "internal"/"Internal" group on the pattern
	}
	if _, err := rx.Compile[Person](`(?P<name>\w+)`); err != nil {
		t.Fatalf("Compile() error = %v, want nil (regex:\"-\" field should be excluded)", err)
	}
}
