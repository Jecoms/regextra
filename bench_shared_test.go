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
	"strings"
	"testing"
)

// ── Typed sinks ───────────────────────────────────────────────────────────────
// One per return kind, assigned in every loop body to defeat dead-store
// elimination of the benchmarked call's result.
var (
	sinkStr      string
	sinkOK       bool
	sinkStrs     []string
	sinkMap      map[string]string
	sinkMapSS    map[string][]string
	sinkSliceMap []map[string]string
	sinkErr      error
	sinkAny      any
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
