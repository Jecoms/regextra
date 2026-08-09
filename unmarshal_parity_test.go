package regextra

// This file pins the lockstep contract documented on setFieldValue and
// resolveConverter: the plan-time resolved converter and the dynamic
// setFieldValue path must dispatch with the same precedence and produce the
// same results and byte-identical error text. Every case runs the SAME
// (type, opts, value) triple through both paths and asserts they agree with
// each other and with the expected outcome — so a change that lands in one
// path but not the other fails here, not in a consumer.
//
// It is an internal (package regextra) test because setFieldValue is no
// longer reachable from the public API for concrete field types — the decode
// plan executes resolved converters — yet it must stay correct: it remains
// the per-call implementation behind interface-typed fields' converters and
// the reference semantics resolveConverter mirrors.

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

// parityStatus implements RegexUnmarshaler with a pointer receiver.
type parityStatus int

const parityStatusOpen parityStatus = 1

func (s *parityStatus) UnmarshalRegex(value string) error {
	if value != "open" {
		return fmt.Errorf("unknown parity status %q", value)
	}
	*s = parityStatusOpen
	return nil
}

// parityValueRecv implements RegexUnmarshaler with a value receiver (cannot
// mutate the field; exists to exercise the value-receiver dispatch).
type parityValueRecv struct{}

func (parityValueRecv) UnmarshalRegex(value string) error {
	if value == "fail" {
		return fmt.Errorf("parityValueRecv rejected %q", value)
	}
	return nil
}

// parityText implements encoding.TextUnmarshaler (and not RegexUnmarshaler).
type parityText struct{ S string }

func (p *parityText) UnmarshalText(text []byte) error {
	if string(text) == "fail" {
		return fmt.Errorf("parityText rejected %q", text)
	}
	p.S = string(text)
	return nil
}

