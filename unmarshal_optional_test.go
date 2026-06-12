package regextra

import (
	"regexp"
	"testing"
)

// Regression tests for https://github.com/Jecoms/regextra/issues/104:
// an optional named group that did not participate in the match must leave
// the field unchanged (matching Decoder behavior) instead of feeding "" to
// the type converter and erroring.

func TestUnmarshal_optionalGroupNotParticipating(t *testing.T) {
	re := regexp.MustCompile(`(?P<name>\w+)(?: (?P<age>\d+))?`)

	type person struct {
		Name string `regex:"name"`
		Age  int    `regex:"age"`
	}

	t.Run("absent optional group leaves field unchanged", func(t *testing.T) {
		var p person
		if err := Unmarshal(re, "Alice", &p); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if p.Name != "Alice" || p.Age != 0 {
			t.Errorf("got %+v, want {Name:Alice Age:0}", p)
		}
	})

	t.Run("participating optional group still decodes", func(t *testing.T) {
		var p person
		if err := Unmarshal(re, "Alice 30", &p); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if p.Name != "Alice" || p.Age != 30 {
			t.Errorf("got %+v, want {Name:Alice Age:30}", p)
		}
	})

	t.Run("default substitutes for absent optional group", func(t *testing.T) {
		type withDefault struct {
			Name string `regex:"name"`
			Age  int    `regex:"age,default=18"`
		}
		var p withDefault
		if err := Unmarshal(re, "Alice", &p); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if p.Age != 18 {
			t.Errorf("Age = %d, want default 18", p.Age)
		}
	})

	t.Run("participating empty-span group leaves field unchanged", func(t *testing.T) {
		// `age` participates in the match here but with a zero-length span —
		// distinct from not participating at all, yet the contract is the
		// same: no default, no write, no conversion error.
		emptySpanRe := regexp.MustCompile(`(?P<name>\w+):(?P<age>\d*)`)
		p := person{Age: 99}
		if err := Unmarshal(emptySpanRe, "Alice:", &p); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if p.Name != "Alice" || p.Age != 99 {
			t.Errorf("got %+v, want {Name:Alice Age:99}", p)
		}
	})

	t.Run("pre-populated field survives absent group", func(t *testing.T) {
		p := person{Age: 99}
		if err := Unmarshal(re, "Alice", &p); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if p.Age != 99 {
			t.Errorf("Age = %d, want 99 (unchanged)", p.Age)
		}
	})

	t.Run("UnmarshalAll skips absent optional groups per match", func(t *testing.T) {
		var people []person
		if err := UnmarshalAll(re, "Alice 30, Bob", &people); err != nil {
			t.Fatalf("UnmarshalAll: %v", err)
		}
		if len(people) != 2 {
			t.Fatalf("got %d matches, want 2", len(people))
		}
		if people[0].Age != 30 || people[1].Age != 0 {
			t.Errorf("got %+v", people)
		}
	})
}

// Unmarshal and Decoder.One must agree on optional-group semantics.
func TestUnmarshal_decoderParityOnOptionalGroups(t *testing.T) {
	const pattern = `(?P<name>\w+)(?: (?P<age>\d+))?`
	type person struct {
		Name string `regex:"name"`
		Age  int    `regex:"age"`
	}

	re := regexp.MustCompile(pattern)
	dec := MustCompile[person](pattern)

	for _, input := range []string{"Alice", "Alice 30"} {
		var fromUnmarshal person
		if err := Unmarshal(re, input, &fromUnmarshal); err != nil {
			t.Fatalf("Unmarshal(%q): %v", input, err)
		}
		fromDecoder, err := dec.One(input)
		if err != nil {
			t.Fatalf("Decoder.One(%q): %v", input, err)
		}
		if fromUnmarshal != fromDecoder {
			t.Errorf("input %q: Unmarshal=%+v Decoder.One=%+v — paths disagree", input, fromUnmarshal, fromDecoder)
		}
	}
}
