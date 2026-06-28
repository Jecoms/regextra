package regextra_test

import (
	"regexp"
	"testing"
	"time"

	rx "github.com/jecoms/regextra"
)

// Typed sinks for the decode results (assigning a struct/slice to `any` would
// box-allocate and pollute allocs/op; a typed sink stores the value directly).
var (
	sinkSimpleSlice []benchSimple
	sinkInt         int
)

// ── Compile / MustCompile ─────────────────────────────────────────────────────
//
// Cost model: regexp.Compile plus a reflect plan build — per exported field:
// parseFieldTag, SubexpIndex, an eager default= probe via setFieldValue, and
// layout= validation. For simple patterns regexp.Compile DOMINATES and hides
// the per-field work, so two isolation harnesses accompany the headline cases:
//   - regexpCompileBaseline: stdlib regexp.Compile of the SAME pattern; the
//     benchstat subtrahend that exposes the reflect/plan cost Decoder caches.
//   - fields1/10/20: a FIXED trivial pattern with varying field count, so the
//     delta across N is pure per-field plan cost with regexp.Compile constant.
// (A third, cleaner isolation that excludes regexp.Compile entirely lives in
// bench_internal_test.go, which can reach the unexported compileDecoder.)

type benchThreeDefaults struct {
	A string `regex:"a,default=x"`
	B int    `regex:"b,default=7"`
	C string `regex:"c,default=z"`
}

const benchThreeDefaultsPattern = `(?P<a>\w+) (?P<b>\d+) (?P<c>\w+)`

type benchLayoutCompile struct {
	TS time.Time `regex:"ts,layout=2006-01-02"`
}

const benchLayoutCompilePattern = `(?P<ts>\S+)`

// Fixed trivial pattern; every field maps to the one declared group so field
// count varies while regexp.Compile cost stays constant.
const benchFixedPattern = `(?P<g>\w+)`

type benchFields1 struct {
	F0 string `regex:"g"`
}

type benchFields10 struct {
	F0, F1, F2, F3, F4, F5, F6, F7, F8, F9 string `regex:"g"`
}

type benchFields20 struct {
	F0, F1, F2, F3, F4, F5, F6, F7, F8, F9 string `regex:"g"`
	G0, G1, G2, G3, G4, G5, G6, G7, G8, G9 string `regex:"g"`
}

func BenchmarkCompile(b *testing.B) {
	// representative — typical compile shapes.
	benchCase(b, "simpleStruct", func() { d, err := rx.Compile[benchSimple](benchSimplePattern); sinkAny, sinkErr = d, err })
	benchCase(b, "wideStruct", func() { d, err := rx.Compile[benchWide](benchWidePattern); sinkAny, sinkErr = d, err })
	benchCase(b, "threeDefaults", func() { d, err := rx.Compile[benchThreeDefaults](benchThreeDefaultsPattern); sinkAny, sinkErr = d, err })
	benchCase(b, "timeFieldWithLayout", func() { d, err := rx.Compile[benchLayoutCompile](benchLayoutCompilePattern); sinkAny, sinkErr = d, err })
	// isolation — subtract this from simpleStruct to get the plan-build cost.
	benchCase(b, "regexpCompileBaseline", func() { re, err := regexp.Compile(benchSimplePattern); sinkAny, sinkErr = re, err })
	// fixed-pattern field-count sweep — slope is pure per-field plan cost.
	benchCase(b, "fields1", func() { d, err := rx.Compile[benchFields1](benchFixedPattern); sinkAny, sinkErr = d, err })
	benchCase(b, "fields10", func() { d, err := rx.Compile[benchFields10](benchFixedPattern); sinkAny, sinkErr = d, err })
	benchCase(b, "fields20", func() { d, err := rx.Compile[benchFields20](benchFixedPattern); sinkAny, sinkErr = d, err })
	// symbol coverage — MustCompile's nil-check + not-taken panic over Compile.
	benchCase(b, "mustCompile", func() { sinkAny = rx.MustCompile[benchSimple](benchSimplePattern) })
}

