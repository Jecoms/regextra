package regextra

import (
	"regexp"
	"testing"
)

// Regression tests for https://github.com/Jecoms/regextra/issues/105:
// patterns that reuse a group name (legal in Go's regexp, e.g. across
// alternation branches) were mishandled in both decode paths — the map
// builders let a non-participating later occurrence clobber the real value
// with "", and the Decoder read only SubexpIndex's first occurrence.

func TestNamedGroups_duplicateNames(t *testing.T) {
	re := regexp.MustCompile(`(?:x(?P<word>a)|y(?P<word>b))`)

	t.Run("first alternation branch participates", func(t *testing.T) {
		got := NamedGroups(re, "xa")
		if got["word"] != "a" {
			t.Errorf(`got word=%q, want "a" (non-participating duplicate must not clobber)`, got["word"])
		}
	})

	t.Run("second alternation branch participates", func(t *testing.T) {
		got := NamedGroups(re, "yb")
		if got["word"] != "b" {
			t.Errorf(`got word=%q, want "b"`, got["word"])
		}
	})

	t.Run("sequential duplicates keep last-wins", func(t *testing.T) {
		seq := regexp.MustCompile(`(?P<word>\w+) (?P<word>\w+)`)
		got := NamedGroups(seq, "hello world")
		if got["word"] != "world" {
			t.Errorf(`got word=%q, want "world" (last participating occurrence wins)`, got["word"])
		}
	})

	t.Run("AllNamedGroups still preserves every occurrence", func(t *testing.T) {
		got := AllNamedGroups(re, "xa")
		want := []string{"a", ""}
		if len(got["word"]) != 2 || got["word"][0] != want[0] || got["word"][1] != want[1] {
			t.Errorf("got word=%q, want %q", got["word"], want)
		}
	})
}

func TestUnmarshal_duplicateNames(t *testing.T) {
	re := regexp.MustCompile(`(?:x(?P<word>a)|y(?P<word>b))`)
	type rec struct {
		Word string `regex:"word"`
	}

	for input, want := range map[string]string{"xa": "a", "yb": "b"} {
		var r rec
		if err := Unmarshal(re, input, &r); err != nil {
			t.Fatalf("Unmarshal(%q): %v", input, err)
		}
		if r.Word != want {
			t.Errorf("Unmarshal(%q): Word = %q, want %q", input, r.Word, want)
		}
	}
}

func TestUnmarshalAll_duplicateNames(t *testing.T) {
	re := regexp.MustCompile(`(?:x(?P<word>a)|y(?P<word>b))`)
	type rec struct {
		Word string `regex:"word"`
	}

	var recs []rec
	if err := UnmarshalAll(re, "xa yb xa", &recs); err != nil {
		t.Fatalf("UnmarshalAll: %v", err)
	}
	want := []string{"a", "b", "a"}
	if len(recs) != len(want) {
		t.Fatalf("got %d matches, want %d", len(recs), len(want))
	}
	for i, w := range want {
		if recs[i].Word != w {
			t.Errorf("match %d: Word = %q, want %q", i, recs[i].Word, w)
		}
	}
}

func TestDecoder_duplicateNames(t *testing.T) {
	type rec struct {
		Word string `regex:"word"`
	}
	dec := MustCompile[rec](`(?:x(?P<word>a)|y(?P<word>b))`)

	t.Run("One finds the participating occurrence in either branch", func(t *testing.T) {
		for input, want := range map[string]string{"xa": "a", "yb": "b"} {
			got, err := dec.One(input)
			if err != nil {
				t.Fatalf("One(%q): %v", input, err)
			}
			if got.Word != want {
				t.Errorf("One(%q): Word = %q, want %q", input, got.Word, want)
			}
		}
	})

	t.Run("All decodes mixed branches", func(t *testing.T) {
		got, err := dec.All("xa yb")
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		if len(got) != 2 || got[0].Word != "a" || got[1].Word != "b" {
			t.Errorf("got %+v, want [{a} {b}]", got)
		}
	})

	t.Run("sequential duplicates agree with Unmarshal (last wins)", func(t *testing.T) {
		const pattern = `(?P<word>\w+) (?P<word>\w+)`
		seqDec := MustCompile[rec](pattern)
		fromDecoder, err := seqDec.One("hello world")
		if err != nil {
			t.Fatalf("One: %v", err)
		}
		var fromUnmarshal rec
		if err := Unmarshal(regexp.MustCompile(pattern), "hello world", &fromUnmarshal); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if fromDecoder.Word != fromUnmarshal.Word {
			t.Errorf("paths disagree: Decoder=%q Unmarshal=%q", fromDecoder.Word, fromUnmarshal.Word)
		}
		if fromDecoder.Word != "world" {
			t.Errorf("Word = %q, want %q", fromDecoder.Word, "world")
		}
	})
}

