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

// ── NewEncoder: happy paths ────────────────────────────────────────────────────

func TestNewEncoder_simpleTemplate(t *testing.T) {
	type P struct {
		Name string `regex:"name"`
		Age  int    `regex:"age"`
	}
	e, err := rx.NewEncoder[P](`{name} is {age}`)
	if err != nil {
		t.Fatalf("NewEncoder returned %v", err)
	}
	if e.Template() != `{name} is {age}` {
		t.Errorf("Template() = %q, want the source string", e.Template())
	}
	got, err := e.Encode(P{Name: "Alice", Age: 30})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "Alice is 30" {
		t.Errorf("Encode = %q, want %q", got, "Alice is 30")
	}
}

func TestNewEncoder_fieldNameFallback(t *testing.T) {
	type P struct {
		Name string // no tag — resolved by field name
		Age  int
	}
	e := rx.MustNewEncoder[P](`{Name} is {Age}`)
	got, err := e.Encode(P{Name: "Bob", Age: 25})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "Bob is 25" {
		t.Errorf("Encode = %q, want %q", got, "Bob is 25")
	}
}

func TestNewEncoder_caseInsensitiveFallback(t *testing.T) {
	type P struct {
		Name string
	}
	// lower-case placeholder folds onto the exported field name.
	e := rx.MustNewEncoder[P](`hi {name}`)
	got, err := e.Encode(P{Name: "Carol"})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "hi Carol" {
		t.Errorf("Encode = %q, want %q", got, "hi Carol")
	}
}

func TestNewEncoder_exactBeatsFold(t *testing.T) {
	// An exact-name field must win over an earlier fold sibling.
	type P struct {
		NAME string `regex:"NAME"`
		Name string `regex:"name"`
	}
	e := rx.MustNewEncoder[P](`{name}`)
	got, err := e.Encode(P{NAME: "upper", Name: "lower"})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "lower" {
		t.Errorf("Encode = %q, want the exact-match field value %q", got, "lower")
	}
}

func TestEncode_literalBraces(t *testing.T) {
	type P struct {
		V string `regex:"v"`
	}
	e := rx.MustNewEncoder[P](`{{ {v} }}`)
	got, err := e.Encode(P{V: "x"})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "{ x }" {
		t.Errorf("Encode = %q, want %q", got, "{ x }")
	}
}