// ── Decoder.One / .All / .Iter / .Pattern ─────────────────────────────────────
//
// The cached hot path: decode walks a precomputed field plan against the match,
// with no per-call reflect of the destination type. Decoders are compiled once
// (package-level) so the loop measures decode, not Compile. The decoded value is
// discarded (sinking it to `any` would box-allocate); b.Loop keeps the call.

var (
	benchSimpleDecoder  = rx.MustCompile[benchSimple](benchSimplePattern)
	benchTimeDecoder    = rx.MustCompile[benchTime](`(?P<ts>\S+) \((?P<took>\S+)\)`)
	benchDefaultDecoder = rx.MustCompile[benchDefault](`(?P<name>\w+)`)
	benchCustomDecoder  = rx.MustCompile[benchCustom](`\[(?P<state>\w+)\]`)
	benchWideDecoder    = rx.MustCompile[benchWide](benchWidePattern)
	benchLogDecoder     = rx.MustCompile[benchLogLine](`\[(?P<level>\w+)\] (?P<msg>[^\n]+)`)
)

func BenchmarkDecoderOne(b *testing.B) {
	// simple pairs with BenchmarkUnmarshal/simpleThreeField (identical fixtures).
	benchCase(b, "simple", func() { _, sinkErr = benchSimpleDecoder.One(benchSimpleInput) })
	benchCase(b, "noMatch", func() { _, sinkErr = benchSimpleDecoder.One(benchNoMatchInput) }) // ErrNoMatch sentinel
	benchCase(b, "withTime", func() { _, sinkErr = benchTimeDecoder.One(benchTimeFirstIn) })
	benchCase(b, "timeLastLayout", func() { _, sinkErr = benchTimeDecoder.One(benchTimeLastIn) }) // fallback-list worst case
	benchCase(b, "withDefault", func() { _, sinkErr = benchDefaultDecoder.One(benchDefaultIn) })
	benchCase(b, "customType", func() { _, sinkErr = benchCustomDecoder.One(benchCustomIn) })
	benchCase(b, "manyFields", func() { _, sinkErr = benchWideDecoder.One(benchWideInput) })
}

func BenchmarkDecoderAll(b *testing.B) {
	benchCase(b, "noMatch", func() { sinkSimpleSlice, sinkErr = benchSimpleDecoder.All(benchNoMatchInput) })
	benchCase(b, "singleMatch", func() { sinkSimpleSlice, sinkErr = benchSimpleDecoder.All(benchSimpleInput) })
	benchCase(b, "matches10", func() { sinkSimpleSlice, sinkErr = benchSimpleDecoder.All(benchBatch10Input) })
	// matches100 pairs with BenchmarkUnmarshalAll/matches100 (identical fixtures).
	benchCase(b, "matches100", func() { sinkSimpleSlice, sinkErr = benchSimpleDecoder.All(benchBatch100Input) })
	benchCase(b, "matches1000", func() { sinkSimpleSlice, sinkErr = benchSimpleDecoder.All(benchBatch1000Input) })
}

func BenchmarkDecoderIter(b *testing.B) {
	// fullIteration decodes all 100 lines (pairs with BenchmarkUnmarshalAll/realisticLogCorpus
	// in input, but skips the result-slice allocation).
	benchCase(b, "fullIteration", func() {
		n := 0
		for _, err := range benchLogDecoder.Iter(benchLogIn) {
			sinkErr = err
			n++
		}
		sinkInt = n
	})
	// earlyBreak shows the lazy-decode benefit: 100 matches found, only 5 decoded.
	benchCase(b, "earlyBreak", func() {
		n := 0
		for _, err := range benchLogDecoder.Iter(benchLogIn) {
			sinkErr = err
			n++
			if n >= 5 {
				break
			}
		}
		sinkInt = n
	})
	benchCase(b, "noMatch", func() {
		n := 0
		for _, err := range benchLogDecoder.Iter(benchNoMatchInput) {
			sinkErr = err
			n++
		}
		sinkInt = n
	})
}

func BenchmarkDecoderPattern(b *testing.B) {
	benchCase(b, "pattern", func() { sinkStr = benchSimpleDecoder.Pattern() })
}