func TestFindNamed_duplicateNames(t *testing.T) {
	re := regexp.MustCompile(`(?:x(?P<word>a)|y(?P<word>b))`)

	if got, ok := FindNamed(re, "yb", "word"); got != "b" || !ok {
		t.Errorf(`FindNamed("yb","word") = (%q,%v), want ("b",true) — must read the participating branch, not SubexpIndex's first`, got, ok)
	}
	if got, ok := FindNamed(re, "xa", "word"); got != "a" || !ok {
		t.Errorf(`FindNamed("xa","word") = (%q,%v), want ("a",true)`, got, ok)
	}
	if got, ok := FindNamed(re, "zz", "word"); ok {
		t.Errorf(`FindNamed("zz","word") = (%q,%v), want ("",false) on no match`, got, ok)
	}
	if got, ok := FindNamed(re, "yb", "missing"); ok {
		t.Errorf(`FindNamed("yb","missing") = (%q,%v), want ("",false) for an undeclared group`, got, ok)
	}
}

func TestFindAllNamed_duplicateNames(t *testing.T) {
	re := regexp.MustCompile(`(?:x(?P<word>a)|y(?P<word>b))`)
	got := FindAllNamed(re, "xa yb", "word")
	want := []string{"a", "b"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("FindAllNamed(%q) = %q, want %q (each match reads its participating branch)", "xa yb", got, want)
	}
}

// Issue-105 follow-up: when a duplicate group name's last occurrence
// participates with an *empty* span, the Decoder and the Unmarshal/NamedGroups
// paths must agree (the Decoder previously read the first occurrence's value).
func TestDecoderUnmarshal_participatingEmptyDuplicate(t *testing.T) {
	const pattern = `(?P<word>a)(?P<word>b*)`
	re := regexp.MustCompile(pattern)
	type rec struct {
		Word string `regex:"word"`
	}

	dec := MustCompile[rec](pattern)
	fromDecoder, err := dec.One("a")
	if err != nil {
		t.Fatalf("One: %v", err)
	}
	fromMap := NamedGroups(re, "a")["word"]
	var fromUnmarshal rec
	if err := Unmarshal(re, "a", &fromUnmarshal); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if fromDecoder.Word != fromMap || fromDecoder.Word != fromUnmarshal.Word {
		t.Errorf("paths disagree: Decoder=%q NamedGroups=%q Unmarshal=%q (want all equal)",
			fromDecoder.Word, fromMap, fromUnmarshal.Word)
	}
	if fromDecoder.Word != "" {
		t.Errorf("Word = %q, want \"\" (the empty b* span is the last participating occurrence)", fromDecoder.Word)
	}
}

// A non-participating optional group on a typed field is left at its zero value
// by both paths; Unmarshal no longer errors trying to convert "" (issue-105
// alignment of the Unmarshal and Decoder paths).
func TestUnmarshalDecoder_nonParticipatingOptionalTyped(t *testing.T) {
	const pattern = `y(?P<num>\d+)?`
	re := regexp.MustCompile(pattern)
	type rec struct {
		Num int `regex:"num"`
	}

	var fromUnmarshal rec
	if err := Unmarshal(re, "y", &fromUnmarshal); err != nil {
		t.Fatalf("Unmarshal: unexpected error %v (a non-participating optional group should leave the field zero)", err)
	}
	if fromUnmarshal.Num != 0 {
		t.Errorf("Unmarshal Num = %d, want 0", fromUnmarshal.Num)
	}

	dec := MustCompile[rec](pattern)
	fromDecoder, err := dec.One("y")
	if err != nil {
		t.Fatalf("One: %v", err)
	}
	if fromDecoder.Num != 0 {
		t.Errorf("Decoder Num = %d, want 0", fromDecoder.Num)
	}
}

// NamedGroups still surfaces a declared-but-non-participating group as "" —
// only the Unmarshal path omits it (the includeNonParticipating split).
func TestNamedGroups_nonParticipatingStillPresent(t *testing.T) {
	re := regexp.MustCompile(`(?:x(?P<a>1)|y(?P<b>2))`)
	got := NamedGroups(re, "y2")
	if v, ok := got["a"]; !ok || v != "" {
		t.Errorf(`NamedGroups("y2")["a"] = (%q,%v), want ("",true) — declared but did not participate`, v, ok)
	}
	if got["b"] != "2" {
		t.Errorf(`NamedGroups("y2")["b"] = %q, want "2"`, got["b"])
	}
}
