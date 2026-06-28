package regextra_test

// Benchmark suite for regextra's exported surface.
//
// Goals (see the package doc's "Performance" section for the user-facing claims
// these numbers back):
//
//  1. Give every exported function a STATISTICALLY REPRESENTATIVE sample of
//     real-world inputs — a small/medium/large progression spanning the
//     realistic distribution of input size and match/group count — not one
//     arbitrary fixture.
//  2. Cover the edge and pathological corners of each cost model: the no-match
//     shapes from the package's no-match contract table, optional/empty groups,
//     duplicate group names, unicode, the time-layout best/worst pair, every
//     setFieldValue dispatch branch, and the argument-validation guards.
//  3. Keep head-to-head pairs (Unmarshal vs Decoder.One, UnmarshalAll vs
//     Decoder.All) on BYTE-IDENTICAL regex + struct + input so benchstat can
//     diff the caching win directly.
//
// Conventions:
//   - One top-level Benchmark<Func> per exported function; cases are flat
//     lowerCamelCase b.Run sub-names drawn from a shared vocabulary
//     (noMatch, undeclaredGroup, singleMatch, small/medium/large, matchesN,
//     fieldsN, groupsN, firstLayout/lastLayout).
//   - Every fixture is a package-level var/const named bn<Group><Case>* —
//     nothing built with strings.Repeat/Join/Sprintf or a variadic literal is
//     constructed inside a benchmark loop, so the harness measures the function
//     under test, not fixture construction.
//   - Every loop body assigns its result to a typed package-level sink so the
//     optimizer cannot fold the call to a no-op store (belt-and-suspenders on
//     top of Go 1.24's b.Loop, which already keeps the call alive).
//
// Run with:
//
//	go test -bench=. -benchmem -run=^$ ./...
//
// A/B a single function across a change with:
//
//	go test -bench=BenchmarkUnmarshal -benchmem -run=^$ -count=10 ./... | tee new.txt
//	benchstat old.txt new.txt

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	rx "github.com/jecoms/regextra"
)

// ── Typed sinks ───────────────────────────────────────────────────────────────
// One per return kind, assigned in every loop body to defeat dead-store
// elimination of the benchmarked call's result.
var (
	sinkStr   string
	sinkOK    bool
	sinkStrs  []string
	sinkMap   map[string]string
	sinkMapSS map[string][]string
	sinkErr   error
	sinkAny   any
)

// benchCase runs fn in an allocation-reporting b.Loop under a named sub-benchmark.
// fn must assign the result of the call under test to a sink. The single indirect
// call benchCase adds is a constant offset across every case (and mirrors how real
// callers invoke these funcs — through a call boundary, not inlined), so it leaves
// relative comparisons and benchstat diffs valid.
func benchCase(b *testing.B, name string, fn func()) {
	b.Helper()
	b.Run(name, func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			fn()
		}
	})
}

// ── Shared fixture generators (run at package init, never in a loop) ──────────

// bnGroupsPattern builds a pattern of n distinct named groups f0..f(n-1)
// separated by spaces, e.g. `(?P<f0>\w+) (?P<f1>\w+)`.
func bnGroupsPattern(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf(`(?P<f%d>\w+)`, i)
	}
	return strings.Join(parts, " ")
}

// bnWords builds n space-separated "w" tokens, the matching input for
// bnGroupsPattern(n).
func bnWords(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "w"
	}
	return strings.Join(parts, " ")
}

// bnNames builds n names prefix0..prefix(n-1), used to pre-build the variadic
// slice for Validate so the backing array is not allocated inside the loop.
func bnNames(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s%d", prefix, i)
	}
	return out
}

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

// ── AllNamedGroups ────────────────────────────────────────────────────────────
//
// Cost model: like NamedGroups, but values are []string, so each distinct group
// costs an extra one-element slice allocation, and a repeated group name forces
// slice append/regrowth under one key — the path this function exists for.

var (
	bnANGDistinctRe = regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
	bnANGDistinctIn = "Alice 30"
	bnANGDupRe      = regexp.MustCompile(`(?P<word>\w+) (?P<word>\w+) (?P<word>\w+)`)
	bnANGDupIn      = "alpha beta gamma"
	bnANGNoMatch    = "nomatch"
	bnANGManyDupRe  = regexp.MustCompile(strings.Repeat(`(?P<w>\w+)\s*`, 20))
	bnANGManyDupIn  = strings.Repeat("x ", 20)
)

func BenchmarkAllNamedGroups(b *testing.B) {
	// distinctGroups pairs with NamedGroups/twoGroups-style input to expose the slice-per-key tax.
	benchCase(b, "distinctGroups", func() { sinkMapSS = rx.AllNamedGroups(bnANGDistinctRe, bnANGDistinctIn) })
	benchCase(b, "duplicateGroupName", func() { sinkMapSS = rx.AllNamedGroups(bnANGDupRe, bnANGDupIn) }) // 3 occurrences under one key
	benchCase(b, "noMatch", func() { sinkMapSS = rx.AllNamedGroups(bnANGDistinctRe, bnANGNoMatch) })
	benchCase(b, "manyDuplicates", func() { sinkMapSS = rx.AllNamedGroups(bnANGManyDupRe, bnANGManyDupIn) }) // 20 appends + regrowth under one key
}
