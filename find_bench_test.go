package regextra_test

import (
	"regexp"
	"strings"
	"testing"

	rx "github.com/jecoms/regextra/v2"
)

// ── FindNamed ─────────────────────────────────────────────────────────────────
//
// Cost model: SubexpIndex (O(declared groups), returns -1 on an undeclared name
// before any matching) then FindStringSubmatch, which dominates — it runs the
// RE2 NFA simulation over the target and allocates the capture slice. Go's
// regexp is RE2: linear time, no backtracking, no ReDoS — so a "pathological"
// pattern stresses NFA simulation, it does not blow up.

var (
	bnFindRe      = regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
	bnFindInput   = "Alice is 30"
	bnFindEmailRe = regexp.MustCompile(`(?P<word>\w+)@(?P<domain>\w+)`)
	bnFindNoMatch = "not-an-email-just-text"
	bnFindZeroRe  = regexp.MustCompile(`(?P<prefix>\w*)@(?P<domain>\w+)`)
	bnFindZeroIn  = "@example.com"
	bnFindMedRe   = regexp.MustCompile(`(?P<tag>[a-z]+):\s*(?P<value>[^;]+)`)
	bnFindMedIn   = strings.Repeat("x:y;", 50) + "target:extracted"
	bnFindStartRe = regexp.MustCompile(`^(?P<header>\w+):`)
	bnFindStartIn = "START:" + strings.Repeat("x", 5000)
	bnFindEndRe   = regexp.MustCompile(`(?P<value>\w+)$`)
	bnFindEndIn   = strings.Repeat("x", 5000) + " final"
	bnFindManyRe  = regexp.MustCompile(`(?P<a>\w+) (?P<b>\w+) (?P<c>\w+) (?P<d>\w+) (?P<e>\w+) (?P<f>\w+) (?P<g>\w+) (?P<h>\w+) (?P<i>\w+) (?P<j>\w+)`)
	bnFindManyIn  = "one two three four five six seven eight nine ten"
	bnFindUTF8Re  = regexp.MustCompile(`(?P<name>\p{L}+) (\p{L}+) (?P<age>\d+)`)
	bnFindUTF8In  = "María José 28" + strings.Repeat(" 日本語", 10)
	bnFindAnchRe  = regexp.MustCompile(`^(?P<method>GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS) (?P<path>/[^\s]+)$`)
	bnFindAnchIn  = "POST /api/users/123"
	bnFindNestRe  = regexp.MustCompile(`(?P<value>a+)+b`)
	bnFindNestIn  = strings.Repeat("a", 25) + "c"
)

func BenchmarkFindNamed(b *testing.B) {
	// representative — the common one-shot extraction fast path.
	benchCase(b, "small", func() { sinkStr, sinkOK = rx.FindNamed(bnFindRe, bnFindInput, "name") })
	benchCase(b, "medium", func() { sinkStr, sinkOK = rx.FindNamed(bnFindMedRe, bnFindMedIn, "tag") })
	benchCase(b, "multibyteUTF8", func() { sinkStr, sinkOK = rx.FindNamed(bnFindUTF8Re, bnFindUTF8In, "name") })
	benchCase(b, "anchored", func() { sinkStr, sinkOK = rx.FindNamed(bnFindAnchRe, bnFindAnchIn, "method") })
	// edge — no-match contract shapes and group boundaries.
	benchCase(b, "undeclaredGroup", func() { sinkStr, sinkOK = rx.FindNamed(bnFindRe, bnFindInput, "missing") }) // SubexpIndex == -1, no scan
	benchCase(b, "noMatch", func() { sinkStr, sinkOK = rx.FindNamed(bnFindEmailRe, bnFindNoMatch, "word") })     // declared group, full no-match scan
	benchCase(b, "zeroLengthMatch", func() { sinkStr, sinkOK = rx.FindNamed(bnFindZeroRe, bnFindZeroIn, "prefix") })
	// scaling — capture-slice allocation and scan-length growth.
	benchCase(b, "manyGroupsExtractOne", func() { sinkStr, sinkOK = rx.FindNamed(bnFindManyRe, bnFindManyIn, "j") }) // 10-element capture slice; SubexpIndex a minor addend
	benchCase(b, "largeMatchAtStart", func() { sinkStr, sinkOK = rx.FindNamed(bnFindStartRe, bnFindStartIn, "header") })
	benchCase(b, "largeMatchAtEnd", func() { sinkStr, sinkOK = rx.FindNamed(bnFindEndRe, bnFindEndIn, "value") })
	// pathological — NFA simulation of a nested quantifier with no match (linear, not catastrophic).
	benchCase(b, "nestedQuantifierNoMatch", func() { sinkStr, sinkOK = rx.FindNamed(bnFindNestRe, bnFindNestIn, "value") })
}

