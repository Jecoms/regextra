package regextra_test

import (
	"regexp"
	"strings"
	"testing"
	"time"

	rx "github.com/jecoms/regextra"
)

// ── Shared decode fixtures ────────────────────────────────────────────────────
//
// These types and inputs are shared by the Unmarshal/UnmarshalAll benchmarks
// here and the Compile/Decoder benchmarks in bench_decoder_test.go. The
// head-to-head pairs MUST stay byte-identical so benchstat can diff the caching
// win: Unmarshal/simpleThreeField vs DecoderOne/simple share benchSimple +
// benchSimplePattern + benchSimpleInput; UnmarshalAll/matches100 vs
// DecoderAll/matches100 share benchSimple + benchBatch100Input.

// benchSimple — the canonical 3-field destination (string + int + bool).
type benchSimple struct {
	Name   string `regex:"name"`
	Age    int    `regex:"age"`
	Active bool   `regex:"active"`
}

const benchSimplePattern = `(?P<name>\w+) is (?P<age>\d+) (?P<active>\w+)`

var (
	benchSimpleRe    = regexp.MustCompile(benchSimplePattern)
	benchSimpleInput = "Alice is 30 true"
	benchSimpleVal   benchSimple // passed by value to hit the non-pointer guard

	// Each "Alice is 30 true and " segment is exactly one match of benchSimpleRe.
	benchBatch10Input   = strings.Repeat("Alice is 30 true and ", 10)
	benchBatch100Input  = strings.Repeat("Alice is 30 true and ", 100)
	benchBatch1000Input = strings.Repeat("Alice is 30 true and ", 1000)
	benchNoMatchInput   = "no matches here"
)

// benchWide — a 20-field destination for the per-field overhead sweep.
type benchWide struct {
	F0  string `regex:"f0"`
	F1  string `regex:"f1"`
	F2  string `regex:"f2"`
	F3  string `regex:"f3"`
	F4  string `regex:"f4"`
	F5  string `regex:"f5"`
	F6  string `regex:"f6"`
	F7  string `regex:"f7"`
	F8  string `regex:"f8"`
	F9  string `regex:"f9"`
	F10 string `regex:"f10"`
	F11 string `regex:"f11"`
	F12 string `regex:"f12"`
	F13 string `regex:"f13"`
	F14 string `regex:"f14"`
	F15 string `regex:"f15"`
	F16 string `regex:"f16"`
	F17 string `regex:"f17"`
	F18 string `regex:"f18"`
	F19 string `regex:"f19"`
}

var (
	benchWidePattern    = bnGroupsPattern(20)
	benchWideRe         = regexp.MustCompile(benchWidePattern)
	benchWideInput      = bnWords(20)
	benchWideMultiInput = strings.Repeat(bnWords(20)+"\n", 50) // 50 matches
)

// benchTime — exercises the time.Time fast path; first vs last layout in the
// fallback list is the best/worst time-parse pair.
type benchTime struct {
	TS   time.Time     `regex:"ts"`
	Took time.Duration `regex:"took"`
}

var (
	benchTimeRe      = regexp.MustCompile(`(?P<ts>\S+) \((?P<took>\S+)\)`)
	benchTimeFirstIn = "2026-04-26T12:34:56Z (1h30m)" // RFC3339Nano (layout 0) parses
	benchTimeLastIn  = "12:34:56 (1h30m)"             // only TimeOnly (last layout) parses
)

// benchNumeric — int/uint/float/bool conversions in one struct.
type benchNumeric struct {
	I int     `regex:"i"`
	U uint    `regex:"u"`
	F float64 `regex:"f"`
	B bool    `regex:"b"`
}

var (
	benchNumericRe = regexp.MustCompile(`(?P<i>-?\d+) (?P<u>\d+) (?P<f>[\d.]+) (?P<b>\w+)`)
	benchNumericIn = "-42 100 3.14 true"
)

// benchEmptyMatch — a group that matches an empty span (no default).
type benchEmptyMatch struct {
	Title string `regex:"title"`
}

var (
	benchEmptyRe = regexp.MustCompile(`x(?P<title>\w*)x`)
	benchEmptyIn = "xx"
)

// benchPointers — pointer fields (allocate-if-nil then recurse to builtin).
type benchPointers struct {
	Name *string `regex:"name"`
	Age  *int    `regex:"age"`
}

var (
	benchPtrRe = regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
	benchPtrIn = "Alice is 30"
)

// benchCaseInsensitive — untagged fields resolved via the case-insensitive
// field-name fallback (matchGroupName, run once when the decode plan is built).
type benchCaseInsensitive struct {
	Name string
	Age  int
}

var (
	benchCIRe = regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
	benchCIIn = "Alice is 30"
)

// benchDefault — group not declared, value supplied by default=.
type benchDefault struct {
	Role string `regex:"role,default=guest"`
}

