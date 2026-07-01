package regextra_test

import (
	"encoding"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	rx "github.com/jecoms/regextra"
)

// mustEncoder derives an Encoder from a compiled Decoder or fails the test — the
// happy-path constructor for the derive-from-decoder API.
func mustEncoder[T any](t *testing.T, pattern string) *rx.Encoder[T] {
	t.Helper()
	e, err := rx.MustCompile[T](pattern).Encoder()
	if err != nil {
		t.Fatalf("Encoder() from %q returned %v", pattern, err)
	}
	return e
}

// ── Decoder.Encoder: derivation happy paths ────────────────────────────────────

func TestEncoder_simplePattern(t *testing.T) {
	type P struct {
		Name string `regex:"name"`
		Age  int    `regex:"age"`
	}
	e := mustEncoder[P](t, `(?P<name>\S+) is (?P<age>\d+)`)
	got, err := e.Encode(P{Name: "Alice", Age: 30})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "Alice is 30" {
		t.Errorf("Encode = %q, want %q", got, "Alice is 30")
	}
}

func TestEncoder_fieldNameFallback(t *testing.T) {
	type P struct {
		Name string // no tag — resolved by field name
		Age  int
	}
	e := mustEncoder[P](t, `(?P<Name>\S+) is (?P<Age>\d+)`)
	got, err := e.Encode(P{Name: "Bob", Age: 25})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "Bob is 25" {
		t.Errorf("Encode = %q, want %q", got, "Bob is 25")
	}
}

func TestEncoder_caseInsensitiveFallback(t *testing.T) {
	type P struct {
		Name string
	}
	// lower-case group folds onto the exported field name.
	e := mustEncoder[P](t, `hi (?P<name>\S+)`)
	got, err := e.Encode(P{Name: "Carol"})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "hi Carol" {
		t.Errorf("Encode = %q, want %q", got, "hi Carol")
	}
}

func TestEncoder_exactBeatsFold(t *testing.T) {
	// An exact-name field must win over an earlier fold sibling. NAME carries a
	// default so Compile tolerates its (undeclared) "NAME" group; the pattern
	// declares only "name", which must resolve to the exact-match Name field.
	type P struct {
		NAME string `regex:"NAME,default=x"`
		Name string `regex:"name"`
	}
	e := mustEncoder[P](t, `(?P<name>\S+)`)
	got, err := e.Encode(P{NAME: "upper", Name: "lower"})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "lower" {
		t.Errorf("Encode = %q, want the exact-match field value %q", got, "lower")
	}
}

func TestEncoder_literalBraces(t *testing.T) {
	// Braces are now ordinary regex literals (the old `{{`/`}}` template escape
	// is gone). `\{`/`\}` in the pattern emit literal braces around the value.
	type P struct {
		V string `regex:"v"`
	}
	e := mustEncoder[P](t, `\{ (?P<v>\S+) \}`)
	got, err := e.Encode(P{V: "x"})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "{ x }" {
		t.Errorf("Encode = %q, want %q", got, "{ x }")
	}
}

func TestEncoder_literalOnly(t *testing.T) {
	type P struct{ V string }
	e := mustEncoder[P](t, `no groups here`)
	got, err := e.Encode(P{V: "ignored"})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "no groups here" {
		t.Errorf("Encode = %q", got)
	}
}

func TestEncoder_anchorsDropped(t *testing.T) {
	// Anchors and word boundaries match no text and are dropped from the plan;
	// what remains re-emits the literal payload without them.
	type P struct {
		V string `regex:"v"`
	}
	e := mustEncoder[P](t, `^\bgo (?P<v>\S+)\b$`)
	got, err := e.Encode(P{V: "lang"})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "go lang" {
		t.Errorf("Encode = %q, want %q", got, "go lang")
	}
}

