package regextra_test

import (
	"errors"
	"strings"
	"testing"

	rx "github.com/jecoms/regextra"
)

// TestBenchmarkFixturesSane asserts that the benchmark fixtures actually
// exercise the paths their names claim. Without this, a fixture that silently
// stopped matching (a typo'd pattern, a changed input) would make its benchmark
// quietly measure the no-match path instead — a misleading regression-free
// "win". This test fails loudly if a representative fixture drifts.
func TestBenchmarkFixturesSane(t *testing.T) {
	// FindNamed: match, undeclared group, and no-match shapes.
	if v, ok := rx.FindNamed(bnFindRe, bnFindInput, "name"); !ok || v != "Alice" {
		t.Errorf("FindNamed small = (%q,%v), want (Alice,true)", v, ok)
	}
	if v, ok := rx.FindNamed(bnFindRe, bnFindInput, "missing"); ok || v != "" {
		t.Errorf("FindNamed undeclaredGroup = (%q,%v), want (\"\",false)", v, ok)
	}
	if _, ok := rx.FindNamed(bnFindEmailRe, bnFindNoMatch, "word"); ok {
		t.Error("FindNamed noMatch fixture unexpectedly matched")
	}
	if v, ok := rx.FindNamed(bnFindManyRe, bnFindManyIn, "j"); !ok || v != "ten" {
		t.Errorf("FindNamed manyGroupsExtractOne = (%q,%v), want (ten,true)", v, ok)
	}
	if v, ok := rx.FindNamed(bnFindEndRe, bnFindEndIn, "value"); !ok || v != "final" {
		t.Errorf("FindNamed largeMatchAtEnd = (%q,%v), want (final,true)", v, ok)
	}

	// FindAllNamed: nil (undeclared) vs empty (no match) vs match-count scaling.
	if got := rx.FindAllNamed(bnFAllRe, bnFAll10, "missing"); got != nil {
		t.Errorf("FindAllNamed undeclaredGroup = %v, want nil", got)
	}
	if got := rx.FindAllNamed(bnFAllRe, "", "word"); got == nil || len(got) != 0 {
		t.Errorf("FindAllNamed noMatch = %v, want empty non-nil", got)
	}
	for _, tc := range []struct {
		in   string
		want int
	}{{bnFAll1, 1}, {bnFAll10, 10}, {bnFAll100, 100}, {bnFAll1000, 1000}, {bnFAll10000, 10000}} {
		if got := len(rx.FindAllNamed(bnFAllRe, tc.in, "word")); got != tc.want {
			t.Errorf("FindAllNamed match count = %d, want %d", got, tc.want)
		}
	}

	// NamedGroups / AllNamedGroups: populated, no-match, and duplicate-name shapes.
	if m := rx.NamedGroups(bnNGRe, bnNGIn); len(m) != 2 {
		t.Errorf("NamedGroups twoGroups size = %d, want 2", len(m))
	}
	if m := rx.NamedGroups(bnNGRe, bnNGNoMatch); m == nil || len(m) != 0 {
		t.Errorf("NamedGroups noMatch = %v, want empty non-nil", m)
	}
	if m := rx.NamedGroups(bnNGManyRe, bnNGManyIn); len(m) != 20 {
		t.Errorf("NamedGroups manyGroups size = %d, want 20", len(m))
	}
	if m := rx.AllNamedGroups(bnANGDupRe, bnANGDupIn); len(m["word"]) != 3 {
		t.Errorf("AllNamedGroups duplicateGroupName word count = %d, want 3", len(m["word"]))
	}
	if m := rx.AllNamedGroups(bnANGManyDupRe, bnANGManyDupIn); len(m["w"]) != 20 {
		t.Errorf("AllNamedGroups manyDuplicates w count = %d, want 20", len(m["w"]))
	}

	// Replace: substitution count and the two early-return passthroughs.
	if got := rx.Replace(bnRepEmailRe, bnRepMulti100In, bnRepDomainMap); strings.Count(got, "redacted") != 100 {
		t.Errorf("Replace matches100 substitutions = %d, want 100", strings.Count(got, "redacted"))
	}
	if got := rx.Replace(bnRepEmailRe, bnRepSingleIn, bnRepEmptyMap); got != bnRepSingleIn {
		t.Errorf("Replace emptyReplacements changed target: %q", got)
	}
	if got := rx.Replace(bnRepEmailRe, bnRepNoMatchIn, bnRepDomainMap); got != bnRepNoMatchIn {
		t.Errorf("Replace noMatch changed target: %q", got)
	}

	// Validate: present vs missing.
	if err := rx.Validate(bnValRe, bnValPresent...); err != nil {
		t.Errorf("Validate allPresentSmall = %v, want nil", err)
	}
	if err := rx.Validate(bnValManyRe, bnValManyMissing...); err == nil {
		t.Error("Validate allMissingMany = nil, want error")
	}

	// Unmarshal: a real decode, the no-match no-op, and the error path.
	var s benchSimple
	if err := rx.Unmarshal(benchSimpleRe, benchSimpleInput, &s); err != nil {
		t.Fatalf("Unmarshal simpleThreeField error: %v", err)
	}
	if s.Name != "Alice" || s.Age != 30 || !s.Active {
		t.Errorf("Unmarshal simpleThreeField = %+v, want {Alice 30 true}", s)
	}
	var s2 benchSimple
	if err := rx.Unmarshal(benchSimpleRe, benchNoMatchInput, &s2); err != nil || s2 != (benchSimple{}) {
		t.Errorf("Unmarshal noMatch = (%v, %+v), want (nil, zero)", err, s2)
	}
	var bad benchBadInt
	if err := rx.Unmarshal(benchBadIntRe, benchBadIntIn, &bad); err == nil {
		t.Error("Unmarshal invalidIntConversion = nil, want error")
	}
	var tm benchTime
	if err := rx.Unmarshal(benchTimeRe, benchTimeLastIn, &tm); err != nil || tm.TS.IsZero() {
		t.Errorf("Unmarshal timeLastLayout = (%v, %v), want (nil, non-zero time)", err, tm.TS)
	}

	// UnmarshalAll / Decoder: aligned 100-match corpus and the streaming count.
	if n := strings.Count(benchBatch100Input, "Alice"); n != 100 {
		t.Fatalf("benchBatch100Input has %d Alice tokens, want 100", n)
	}
	var dst []benchSimple
	if err := rx.UnmarshalAll(benchSimpleRe, benchBatch100Input, &dst); err != nil || len(dst) != 100 {
		t.Errorf("UnmarshalAll matches100 = (%v, len %d), want (nil, 100)", err, len(dst))
	}
	if _, err := benchSimpleDecoder.One(benchNoMatchInput); !errors.Is(err, rx.ErrNoMatch) {
		t.Errorf("DecoderOne noMatch err = %v, want ErrNoMatch", err)
	}
	all, err := benchSimpleDecoder.All(benchBatch100Input)
	if err != nil || len(all) != 100 {
		t.Errorf("DecoderAll matches100 = (%v, len %d), want (nil, 100)", err, len(all))
	}
	n := 0
	for _, err := range benchLogDecoder.Iter(benchLogIn) {
		if err != nil {
			t.Fatalf("DecoderIter error: %v", err)
		}
		n++
	}
	if n != 100 {
		t.Errorf("DecoderIter fullIteration count = %d, want 100", n)
	}
}