func TestEncode_literalOnly(t *testing.T) {
	type P struct{ V string }
	e := rx.MustNewEncoder[P](`no placeholders here`)
	got, err := e.Encode(P{V: "ignored"})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "no placeholders here" {
		t.Errorf("Encode = %q", got)
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
	e := rx.MustNewEncoder[Wide](`{s}|{i}|{i8}|{u}|{u16}|{f32}|{f64}|{b}`)
	got, err := e.Encode(Wide{S: "hi", I: -7, I8: -128, U: 9, U16: 65535, F32: 1.5, F64: 2.25, B: true})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	want := "hi|-7|-128|9|65535|1.5|2.25|true"
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
	e := rx.MustNewEncoder[Z](`[{s}][{i}][{b}]`)
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
	e := rx.MustNewEncoder[Ev](`{at}`)
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
	e := rx.MustNewEncoder[Ev](`{at}`)
	got, err := e.Encode(Ev{At: time.Date(2024, 3, 2, 15, 4, 5, 0, time.UTC)})
	if err != nil {
		t.Fatalf("Encode returned %v", err)
	}
	if got != "2024-03-02" {
		t.Errorf("Encode = %q, want %q", got, "2024-03-02")
	}
}

func TestEncode_duration(t *testing.T) {
	type D struct {
		Took time.Duration `regex:"took"`
	}
	e := rx.MustNewEncoder[D](`{took}`)
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
	e := rx.MustNewEncoder[P](`{age}`)
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
	e := rx.MustNewEncoder[P](`{age}`)
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

// A field whose static type is an interface implementing encoding.TextMarshaler
// passes encodableType at construction, but a nil value has no concrete value to
// render. Encode must surface a nil-style *EncodeError (mirroring the nil-pointer
// path) rather than falling through to the kind-switch default with a misleading
// "unsupported field type: interface".
func TestEncode_nilInterfaceErrors(t *testing.T) {
	type M struct {
		V encoding.TextMarshaler `regex:"v"`
	}
	e := rx.MustNewEncoder[M](`{v}`)
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
	e := rx.MustNewEncoder[Host](`{addr}`)
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

func TestEncode_regexMarshaler(t *testing.T) {
	type Ticket struct {
		State status `regex:"state"`
	}
	e := rx.MustNewEncoder[Ticket](`[{state}]`)
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
	e := rx.MustNewEncoder[Ticket](`[{state}]`)
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
	e := rx.MustNewEncoder[P](`{v}`)
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

// ── NewEncoder / MustNewEncoder: error paths ──────────────────────────────────

func TestNewEncoder_notAStruct(t *testing.T) {
	_, err := rx.NewEncoder[string](`{x}`)
	if err == nil {
		t.Fatal("NewEncoder returned nil for non-struct T, want error")
	}
	if !strings.Contains(err.Error(), "must be a struct") {
		t.Errorf("error = %q, want it to mention 'must be a struct'", err.Error())
	}
}

func TestNewEncoder_unknownPlaceholder(t *testing.T) {
	type P struct {
		Name string `regex:"name"`
	}
	_, err := rx.NewEncoder[P](`{missing}`)
	if err == nil {
		t.Fatal("NewEncoder returned nil for unknown placeholder, want error")
	}
	if !strings.Contains(err.Error(), "no exported field") {
		t.Errorf("error = %q, want it to mention 'no exported field'", err.Error())
	}
}

func TestNewEncoder_excludedFieldNotReferenceable(t *testing.T) {
	type P struct {
		Secret string `regex:"-"`
	}
	if _, err := rx.NewEncoder[P](`{Secret}`); err == nil {
		t.Fatal("NewEncoder referenced a regex:\"-\" field, want error")
	}
	if _, err := rx.NewEncoder[P](`{secret}`); err == nil {
		t.Fatal("NewEncoder folded onto a regex:\"-\" field, want error")
	}
}

func TestNewEncoder_malformedTemplate(t *testing.T) {
	type P struct {
		V string `regex:"v"`
	}
	for _, tc := range []struct {
		name, tmpl, wantSub string
	}{
		{"unterminated", `{v`, "unterminated"},
		{"empty", `a{}b`, "empty placeholder"},
		{"loneClose", `a}b`, "unescaped '}'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := rx.NewEncoder[P](tc.tmpl)
			if err == nil {
				t.Fatalf("NewEncoder(%q) returned nil, want error", tc.tmpl)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestNewEncoder_unsupportedType(t *testing.T) {
	type P struct {
		Data []byte `regex:"data"` // slice is not a supported scalar
	}
	_, err := rx.NewEncoder[P](`{data}`)
	if err == nil {
		t.Fatal("NewEncoder returned nil for unsupported field type, want error")
	}
	if !strings.Contains(err.Error(), "unsupported type") {
		t.Errorf("error = %q, want it to mention 'unsupported type'", err.Error())
	}
}

func TestNewEncoder_layoutOnNonTime(t *testing.T) {
	type P struct {
		Name string `regex:"name,layout=2006"`
	}
	_, err := rx.NewEncoder[P](`{name}`)
	if err == nil {
		t.Fatal("NewEncoder returned nil for layout= on non-time field, want error")
	}
	if !strings.Contains(err.Error(), "layout=") {
		t.Errorf("error = %q, want it to mention 'layout='", err.Error())
	}
}

func TestMustNewEncoder_panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustNewEncoder did not panic on an invalid template")
		}
	}()
	type P struct {
		V string `regex:"v"`
	}
	_ = rx.MustNewEncoder[P](`{missing}`)
}

// ── Round-trip: the core contract ──────────────────────────────────────────────

func TestEncode_roundTripScalars(t *testing.T) {
	type P struct {
		Name   string `regex:"name"`
		Age    int    `regex:"age"`
		Active bool   `regex:"active"`
	}
	enc := rx.MustNewEncoder[P](`{name} is {age} {active}`)
	dec := rx.MustCompile[P](`(?P<name>\S+) is (?P<age>\d+) (?P<active>\S+)`)

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
	enc := rx.MustNewEncoder[Ev](`at={at}`)
	dec := rx.MustCompile[Ev](`at=(?P<at>\S+)`)

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
	enc := rx.MustNewEncoder[Rec](`[{state}] {count}`)
	dec := rx.MustCompile[Rec](`\[(?P<state>\w+)\] (?P<count>\d+)`)

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

func ExampleEncoder() {
	type Person struct {
		Name string `regex:"name"`
		Age  int    `regex:"age"`
	}
	enc := rx.MustNewEncoder[Person](`{name} is {age}`)
	s, _ := enc.Encode(Person{Name: "Alice", Age: 30})
	fmt.Println(s)
	// Output: Alice is 30
}

func ExampleEncoder_roundTrip() {
	type Person struct {
		Name string `regex:"name"`
		Age  int    `regex:"age"`
	}
	enc := rx.MustNewEncoder[Person](`{name} is {age}`)
	dec := rx.MustCompile[Person](`(?P<name>\S+) is (?P<age>\d+)`)

	s, _ := enc.Encode(Person{Name: "Alice", Age: 30})
	back, _ := dec.One(s)
	fmt.Printf("%q -> %+v\n", s, back)
	// Output: "Alice is 30" -> {Name:Alice Age:30}
}