func TestConverterSetFieldValueParity(t *testing.T) {
	mustDate := func(layout, v string) time.Time {
		tv, err := time.Parse(layout, v)
		if err != nil {
			t.Fatalf("bad fixture: %v", err)
		}
		return tv
	}

	cases := []struct {
		name          string
		typ           reflect.Type
		opts          map[string]string
		value         string
		want          any    // expected stored value; ignored when an error is expected
		wantErr       string // exact expected error text; "" means no exact-text expectation
		wantErrPrefix string // expected error prefix, for texts embedding stdlib detail
	}{
		// Built-in kind switch, success and error arms.
		{name: "string", typ: reflect.TypeOf(""), value: "hello", want: "hello"},
		{name: "int ok", typ: reflect.TypeOf(int(0)), value: "42", want: 42},
		{name: "int malformed", typ: reflect.TypeOf(int(0)), value: "abc",
			wantErr: `cannot convert "abc" to int: strconv.ParseInt: parsing "abc": invalid syntax`},
		{name: "int8 out of range at field width", typ: reflect.TypeOf(int8(0)), value: "999",
			wantErr: `cannot convert "999" to int8: strconv.ParseInt: parsing "999": value out of range`},
		{name: "uint16 ok", typ: reflect.TypeOf(uint16(0)), value: "65535", want: uint16(65535)},
		{name: "uint rejects negative", typ: reflect.TypeOf(uint(0)), value: "-1",
			wantErr: `cannot convert "-1" to uint: strconv.ParseUint: parsing "-1": invalid syntax`},
		{name: "float32 ok", typ: reflect.TypeOf(float32(0)), value: "2.5", want: float32(2.5)},
		{name: "float64 malformed", typ: reflect.TypeOf(float64(0)), value: "x",
			wantErr: `cannot convert "x" to float64: strconv.ParseFloat: parsing "x": invalid syntax`},
		{name: "bool ok", typ: reflect.TypeOf(false), value: "true", want: true},
		{name: "bool malformed", typ: reflect.TypeOf(false), value: "yes",
			wantErr: `cannot convert "yes" to bool: strconv.ParseBool: parsing "yes": invalid syntax`},

		// time.Time: fallback-list parse, layout= override, both error arms.
		{name: "time.Time fallback list", typ: reflect.TypeOf(time.Time{}),
			value: "2026-04-26T12:34:56Z", want: mustDate(time.RFC3339, "2026-04-26T12:34:56Z")},
		{name: "time.Time unparseable", typ: reflect.TypeOf(time.Time{}),
			value: "not-a-date", wantErrPrefix: `cannot convert "not-a-date" to time.Time`},
		{name: "time.Time layout ok", typ: reflect.TypeOf(time.Time{}),
			opts:  map[string]string{"layout": "2006-01-02"},
			value: "2026-04-26", want: mustDate("2006-01-02", "2026-04-26")},
		{name: "time.Time layout mismatch", typ: reflect.TypeOf(time.Time{}),
			opts:          map[string]string{"layout": "2006-01-02"},
			value:         "26/04/2026",
			wantErrPrefix: `cannot convert "26/04/2026" to time.Time using layout "2006-01-02"`},
		{name: "time.Duration ok", typ: reflect.TypeOf(time.Duration(0)),
			value: "1h30m", want: 90 * time.Minute},
		{name: "time.Duration malformed", typ: reflect.TypeOf(time.Duration(0)),
			value:   "fast",
			wantErr: `cannot convert "fast" to time.Duration: time: invalid duration "fast"`},

		// RegexUnmarshaler dispatch: pointer receiver via Addr, value receiver.
		{name: "RegexUnmarshaler pointer receiver", typ: reflect.TypeOf(parityStatus(0)),
			value: "open", want: parityStatusOpen},
		{name: "RegexUnmarshaler pointer receiver error", typ: reflect.TypeOf(parityStatus(0)),
			value: "bogus", wantErr: `unknown parity status "bogus"`},
		{name: "RegexUnmarshaler value receiver", typ: reflect.TypeOf(parityValueRecv{}),
			value: "ok", want: parityValueRecv{}},
		{name: "RegexUnmarshaler value receiver error", typ: reflect.TypeOf(parityValueRecv{}),
			value: "fail", wantErr: `parityValueRecv rejected "fail"`},

		// TextUnmarshaler fallback (ranks below RegexUnmarshaler and time).
		{name: "TextUnmarshaler ok", typ: reflect.TypeOf(parityText{}),
			value: "hello", want: parityText{S: "hello"}},
		{name: "TextUnmarshaler error", typ: reflect.TypeOf(parityText{}),
			value:   "fail",
			wantErr: `cannot convert "fail" to regextra.parityText: parityText rejected "fail"`},

		// Pointer fields: nil-alloc then recurse; pointer-owned RegexUnmarshaler;
		// multi-level indirection; layout= carried through the indirection.
		{name: "*int allocates and recurses", typ: reflect.TypeOf((*int)(nil)),
			value: "7", want: ptrTo(7)},
		{name: "*int conversion error", typ: reflect.TypeOf((*int)(nil)), value: "abc",
			wantErr: `cannot convert "abc" to int: strconv.ParseInt: parsing "abc": invalid syntax`},
		{name: "*parityStatus via pointer's own method", typ: reflect.TypeOf((*parityStatus)(nil)),
			value: "open", want: ptrTo(parityStatusOpen)},
		{name: "**parityStatus recurses per level", typ: reflect.TypeOf((**parityStatus)(nil)),
			value: "open", want: ptrTo(ptrTo(parityStatusOpen))},
		{name: "*time.Time with layout", typ: reflect.TypeOf((*time.Time)(nil)),
			opts:  map[string]string{"layout": "2006-01-02"},
			value: "2026-04-26", want: ptrTo(mustDate("2006-01-02", "2026-04-26"))},

		// Unsupported kinds error at conversion time, identically.
		{name: "unsupported slice", typ: reflect.TypeOf([]string(nil)), value: "x",
			wantErr: "unsupported field type: slice"},
		{name: "unsupported map", typ: reflect.TypeOf(map[string]string(nil)), value: "x",
			wantErr: "unsupported field type: map"},
		{name: "unsupported nested struct", typ: reflect.TypeOf(struct{ X string }{}), value: "x",
			wantErr: "unsupported field type: struct"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conv := resolveConverter(tc.typ, tc.opts)
			planned := reflect.New(tc.typ).Elem()
			plannedErr := conv(planned, tc.value)

			dynamic := reflect.New(tc.typ).Elem()
			dynamicErr := setFieldValue(dynamic, tc.value, tc.opts)

			// Lockstep: both paths agree on error presence AND text.
			if (plannedErr == nil) != (dynamicErr == nil) {
				t.Fatalf("paths disagree: resolveConverter err = %v, setFieldValue err = %v",
					plannedErr, dynamicErr)
			}
			if plannedErr != nil && plannedErr.Error() != dynamicErr.Error() {
				t.Fatalf("error text drift:\n resolveConverter: %q\n setFieldValue:    %q",
					plannedErr, dynamicErr)
			}
			// Lockstep: both paths produce the same stored value.
			if !reflect.DeepEqual(planned.Interface(), dynamic.Interface()) {
				t.Fatalf("value drift: resolveConverter stored %#v, setFieldValue stored %#v",
					planned.Interface(), dynamic.Interface())
			}

			// Expected outcome.
			switch {
			case tc.wantErr != "":
				if plannedErr == nil {
					t.Fatalf("err = nil, want %q", tc.wantErr)
				}
				if plannedErr.Error() != tc.wantErr {
					t.Fatalf("err = %q, want %q", plannedErr, tc.wantErr)
				}
			case tc.wantErrPrefix != "":
				// Error text past the prefix embeds stdlib parse detail; the
				// prefix is the stable contract.
				if plannedErr == nil {
					t.Fatalf("err = nil, want prefix %q", tc.wantErrPrefix)
				}
				if !strings.HasPrefix(plannedErr.Error(), tc.wantErrPrefix) {
					t.Fatalf("err = %q, want prefix %q", plannedErr, tc.wantErrPrefix)
				}
			default:
				if plannedErr != nil {
					t.Fatalf("err = %v, want nil", plannedErr)
				}
				if !reflect.DeepEqual(planned.Interface(), tc.want) {
					t.Fatalf("stored %#v, want %#v", planned.Interface(), tc.want)
				}
			}
		})
	}
}

// ptrTo returns a pointer to v; test helper for expected pointer-field values.
func ptrTo[T any](v T) *T { return &v }
