package regextra

import (
	"regexp"
	"testing"
)

// Regression tests for https://github.com/Jecoms/regextra/issues/107:
// when nested named groups start at the same offset, the documented
// "outermost wins" rule must hold deterministically, not depend on
// sort internals.

func TestReplace_nestedSameStartOutermostWins(t *testing.T) {
	re := regexp.MustCompile(`(?P<outer>(?P<inner>x)y)`)

	t.Run("both groups in map: outer wins", func(t *testing.T) {
		got := Replace(re, "axyb", map[string]string{
			"outer": "OUTER",
			"inner": "INNER",
		})
		if got != "aOUTERb" {
			t.Errorf("got %q, want %q", got, "aOUTERb")
		}
	})

	t.Run("only inner in map: inner replaced", func(t *testing.T) {
		got := Replace(re, "axyb", map[string]string{"inner": "I"})
		if got != "aIyb" {
			t.Errorf("got %q, want %q", got, "aIyb")
		}
	})

	t.Run("only outer in map: outer replaced", func(t *testing.T) {
		got := Replace(re, "axyb", map[string]string{"outer": "O"})
		if got != "aOb" {
			t.Errorf("got %q, want %q", got, "aOb")
		}
	})

	// Three levels of same-start nesting; outermost must still win even when
	// the slice has more than two same-start spans (where an unstable sort's
	// behavior would be least predictable).
	t.Run("three nested levels", func(t *testing.T) {
		re3 := regexp.MustCompile(`(?P<a>(?P<b>(?P<c>x)y)z)`)
		got := Replace(re3, "-xyz-", map[string]string{
			"a": "A",
			"b": "B",
			"c": "C",
		})
		if got != "-A-" {
			t.Errorf("got %q, want %q", got, "-A-")
		}
	})
}

func TestReplace_nestedNotSameStart(t *testing.T) {
	// The outer group starts before the inner; the earlier span wins and the
	// inner group inside the already-replaced span is skipped.
	re := regexp.MustCompile(`(?P<outer>a(?P<inner>x)b)`)
	got := Replace(re, "-axb-", map[string]string{
		"outer": "O",
		"inner": "I",
	})
	if got != "-O-" {
		t.Errorf("got %q, want %q", got, "-O-")
	}

	// Inner only: replaced inside untouched outer text.
	got = Replace(re, "-axb-", map[string]string{"inner": "I"})
	if got != "-aIb-" {
		t.Errorf("got %q, want %q", got, "-aIb-")
	}
}
