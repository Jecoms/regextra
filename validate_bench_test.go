package regextra_test

import (
	"regexp"
	"testing"

	rx "github.com/jecoms/regextra/v2"
)

// ── Validate ──────────────────────────────────────────────────────────────────
//
// Cost model: build a set of declared names from SubexpNames (scales with
// declared group count), then look up each required name; misses append to a
// list joined into the error message. Best case (all present) skips the
// allocation of the missing list and the strings.Join.
//
// The variadic required names are passed as pre-built package-level slices via
// `slice...` so the backing array is not allocated inside the loop (which would
// otherwise be wrongly attributed to Validate).

var (
	bnValRe          = regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
	bnValPresent     = []string{"name", "age"}
	bnValPartial     = []string{"name", "missing"}
	bnValManyRe      = regexp.MustCompile(bnGroupsPattern(20))
	bnValManyPresent = bnNames("f", 20) // f0..f19 — all declared on bnValManyRe
	bnValManyMissing = bnNames("m", 20) // m0..m19 — none declared
	bnValEmptyRe     = regexp.MustCompile(`\w+ \w+`)
	bnValSome        = []string{"a", "b", "c"}
	bnValNone        = []string{}
)

func BenchmarkValidate(b *testing.B) {
	benchCase(b, "allPresentSmall", func() { sinkErr = rx.Validate(bnValRe, bnValPresent...) })        // nil, no missing-list alloc
	benchCase(b, "partialMissSmall", func() { sinkErr = rx.Validate(bnValRe, bnValPartial...) })       // 1 missing
	benchCase(b, "allPresentMany", func() { sinkErr = rx.Validate(bnValManyRe, bnValManyPresent...) }) // 20-name set build + 20 hits
	benchCase(b, "allMissingMany", func() { sinkErr = rx.Validate(bnValManyRe, bnValManyMissing...) }) // 20 misses + big Join
	benchCase(b, "noDeclaredSomeRequired", func() { sinkErr = rx.Validate(bnValEmptyRe, bnValSome...) })
	benchCase(b, "zeroEverything", func() { sinkErr = rx.Validate(bnValEmptyRe, bnValNone...) }) // fixed overhead floor
}
