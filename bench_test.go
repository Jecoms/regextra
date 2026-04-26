package regextra

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

// Benchmarks for the hot paths users actually call. The goal is twofold:
//
//  1. Establish a baseline against which v0.5.0's Decoder[T] (re-3e2) can
//     demonstrate its caching win.
//  2. Catch regressions on the existing free-function path. v0.4.0 added four
//     dispatch checks at the top of setFieldValue (pointer, RegexUnmarshaler,
//     time.Time, time.Duration) — keep the absolute cost visible so future
//     additions are vetted against a budget rather than guessed about.
//
// Run with:
//
//	go test -bench=. -benchmem -run=^$ ./...

// ── Find / NamedGroups baseline ───────────────────────────────────────────────

var benchFindRe = regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)

// BenchmarkFindNamed measures the single-group, single-match extraction path —
// the cheapest function in the package and the baseline for everything else.
func BenchmarkFindNamed(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, _ = FindNamed(benchFindRe, "Alice is 30", "name")
	}
}

// BenchmarkNamedGroups measures the all-groups-as-map path. The map allocation
// dominates the cost vs. FindNamed.
func BenchmarkNamedGroups(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = NamedGroups(benchFindRe, "Alice is 30")
	}
}

// BenchmarkFindAllNamed measures the all-matches projection of one named group.
// Allocation scales with match count.
func BenchmarkFindAllNamed(b *testing.B) {
	re := regexp.MustCompile(`(?P<word>\S+)`)
	target := "alpha beta gamma delta epsilon zeta eta theta"
	b.ReportAllocs()
	for b.Loop() {
		_ = FindAllNamed(re, target, "word")
	}
}

// ── Replace ───────────────────────────────────────────────────────────────────

// BenchmarkReplace measures the substitution path: scan all matches, sort
// per-match group spans, build the output string. Three matches in the input
// to exercise the multi-match branch.
func BenchmarkReplace(b *testing.B) {
	re := regexp.MustCompile(`(?P<user>\w+)@(?P<domain>[\w.]+)`)
	target := "alice@example.com bob@other.org carol@third.net"
	repl := map[string]string{"domain": "redacted"}
	b.ReportAllocs()
	for b.Loop() {
		_ = Replace(re, target, repl)
	}
}

// ── Unmarshal: simple struct (string + int + bool) ───────────────────────────
//
// Baseline for the most common shape. Hits the kind switch path with no
// pointer / RegexUnmarshaler / time.Time fast-paths — measures the cost of
// the four dispatch checks added in v0.4.0 against a "boring" struct.

// benchSimple is a 3-field destination struct using only the kind-switch
// branches of setFieldValue: string, int, bool. The benchmark measures that
// path's per-call cost.
type benchSimple struct {
	Name   string `regex:"name"`
	Age    int    `regex:"age"`
	Active bool   `regex:"active"`
}