var (
	benchDefaultRe = regexp.MustCompile(`(?P<name>\w+)`)
	benchDefaultIn = "Alice"
)

// RegexUnmarshaler fixtures covering the three setFieldValue dispatch branches:
//   - benchCustom    : non-pointer field, dispatched via field.Addr() (line ~353)
//   - benchCustomPtr : pointer field whose *T implements it, dispatched without
//     recursing into the pointee (line ~343)
//   - benchCustomVal : a value-receiver implementer; still dispatched via the
//     addr branch because *T is also in the method set (line ~358's
//     Type().Implements path is unreachable for addressable struct fields — a
//     candidate for the issue #109 dead-code cleanup).
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

type benchCustomPtr struct {
	State *benchStatus `regex:"state"`
}

type benchValStatus int

func (s benchValStatus) UnmarshalRegex(string) error { return nil } // value receiver

type benchCustomVal struct {
	State benchValStatus `regex:"state"`
}

var (
	benchCustomRe = regexp.MustCompile(`\[(?P<state>\w+)\]`)
	benchCustomIn = "[open]"
)

// benchLayout — custom layout= on a time.Time field (no fallback list).
type benchLayout struct {
	TS time.Time `regex:"ts,layout=02/Jan/2006:15:04:05 -0700"`
}

var (
	benchLayoutRe = regexp.MustCompile(`(?P<ts>.+)`)
	benchLayoutIn = "26/Apr/2026:12:34:56 +0000"
)

// benchDuration — time.Duration conversion in isolation.
type benchDuration struct {
	D time.Duration `regex:"d"`
}

var (
	benchDurRe = regexp.MustCompile(`(?P<d>\S+)`)
	benchDurIn = "1h30m"
)

// benchBadInt — strconv.ParseInt error path (matched group, bad value).
type benchBadInt struct {
	Age int `regex:"age"`
}

var (
	benchBadIntRe = regexp.MustCompile(`(?P<age>\w+)`)
	benchBadIntIn = "notanumber"
)

// benchUnsupported — the setFieldValue default-case error (unsupported kind).
type benchUnsupported struct {
	C complex128 `regex:"c"`
}

var (
	benchUnsupRe = regexp.MustCompile(`(?P<c>\S+)`)
	benchUnsupIn = "1+2i"
)

// benchIntRow / benchLogLine — UnmarshalAll scaling corpora.
type benchIntRow struct {
	N int `regex:"n"`
}

var (
	benchIntRowRe = regexp.MustCompile(`(?P<n>\d+)`)
	benchIntRowIn = strings.TrimSpace(strings.Repeat("123 ", 1000)) // 1000 int rows
)

type benchLogLine struct {
	Level string `regex:"level"`
	Msg   string `regex:"msg"`
}

var (
	benchLogRe = regexp.MustCompile(`\[(?P<level>\w+)\] (?P<msg>[^\n]+)`)
	benchLogIn = strings.Repeat("[info] message body here\n", 100) // 100 log lines
)

// ── Unmarshal ─────────────────────────────────────────────────────────────────
//
// Cost model per call: argument-validation guard (reflect.Kind checks),
// FindStringSubmatchIndex, then buildDecodePlan once (parseFieldTag +
// group-index resolution per field) and runDecodePlan iterating the plan to
// call setFieldValue. setFieldValue dispatches: pointer → RegexUnmarshaler
// (addr / value-receiver) → time.Time / time.Duration → kind switch
// (string/int/uint/float/bool) → unsupported-type error.