func TestEncoder_literalUnnamedGroup(t *testing.T) {
	// An unnamed group whose body reduces to pure literal text (here a
	// concatenation of literals with a dropped word-boundary between them) is
	// treated as that literal.
	type P struct {
		V string `regex:"v"`
	}
	e := mustEncoder[P](t, `(a\bc)(?P<v>\d+)`)
	got, err := e.Encode(P{V: "7"})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "ac7" {
		t.Errorf("Encode = %q, want %q", got, "ac7")
	}
}

// ── Encode: type coverage ──────────────────────────────────────────────────────

func TestEncode_scalarTypes(t *testing.T) {
	type Wide struct {
		S   string  `regex:"s"`
		I   int     `regex:"i"`
		I8  int8    `regex:"i8"`
		U   uint    `regex:"u"`
		U16 uint16  `regex:"u16"`
		F32 float32 `regex:"f32"`
		F64 float64 `regex:"f64"`
		B   bool    `regex:"b"`
	}
	e := mustEncoder[Wide](t, `(?P<s>\S+):(?P<i>\S+):(?P<i8>\S+):(?P<u>\S+):(?P<u16>\S+):(?P<f32>\S+):(?P<f64>\S+):(?P<b>\S+)`)
	got, err := e.Encode(Wide{S: "hi", I: -7, I8: -128, U: 9, U16: 65535, F32: 1.5, F64: 2.25, B: true})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	want := "hi:-7:-128:9:65535:1.5:2.25:true"
	if got != want {
		t.Errorf("Encode = %q, want %q", got, want)
	}
}

func TestEncode_zeroValues(t *testing.T) {
	type Z struct {
		S string `regex:"s"`
		I int    `regex:"i"`
		B bool   `regex:"b"`
	}
	e := mustEncoder[Z](t, `\[(?P<s>\S*)\]\[(?P<i>\S+)\]\[(?P<b>\S+)\]`)
	got, err := e.Encode(Z{})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "[][0][false]" {
		t.Errorf("Encode = %q, want %q", got, "[][0][false]")
	}
}

func TestEncode_timeCanonicalLayout(t *testing.T) {
	type Ev struct {
		At time.Time `regex:"at"`
	}
	e := mustEncoder[Ev](t, `(?P<at>\S+)`)
	ts := time.Date(2024, 3, 2, 15, 4, 5, 0, time.UTC)
	got, err := e.Encode(Ev{At: ts})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "2024-03-02T15:04:05Z" {
		t.Errorf("Encode = %q, want RFC3339 %q", got, "2024-03-02T15:04:05Z")
	}
}

func TestEncode_timeLayoutOption(t *testing.T) {
	type Ev struct {
		At time.Time `regex:"at,layout=2006-01-02"`
	}
	e := mustEncoder[Ev](t, `(?P<at>\S+)`)
	got, err := e.Encode(Ev{At: time.Date(2024, 3, 2, 15, 4, 5, 0, time.UTC)})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "2024-03-02" {
		t.Errorf("Encode = %q, want %q", got, "2024-03-02")
	}
}

// The `layout=` tag drives both directions from one field declaration: Encode
// formats with it and Decoder parses with it, so a non-RFC3339 timestamp
// round-trips symmetrically.
func TestEncode_timeLayoutSymmetry(t *testing.T) {
	type Ev struct {
		At time.Time `regex:"at,layout=2006-01-02 15:04:05"`
	}
	pattern := `at=(?P<at>[\d :-]+)`
	dec := rx.MustCompile[Ev](pattern)
	enc, err := dec.Encoder()
	if err != nil {
		t.Fatalf("Encoder() returned %v", err)
	}
	want := Ev{At: time.Date(2024, 3, 2, 15, 4, 5, 0, time.UTC)}
	s, err := enc.Encode(want)
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if s != "at=2024-03-02 15:04:05" {
		t.Errorf("Encode = %q, want %q", s, "at=2024-03-02 15:04:05")
	}
	got, err := dec.One(s)
	if err != nil {
		t.Fatalf("decode of %q returned %v", s, err)
	}
	if !got.At.Equal(want.At) {
		t.Errorf("round-trip time = %v, want %v (via %q)", got.At, want.At, s)
	}
}

