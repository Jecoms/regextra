package regextra

import (
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"regexp/syntax"
	"strconv"
	"strings"
	"time"
)

// ErrNotInvertible categorizes a [Decoder.Encoder] failure where the decoder's
// pattern contains a construct that has no single string to emit when encoding —
// an alternation (`|`), a quantifier (`*`, `+`, `?`, `{n,m}`), a character class
// (`[...]`), an any-character wildcard (`.`), or an unnamed group with non-literal
// content — appearing outside a named capture group. Callers can branch on the
// failure kind with errors.Is rather than parsing the message. Like [ErrNoMatch]
// and [ErrInvalidPattern], it carries the bare `regextra:` prefix reserved for
// package-level sentinels.
//
// Inside a named capture group such constructs are fine: the struct field's value
// fills the group, so the sub-pattern describing what the group matches is
// irrelevant to encoding.
var ErrNotInvertible = errors.New("regextra: pattern is not invertible")

// Encoder is the typed inverse of [Decoder]: it renders a value of T back into a
// string so that an Encode followed by an [Unmarshal] / [Decoder.One] on the same
// pattern round-trips the original struct. Construct one with [Decoder.Encoder],
// which derives the encoder from the decoder's own compiled pattern — write the
// pattern once, get the inverse for free.
//
// # Derivation
//
// [Decoder.Encoder] parses the decoder's pattern with regexp/syntax and inverts
// the invertible subset of the grammar into an ordered plan of literal runs and
// field substitutions:
//
//   - Literal text is emitted verbatim (regexp escapes like `\.` are already
//     decoded by the parser).
//   - A named capture group `(?P<name>…)` becomes a field substitution: name
//     resolves to a struct field with the same rules [Decoder] uses — the field's
//     `regex:"name"` tag matched exactly, otherwise the field's own name matched
//     exactly then case-insensitively via Unicode simple case folding; a `regex:"-"` field
//     is excluded. The group's sub-pattern is discarded — the field's value fills
//     the span.
//   - Anchors and zero-width assertions (`^`, `$`, `\A`, `\z`, `\b`, …) match no
//     text and are dropped.
//   - An unnamed group whose body is pure literal text is treated as that literal.
//
// Any construct with no single string to emit — an alternation, a quantifier, a
// character class, an any-character wildcard, or an unnamed group with
// non-literal content — appearing outside a named capture group makes the pattern
// non-invertible, and [Decoder.Encoder] fails fast with [ErrNotInvertible].
//
// [Decoder.Encoder] builds the plan once; [Encoder.Encode] walks it and
// concatenates with a strings.Builder. It still reflects on the value each call
// to read fields, but does no per-call field-mapping reflection — the field↦group
// resolution is cached in the plan at construction.
//
// # Supported field types
//
// A named group resolves to a field whose type Encode can render — the same set
// [Unmarshal] accepts on the decode side: string, bool, all int/uint widths,
// float32/float64, time.Time, time.Duration, a type implementing [RegexMarshaler]
// or [encoding.TextMarshaler], and pointers (any depth) to any of these. A
// mapped field of any other type makes [Decoder.Encoder] fail with
// [ErrInvalidStruct]. A time.Time field honors a `layout=` tag option and
// otherwise emits RFC3339Nano.
//
// # Round-trip contract
//
// Encode(v) re-decodes to v when each encoded value re-matches the sub-pattern of
// the group it fills. The caller owns that pairing by writing value-appropriate
// sub-patterns in the decode regex (a captured word wants `\S+`, not `.*`).
// Values that collide with a surrounding literal delimiter, or two adjacent
// captures with no literal between them, have no unambiguous decode boundary and
// are out of scope. (A future option is to re-match each encoded value against
// its group's sub-pattern at Encode time; that is deliberately not done here.)
//
// Encoders are safe for concurrent use — no shared mutable state after
// construction.
type Encoder[T any] struct {
	rtype    reflect.Type
	segments []encodeSegment
}

