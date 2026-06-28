package regextra_test

import (
	"regexp"
	"strings"
	"testing"

	rx "github.com/jecoms/regextra"
)

// ── Replace ───────────────────────────────────────────────────────────────────
//
// Cost model: an empty replacements map returns before any scan. Otherwise
// FindAllStringSubmatchIndex scans the whole target and allocates the index
// matrix; then per match a span slice is built, sort.Slice-d by start, and
// written to a strings.Builder with cursor tracking for the outermost-wins
// overlap rule. Cost dimensions: match count, named-groups-per-match (sort
// size), and replacement length (Builder growth).
//
// Note: the nestedGroups case exercises the documented "outermost wins" path,
// which issue #107 shows is not guaranteed because sort.Slice is unstable. This
// benchmark measures the path's cost; it does not assert the tie-break.

var (
	bnRepEmailRe     = regexp.MustCompile(`(?P<user>\w+)@(?P<domain>[\w.]+)`)
	bnRepSingleIn    = "alice@example.com"
	bnRepDomainMap   = map[string]string{"domain": "redacted"}
	bnRepBothMap     = map[string]string{"user": "anon", "domain": "redacted"}
	bnRepEmptyMap    = map[string]string{}
	bnRepMissMap     = map[string]string{"nonexistent": "x"}
	bnRepNoMatchIn   = "no matches here at all, just plain words"
	bnRepMulti100In  = strings.TrimSpace(strings.Repeat("alice@example.com ", 100))
	bnRepMulti1200In = strings.TrimSpace(strings.Repeat("alice@example.com ", 1200))
	bnRepNestRe      = regexp.MustCompile(`(?P<outer>(?P<inner>\w+)@[\w.]+)`)
	bnRepNestMap     = map[string]string{"outer": "X", "inner": "Y"}
	bnRepOptRe       = regexp.MustCompile(`(?P<word>\w+)(?P<bang>!)?`)
	bnRepOptIn       = "hello world foo bar baz"
	bnRepOptMap      = map[string]string{"word": "W", "bang": "B"}
	bnRepManyGRe     = regexp.MustCompile(bnGroupsPattern(8))
	bnRepManyGIn     = bnWords(8)
	bnRepManyGMap    = map[string]string{"f0": "a", "f1": "b", "f2": "c", "f3": "d", "f4": "e", "f5": "f", "f6": "g", "f7": "h"}
	bnRepLongMap     = map[string]string{"domain": strings.Repeat("x", 200)}
)

func BenchmarkReplace(b *testing.B) {
	// representative — one match, then a realistic 100-match batch.
	benchCase(b, "singleGroup", func() { sinkStr = rx.Replace(bnRepEmailRe, bnRepSingleIn, bnRepDomainMap) })
	benchCase(b, "twoGroups", func() { sinkStr = rx.Replace(bnRepEmailRe, bnRepSingleIn, bnRepBothMap) })
	benchCase(b, "matches100", func() { sinkStr = rx.Replace(bnRepEmailRe, bnRepMulti100In, bnRepDomainMap) })
	// edge — the two early returns and the loop-with-zero-spans passthrough.
	benchCase(b, "emptyReplacements", func() { sinkStr = rx.Replace(bnRepEmailRe, bnRepSingleIn, bnRepEmptyMap) }) // len==0 early return
	benchCase(b, "noMatch", func() { sinkStr = rx.Replace(bnRepEmailRe, bnRepNoMatchIn, bnRepDomainMap) })         // no matches early return
	benchCase(b, "groupsNotInMap", func() { sinkStr = rx.Replace(bnRepEmailRe, bnRepSingleIn, bnRepMissMap) })     // matches, but 0 spans appended
	// pathological — overlap (outermost wins) and optional non-participating group.
	benchCase(b, "nestedGroups", func() { sinkStr = rx.Replace(bnRepNestRe, bnRepSingleIn, bnRepNestMap) })
	benchCase(b, "optionalNonParticipating", func() { sinkStr = rx.Replace(bnRepOptRe, bnRepOptIn, bnRepOptMap) }) // bang group index -1 skip
	// scaling — sort size per match, match count, and replacement length.
	benchCase(b, "manyGroupsPerMatch", func() { sinkStr = rx.Replace(bnRepManyGRe, bnRepManyGIn, bnRepManyGMap) }) // sort.Slice(8)
	benchCase(b, "largeManyMatches", func() { sinkStr = rx.Replace(bnRepEmailRe, bnRepMulti1200In, bnRepDomainMap) })
	benchCase(b, "longReplacement", func() { sinkStr = rx.Replace(bnRepEmailRe, bnRepMulti100In, bnRepLongMap) }) // Builder growth
}

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
	benchCase(b, "allPresentSmall", func() { sinkErr = rx.Validate(bnValRe, bnValPresent...) })     // nil, no missing-list alloc
	benchCase(b, "partialMissSmall", func() { sinkErr = rx.Validate(bnValRe, bnValPartial...) })    // 1 missing
	benchCase(b, "allPresentMany", func() { sinkErr = rx.Validate(bnValManyRe, bnValManyPresent...) }) // 20-name set build + 20 hits
	benchCase(b, "allMissingMany", func() { sinkErr = rx.Validate(bnValManyRe, bnValManyMissing...) }) // 20 misses + big Join
	benchCase(b, "noDeclaredSomeRequired", func() { sinkErr = rx.Validate(bnValEmptyRe, bnValSome...) })
	benchCase(b, "zeroEverything", func() { sinkErr = rx.Validate(bnValEmptyRe, bnValNone...) }) // fixed overhead floor
}
