package regextra_test

import (
	"encoding"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	rx "github.com/jecoms/regextra/v2"
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

// A directly-constructed EncodeError with no underlying Err must not render a
// dangling "field : <nil>" message. The encode path always sets a cause, but the
// type is exported, so guard the empty-payload case — matching the DecodeError,
// RequiredGroupError, and MissingNamedGroupsError siblings.
func TestEncodeError_nilErrRendersCleanMessage(t *testing.T) {
	if got, want := (&rx.EncodeError{}).Error(), "no encode error"; got != want {
		t.Errorf("(&EncodeError{}).Error() = %q, want %q", got, want)
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

// ── EncodeStrict: the per-group re-match check ─────────────────────────────────

func TestEncodeStrict_matchesEncodeOnSuccess(t *testing.T) {
	type P struct {
		Name string `regex:"name"`
		Age  int    `regex:"age"`
	}
	e := mustEncoder[P](t, `(?P<name>\S+) is (?P<age>\d+)`)
	v := P{Name: "Alice", Age: 30}
	lenient, err := e.Encode(v)
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	strict, err := e.EncodeStrict(v)
	if err != nil {
		t.Fatalf("EncodeStrict returned %v", err)
	}
	if strict != lenient {
		t.Errorf("EncodeStrict = %q, want Encode's output %q", strict, lenient)
	}
}

func TestEncodeStrict_mismatchTypedError(t *testing.T) {
	type P struct {
		V string `regex:"v"`
	}
	e := mustEncoder[P](t, `(?P<v>\d+)`)
	_, err := e.EncodeStrict(P{V: "not-a-number"})
	if err == nil {
		t.Fatal("EncodeStrict accepted a value that does not match the sub-pattern")
	}
	if !errors.Is(err, rx.ErrValueMismatch) {
		t.Errorf("errors.Is(err, ErrValueMismatch) = false, want true (err = %v)", err)
	}
	var ee *rx.EncodeError
	if !errors.As(err, &ee) {
		t.Fatalf("errors.As(err, *EncodeError) = false, want true (err = %v)", err)
	}
	if ee.Field != "V" || ee.Group != "v" {
		t.Errorf("EncodeError Field/Group = %q/%q, want %q/%q", ee.Field, ee.Group, "V", "v")
	}
	if !strings.Contains(err.Error(), "regextra.Encoder.EncodeStrict:") {
		t.Errorf("error = %q, want the regextra.Encoder.EncodeStrict: prefix", err.Error())
	}
	// The message carries the AST re-render of the sub-pattern, which may
	// normalize spelling — `\d+` renders as `[0-9]+`.
	if !strings.Contains(err.Error(), `"not-a-number"`) || !strings.Contains(err.Error(), "`[0-9]+`") {
		t.Errorf("error = %q, want it to name the rendered value and the sub-pattern", err.Error())
	}
}

func TestEncodeStrict_anchoredNotPartial(t *testing.T) {
	// `\d+` matches a prefix of "123x"; the check must be a full anchored match.
	type P struct {
		V string `regex:"v"`
	}
	e := mustEncoder[P](t, `(?P<v>\d+)`)
	if _, err := e.EncodeStrict(P{V: "123x"}); !errors.Is(err, rx.ErrValueMismatch) {
		t.Errorf("EncodeStrict(123x) err = %v, want ErrValueMismatch (partial match must not pass)", err)
	}
	if _, err := e.EncodeStrict(P{V: "123"}); err != nil {
		t.Errorf("EncodeStrict(123) returned %v, want nil", err)
	}
}

func TestEncodeStrict_emptyValue(t *testing.T) {
	type P struct {
		V string `regex:"v"`
	}
	plus := mustEncoder[P](t, `v=(?P<v>\w+)`)
	if _, err := plus.EncodeStrict(P{V: ""}); !errors.Is(err, rx.ErrValueMismatch) {
		t.Errorf("EncodeStrict empty vs \\w+ err = %v, want ErrValueMismatch", err)
	}
	star := mustEncoder[P](t, `v=(?P<v>\w*)`)
	if got, err := star.EncodeStrict(P{V: ""}); err != nil || got != "v=" {
		t.Errorf("EncodeStrict empty vs \\w* = %q, %v; want %q, nil", got, err, "v=")
	}
}

func TestEncodeStrict_foldFlagSurvives(t *testing.T) {
	// The sub-pattern is re-rendered from its parsed AST, where a (?i) flag is
	// baked per node — the standalone matcher must keep the case-insensitivity.
	type P struct {
		V string `regex:"v"`
	}
	e := mustEncoder[P](t, `(?i)state=(?P<v>open|closed)`)
	if _, err := e.EncodeStrict(P{V: "OPEN"}); err != nil {
		t.Errorf("EncodeStrict(OPEN) under (?i) returned %v, want nil", err)
	}
	if _, err := e.EncodeStrict(P{V: "ajar"}); !errors.Is(err, rx.ErrValueMismatch) {
		t.Errorf("EncodeStrict(ajar) err = %v, want ErrValueMismatch", err)
	}
}

func TestEncodeStrict_nestedNamedGroupRecompiles(t *testing.T) {
	// A named group nested inside a checked group must recompile standalone in
	// the anchored matcher and participate in the check.
	type P struct {
		Outer string `regex:"outer"`
	}
	e := mustEncoder[P](t, `(?P<outer>x(?P<inner>\d+))`)
	if _, err := e.EncodeStrict(P{Outer: "x42"}); err != nil {
		t.Errorf("EncodeStrict(x42) returned %v, want nil", err)
	}
	if _, err := e.EncodeStrict(P{Outer: "y42"}); !errors.Is(err, rx.ErrValueMismatch) {
		t.Errorf("EncodeStrict(y42) err = %v, want ErrValueMismatch", err)
	}
}

func TestEncodeStrict_edgeAssertionFalsePassRejected(t *testing.T) {
	// A `\b` at the group's edge is context-dependent: `foo` alone matches
	// `\A(?:\bfoo)\z` (start-of-text is a boundary), but in the emitted string
	// the group follows the word rune `X`, so `X(?P<v>\bfoo)` can never decode
	// "Xfoo". The strict matcher bakes in the adjacent literal rune and must
	// reject rather than falsely pass.
	type P struct {
		V string `regex:"v"`
	}
	d := rx.MustCompile[P](`X(?P<v>\bfoo)`)
	e, err := d.Encoder()
	if err != nil {
		t.Fatalf("Encoder() returned %v", err)
	}
	if _, derr := d.One("Xfoo"); !errors.Is(derr, rx.ErrNoMatch) {
		t.Fatalf("fixture sanity: One(Xfoo) err = %v, want ErrNoMatch", derr)
	}
	if _, err := e.EncodeStrict(P{V: "foo"}); !errors.Is(err, rx.ErrValueMismatch) {
		t.Errorf("EncodeStrict(foo) err = %v, want ErrValueMismatch (output would not decode)", err)
	}

	// Same shape on the trailing edge: `X` after the group kills the `\b`.
	d2 := rx.MustCompile[P](`(?P<v>foo\b)X`)
	e2, err := d2.Encoder()
	if err != nil {
		t.Fatalf("Encoder() returned %v", err)
	}
	if _, err := e2.EncodeStrict(P{V: "foo"}); !errors.Is(err, rx.ErrValueMismatch) {
		t.Errorf("EncodeStrict(foo) trailing err = %v, want ErrValueMismatch", err)
	}

	// A multiline `$` at the edge is context-dependent the same way: it holds
	// before `\n` but not before `b`.
	d3 := rx.MustCompile[P](`a(?P<v>(?m:foo$))b`)
	e3, err := d3.Encoder()
	if err != nil {
		t.Fatalf("Encoder() returned %v", err)
	}
	if _, err := e3.EncodeStrict(P{V: "foo"}); !errors.Is(err, rx.ErrValueMismatch) {
		t.Errorf("EncodeStrict(foo) multiline-$ err = %v, want ErrValueMismatch", err)
	}
}

func TestEncodeStrict_edgeAssertionFalseRejectFixed(t *testing.T) {
	// The inverse direction: `\B` at the group's edge holds in the emitted
	// context (`x` before `foo`, both word runes) even though `foo` alone fails
	// `\A(?:\Bfoo)\z`. The value round-trips, so strict must pass.
	type P struct {
		V string `regex:"v"`
	}
	d := rx.MustCompile[P](`x(?P<v>\Bfoo)`)
	e, err := d.Encoder()
	if err != nil {
		t.Fatalf("Encoder() returned %v", err)
	}
	got, err := e.EncodeStrict(P{V: "foo"})
	if err != nil {
		t.Fatalf("EncodeStrict(foo) returned %v, want nil (value round-trips)", err)
	}
	if got != "xfoo" {
		t.Fatalf("EncodeStrict = %q, want %q", got, "xfoo")
	}
	rt, err := d.One(got)
	if err != nil || rt.V != "foo" {
		t.Errorf("round-trip One(%q) = (%+v, %v), want V=foo, nil", got, rt, err)
	}

	// A `\b` whose neighbor is a non-word rune still holds with context baked
	// in — the delimiter case must keep passing.
	d2 := rx.MustCompile[P](`(?P<v>foo\b)-bar`)
	e2, err := d2.Encoder()
	if err != nil {
		t.Fatalf("Encoder() returned %v", err)
	}
	if got, err := e2.EncodeStrict(P{V: "foo"}); err != nil || got != "foo-bar" {
		t.Errorf("EncodeStrict(foo) = (%q, %v), want (%q, nil)", got, err, "foo-bar")
	}
}

func TestEncodeStrict_edgeAssertionAtPlanBoundary(t *testing.T) {
	// With no neighboring literal, start/end-of-text is the true emitted
	// context, so the bare anchored matcher is already exact: `\bfoo\b` alone
	// accepts "foo" and rejects a value that breaks the boundary.
	type P struct {
		V string `regex:"v"`
	}
	e := mustEncoder[P](t, `(?P<v>\bfoo\b)`)
	if got, err := e.EncodeStrict(P{V: "foo"}); err != nil || got != "foo" {
		t.Errorf("EncodeStrict(foo) = (%q, %v), want (%q, nil)", got, err, "foo")
	}
	if _, err := e.EncodeStrict(P{V: "food"}); !errors.Is(err, rx.ErrValueMismatch) {
		t.Errorf("EncodeStrict(food) err = %v, want ErrValueMismatch", err)
	}
}

func TestEncodeStrict_reportsRightField(t *testing.T) {
	type P struct {
		Name string `regex:"name"`
		Age  string `regex:"age"`
	}
	e := mustEncoder[P](t, `(?P<name>\w+) is (?P<age>\d+)`)
	_, err := e.EncodeStrict(P{Name: "Alice", Age: "old"})
	var ee *rx.EncodeError
	if !errors.As(err, &ee) {
		t.Fatalf("errors.As(err, *EncodeError) = false, want true (err = %v)", err)
	}
	if ee.Field != "Age" || ee.Group != "age" {
		t.Errorf("EncodeError Field/Group = %q/%q, want %q/%q", ee.Field, ee.Group, "Age", "age")
	}
}

func TestEncodeStrict_runtimeEncodeErrorStillTyped(t *testing.T) {
	// A field that fails to render at all (nil pointer) surfaces the same
	// EncodeError EncodeStrict's mismatch path uses — under the strict
	// entrypoint prefix, without ErrValueMismatch.
	type P struct {
		V *int `regex:"v"`
	}
	e := mustEncoder[P](t, `(?P<v>\d+)`)
	_, err := e.EncodeStrict(P{V: nil})
	var ee *rx.EncodeError
	if !errors.As(err, &ee) {
		t.Fatalf("errors.As(err, *EncodeError) = false, want true (err = %v)", err)
	}
	if errors.Is(err, rx.ErrValueMismatch) {
		t.Errorf("nil-pointer failure wrongly wraps ErrValueMismatch (err = %v)", err)
	}
	if !strings.Contains(err.Error(), "regextra.Encoder.EncodeStrict:") {
		t.Errorf("error = %q, want the regextra.Encoder.EncodeStrict: prefix", err.Error())
	}
}

// ── MustEncoder ───────────────────────────────────────────────────────────────

func TestMustEncoder_returnsEncoder(t *testing.T) {
	type P struct {
		Name string `regex:"name"`
	}
	d := rx.MustCompile[P](`name=(?P<name>\w+)`)
	e := d.MustEncoder()
	if e == nil {
		t.Fatal("MustEncoder returned nil")
	}
	s, err := e.Encode(P{Name: "Alice"})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if s != "name=Alice" {
		t.Errorf("Encode = %q, want %q", s, "name=Alice")
	}
}

func TestMustEncoder_recoversSentinel(t *testing.T) {
	type P struct {
		X string `regex:"x"`
	}
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("MustEncoder did not panic on non-invertible pattern")
		}
		err, ok := r.(error)
		if !ok {
			t.Fatalf("recovered value is %T, want error", r)
		}
		if !errors.Is(err, rx.ErrNotInvertible) {
			t.Errorf("recovered errors.Is(err, ErrNotInvertible) = false, want true (err = %v)", err)
		}
	}()
	_ = rx.MustCompile[P](`(?P<x>\w+)-\d+`).MustEncoder()
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

func ExampleEncoder_EncodeStrict() {
	type Person struct {
		Name string `regex:"name"`
		Age  string `regex:"age"`
	}
	dec := rx.MustCompile[Person](`(?P<name>\S+) is (?P<age>\d+)`)
	enc := dec.MustEncoder()

	// "old" does not re-match `\d+`, so the output would not decode back.
	_, err := enc.EncodeStrict(Person{Name: "Alice", Age: "old"})
	fmt.Println(errors.Is(err, rx.ErrValueMismatch))

	s, _ := enc.EncodeStrict(Person{Name: "Alice", Age: "30"})
	fmt.Println(s)
	// Output:
	// true
	// Alice is 30
}

func ExampleDecoder_MustEncoder() {
	type Person struct {
		Name string `regex:"name"`
		Age  int    `regex:"age"`
	}
	// The blessed usage is package-level, so an invertibility problem fails at
	// startup alongside MustCompile's pattern validation:
	//
	//	var personDecoder = regextra.MustCompile[Person](`(?P<name>\S+) is (?P<age>\d+)`)
	//	var personEncoder = personDecoder.MustEncoder()
	dec := rx.MustCompile[Person](`(?P<name>\S+) is (?P<age>\d+)`)
	enc := dec.MustEncoder()
	s, _ := enc.Encode(Person{Name: "Alice", Age: 30})
	fmt.Println(s)
	// Output: Alice is 30
}

// ── Embedded-struct promotion: `regex:",inline"` ──────────────────────────────

func TestEncoder_inlinePromotedFields(t *testing.T) {
	type Meta struct {
		Host string `regex:"host"`
		Code int    `regex:"code"`
	}
	type Line struct {
		Meta `regex:",inline"`
		Path string `regex:"path"`
	}
	e := mustEncoder[Line](t, `(?P<host>\S+) (?P<code>\d+) (?P<path>\S+)`)
	got, err := e.Encode(Line{Meta: Meta{Host: "example.com", Code: 200}, Path: "/idx"})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if want := "example.com 200 /idx"; got != want {
		t.Errorf("Encode = %q, want %q", got, want)
	}
}

func TestEncoder_inlineNestedPromotion(t *testing.T) {
	type Inner struct {
		ID string `regex:"id"`
	}
	type Middle struct {
		Inner `regex:",inline"`
		Kind  string `regex:"kind"`
	}
	type Outer struct {
		Middle `regex:",inline"`
		Name   string `regex:"name"`
	}
	e := mustEncoder[Outer](t, `(?P<id>\w+)/(?P<kind>\w+)/(?P<name>\w+)`)
	got, err := e.Encode(Outer{Middle: Middle{Inner: Inner{ID: "i42"}, Kind: "widget"}, Name: "alpha"})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if want := "i42/widget/alpha"; got != want {
		t.Errorf("Encode = %q, want %q", got, want)
	}
}

func TestEncoder_inlinePointerEmbedded(t *testing.T) {
	type Meta struct {
		Host string `regex:"host"`
	}
	type Line struct {
		*Meta `regex:",inline"`
		Path  string `regex:"path"`
	}
	e := mustEncoder[Line](t, `(?P<host>\S+) (?P<path>\S+)`)
	got, err := e.Encode(Line{Meta: &Meta{Host: "example.com"}, Path: "/idx"})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if want := "example.com /idx"; got != want {
		t.Errorf("Encode = %q, want %q", got, want)
	}
}

func TestEncoder_inlineNilEmbeddedPointerErrors(t *testing.T) {
	type Meta struct {
		Host string `regex:"host"`
	}
	type Line struct {
		*Meta `regex:",inline"`
		Path  string `regex:"path"`
	}
	e := mustEncoder[Line](t, `(?P<host>\S+) (?P<path>\S+)`)
	_, err := e.Encode(Line{Path: "/idx"})
	var ee *rx.EncodeError
	if !errors.As(err, &ee) {
		t.Fatalf("Encode returned %v, want *EncodeError", err)
	}
	if ee.Field != "Host" || ee.Group != "host" {
		t.Errorf("EncodeError = %+v, want Field Host / Group host", ee)
	}
	if !strings.Contains(ee.Err.Error(), "nil embedded pointer") {
		t.Errorf("cause %q does not name the nil embedded pointer", ee.Err)
	}
}

func TestEncoder_inlineShadowingOuterFieldWins(t *testing.T) {
	type Meta struct {
		Host string `regex:"host"`
	}
	type Line struct {
		Meta `regex:",inline"`
		Host string `regex:"host"` // shallower binding shadows the promoted one
	}
	e := mustEncoder[Line](t, `(?P<host>\S+)`)
	got, err := e.Encode(Line{Meta: Meta{Host: "inner.example"}, Host: "outer.example"})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "outer.example" {
		t.Errorf("Encode = %q, want the outer field's value", got)
	}
}

func TestEncoder_inlineTaggedBeatsUntaggedExactAtEqualDepth(t *testing.T) {
	// Mirror of the decode side's tagged-beats-untagged tiebreak: the untagged
	// struct comes first in field order and its field name matches the group
	// exactly, but the tagged binding is the decode winner, so it must be the
	// encode source too.
	type Metadata struct {
		Host string // untagged: exact-matches group "Host"
	}
	type ServerInfo struct {
		Addr string `regex:"Host"`
	}
	type Request struct {
		Metadata   `regex:",inline"`
		ServerInfo `regex:",inline"`
	}
	e := mustEncoder[Request](t, `(?P<Host>\S+)`)
	got, err := e.Encode(Request{Metadata: Metadata{Host: "dropped.example"}, ServerInfo: ServerInfo{Addr: "tagged.example"}})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "tagged.example" {
		t.Errorf("Encode = %q, want the tagged field's value", got)
	}
}

func TestEncoder_inlineTaggedBeatsUntaggedFoldAtEqualDepth(t *testing.T) {
	// Fold variant of the tiebreak mirror: the untagged sibling only
	// fold-matches, so the exact-pass-before-fold-pass order already prefers
	// the tagged binding — pinned here against regression.
	type Metadata struct {
		Host string // untagged: fold-matches group "host"
	}
	type ServerInfo struct {
		Addr string `regex:"host"`
	}
	type Request struct {
		Metadata   `regex:",inline"`
		ServerInfo `regex:",inline"`
	}
	e := mustEncoder[Request](t, `(?P<host>\S+)`)
	got, err := e.Encode(Request{Metadata: Metadata{Host: "dropped.example"}, ServerInfo: ServerInfo{Addr: "tagged.example"}})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "tagged.example" {
		t.Errorf("Encode = %q, want the tagged field's value", got)
	}
}

func TestEncoder_inlineFoldFallbackOnPromotedField(t *testing.T) {
	type Meta struct {
		Host string // untagged: folds against group "HOST"
	}
	type Line struct {
		Meta `regex:",inline"`
	}
	e := mustEncoder[Line](t, `(?P<HOST>\S+)`)
	got, err := e.Encode(Line{Meta: Meta{Host: "example.com"}})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "example.com" {
		t.Errorf("Encode = %q, want example.com", got)
	}
}

func TestEncodeStrict_inlinePromotedFieldChecked(t *testing.T) {
	type Meta struct {
		Code int `regex:"code"`
	}
	type Line struct {
		Meta `regex:",inline"`
	}
	e := mustEncoder[Line](t, `(?P<code>\d\d\d)`)
	if _, err := e.EncodeStrict(Line{Meta: Meta{Code: 200}}); err != nil {
		t.Fatalf("EncodeStrict returned %v for a matching value", err)
	}
	_, err := e.EncodeStrict(Line{Meta: Meta{Code: 7}})
	if !errors.Is(err, rx.ErrValueMismatch) {
		t.Fatalf("EncodeStrict returned %v, want ErrValueMismatch through a promoted field", err)
	}
	var ee *rx.EncodeError
	if !errors.As(err, &ee) || ee.Field != "Code" {
		t.Errorf("EncodeError = %+v, want Field Code", ee)
	}
}

func TestEncode_inlineRoundTrip(t *testing.T) {
	type Meta struct {
		Host string `regex:"host"`
		Code int    `regex:"code"`
	}
	type Line struct {
		Meta `regex:",inline"`
		Path string `regex:"path"`
	}
	pattern := `(?P<host>\S+) (?P<code>\d+) (?P<path>\S+)`
	d := rx.MustCompile[Line](pattern)
	e, err := d.Encoder()
	if err != nil {
		t.Fatalf("Encoder returned %v", err)
	}
	orig := Line{Meta: Meta{Host: "example.com", Code: 200}, Path: "/index.html"}
	s, err := e.Encode(orig)
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	back, err := d.One(s)
	if err != nil {
		t.Fatalf("One returned %v", err)
	}
	if back != orig {
		t.Errorf("round trip = %+v, want %+v", back, orig)
	}
}
