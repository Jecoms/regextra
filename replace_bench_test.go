package regextra_test

import (
	"regexp"
	"strings"
	"testing"

	rx "github.com/jecoms/regextra/v2"
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

// ── ReplaceFirst ────────────────────────────────────────────────────────────
//
// Same span engine as Replace, but limit=1 stops the scan after the first
// match: only that match's named spans are collected, sorted, and written,
// after which the shared trailing-cursor write copies everything past the first
// match — every later match and all outside text — byte-for-byte. So unlike
// Replace, cost is bounded by the first match's group count plus the length of
// the verbatim remainder, not by total match count. The multi-match and
// largeManyMatches cases exercise that remainder copy; the early-return and
// per-match edge cases mirror Replace's. Reuses Replace's bnRep* fixtures.
func BenchmarkReplaceFirst(b *testing.B) {
	// representative — one match, then a batch where only the first is rewritten.
	benchCase(b, "singleGroup", func() { sinkStr = rx.ReplaceFirst(bnRepEmailRe, bnRepSingleIn, bnRepDomainMap) })
	benchCase(b, "twoGroups", func() { sinkStr = rx.ReplaceFirst(bnRepEmailRe, bnRepSingleIn, bnRepBothMap) })
	benchCase(b, "matches100", func() { sinkStr = rx.ReplaceFirst(bnRepEmailRe, bnRepMulti100In, bnRepDomainMap) }) // first replaced, 99 + tail verbatim
	// edge — the two early returns and the loop-with-zero-spans passthrough.
	benchCase(b, "emptyReplacements", func() { sinkStr = rx.ReplaceFirst(bnRepEmailRe, bnRepSingleIn, bnRepEmptyMap) }) // len==0 early return
	benchCase(b, "noMatch", func() { sinkStr = rx.ReplaceFirst(bnRepEmailRe, bnRepNoMatchIn, bnRepDomainMap) })         // no matches early return
	benchCase(b, "groupsNotInMap", func() { sinkStr = rx.ReplaceFirst(bnRepEmailRe, bnRepSingleIn, bnRepMissMap) })     // matches, but 0 spans appended
	// pathological — overlap (outermost wins) and optional non-participating group.
	benchCase(b, "nestedGroups", func() { sinkStr = rx.ReplaceFirst(bnRepNestRe, bnRepSingleIn, bnRepNestMap) })
	benchCase(b, "optionalNonParticipating", func() { sinkStr = rx.ReplaceFirst(bnRepOptRe, bnRepOptIn, bnRepOptMap) }) // bang group index -1 skip
	// scaling — verbatim-remainder copy dominates as match count grows.
	benchCase(b, "largeManyMatches", func() { sinkStr = rx.ReplaceFirst(bnRepEmailRe, bnRepMulti1200In, bnRepDomainMap) })
}

// ── ReplaceFunc ─────────────────────────────────────────────────────────────
//
// Same span engine as Replace, but the replacement is computed per applied span
// by a caller callback over the matched value rather than a map lookup. Cost
// dimensions mirror Replace (match count, groups-per-match sort size); the extra
// axis is callback cost — bnRepFuncIdentity returns the match verbatim to isolate
// dispatch overhead, bnRepFuncUpper does real per-span work.

var (
	bnRepFuncIdentity = func(_, m string) string { return m }
	bnRepFuncUpper    = func(_, m string) string { return strings.ToUpper(m) }
)

func BenchmarkReplaceFunc(b *testing.B) {
	// representative — one match, then a realistic 100-match batch.
	benchCase(b, "singleGroup", func() { sinkStr = rx.ReplaceFunc(bnRepEmailRe, bnRepSingleIn, bnRepFuncUpper) })
	benchCase(b, "matches100", func() { sinkStr = rx.ReplaceFunc(bnRepEmailRe, bnRepMulti100In, bnRepFuncUpper) })
	// edge — no match takes the early return; fn is never called.
	benchCase(b, "noMatch", func() { sinkStr = rx.ReplaceFunc(bnRepEmailRe, bnRepNoMatchIn, bnRepFuncUpper) })
	// dispatch-only — identity callback isolates closure/indirection cost.
	benchCase(b, "identity", func() { sinkStr = rx.ReplaceFunc(bnRepEmailRe, bnRepMulti100In, bnRepFuncIdentity) })
	// pathological — overlap (outermost wins, inner fn suppressed) and optional skip.
	benchCase(b, "nestedGroups", func() { sinkStr = rx.ReplaceFunc(bnRepNestRe, bnRepSingleIn, bnRepFuncUpper) })
	benchCase(b, "optionalNonParticipating", func() { sinkStr = rx.ReplaceFunc(bnRepOptRe, bnRepOptIn, bnRepFuncUpper) })
	// scaling — sort size per match and match count.
	benchCase(b, "manyGroupsPerMatch", func() { sinkStr = rx.ReplaceFunc(bnRepManyGRe, bnRepManyGIn, bnRepFuncUpper) })
	benchCase(b, "largeManyMatches", func() { sinkStr = rx.ReplaceFunc(bnRepEmailRe, bnRepMulti1200In, bnRepFuncUpper) })
}

// ── ReplaceFuncFirst ────────────────────────────────────────────────────────
//
// ReplaceFunc's callback dispatch on ReplaceFirst's limit=1 scan: only the
// first match's named spans reach fn, after which the shared trailing-cursor
// write copies every later match and all outside text byte-for-byte. Cost is
// bounded by the first match's group count plus the verbatim remainder length,
// not total match count; the callback axis mirrors ReplaceFunc (identity
// isolates dispatch overhead). Reuses the bnRep* fixtures.
func BenchmarkReplaceFuncFirst(b *testing.B) {
	// representative — one match, then a batch where only the first is rewritten.
	benchCase(b, "singleGroup", func() { sinkStr = rx.ReplaceFuncFirst(bnRepEmailRe, bnRepSingleIn, bnRepFuncUpper) })
	benchCase(b, "matches100", func() { sinkStr = rx.ReplaceFuncFirst(bnRepEmailRe, bnRepMulti100In, bnRepFuncUpper) }) // first replaced, 99 + tail verbatim
	// edge — no match takes the early return; fn is never called.
	benchCase(b, "noMatch", func() { sinkStr = rx.ReplaceFuncFirst(bnRepEmailRe, bnRepNoMatchIn, bnRepFuncUpper) })
	// dispatch-only — identity callback isolates closure/indirection cost.
	benchCase(b, "identity", func() { sinkStr = rx.ReplaceFuncFirst(bnRepEmailRe, bnRepMulti100In, bnRepFuncIdentity) })
	// pathological — overlap (outermost wins, inner fn suppressed) and optional skip.
	benchCase(b, "nestedGroups", func() { sinkStr = rx.ReplaceFuncFirst(bnRepNestRe, bnRepSingleIn, bnRepFuncUpper) })
	benchCase(b, "optionalNonParticipating", func() { sinkStr = rx.ReplaceFuncFirst(bnRepOptRe, bnRepOptIn, bnRepFuncUpper) })
	// scaling — verbatim-remainder copy dominates as match count grows.
	benchCase(b, "largeManyMatches", func() { sinkStr = rx.ReplaceFuncFirst(bnRepEmailRe, bnRepMulti1200In, bnRepFuncUpper) })
}
