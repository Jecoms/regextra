package regextra_test

import (
	"testing"
	"time"

	rx "github.com/jecoms/regextra"
)

// ── NewEncoder / MustNewEncoder ────────────────────────────────────────────────
//
// Cost model: a one-time template parse plus, per placeholder, a field resolve
// (parseFieldTag over T's fields) and an encodability check. This is the cold
// construction path — callers build one Encoder and reuse it — so it is measured
// only to keep the parse allocation-aware, not because it sits on the hot path.

func BenchmarkNewEncoder(b *testing.B) {
	benchCase(b, "simpleStruct", func() { e, err := rx.NewEncoder[benchSimple](benchEncSimpleTemplate); sinkAny, sinkErr = e, err })
	benchCase(b, "wideStruct", func() { e, err := rx.NewEncoder[benchEncWide](benchEncWideTemplate); sinkAny, sinkErr = e, err })
	benchCase(b, "mustNewEncoder", func() { sinkAny = rx.MustNewEncoder[benchSimple](benchEncSimpleTemplate) })
}

// ── Encoder.Encode ─────────────────────────────────────────────────────────────
//
// The hot path: Encode walks a precomputed segment list, rendering each field
// with encodeFieldValue and concatenating via strings.Builder — no per-call
// reflect of T's fields. Encoders are constructed once (package-level) so the
// loop measures Encode, not NewEncoder. simple pairs with
// BenchmarkDecoderOne/simple on the same fixture shape (the inverse direction).

var (
	benchEncSimpleEncoder = rx.MustNewEncoder[benchSimple](benchEncSimpleTemplate)
	benchEncTimeEncoder   = rx.MustNewEncoder[benchEncTime](`{at}`)
	benchEncWideEncoder   = rx.MustNewEncoder[benchEncWide](benchEncWideTemplate)
)

const (
	benchEncSimpleTemplate = `{name} is {age} {active}`
	benchEncWideTemplate   = `{f0} {f1} {f2} {f3} {f4} {f5} {f6} {f7} {f8} {f9}`
)

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

func BenchmarkEncoderTemplate(b *testing.B) {
	benchCase(b, "template", func() { sinkStr = benchEncSimpleEncoder.Template() })
}