// ── FindAllNamed ──────────────────────────────────────────────────────────────
//
// Cost model: FindAllStringSubmatch scans the whole target and allocates a
// [][]string of every match; FindAllNamed then projects one group into an
// out []string whose size scales with match count. Undeclared group → nil
// before any scan; declared-but-no-match → empty slice.

var (
	bnFAllRe       = regexp.MustCompile(`(?P<word>\S+)`)
	bnFAll1        = "alpha"
	bnFAll10       = strings.TrimSpace(strings.Repeat("w ", 10))
	bnFAll100      = strings.TrimSpace(strings.Repeat("w ", 100))
	bnFAll1000     = strings.TrimSpace(strings.Repeat("w ", 1000))
	bnFAll10000    = strings.TrimSpace(strings.Repeat("w ", 10000))
	bnFAllSparseRe = regexp.MustCompile(`(?P<num>\d+)`)
	bnFAllSparseIn = strings.Repeat("abcdefghij", 1000) + " 1 2 3" // large target, few matches
	bnFAllDenseRe  = regexp.MustCompile(`(?P<c>\w)`)
	bnFAllDenseIn  = strings.Repeat("a", 500) // many tiny matches in a small target
)

func BenchmarkFindAllNamed(b *testing.B) {
	// edge — the two no-match shapes (nil vs empty slice).
	benchCase(b, "undeclaredGroup", func() { sinkStrs = rx.FindAllNamed(bnFAllRe, bnFAll10, "missing") }) // nil path, no scan
	benchCase(b, "noMatch", func() { sinkStrs = rx.FindAllNamed(bnFAllRe, "", "word") })                  // declared, empty slice
	// representative + scaling — allocation scales with match count.
	benchCase(b, "singleMatch", func() { sinkStrs = rx.FindAllNamed(bnFAllRe, bnFAll1, "word") })
	benchCase(b, "matches10", func() { sinkStrs = rx.FindAllNamed(bnFAllRe, bnFAll10, "word") })
	benchCase(b, "matches100", func() { sinkStrs = rx.FindAllNamed(bnFAllRe, bnFAll100, "word") })
	benchCase(b, "matches1000", func() { sinkStrs = rx.FindAllNamed(bnFAllRe, bnFAll1000, "word") })
	benchCase(b, "matches10000", func() { sinkStrs = rx.FindAllNamed(bnFAllRe, bnFAll10000, "word") })
	// pathological — target size vs match density.
	benchCase(b, "sparseLarge", func() { sinkStrs = rx.FindAllNamed(bnFAllSparseRe, bnFAllSparseIn, "num") })
	benchCase(b, "denseSmall", func() { sinkStrs = rx.FindAllNamed(bnFAllDenseRe, bnFAllDenseIn, "c") })
}

// ── NamedGroups ───────────────────────────────────────────────────────────────
//
// Cost model: FindStringSubmatch, then a SubexpNames loop populating a map
// whose insertion cost scales with declared (named) group count. No-match still
// allocates the empty (non-nil) map and returns before the loop.

var (
	bnNGRe      = regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
	bnNGIn      = "Alice is 30"
	bnNGNoMatch = "no match here"
	bnNGManyRe  = regexp.MustCompile(bnGroupsPattern(20))
	bnNGManyIn  = bnWords(20)
)