var benchSimpleRe = regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+) (?P<active>\w+)`)

// BenchmarkUnmarshal_simpleStruct measures the kind-switch path with no fast
// paths hit. Establishes the baseline that Decoder[T] should beat.
func BenchmarkUnmarshal_simpleStruct(b *testing.B) {
	target := "Alice is 30 true"
	b.ReportAllocs()
	for b.Loop() {
		var s benchSimple
		if err := Unmarshal(benchSimpleRe, target, &s); err != nil {
			b.Fatal(err)
		}
	}
}

// benchSimpleDecoder is the same struct decoded via a precompiled Decoder.
// Compare against BenchmarkUnmarshal_simpleStruct to see the v0.5.0 win.
var benchSimpleDecoder = MustCompile[benchSimple](`(?P<name>\w+) is (?P<age>\d+) (?P<active>\w+)`)

// BenchmarkDecoder_simpleStruct.One measures the same shape as
// BenchmarkUnmarshal_simpleStruct via a precompiled Decoder. Both reuse
// setFieldValue, but the Decoder skips the per-call SubexpNames loop and
// per-field parseFieldTag work — that's where the win comes from.
func BenchmarkDecoder_simpleStruct(b *testing.B) {
	target := "Alice is 30 true"
	b.ReportAllocs()
	for b.Loop() {
		_, err := benchSimpleDecoder.One(target)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ── Unmarshal: pointer fields ────────────────────────────────────────────────
//
// Exercises the new pointer-dispatch step (allocate-if-nil + recurse). Cost
// per field: one Kind() check + one IsNil() check + one allocation per nil
// pointer.

// benchPointers exercises the pointer dispatch step (allocate-if-nil + recurse).
type benchPointers struct {
	Name *string `regex:"name"`
	Age  *int    `regex:"age"`
}

// BenchmarkUnmarshal_pointerFields measures the cost of the pointer dispatch
// step plus the per-field allocation that nil pointers incur.
func BenchmarkUnmarshal_pointerFields(b *testing.B) {
	target := "Alice is 30"
	re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
	b.ReportAllocs()
	for b.Loop() {
		var p benchPointers
		if err := Unmarshal(re, target, &p); err != nil {
			b.Fatal(err)
		}
	}
}

// ── Unmarshal: time.Time + time.Duration ─────────────────────────────────────
//
// Hits the time-types fast path (Type-equality match before the Kind switch).
// The fallback time.Parse loop runs only on first-layout-miss; this benchmark
// uses RFC3339 input so the first layout matches.

// benchTime exercises the time-types fast paths (Type-equality match before
// the kind switch).
type benchTime struct {
	TS   time.Time     `regex:"ts"`
	Took time.Duration `regex:"took"`
}

var benchTimeRe = regexp.MustCompile(`(?P<ts>\S+) \((?P<took>\S+)\)`)

// BenchmarkUnmarshal_timeFields measures the time.Time + time.Duration fast
// paths. Input is RFC3339 so the first layout in the fallback list matches —
// worst-case (last-layout-wins) is not measured here.
func BenchmarkUnmarshal_timeFields(b *testing.B) {
	target := "2026-04-26T12:34:56Z (1h30m)"
	b.ReportAllocs()
	for b.Loop() {
		var t benchTime
		if err := Unmarshal(benchTimeRe, target, &t); err != nil {
			b.Fatal(err)
		}
	}
}

// ── Unmarshal: RegexUnmarshaler-implementing field ───────────────────────────
//
// Exercises the interface-dispatch step. The test type is intentionally simple
// so the benchmark measures dispatch cost, not the body of UnmarshalRegex.

// benchStatus is a caller-defined enum whose pointer satisfies
// RegexUnmarshaler. Used to measure the interface-dispatch step.
type benchStatus int

const (
	benchStatusUnknown benchStatus = iota
	benchStatusOpen
	benchStatusClosed
)

// UnmarshalRegex maps the matched string to a benchStatus value. Body kept
// minimal so the benchmark measures dispatch cost, not parse cost.
func (s *benchStatus) UnmarshalRegex(value string) error {
	switch value {
	case "open":
		*s = benchStatusOpen
	case "closed":
		*s = benchStatusClosed
	default:
		*s = benchStatusUnknown
	}
	return nil
}

// benchCustom is the destination struct for the RegexUnmarshaler benchmark.
type benchCustom struct {
	State benchStatus `regex:"state"`
}

var benchCustomRe = regexp.MustCompile(`\[(?P<state>\w+)\]`)

// BenchmarkUnmarshal_customUnmarshaler measures the RegexUnmarshaler interface
// dispatch step in setFieldValue. The custom UnmarshalRegex body is trivial so
// the timing reflects dispatch overhead rather than user-defined work.
func BenchmarkUnmarshal_customUnmarshaler(b *testing.B) {
	target := "[open]"
	b.ReportAllocs()
	for b.Loop() {
		var c benchCustom
		if err := Unmarshal(benchCustomRe, target, &c); err != nil {
			b.Fatal(err)
		}
	}
}

// ── UnmarshalAll: 100 log-line iteration ─────────────────────────────────────
//
// The realistic hot path for log parsers. Measures per-line amortized cost
// across a batch — the metric Decoder[T] (re-3e2) is designed to improve.

// BenchmarkUnmarshalAll_logLines measures amortized per-line decode cost
// across a 100-line batch. This is the hot path Decoder[T] (re-3e2) is
// designed to optimize — each iteration today rebuilds the reflect plan
// from scratch.
func BenchmarkUnmarshalAll_logLines(b *testing.B) {
	type line struct {
		Level string `regex:"level"`
		Msg   string `regex:"msg"`
	}
	re := regexp.MustCompile(`\[(?P<level>\w+)\] (?P<msg>[^\n]+)`)
	// 100 lines of input
	target := strings.Repeat("[info] message body here\n", 100)
	b.ReportAllocs()
	for b.Loop() {
		var lines []line
		if err := UnmarshalAll(re, target, &lines); err != nil {
			b.Fatal(err)
		}
	}
}
