package regextra

import (
	"regexp"
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

func BenchmarkFindNamed(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, _ = FindNamed(benchFindRe, "Alice is 30", "name")
	}
}

func BenchmarkNamedGroups(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = NamedGroups(benchFindRe, "Alice is 30")
	}
}

func BenchmarkFindAllNamed(b *testing.B) {
	re := regexp.MustCompile(`(?P<word>\S+)`)
	target := "alpha beta gamma delta epsilon zeta eta theta"
	b.ReportAllocs()
	for b.Loop() {
		_ = FindAllNamed(re, target, "word")
	}
}

// ── Replace ───────────────────────────────────────────────────────────────────

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

type benchSimple struct {
	Name   string `regex:"name"`
	Age    int    `regex:"age"`
	Active bool   `regex:"active"`
}

var benchSimpleRe = regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+) (?P<active>\w+)`)

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

// ── Unmarshal: pointer fields ────────────────────────────────────────────────
//
// Exercises the new pointer-dispatch step (allocate-if-nil + recurse). Cost
// per field: one Kind() check + one IsNil() check + one allocation per nil
// pointer.

type benchPointers struct {
	Name *string `regex:"name"`
	Age  *int    `regex:"age"`
}

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

type benchTime struct {
	TS   time.Time     `regex:"ts"`
	Took time.Duration `regex:"took"`
}

var benchTimeRe = regexp.MustCompile(`(?P<ts>\S+) \((?P<took>\S+)\)`)

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

type benchStatus int

const (
	benchStatusUnknown benchStatus = iota
	benchStatusOpen
	benchStatusClosed
)

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

type benchCustom struct {
	State benchStatus `regex:"state"`
}

var benchCustomRe = regexp.MustCompile(`\[(?P<state>\w+)\]`)

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

func BenchmarkUnmarshalAll_logLines(b *testing.B) {
	type line struct {
		Level string `regex:"level"`
		Msg   string `regex:"msg"`
	}
	re := regexp.MustCompile(`\[(?P<level>\w+)\] (?P<msg>[^\n]+)`)
	// Build 100 lines of input
	var target string
	for i := 0; i < 100; i++ {
		target += "[info] message body here\n"
	}
	b.ReportAllocs()
	for b.Loop() {
		var lines []line
		if err := UnmarshalAll(re, target, &lines); err != nil {
			b.Fatal(err)
		}
	}
}
