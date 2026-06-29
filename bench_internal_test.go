package regextra

import (
	"regexp"
	"testing"
)

// Internal benchmark (package regextra, not regextra_test) so it can call the
// unexported compileDecoder. This is the cleanest isolation of the compile-side
// work Decoder caching eliminates: regexp.Compile is done ONCE before the loop
// (package-level bnInternalRe), so the loop measures only reflect.TypeOf + the
// per-field plan build (parseFieldTag, SubexpIndex, eager default probe).
//
// Compare against BenchmarkCompile/regexpCompileBaseline (the regexp.Compile
// cost) and BenchmarkCompile/simpleStruct (the two combined) in
// bench_decoder_test.go.

type bnInternalSimple struct {
	Name   string `regex:"name"`
	Age    int    `regex:"age"`
	Active bool   `regex:"active"`
}

const bnInternalPattern = `(?P<name>\w+) is (?P<age>\d+) (?P<active>\w+)`

var bnInternalRe = regexp.MustCompile(bnInternalPattern)

func BenchmarkCompilePlanOnly(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		d, err := compileDecoder[bnInternalSimple](bnInternalPattern, bnInternalRe)
		if err != nil {
			b.Fatal(err)
		}
		_ = d
	}
}