func BenchmarkUnmarshal(b *testing.B) {
	// representative — the common shapes.
	benchCase(b, "simpleThreeField", func() { var s benchSimple; sinkErr = rx.Unmarshal(benchSimpleRe, benchSimpleInput, &s) })
	benchCase(b, "allNumericTypes", func() { var s benchNumeric; sinkErr = rx.Unmarshal(benchNumericRe, benchNumericIn, &s) })
	benchCase(b, "customUnmarshaler", func() { var s benchCustom; sinkErr = rx.Unmarshal(benchCustomRe, benchCustomIn, &s) })
	benchCase(b, "timeDurationField", func() { var s benchDuration; sinkErr = rx.Unmarshal(benchDurRe, benchDurIn, &s) })
	// time best/worst pair — first layout hit vs full fallback-list walk.
	benchCase(b, "timeFirstLayout", func() { var s benchTime; sinkErr = rx.Unmarshal(benchTimeRe, benchTimeFirstIn, &s) })
	benchCase(b, "timeLastLayout", func() { var s benchTime; sinkErr = rx.Unmarshal(benchTimeRe, benchTimeLastIn, &s) })
	benchCase(b, "layoutCustomTime", func() { var s benchLayout; sinkErr = rx.Unmarshal(benchLayoutRe, benchLayoutIn, &s) })
	// edge — fallback resolution paths and the no-match contract shape.
	benchCase(b, "noMatch", func() { var s benchSimple; sinkErr = rx.Unmarshal(benchSimpleRe, benchNoMatchInput, &s) })
	benchCase(b, "emptyGroupMatch", func() { var s benchEmptyMatch; sinkErr = rx.Unmarshal(benchEmptyRe, benchEmptyIn, &s) })
	benchCase(b, "caseInsensitiveFallback", func() { var s benchCaseInsensitive; sinkErr = rx.Unmarshal(benchCIRe, benchCIIn, &s) })
	benchCase(b, "defaultValueFallback", func() { var s benchDefault; sinkErr = rx.Unmarshal(benchDefaultRe, benchDefaultIn, &s) })
	// setFieldValue dispatch-branch coverage.
	benchCase(b, "pointerFieldNil", func() { var s benchPointers; sinkErr = rx.Unmarshal(benchPtrRe, benchPtrIn, &s) })
	benchCase(b, "pointerImplementsUnmarshaler", func() { var s benchCustomPtr; sinkErr = rx.Unmarshal(benchCustomRe, benchCustomIn, &s) })
	benchCase(b, "valueReceiverUnmarshaler", func() { var s benchCustomVal; sinkErr = rx.Unmarshal(benchCustomRe, benchCustomIn, &s) })
	// pointerFieldPreallocated needs non-nil pointers carried across iterations,
	// so it can't share fresh-struct-per-call; written inline.
	b.Run("pointerFieldPreallocated", func(b *testing.B) {
		var p benchPointers
		p.Name = new(string)
		p.Age = new(int)
		b.ReportAllocs()
		for b.Loop() {
			sinkErr = rx.Unmarshal(benchPtrRe, benchPtrIn, &p)
		}
	})
	// scaling — per-field overhead with a 20-field struct.
	benchCase(b, "wideStruct", func() { var s benchWide; sinkErr = rx.Unmarshal(benchWideRe, benchWideInput, &s) })
	// error / guard paths — sink the error, do not fatal (the error IS the path).
	benchCase(b, "invalidIntConversion", func() { var s benchBadInt; sinkErr = rx.Unmarshal(benchBadIntRe, benchBadIntIn, &s) })
	benchCase(b, "unsupportedFieldType", func() { var s benchUnsupported; sinkErr = rx.Unmarshal(benchUnsupRe, benchUnsupIn, &s) })
	benchCase(b, "invalidPointerArg", func() { sinkErr = rx.Unmarshal(benchSimpleRe, benchSimpleInput, benchSimpleVal) }) // non-pointer guard
}

// ── UnmarshalAll ──────────────────────────────────────────────────────────────
//
// Cost model: FindAllStringSubmatchIndex (whole-target scan + index matrix),
// buildDecodePlan once for the call, then per match a runDecodePlan pass into a
// pre-sized slice (reflect.MakeSlice). The plan's group indexes and parsed
// options are reused across matches, so per-match cost is just the index walk
// and per-field conversion. Scales with match count and per-match field work.

func BenchmarkUnmarshalAll(b *testing.B) {
	// edge / no-match contract shape (slice length set to 0).
	benchCase(b, "noMatch", func() { var dst []benchSimple; sinkErr = rx.UnmarshalAll(benchSimpleRe, benchNoMatchInput, &dst) })
	benchCase(b, "singleMatch", func() { var dst []benchSimple; sinkErr = rx.UnmarshalAll(benchSimpleRe, benchSimpleInput, &dst) })
	// match-count scaling (matches100 aligns with DecoderAll/matches100).
	benchCase(b, "matches10", func() { var dst []benchSimple; sinkErr = rx.UnmarshalAll(benchSimpleRe, benchBatch10Input, &dst) })
	benchCase(b, "matches100", func() { var dst []benchSimple; sinkErr = rx.UnmarshalAll(benchSimpleRe, benchBatch100Input, &dst) })
	benchCase(b, "matches1000", func() { var dst []benchSimple; sinkErr = rx.UnmarshalAll(benchSimpleRe, benchBatch1000Input, &dst) })
	// per-field width and per-row conversion cost.
	benchCase(b, "wideStruct", func() { var dst []benchWide; sinkErr = rx.UnmarshalAll(benchWideRe, benchWideMultiInput, &dst) })
	benchCase(b, "manyIntConversions", func() { var dst []benchIntRow; sinkErr = rx.UnmarshalAll(benchIntRowRe, benchIntRowIn, &dst) })
	benchCase(b, "realisticLogCorpus", func() { var dst []benchLogLine; sinkErr = rx.UnmarshalAll(benchLogRe, benchLogIn, &dst) })
	// guard path — pointer to non-slice.
	benchCase(b, "invalidSliceArg", func() { sinkErr = rx.UnmarshalAll(benchSimpleRe, benchSimpleInput, &benchSimpleVal) })
}