func TestEncode_duration(t *testing.T) {
	type D struct {
		Took time.Duration `regex:"took"`
	}
	e := mustEncoder[D](t, `(?P<took>\S+)`)
	got, err := e.Encode(D{Took: 90 * time.Minute})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "1h30m0s" {
		t.Errorf("Encode = %q, want %q", got, "1h30m0s")
	}
}

func TestEncode_pointerField(t *testing.T) {
	type P struct {
		Age *int `regex:"age"`
	}
	n := 42
	e := mustEncoder[P](t, `(?P<age>\d+)`)
	got, err := e.Encode(P{Age: &n})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "42" {
		t.Errorf("Encode = %q, want %q", got, "42")
	}
}

func TestEncode_nilPointerErrors(t *testing.T) {
	type P struct {
		Age *int `regex:"age"`
	}
	e := mustEncoder[P](t, `(?P<age>\d+)`)
	_, err := e.Encode(P{})
	if err == nil {
		t.Fatal("Encode of nil pointer field returned nil, want error")
	}
	var ee *rx.EncodeError
	if !errors.As(err, &ee) {
		t.Fatalf("error %v is not an *EncodeError", err)
	}
	if ee.Field != "Age" || ee.Group != "age" {
		t.Errorf("EncodeError = %+v, want Field=Age Group=age", ee)
	}
}

// On a fold match — an untagged field bound to a differently-cased group name —
// EncodeError.Group carries the declared group name, not the field name. Mirrors
// the fold-case assertion #142 added on the decode side and pins the Group-godoc
// contract. Only untagged field names fold; an explicit `regex:` tag is matched
// exactly (see resolveEncodeField), so the fold path is reached via the field
// name here.
func TestEncode_foldMatchGroupName(t *testing.T) {
	type P struct {
		Age *int // untagged; folds to the group AGE
	}
	e := mustEncoder[P](t, `(?P<AGE>\d+)`)
	_, err := e.Encode(P{}) // nil pointer -> EncodeError
	if err == nil {
		t.Fatal("Encode of nil pointer field returned nil, want error")
	}
	var ee *rx.EncodeError
	if !errors.As(err, &ee) {
		t.Fatalf("error %v is not an *EncodeError", err)
	}
	if ee.Field != "Age" || ee.Group != "AGE" {
		t.Errorf("EncodeError = %+v, want Field=Age Group=AGE (declared group name, not field name)", ee)
	}
}

// A field whose static type is an interface implementing encoding.TextMarshaler
// passes encodableType at construction, but a nil value has no concrete value to
// render. Encode must surface a nil-style *EncodeError (mirroring the nil-pointer
// path) rather than falling through to the kind-switch default with a misleading
// "unsupported field type: interface".
func TestEncode_nilInterfaceErrors(t *testing.T) {
	type M struct {
		V encoding.TextMarshaler `regex:"v"`
	}
	e := mustEncoder[M](t, `(?P<v>\S+)`)
	_, err := e.Encode(M{}) // V is a nil interface
	if err == nil {
		t.Fatal("Encode of nil interface field returned nil, want error")
	}
	var ee *rx.EncodeError
	if !errors.As(err, &ee) {
		t.Fatalf("error %v is not an *EncodeError", err)
	}
	if ee.Field != "V" || ee.Group != "v" {
		t.Errorf("EncodeError = %+v, want Field=V Group=v", ee)
	}
	if strings.Contains(err.Error(), "unsupported field type") {
		t.Errorf("nil interface produced fallthrough error %q, want nil-style message", err.Error())
	}
	if !strings.Contains(err.Error(), "nil interface") {
		t.Errorf("error %q does not mention nil interface", err.Error())
	}
}