func BenchmarkNamedGroups(b *testing.B) {
	benchCase(b, "twoGroups", func() { sinkMap = rx.NamedGroups(bnNGRe, bnNGIn) })          // representative
	benchCase(b, "noMatch", func() { sinkMap = rx.NamedGroups(bnNGRe, bnNGNoMatch) })       // empty non-nil map, pre-loop return
	benchCase(b, "manyGroups", func() { sinkMap = rx.NamedGroups(bnNGManyRe, bnNGManyIn) }) // map-insertion scaling (20 groups)
}

// ── NamedGroupOccurrences ─────────────────────────────────────────────────────
//
// Cost model: like NamedGroups, but values are []string, so each distinct group
// costs an extra one-element slice allocation, and a repeated group name forces
// slice append/regrowth under one key — the path this function exists for.

var (
	bnNGODistinctRe = regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
	bnNGODistinctIn = "Alice 30"
	bnNGODupRe      = regexp.MustCompile(`(?P<word>\w+) (?P<word>\w+) (?P<word>\w+)`)
	bnNGODupIn      = "alpha beta gamma"
	bnNGONoMatch    = "nomatch"
	bnNGOManyDupRe  = regexp.MustCompile(strings.Repeat(`(?P<w>\w+)\s*`, 20))
	bnNGOManyDupIn  = strings.Repeat("x ", 20)
)

func BenchmarkNamedGroupOccurrences(b *testing.B) {
	// distinctGroups pairs with NamedGroups/twoGroups-style input to expose the slice-per-key tax.
	benchCase(b, "distinctGroups", func() { sinkMapSS = rx.NamedGroupOccurrences(bnNGODistinctRe, bnNGODistinctIn) })
	benchCase(b, "duplicateGroupName", func() { sinkMapSS = rx.NamedGroupOccurrences(bnNGODupRe, bnNGODupIn) }) // 3 occurrences under one key
	benchCase(b, "noMatch", func() { sinkMapSS = rx.NamedGroupOccurrences(bnNGODistinctRe, bnNGONoMatch) })
	benchCase(b, "manyDuplicates", func() { sinkMapSS = rx.NamedGroupOccurrences(bnNGOManyDupRe, bnNGOManyDupIn) }) // 20 appends + regrowth under one key
}

// ── NamedGroupsPerMatch ───────────────────────────────────────────────────────
//
// Cost model: like NamedGroups but over every match — FindAllStringSubmatchIndex
// scans the whole target and allocates the index matrix, then one map is built
// per match (map alloc + per-group insertion). The Seq form skips the outer
// []map[string]string allocation but builds the same per-match maps. Cost
// dimensions: match count and groups-per-match.

var (
	bnNGPMRe        = regexp.MustCompile(`(?P<key>\w+)=(?P<value>\w+)`)
	bnNGPMIn        = strings.TrimSpace(strings.Repeat("a=1 b=2 ", 50)) // 100 matches
	bnNGPMSingleIn  = "a=1"
	bnNGPMNoMatchIn = "no key value pairs here"
)

func BenchmarkNamedGroupsPerMatch(b *testing.B) {
	benchCase(b, "matches100", func() { sinkSliceMap = rx.NamedGroupsPerMatch(bnNGPMRe, bnNGPMIn) })     // representative batch
	benchCase(b, "single", func() { sinkSliceMap = rx.NamedGroupsPerMatch(bnNGPMRe, bnNGPMSingleIn) })   // one match
	benchCase(b, "noMatch", func() { sinkSliceMap = rx.NamedGroupsPerMatch(bnNGPMRe, bnNGPMNoMatchIn) }) // empty non-nil slice, early return
}

func BenchmarkNamedGroupsPerMatchSeq(b *testing.B) {
	// Drains the iterator into the same per-match map sink; isolates the
	// slice-allocation saving versus the eager NamedGroupsPerMatch above.
	benchCase(b, "matches100", func() {
		for m := range rx.NamedGroupsPerMatchSeq(bnNGPMRe, bnNGPMIn) {
			sinkMap = m
		}
	})
}
