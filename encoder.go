package regextra

import (
	"encoding"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Encoder is the typed inverse of [Decoder]: it renders a value of T back into
// a string using a reversible template. Where [Decoder] pulls named capture
// groups out of a match, Encoder substitutes struct-field values into named
// placeholders, so an Encode followed by an [Unmarshal] / [Decoder.One] on a
// compatible pattern round-trips the original struct.
//
// The template is NOT a regular expression. Go's regexp patterns are not
// generically reversible (a class like `\d+` describes many strings, not one),
// so Encoder compiles its own small template language instead:
//
//   - Literal text is emitted verbatim.
//   - A {name} placeholder is replaced with the value of the struct field that
//     resolves to name — the same field-mapping rules [Decoder] uses: the
//     field's `regex:"name"` tag if present, otherwise the field's own name
//     (matched case-insensitively via Unicode simple-fold). A `regex:"-"` field
//     is excluded and cannot be referenced.
//   - A literal brace is written by doubling it: `{{` emits `{` and `}}` emits `}`.
//
// The template is parsed once by [NewEncoder] into an ordered segment list;
// [Encoder.Encode] walks that list and concatenates with a strings.Builder,
// running no per-call reflect of T's fields.
//
// Round-trip contract (v1). Encode(v) re-decodes to v when each encoded value
// re-matches the sub-pattern its placeholder maps to in the decode regex. The
// caller owns that pairing — the same names in both the template and the regex,
// and value-appropriate sub-patterns (a captured word wants `\S+`, not `.*`).
// Values that collide with a surrounding literal delimiter, or two placeholders
// with no literal between them, have no unambiguous decode boundary and are out
// of scope for v1.
//
// Encoders are safe for concurrent use — no shared mutable state after
// construction. Use [NewEncoder] (returns error) or [MustNewEncoder] (panics)
// to construct.
type Encoder[T any] struct {
	template string
	rtype    reflect.Type
	segments []encodeSegment
}

// encodeSegment is one piece of a parsed template: either a literal run emitted
// verbatim, or a reference to a struct field whose value is substituted.
type encodeSegment struct {
	// literal holds the verbatim text when field is false.
	literal string
	// field reports whether this segment substitutes a field (true) or emits
	// literal text (false).
	field bool
	// fieldIndex is the index into T's struct fields (StructField.Index[0]),
	// valid only when field is true.
	fieldIndex int
	// name is the placeholder name the segment resolved from, retained for
	// EncodeError.Group. Valid only when field is true.
	name string
	// opts is the parsed tag options map for the field (e.g. {"layout": "..."}).
	// Nil if the field has no options.
	opts map[string]string
}

// RegexMarshaler is the interface implemented by types that render themselves
// into a string for the regextra encode path. It is the encode-side mirror of
// [RegexUnmarshaler] and of [encoding.TextMarshaler]: when an [Encoder] field's
// type satisfies this interface, [Encoder.Encode] calls MarshalRegex instead of
// the built-in string/int/uint/float/bool conversion.
//
// A type that implements both RegexMarshaler and [RegexUnmarshaler] round-trips
// symmetrically through [Encoder] and [Decoder].
//
// Example:
//
//	type Status int
//
//	func (s Status) MarshalRegex() (string, error) {
//	    switch s {
//	    case StatusOpen:   return "open", nil
//	    case StatusClosed: return "closed", nil
//	    default:           return "", fmt.Errorf("unknown status: %d", s)
//	    }
//	}
type RegexMarshaler interface {
	MarshalRegex() (string, error)
}

// regexMarshalerType and textMarshalerType are the reflect.Types of the two
// marshal interfaces, cached so the implements-check on every field doesn't pay
// the reflect.TypeOf cost — mirrors regexUnmarshalerType / textUnmarshalerType
// on the decode side.
var (
	regexMarshalerType = reflect.TypeOf((*RegexMarshaler)(nil)).Elem()
	textMarshalerType  = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
)

// EncodeError reports the failure to render a struct field into its template
// placeholder. It is the encode-side mirror of [DecodeError], returned (wrapped
// with the calling entrypoint's prefix) by [Encoder.Encode] when a field's
// value cannot be converted to a string. Recover it with [errors.As] to branch
// on the failure without parsing message text:
//
//	var ee *regextra.EncodeError
//	if errors.As(err, &ee) {
//	    log.Printf("field %s (placeholder %s) of type %s could not encode: %v", ee.Field, ee.Group, ee.Type, ee.Err)
//	}
//
// Err holds the underlying cause (e.g. an error from a custom [RegexMarshaler]
// or [encoding.TextMarshaler], or a nil-pointer field) and is reachable via
// [errors.Is]/[errors.As] through Unwrap.
type EncodeError struct {
	// Field is the source struct field name.
	Field string
	// Group is the template placeholder the field resolved from — the field's
	// `regex:"..."` tag name when set, otherwise the field's own name.
	Group string
	// Type is the source field's type, rendered (e.g. "int", "time.Time").
	Type string
	// Err is the underlying encode error.
	Err error
}

// Error implements the error interface. The calling entrypoint prepends its own
// `regextra.<Entrypoint>:` prefix when wrapping.
func (e *EncodeError) Error() string {
	return fmt.Sprintf("field %s: %v", e.Field, e.Err)
}

// Unwrap returns the underlying encode error so [errors.Is]/[errors.As] can
// reach it.
func (e *EncodeError) Unwrap() error { return e.Err }

// NewEncoder parses template and validates it against T's struct fields.
//
// Returns an error if:
//   - T is not a struct type
//   - template is malformed (an unterminated `{`, an empty `{}`, or an
//     unescaped literal `}`)
//   - a {name} placeholder references no exported, non-excluded field of T
//   - a referenced field's type cannot be encoded (see [Encoder] for the
//     supported set)
//   - a referenced field uses `layout=` but is not a time.Time
//
// Once NewEncoder returns nil, the resulting Encoder is fully validated: the
// only errors [Encoder.Encode] can then surface are runtime value failures
// (a custom marshaler returning an error, or a nil pointer field).
func NewEncoder[T any](template string) (*Encoder[T], error) {
	var zero T
	rt := reflect.TypeOf(zero)
	if rt == nil || rt.Kind() != reflect.Struct {
		return nil, fmt.Errorf("regextra.NewEncoder: T must be a struct type, got %v", rt)
	}

	segments, err := parseTemplate(rt, template)
	if err != nil {
		return nil, err
	}

	return &Encoder[T]{
		template: template,
		rtype:    rt,
		segments: segments,
	}, nil
}

// MustNewEncoder is like [NewEncoder] but panics on error. Intended for
// package-level vars where startup-time failure is the right behavior:
//
//	var personEncoder = regextra.MustNewEncoder[Person](`{name} is {age}`)
func MustNewEncoder[T any](template string) *Encoder[T] {
	e, err := NewEncoder[T](template)
	if err != nil {
		panic(err)
	}
	return e
}

// parseTemplate compiles template into the ordered segment list, resolving each
// {name} placeholder to an exported, non-excluded field of rt and validating
// that field is encodable. It is the single template-construction path behind
// [NewEncoder] / [MustNewEncoder].
func parseTemplate(rt reflect.Type, template string) ([]encodeSegment, error) {
	var segments []encodeSegment
	var lit strings.Builder

	flushLiteral := func() {
		if lit.Len() > 0 {
			segments = append(segments, encodeSegment{literal: lit.String()})
			lit.Reset()
		}
	}

	for i := 0; i < len(template); i++ {
		c := template[i]
		switch c {
		case '{':
			// Doubled brace is an escaped literal '{'.
			if i+1 < len(template) && template[i+1] == '{' {
				lit.WriteByte('{')
				i++
				continue
			}
			end := strings.IndexByte(template[i+1:], '}')
			if end == -1 {
				return nil, fmt.Errorf("regextra.NewEncoder: unterminated placeholder in template: %q", template[i:])
			}
			name := template[i+1 : i+1+end]
			if name == "" {
				return nil, fmt.Errorf("regextra.NewEncoder: empty placeholder {} in template")
			}
			idx, opts, ok := resolveEncodeField(rt, name)
			if !ok {
				return nil, fmt.Errorf("regextra.NewEncoder: placeholder %q references no exported field of %v", name, rt)
			}
			if err := validateEncodeField(rt.Field(idx), opts); err != nil {
				return nil, err
			}
			flushLiteral()
			segments = append(segments, encodeSegment{
				field:      true,
				fieldIndex: idx,
				name:       name,
				opts:       opts,
			})
			i += end + 1 // advance past the closing '}'
		case '}':
			// Doubled brace is an escaped literal '}'. A lone '}' is a template
			// error — it would otherwise silently swallow a typo'd placeholder.
			if i+1 < len(template) && template[i+1] == '}' {
				lit.WriteByte('}')
				i++
				continue
			}
			return nil, fmt.Errorf("regextra.NewEncoder: unescaped '}' in template (write '}}' for a literal brace)")
		default:
			lit.WriteByte(c)
		}
	}
	flushLiteral()
	return segments, nil
}

// resolveEncodeField maps a placeholder name to an exported, non-excluded field
// of rt, using the same field-mapping rules the decode side applies: a field's
// `regex:"name"` tag when set, otherwise the field's own name, matched exactly
// first and then case-insensitively via Unicode simple-fold (mirroring
// matchGroupName). Returns the field index, its parsed tag options, and true on
// a match; ("", nil, false) when no field resolves.
func resolveEncodeField(rt reflect.Type, name string) (int, map[string]string, bool) {
	// Exact pass first so an exact name never loses to an earlier fold sibling.
	for i := range rt.NumField() {
		sf := rt.Field(i)
		if !sf.IsExported() {
			continue
		}
		candidate, opts, skip := fieldCandidateName(sf)
		if skip {
			continue
		}
		if candidate == name {
			return i, opts, true
		}
	}
	for i := range rt.NumField() {
		sf := rt.Field(i)
		if !sf.IsExported() {
			continue
		}
		candidate, opts, skip := fieldCandidateName(sf)
		if skip {
			continue
		}
		if strings.EqualFold(candidate, name) {
			return i, opts, true
		}
	}
	return 0, nil, false
}

// fieldCandidateName returns the name a field is addressable by in a template —
// its `regex:"name"` tag name when set, otherwise its own field name — plus the
// parsed tag options and whether the field is excluded (`regex:"-"`).
func fieldCandidateName(sf reflect.StructField) (name string, opts map[string]string, skip bool) {
	// required is a decode-side presence flag; encoding always emits the field's
	// actual value, so it is irrelevant here.
	tagName, opts, _, skip := parseFieldTag(sf)
	if skip {
		return "", nil, true
	}
	if tagName == "" {
		return sf.Name, opts, false
	}
	return tagName, opts, false
}

// validateEncodeField rejects, at construction time, a placeholder field that
// [Encoder.Encode] could never render: a `layout=` option on a non-time.Time
// field, or a type outside the supported set. This gives [NewEncoder] the same
// "a successful construction can't produce a mapping error later" guarantee that
// [Compile] gives [Decoder].
func validateEncodeField(sf reflect.StructField, opts map[string]string) error {
	ft := sf.Type
	if _, ok := opts["layout"]; ok {
		base := ft
		if base.Kind() == reflect.Ptr {
			base = base.Elem()
		}
		if base != timeTimeType {
			return fmt.Errorf("regextra.NewEncoder: field %s has `layout=` option but is %v, not time.Time", sf.Name, ft)
		}
	}
	if !encodableType(ft) {
		return fmt.Errorf("regextra.NewEncoder: field %s has unsupported type %v", sf.Name, ft)
	}
	return nil
}

// encodableType reports whether a field of type t can be rendered by
// encodeFieldValue: a type implementing [RegexMarshaler] or
// [encoding.TextMarshaler] (directly or via its pointer), the time special
// cases, one of the supported scalar kinds, or a single-level pointer to any of
// these.
func encodableType(t reflect.Type) bool {
	if t.Implements(regexMarshalerType) || reflect.PointerTo(t).Implements(regexMarshalerType) {
		return true
	}
	if t.Implements(textMarshalerType) || reflect.PointerTo(t).Implements(textMarshalerType) {
		return true
	}
	if t == timeTimeType || t == timeDurationType {
		return true
	}
	switch t.Kind() {
	case reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		reflect.Bool:
		return true
	case reflect.Ptr:
		return encodableType(t.Elem())
	default:
		return false
	}
}

// Encode renders v into a string by walking e's parsed template: literal
// segments pass through and each placeholder is replaced with the encoded value
// of its struct field.
//
// Returns an [EncodeError] (wrapped with the entrypoint prefix) if a field
// cannot be rendered at runtime — a custom [RegexMarshaler] / [encoding.TextMarshaler]
// returning an error, or a nil pointer field, which has no string form.
//
// The `default=` tag option does not affect encoding: it is a decode-side
// substitution for an absent group, whereas Encode always emits the field's
// actual value. `layout=` is honored so a time.Time re-parses under [Decoder]'s
// exclusive-layout rule.
func (e *Encoder[T]) Encode(v T) (string, error) {
	// An addressable copy of v so fields with pointer-receiver marshalers
	// dispatch via Addr() — the same reason setFieldValue relies on
	// addressability on the decode side. reflect.ValueOf(v) alone is not
	// addressable.
	rv := reflect.New(e.rtype).Elem()
	rv.Set(reflect.ValueOf(v))

	var b strings.Builder
	for _, seg := range e.segments {
		if !seg.field {
			b.WriteString(seg.literal)
			continue
		}
		field := rv.Field(seg.fieldIndex)
		s, err := encodeFieldValue(field, seg.opts)
		if err != nil {
			sf := e.rtype.Field(seg.fieldIndex)
			return "", fmt.Errorf("regextra.Encoder.Encode: %w", &EncodeError{
				Field: sf.Name,
				Group: seg.name,
				Type:  field.Type().String(),
				Err:   err,
			})
		}
		b.WriteString(s)
	}
	return b.String(), nil
}

// Template returns the template source this Encoder was constructed from.
// Useful for logging and debugging — the encode-side mirror of [Decoder.Pattern].
func (e *Encoder[T]) Template() string { return e.template }

// encodeFieldValue renders one struct field to its string form — the inverse of
// setFieldValue, dispatching in the same precedence order so a type round-trips
// symmetrically: custom [RegexMarshaler] first, then the time.Time /
// time.Duration special cases, then [encoding.TextMarshaler], then the built-in
// kind switch. `opts` carries the field's parsed tag options; only `layout` (for
// time.Time) is consulted.
func encodeFieldValue(field reflect.Value, opts map[string]string) (string, error) {
	// 0. Pointer fields: a nil pointer has no string form (every v1 placeholder
	//    is required), so it is an error; otherwise dispatch on the pointer's own
	//    RegexMarshaler or recurse into the pointee. Single-level handling
	//    mirrors setFieldValue; deeper indirection recurses.
	if field.Kind() == reflect.Ptr {
		if field.IsNil() {
			return "", fmt.Errorf("cannot encode nil pointer of type %s", field.Type())
		}
		if m, ok := field.Interface().(RegexMarshaler); ok {
			return m.MarshalRegex()
		}
		return encodeFieldValue(field.Elem(), opts)
	}

	// 0b. Interface fields: a nil interface (e.g. a field statically typed as
	//     encoding.TextMarshaler or RegexMarshaler holding no value) has no
	//     concrete value to render, so it is an error mirroring the nil-pointer
	//     case above — otherwise the type-assertions below yield ok=false and it
	//     falls through to the kind-switch default with a misleading
	//     "unsupported field type: interface". A non-nil interface passes
	//     through to the marshaler checks / kind switch on its dynamic value.
	if field.Kind() == reflect.Interface && field.IsNil() {
		return "", fmt.Errorf("cannot encode nil interface of type %s", field.Type())
	}

	// 1. RegexMarshaler wins for non-pointer fields — the package-specific hook
	//    beats the stdlib special cases, so a `type MyTime time.Time` with its
	//    own MarshalRegex is not pre-empted by the time.Time fast path. Both
	//    pointer- and value-receiver implementations dispatch (fields are
	//    addressable — see Encode).
	if field.CanAddr() {
		if m, ok := field.Addr().Interface().(RegexMarshaler); ok {
			return m.MarshalRegex()
		}
	}
	if field.Type().Implements(regexMarshalerType) {
		if m, ok := field.Interface().(RegexMarshaler); ok {
			return m.MarshalRegex()
		}
	}

	// 2. time.Time and time.Duration. Caught by Type before the Kind switch
	//    because time.Duration's underlying Kind is reflect.Int64. time.Time
	//    encodes with the `layout` option when set (matching Decoder's
	//    exclusive-layout rule), else RFC3339Nano — the first layout in the
	//    decode fallback list, so the output re-parses, and it preserves
	//    sub-second precision that a plain RFC3339 would drop.
	switch field.Type() {
	case timeTimeType:
		t := field.Interface().(time.Time)
		layout := time.RFC3339Nano
		if l, ok := opts["layout"]; ok && l != "" {
			layout = l
		}
		return t.Format(layout), nil
	case timeDurationType:
		return field.Interface().(time.Duration).String(), nil
	}

	// 3. encoding.TextMarshaler fallback. Ranks below RegexMarshaler and below
	//    the time special cases (time.Time implements TextMarshaler but its
	//    MarshalText emits only RFC3339 with no layout control), mirroring the
	//    decode side. Addressable fields via Addr(); interface-typed fields via
	//    Implements.
	if field.CanAddr() {
		if m, ok := field.Addr().Interface().(encoding.TextMarshaler); ok {
			b, err := m.MarshalText()
			if err != nil {
				return "", fmt.Errorf("cannot encode %s: %w", field.Type(), err)
			}
			return string(b), nil
		}
	}
	if field.Type().Implements(textMarshalerType) {
		if m, ok := field.Interface().(encoding.TextMarshaler); ok {
			b, err := m.MarshalText()
			if err != nil {
				return "", fmt.Errorf("cannot encode %s: %w", field.Type(), err)
			}
			return string(b), nil
		}
	}

	switch field.Kind() {
	case reflect.String:
		return field.String(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(field.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(field.Uint(), 10), nil
	case reflect.Float32:
		return strconv.FormatFloat(field.Float(), 'g', -1, 32), nil
	case reflect.Float64:
		return strconv.FormatFloat(field.Float(), 'g', -1, 64), nil
	case reflect.Bool:
		return strconv.FormatBool(field.Bool()), nil
	default:
		return "", fmt.Errorf("unsupported field type: %s", field.Kind())
	}
}