// ── Encode: TextMarshaler ──────────────────────────────────────────────────────

func TestEncode_textMarshaler(t *testing.T) {
	type Host struct {
		Addr netip.Addr `regex:"addr"`
	}
	e := mustEncoder[Host](t, `(?P<addr>\S+)`)
	got, err := e.Encode(Host{Addr: netip.MustParseAddr("192.168.0.1")})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "192.168.0.1" {
		t.Errorf("Encode = %q, want %q", got, "192.168.0.1")
	}
}

// ── Encode: custom RegexMarshaler ──────────────────────────────────────────────
//
// status (and its pointer-receiver UnmarshalRegex) is defined in
// unmarshal_test.go; the value-receiver MarshalRegex below makes it a symmetric
// round-trip type for the encode path.

func (s status) MarshalRegex() (string, error) {
	switch s {
	case statusOpen:
		return stateOpen, nil
	case statusClosed:
		return stateClosed, nil
	default:
		return "", fmt.Errorf("unknown status: %d", int(s))
	}
}

func ExampleRegexMarshaler() {
	type Severity int
	const (
		_ Severity = iota
		Low
		Medium
		High
	)
	// In real code this would be a type defined in the same package as
	// the call to Encode, with `func (s Severity) MarshalRegex() (string, error)`.
	// Compile-time check elided here for example brevity.
	_ = Low
	_ = Medium
	_ = High
	fmt.Println("see TestEncode_regexMarshaler for a runnable demo")
	// Output: see TestEncode_regexMarshaler for a runnable demo
}

func TestEncode_regexMarshaler(t *testing.T) {
	type Ticket struct {
		State status `regex:"state"`
	}
	e := mustEncoder[Ticket](t, `\[(?P<state>\w+)\]`)
	got, err := e.Encode(Ticket{State: statusClosed})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "[closed]" {
		t.Errorf("Encode = %q, want %q", got, "[closed]")
	}
}

func TestEncode_regexMarshalerError(t *testing.T) {
	type Ticket struct {
		State status `regex:"state"`
	}
	e := mustEncoder[Ticket](t, `\[(?P<state>\w+)\]`)
	_, err := e.Encode(Ticket{State: status(99)})
	if err == nil {
		t.Fatal("Encode with failing marshaler returned nil, want error")
	}
	var ee *rx.EncodeError
	if !errors.As(err, &ee) {
		t.Fatalf("error %v is not an *EncodeError", err)
	}
	if ee.Field != "State" {
		t.Errorf("EncodeError.Field = %q, want State", ee.Field)
	}
}

// ── Encode: error unwrapping ───────────────────────────────────────────────────

var errBadText = errors.New("bad text marshal")

type badText struct{}

func (badText) MarshalText() ([]byte, error) { return nil, errBadText }

func TestEncode_textMarshalerErrorUnwraps(t *testing.T) {
	type P struct {
		V badText `regex:"v"`
	}
	e := mustEncoder[P](t, `(?P<v>\S+)`)
	_, err := e.Encode(P{})
	if err == nil {
		t.Fatal("Encode with failing TextMarshaler returned nil, want error")
	}
	// errors.Is reaches the underlying cause through EncodeError.Unwrap.
	if !errors.Is(err, errBadText) {
		t.Errorf("errors.Is(err, errBadText) = false, want true (err=%v)", err)
	}
	var ee *rx.EncodeError
	if !errors.As(err, &ee) || ee.Field != "V" {
		t.Errorf("errors.As EncodeError = %+v, want Field=V", ee)
	}
}

// ── Decoder.Encoder: derivation error paths ────────────────────────────────────

