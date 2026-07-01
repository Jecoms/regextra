package regextra_test

import (
	"testing"
	"time"

	rx "github.com/jecoms/regextra"
)

// mustDeriveEncoder derives an Encoder from a compiled Decoder or panics —
// package-level construction for the benchmark fixtures.
func mustDeriveEncoder[T any](d *rx.Decoder[T]) *rx.Encoder[T] {
	e, err := d.Encoder()
	if err != nil {
		panic(err)
	}
	return e
}

// ── Decoder.Encoder ────────────────────────────────────────────────────────────
//
// Cost model: a one-time AST parse of the decoder's pattern plus, per named
// capture group, a field resolve (parseFieldTag over T's fields) and an
// encodability check. This is the cold derivation path — callers derive one
// Encoder from a Decoder and reuse it — so it is measured only to keep the walk
// allocation-aware, not because it sits on the hot path.

var (
	benchEncSimpleDecoder = rx.MustCompile[benchSimple](benchSimplePattern)
	benchEncTimeDecoder   = rx.MustCompile[benchEncTime](`(?P<at>\S+)`)
	benchEncWideDecoder   = rx.MustCompile[benchEncWide](benchEncWidePattern)
)

func BenchmarkDeriveEncoder(b *testing.B) {
	benchCase(b, "simpleStruct", func() { e, err := benchEncSimpleDecoder.Encoder(); sinkAny, sinkErr = e, err })
	benchCase(b, "wideStruct", func() { e, err := benchEncWideDecoder.Encoder(); sinkAny, sinkErr = e, err })
}

// ── Encoder.Encode ─────────────────────────────────────────────────────────────
//
// The hot path: Encode walks a precomputed segment list, rendering each field
// with encodeFieldValue and concatenating via strings.Builder — no per-call
// reflect of T's fields. Encoders are derived once (package-level) so the loop
// measures Encode, not the derivation. simple reuses benchSimple/benchSimplePattern
// so it pairs with BenchmarkDecoderOne/simple on the same fixture (the inverse
// direction).

var (
	benchEncSimpleEncoder = mustDeriveEncoder(benchEncSimpleDecoder)
	benchEncTimeEncoder   = mustDeriveEncoder(benchEncTimeDecoder)
	benchEncWideEncoder   = mustDeriveEncoder(benchEncWideDecoder)
)

// benchEncWidePattern is the invertible decode pattern the wide encoder derives
// from — ten named groups separated by single-space literals.
const benchEncWidePattern = `(?P<f0>\S+) (?P<f1>\S+) (?P<f2>\S+) (?P<f3>\S+) (?P<f4>\S+) (?P<f5>\S+) (?P<f6>\S+) (?P<f7>\S+) (?P<f8>\S+) (?P<f9>\S+)`

type benchEncTime struct {
	At time.Time `regex:"at"`
}

type benchEncWide struct {
	F0 string `regex:"f0"`
	F1 string `regex:"f1"`
	F2 string `regex:"f2"`
	F3 string `regex:"f3"`
	F4 string `regex:"f4"`
	F5 string `regex:"f5"`
	F6 string `regex:"f6"`
	F7 string `regex:"f7"`
	F8 string `regex:"f8"`
	F9 string `regex:"f9"`
}

var (
	benchEncSimpleVal = benchSimple{Name: "Alice", Age: 30, Active: true}
	benchEncTimeVal   = benchEncTime{At: time.Date(2024, 3, 2, 15, 4, 5, 0, time.UTC)}
	benchEncWideVal   = benchEncWide{
		F0: "a", F1: "b", F2: "c", F3: "d", F4: "e",
		F5: "f", F6: "g", F7: "h", F8: "i", F9: "j",
	}
)

func BenchmarkEncode(b *testing.B) {
	benchCase(b, "simple", func() { sinkStr, sinkErr = benchEncSimpleEncoder.Encode(benchEncSimpleVal) })
	benchCase(b, "withTime", func() { sinkStr, sinkErr = benchEncTimeEncoder.Encode(benchEncTimeVal) })
	benchCase(b, "manyFields", func() { sinkStr, sinkErr = benchEncWideEncoder.Encode(benchEncWideVal) })
}