// encodeSegment is one piece of a derived encode plan: either a literal run
// emitted verbatim, or a reference to a struct field whose value is substituted.
type encodeSegment struct {
	// literal holds the verbatim text when field is false.
	literal string
	// field reports whether this segment substitutes a field (true) or emits
	// literal text (false).
	field bool
	// fieldIndex is the index into T's struct fields (StructField.Index[0]),
	// valid only when field is true.
	fieldIndex int
	// name is the capture-group name the segment resolved from, retained for
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

// EncodeError reports the failure to render a struct field into its capture-group
// slot. It is the encode-side mirror of [DecodeError], returned (wrapped with the
// calling entrypoint's prefix) by [Encoder.Encode] when a field's value cannot be
// converted to a string. Recover it with [errors.As] to branch on the failure
// without parsing message text:
//
//	var ee *regextra.EncodeError
//	if errors.As(err, &ee) {
//	    log.Printf("field %s (group %s) of type %s could not encode: %v", ee.Field, ee.Group, ee.Type, ee.Err)
//	}
//
// Err holds the underlying cause (e.g. an error from a custom [RegexMarshaler]
// or [encoding.TextMarshaler], or a nil-pointer or nil-interface field) and is
// reachable via [errors.Is]/[errors.As] through Unwrap.
type EncodeError struct {
	// Field is the source struct field name.
	Field string
	// Group is the capture-group name the field resolved from: the field's
	// `regex:"..."` tag name when set, otherwise the declared group whose name
	// matches the field name — which may differ from the field name in case when
	// the two matched via Unicode simple case folding. Mirrors [DecodeError].Group.
	Group string
	// Type is the source field's type, rendered (e.g. "int", "time.Time").
	Type string
	// Err is the underlying encode error.
	Err error
}

// Error implements the error interface. The calling entrypoint prepends its own
// `regextra.<Entrypoint>:` prefix when wrapping. When Err is nil (only reachable
// by constructing the value directly — the encode path always sets an underlying
// cause) it reports "no encode error" rather than a message with a dangling
// "<nil>" cause, mirroring [DecodeError], [RequiredGroupError], and
// [MissingNamedGroupsError].
func (e *EncodeError) Error() string {
	if e.Err == nil {
		return "no encode error"
	}
	return fmt.Sprintf("field %s: %v", e.Field, e.Err)
}

// Unwrap returns the underlying encode error so [errors.Is]/[errors.As] can
// reach it.
func (e *EncodeError) Unwrap() error { return e.Err }

// Encoder derives the typed inverse of d by inverting d's compiled pattern: it
// parses the pattern's AST and walks the invertible subset (literal runs, named
// capture groups, anchors, pure-literal unnamed groups) into an ordered encode
// plan. Named capture groups resolve to struct fields with the same field-mapping
// rules [Decoder] uses. Write the pattern once and get the encoder for free —
// there is no separate template to keep in sync.
//
// Returns an error if:
//   - the pattern contains a construct that is not invertible outside a named
//     capture group — an alternation (`|`), a quantifier (`*`, `+`, `?`,
//     `{n,m}`), a character class (`[...]`), an any-character wildcard (`.`), or
//     an unnamed group with non-literal content — wrapping [ErrNotInvertible]
//   - a named capture group maps to no exported, non-excluded field of T
//   - a mapped field's type cannot be encoded (see [Encoder] for the supported
//     set)
//
// The latter two wrap [ErrInvalidStruct], mirroring [Compile]. Once Encoder
// returns nil, the resulting Encoder is fully validated: the only errors
// [Encoder.Encode] can then surface are runtime value failures (a custom
// marshaler returning an error, or a nil pointer or nil interface field).
func (d *Decoder[T]) Encoder() (*Encoder[T], error) {
	var zero T
	rt := reflect.TypeOf(zero)
	// rt is always a struct: a *Decoder[T] can only be constructed by Compile,
	// which rejects a non-struct T, so no re-validation is needed here.

	ast, err := syntax.Parse(d.Pattern(), syntax.Perl)
	if err != nil {
		// Unreachable in practice — the pattern already parsed under the same
		// syntax.Perl flags in Compile — but surfaced defensively rather than
		// panicked.
		return nil, fmt.Errorf("%w: %w", ErrInvalidPattern, err)
	}

	var sb encodeSegmentBuilder
	if err := walkEncodeAST(rt, ast, &sb); err != nil {
		return nil, err
	}
	sb.flushLiteral()

	return &Encoder[T]{
		rtype:    rt,
		segments: sb.segments,
	}, nil
}

// encodeSegmentBuilder accumulates the derived encode plan, coalescing adjacent
// literal runs into one segment (an anchor dropped between two literals, for
// example, leaves them contiguous) so Encode walks the minimal segment list.
type encodeSegmentBuilder struct {
	segments []encodeSegment
	lit      strings.Builder
}

func (sb *encodeSegmentBuilder) writeLiteral(s string) { sb.lit.WriteString(s) }

func (sb *encodeSegmentBuilder) flushLiteral() {
	if sb.lit.Len() > 0 {
		sb.segments = append(sb.segments, encodeSegment{literal: sb.lit.String()})
		sb.lit.Reset()
	}
}

func (sb *encodeSegmentBuilder) addField(seg encodeSegment) {
	sb.flushLiteral()
	sb.segments = append(sb.segments, seg)
}

// walkEncodeAST inverts one node of a regexp/syntax AST into the encode plan,
// recursing over concatenations. It drops anchors and zero-width assertions,
// emits literals verbatim, turns named captures into field substitutions, and
// rejects any construct that has no single string to emit outside a named
// capture — the derivation's fail-fast core.
func walkEncodeAST(rt reflect.Type, re *syntax.Regexp, sb *encodeSegmentBuilder) error {
	switch re.Op {
	case syntax.OpLiteral:
		// The parser has already decoded regexp escapes, so the runes are the
		// literal text.
		sb.writeLiteral(string(re.Rune))
		return nil
	case syntax.OpConcat:
		for _, sub := range re.Sub {
			if err := walkEncodeAST(rt, sub, sb); err != nil {
				return err
			}
		}
		return nil
	case syntax.OpCapture:
		return walkCapture(rt, re, sb)
	case syntax.OpBeginLine, syntax.OpBeginText,
		syntax.OpEndLine, syntax.OpEndText,
		syntax.OpWordBoundary, syntax.OpNoWordBoundary,
		syntax.OpEmptyMatch:
		// Anchors and zero-width assertions match no text — nothing to emit.
		return nil
	case syntax.OpAlternate:
		return notInvertibleError("an alternation (`|`)")
	case syntax.OpStar, syntax.OpPlus, syntax.OpQuest, syntax.OpRepeat:
		return notInvertibleError("a quantifier (`*`, `+`, `?`, or `{n,m}`)")
	case syntax.OpCharClass:
		return notInvertibleError("a character class (`[...]`)")
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		return notInvertibleError("an any-character wildcard (`.`)")
	default:
		return notInvertibleError(fmt.Sprintf("a non-invertible construct (%s)", re.Op))
	}
}

// walkCapture inverts a capture group. A named group becomes a field
// substitution (resolved and validated exactly as the decode side would); an
// unnamed group is invertible only when its body reduces to pure literal text,
// since no field exists to fill it.
func walkCapture(rt reflect.Type, re *syntax.Regexp, sb *encodeSegmentBuilder) error {
	if re.Name == "" {
		s, ok := literalString(re.Sub[0])
		if !ok {
			return notInvertibleError("an unnamed capturing group with non-literal content")
		}
		sb.writeLiteral(s)
		return nil
	}
	idx, opts, ok := resolveEncodeField(rt, re.Name)
	if !ok {
		return fmt.Errorf("%w: capture group %q maps to no exported field of %v", ErrInvalidStruct, re.Name, rt)
	}
	if err := validateEncodeField(rt.Field(idx)); err != nil {
		return err
	}
	sb.addField(encodeSegment{
		field:      true,
		fieldIndex: idx,
		name:       re.Name,
		opts:       opts,
	})
	return nil
}

// literalString reports whether re reduces to a fixed literal string with no
// variable-matching content, returning that string. Literals concatenate,
// zero-width assertions contribute nothing, and a nested unnamed group recurses;
// a named capture (which would need a field) or any variable construct makes the
// subtree non-literal. Used to decide whether an unnamed group is invertible.
func literalString(re *syntax.Regexp) (string, bool) {
	switch re.Op {
	case syntax.OpLiteral:
		return string(re.Rune), true
	case syntax.OpEmptyMatch,
		syntax.OpBeginLine, syntax.OpBeginText,
		syntax.OpEndLine, syntax.OpEndText,
		syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return "", true
	case syntax.OpConcat:
		var b strings.Builder
		for _, sub := range re.Sub {
			s, ok := literalString(sub)
			if !ok {
				return "", false
			}
			b.WriteString(s)
		}
		return b.String(), true
	case syntax.OpCapture:
		if re.Name != "" {
			// A named capture needs a field to fill it — not a pure literal.
			return "", false
		}
		return literalString(re.Sub[0])
	default:
		return "", false
	}
}

// notInvertibleError builds an [ErrNotInvertible]-wrapped error naming the
// offending construct.
func notInvertibleError(construct string) error {
	return fmt.Errorf("%w: contains %s outside a named capture group", ErrNotInvertible, construct)
}

// resolveEncodeField maps a capture-group name to an exported, non-excluded field
// of rt, using the same field-mapping rules the decode side applies: a field's
// `regex:"name"` tag matched exactly, otherwise the field's own name matched
// exactly first and then case-insensitively via Unicode simple-fold (mirroring
// matchGroupName). Returns the field index, its parsed tag options, and true on
// a match; ("", nil, false) when no field resolves.
func resolveEncodeField(rt reflect.Type, name string) (int, map[string]string, bool) {
	// Exact pass first so an exact name never loses to an earlier fold sibling.
	for i := range rt.NumField() {
		sf := rt.Field(i)
		if !sf.IsExported() {
			continue
		}
		candidate, opts, _, skip := fieldCandidateName(sf)
		if skip {
			continue
		}
		if candidate == name {
			return i, opts, true
		}
	}
	// Fold pass: only untagged fields fold. The decode side folds solely the
	// field-name fallback (matchGroupName); an explicit `regex:` tag is matched
	// exactly (subexpIndexes). Folding a tag here would bind a group that the
	// decoder maps back to nothing, silently corrupting the round-trip — e.g. a
	// field `regex:"ID,default=x"` against a `(?P<id>…)` group would Encode via
	// the fold yet Decode to the default. See buildDecodePlan in decoder.go.
	for i := range rt.NumField() {
		sf := rt.Field(i)
		if !sf.IsExported() {
			continue
		}
		candidate, opts, tagged, skip := fieldCandidateName(sf)
		if skip || tagged {
			continue
		}
		if strings.EqualFold(candidate, name) {
			return i, opts, true
		}
	}
	return 0, nil, false
}

// fieldCandidateName returns the name a field is addressable by — its
// `regex:"name"` tag name when set, otherwise its own field name — plus the
// parsed tag options, whether the name came from an explicit tag (tagged), and
// whether the field is excluded (`regex:"-"`). Callers fold only untagged
// candidates, mirroring the decoder's exact-tag / fold-field-name split.
func fieldCandidateName(sf reflect.StructField) (name string, opts map[string]string, tagged, skip bool) {
	// required is a decode-side presence flag; encoding always emits the field's
	// actual value, so it is irrelevant here.
	tagName, opts, _, skip := parseFieldTag(sf)
	if skip {
		return "", nil, false, true
	}
	if tagName == "" {
		return sf.Name, opts, false, false
	}
	return tagName, opts, true, false
}

// validateEncodeField rejects, at construction time, a mapped field whose type
// [Encoder.Encode] could never render, giving [Decoder.Encoder] the same "a
// successful construction can't produce a mapping error later" guarantee that
// [Compile] gives [Decoder], wrapping [ErrInvalidStruct] like Compile does.
//
// It does not re-check `layout=` placement: [Compile] already rejects `layout=`
// on a non-time.Time field, so any field reaching here through a compiled
// [Decoder] has a valid layout option.
func validateEncodeField(sf reflect.StructField) error {
	if !encodableType(sf.Type) {
		return fmt.Errorf("%w: field %s has unsupported type %v", ErrInvalidStruct, sf.Name, sf.Type)
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

// Encode renders v into a string by walking e's derived plan: literal segments
// pass through and each named-group slot is replaced with the encoded value of
// its struct field.
//
// Returns an [EncodeError] (wrapped with the entrypoint prefix) if a field
// cannot be rendered at runtime — a custom [RegexMarshaler] / [encoding.TextMarshaler]
// returning an error, or a nil pointer or nil interface field, which has no
// string form.
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

// encodeFieldValue renders one struct field to its string form — the inverse of
// setFieldValue, dispatching in the same precedence order so a type round-trips
// symmetrically: custom [RegexMarshaler] first, then the time.Time /
// time.Duration special cases, then [encoding.TextMarshaler], then the built-in
// kind switch. `opts` carries the field's parsed tag options; only `layout` (for
// time.Time) is consulted.
func encodeFieldValue(field reflect.Value, opts map[string]string) (string, error) {
	// 0. Pointer fields: a nil pointer has no string form (every derived slot is
	//    required), so it is an error; otherwise dispatch on the pointer's own
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