// Non-invertible constructs outside a named capture have no single string to
// emit, so Encoder() fails fast with ErrNotInvertible naming the construct.
func TestEncoder_nonInvertibleRejected(t *testing.T) {
	type P struct {
		V string `regex:"v"`
	}
	for _, tc := range []struct {
		name, pattern, wantSub string
	}{
		{"alternation", `(?P<v>\w+)|x`, "alternation"},
		{"quantifierStar", `x*(?P<v>\w+)`, "quantifier"},
		{"quantifierPlus", `x+(?P<v>\w+)`, "quantifier"},
		{"quantifierQuest", `x?(?P<v>\w+)`, "quantifier"},
		{"quantifierRepeat", `x{2,3}(?P<v>\w+)`, "quantifier"},
		{"charClass", `[0-9](?P<v>\w+)`, "character class"},
		{"anyChar", `(?P<v>\w+).end`, "any-character"},
		{"unnamedGroup", `(\d+)(?P<v>\w+)`, "unnamed capturing group"},
		{"nestedNamedInUnnamed", `((?P<v>\d+))`, "unnamed capturing group"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := rx.MustCompile[P](tc.pattern).Encoder()
			if err == nil {
				t.Fatalf("Encoder() from %q returned nil, want error", tc.pattern)
			}
			if !errors.Is(err, rx.ErrNotInvertible) {
				t.Errorf("error %v is not ErrNotInvertible", err)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not name the construct %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestEncoder_unknownGroupRejected(t *testing.T) {
	type P struct {
		Name string `regex:"name"`
	}
	// The pattern declares an extra group no field maps to; Name still maps to
	// its own group so Compile succeeds and the failure surfaces in Encoder().
	_, err := rx.MustCompile[P](`(?P<name>\S+) (?P<missing>\S+)`).Encoder()
	if err == nil {
		t.Fatal("Encoder() with an unmapped group returned nil, want error")
	}
	if !errors.Is(err, rx.ErrInvalidStruct) {
		t.Errorf("error %v is not ErrInvalidStruct", err)
	}
	if !strings.Contains(err.Error(), "no exported field") {
		t.Errorf("error = %q, want it to mention 'no exported field'", err.Error())
	}
}

func TestEncoder_excludedFieldNotMappable(t *testing.T) {
	type P struct {
		Secret string `regex:"-"`
	}
	// A group named after an excluded field maps to nothing.
	if _, err := rx.MustCompile[P](`(?P<Secret>\S+)`).Encoder(); err == nil {
		t.Fatal("Encoder() mapped a regex:\"-\" field, want error")
	}
}

// An explicit `regex:` tag binds to a capture group exactly, never via fold —
// mirroring the decode side, where an explicit tag is matched exactly
// (subexpIndexes) while only the untagged field-name fallback folds
// (matchGroupName). Folding a case-mismatched tag would silently corrupt the
// round-trip: a field `regex:"ID,default=x"` against `(?P<id>…)` would Encode
// via the fold, but Decode maps ID to the absent group ID and returns the
// default. Encoder() must instead fail fast — group id maps to no field.
func TestEncoder_explicitTagMatchesExactly(t *testing.T) {
	type P struct {
		ID string `regex:"ID,default=x"`
	}
	// Compile succeeds: the default makes the absent group ID intentional.
	_, err := rx.MustCompile[P](`v=(?P<id>\S+)`).Encoder()
	if err == nil {
		t.Fatal("Encoder() fold-bound an explicit tag to a case-mismatched group, want error")
	}
	if !errors.Is(err, rx.ErrInvalidStruct) {
		t.Errorf("error %v is not ErrInvalidStruct", err)
	}
	if !strings.Contains(err.Error(), "no exported field") {
		t.Errorf("error = %q, want it to mention 'no exported field'", err.Error())
	}
}

func TestEncoder_unsupportedType(t *testing.T) {
	type P struct {
		Data []byte `regex:"data"` // slice is not a supported scalar
	}
	_, err := rx.MustCompile[P](`(?P<data>\S+)`).Encoder()
	if err == nil {
		t.Fatal("Encoder() returned nil for unsupported field type, want error")
	}
	if !errors.Is(err, rx.ErrInvalidStruct) {
		t.Errorf("error %v is not ErrInvalidStruct", err)
	}
	if !strings.Contains(err.Error(), "unsupported type") {
		t.Errorf("error = %q, want it to mention 'unsupported type'", err.Error())
	}
}

// ── Round-trip: the core contract ──────────────────────────────────────────────

func TestEncode_roundTripScalars(t *testing.T) {
	type P struct {
		Name   string `regex:"name"`
		Age    int    `regex:"age"`
		Active bool   `regex:"active"`
	}
	pattern := `(?P<name>\S+) is (?P<age>\d+) (?P<active>\S+)`
	dec := rx.MustCompile[P](pattern)
	enc, err := dec.Encoder()
	if err != nil {
		t.Fatalf("Encoder() returned %v", err)
	}

	want := P{Name: "Alice", Age: 30, Active: true}
	s, err := enc.Encode(want)
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	got, err := dec.One(s)
	if err != nil {
		t.Fatalf("decode of %q returned %v", s, err)
	}
	if got != want {
		t.Errorf("round-trip = %+v, want %+v (via %q)", got, want, s)
	}
}

func TestEncode_roundTripTime(t *testing.T) {
	type Ev struct {
		At time.Time `regex:"at"`
	}
	dec := rx.MustCompile[Ev](`at=(?P<at>\S+)`)
	enc, err := dec.Encoder()
	if err != nil {
		t.Fatalf("Encoder() returned %v", err)
	}

	// A sub-second timestamp exercises RFC3339Nano fidelity.
	want := time.Date(2024, 3, 2, 15, 4, 5, 123456000, time.UTC)
	s, err := enc.Encode(Ev{At: want})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	got, err := dec.One(s)
	if err != nil {
		t.Fatalf("decode of %q returned %v", s, err)
	}
	if !got.At.Equal(want) {
		t.Errorf("round-trip time = %v, want %v (via %q)", got.At, want, s)
	}
}

func TestEncode_roundTripCustomAndPointer(t *testing.T) {
	type Rec struct {
		State status `regex:"state"`
		Count *int   `regex:"count"`
	}
	dec := rx.MustCompile[Rec](`\[(?P<state>\w+)\] (?P<count>\d+)`)
	enc, err := dec.Encoder()
	if err != nil {
		t.Fatalf("Encoder() returned %v", err)
	}

	n := 7
	want := Rec{State: statusOpen, Count: &n}
	s, err := enc.Encode(want)
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	got, err := dec.One(s)
	if err != nil {
		t.Fatalf("decode of %q returned %v", s, err)
	}
	if got.State != want.State || got.Count == nil || *got.Count != *want.Count {
		t.Errorf("round-trip = %+v (count=%v), want %+v (via %q)", got, deref(got.Count), want, s)
	}
}

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// ── Examples (appear in godoc) ─────────────────────────────────────────────────

func ExampleDecoder_Encoder() {
	type Person struct {
		Name string `regex:"name"`
		Age  int    `regex:"age"`
	}
	dec := rx.MustCompile[Person](`(?P<name>\S+) is (?P<age>\d+)`)
	enc, _ := dec.Encoder()
	s, _ := enc.Encode(Person{Name: "Alice", Age: 30})
	fmt.Println(s)
	// Output: Alice is 30
}

func ExampleDecoder_Encoder_roundTrip() {
	type Person struct {
		Name string `regex:"name"`
		Age  int    `regex:"age"`
	}
	dec := rx.MustCompile[Person](`(?P<name>\S+) is (?P<age>\d+)`)
	enc, _ := dec.Encoder()

	s, _ := enc.Encode(Person{Name: "Alice", Age: 30})
	back, _ := dec.One(s)
	fmt.Printf("%q -> %+v\n", s, back)
	// Output: "Alice is 30" -> {Name:Alice Age:30}
}
