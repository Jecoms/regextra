package regextra

import (
	"encoding"
	"fmt"
	"reflect"
	"regexp"
	"regexp/syntax"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

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
//     is excluded. Fields of an embedded struct tagged `regex:",inline"` are
//     promoted candidates too, searched depth-by-depth so a shallower field
//     shadows a deeper promoted one — the same precedence the decode plan
//     applies (see [Unmarshal]). Reading a promoted field through a nil
//     embedded pointer fails Encode with an [EncodeError]: the decode side
//     allocates the intermediate, but an absent source cannot be read. The
//     field's value fills the span; the group's sub-pattern is
//     not part of the emitted text but is retained (compiled as an anchored
//     matcher) for [Encoder.EncodeStrict]'s re-match check.
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
// are out of scope. [Encoder.EncodeStrict] verifies the per-group condition at
// encode time — each encoded value is re-matched (fully anchored) against its
// group's sub-pattern, and a miss fails with [ErrValueMismatch] — while the
// delimiter-collision and adjacent-captures cases remain out of scope for it
// too. [Encoder.Encode] performs no verification.
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
	// fieldIndex is the index path from T to the field, in the shape of
	// [reflect.StructField.Index]: one element for a top-level field, one
	// additional element per `inline`-promoted embedding level. Encode walks
	// the path with encodeFieldPath, which errors on a nil embedded pointer
	// (the decode side allocates instead — an absent value can be created, but
	// one cannot be read). Valid only when field is true.
	fieldIndex []int
	// name is the capture-group name the segment resolved from, retained for
	// EncodeError.Group. Valid only when field is true.
	name string
	// opts is the parsed tag options map for the field (e.g. {"layout": "..."}).
	// Nil if the field has no options.
	opts map[string]string
	// subPattern is the source text of the group's sub-pattern (re-rendered from
	// its parsed AST), retained for the EncodeStrict mismatch message. Valid only
	// when field is true.
	subPattern string
	// strictRE is subPattern compiled as the anchored matcher `\A(?:sub)\z`,
	// consulted only by EncodeStrict to verify the encoded value re-matches the
	// group it fills. When the sub-pattern contains a zero-width assertion,
	// compileStrictContext rebuilds it with one rune of adjacent literal context
	// baked in on each side (`\A<prev>(?:sub)<next>\z`) so edge assertions
	// evaluate against the characters the emitted string actually places there.
	// Compiled eagerly at derivation so Encoders keep no shared mutable state
	// after construction. Valid only when field is true.
	strictRE *regexp.Regexp
	// strictPrefix and strictSuffix are the context runes baked into strictRE
	// (empty when strictRE is the bare anchored matcher). EncodeStrict matches
	// strictRE against strictPrefix+value+strictSuffix. Valid only when field is
	// true.
	strictPrefix string
	strictSuffix string
	// ctxSensitive records whether subPattern contains a zero-width assertion
	// (`\b`, `\B`, `^`, `$`, `\A`, `\z`) whose truth at the value's edges depends
	// on adjacent characters. Consulted only by compileStrictContext during
	// derivation. Valid only when field is true.
	ctxSensitive bool
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
// Conversion precedence mirrors the decode side (see [RegexUnmarshaler]): for
// each field, Encode tries (1) RegexMarshaler, (2) the time.Time /
// time.Duration special-cases, (3) [encoding.TextMarshaler], (4) the built-in
// string/int/uint/float/bool conversion. A type implementing both
// RegexMarshaler and [encoding.TextMarshaler] therefore dispatches on
// MarshalRegex.
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
// The latter two wrap [ErrInvalidStruct], mirroring [Compile].
// [Decoder.MustEncoder] panics with the same wrapped error. Once Encoder
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
	if err := compileStrictContext(sb.segments); err != nil {
		return nil, err
	}

	return &Encoder[T]{
		rtype:    rt,
		segments: sb.segments,
	}, nil
}

// compileStrictContext rebuilds the strict matcher of every field segment whose
// sub-pattern contains a zero-width assertion, baking one rune of adjacent
// literal context into each side (`\A<prev>(?:sub)<next>\z`) so assertions at
// the value's edges — `\b`, `\B`, a multiline `^`/`$`, `\A`, `\z` — evaluate
// against the characters the emitted string actually places there, not against
// the bare value in isolation. Every RE2 zero-width assertion examines at most
// one adjacent rune, so one rune of context is exact; and because the quoted
// context runes are fixed-length between the `\A`/`\z` anchors, the sub-pattern
// is still forced to match exactly the value span. A side with no adjacent
// literal contributes no context: at the plan's boundary the bare anchored
// matcher is already the true context, and a side adjoining another field
// segment (adjacent captures, no literal between) is unknowable — that
// ambiguity is already out of scope for the round-trip contract.
func compileStrictContext(segs []encodeSegment) error {
	for i := range segs {
		if !segs[i].field || !segs[i].ctxSensitive {
			continue
		}
		var prev, next string
		if i > 0 && !segs[i-1].field {
			r, _ := utf8.DecodeLastRuneInString(segs[i-1].literal)
			prev = string(r)
		}
		if i+1 < len(segs) && !segs[i+1].field {
			r, _ := utf8.DecodeRuneInString(segs[i+1].literal)
			next = string(r)
		}
		if prev == "" && next == "" {
			continue
		}
		re, err := regexp.Compile(`\A` + regexp.QuoteMeta(prev) + `(?:` + segs[i].subPattern + `)` + regexp.QuoteMeta(next) + `\z`)
		if err != nil {
			// Unreachable in practice — the bare form of the same sub-pattern
			// already compiled in walkCapture — but surfaced defensively rather
			// than panicked, mirroring that branch.
			return fmt.Errorf("%w: %w", ErrInvalidPattern, err)
		}
		segs[i].strictRE = re
		segs[i].strictPrefix = prev
		segs[i].strictSuffix = next
	}
	return nil
}

// containsZeroWidthAssertion reports whether re's subtree contains a zero-width
// assertion (`^`, `$`, `\A`, `\z`, `\b`, `\B`) — the constructs whose truth at
// the edges of a matched span depends on the characters adjacent to it.
func containsZeroWidthAssertion(re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpBeginLine, syntax.OpEndLine,
		syntax.OpBeginText, syntax.OpEndText,
		syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return true
	}
	for _, sub := range re.Sub {
		if containsZeroWidthAssertion(sub) {
			return true
		}
	}
	return false
}

// MustEncoder is like [Decoder.Encoder] but panics on error. Intended for
// package-level vars where startup-time failure is the right behavior,
// mirroring [MustCompile]:
//
//	var personDecoder = regextra.MustCompile[Person](`(?P<name>\w+) is (?P<age>\d+)`)
//	var personEncoder = personDecoder.MustEncoder()
func (d *Decoder[T]) MustEncoder() *Encoder[T] {
	e, err := d.Encoder()
	if err != nil {
		panic(err)
	}
	return e
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
	if err := validateEncodeField(rt.FieldByIndex(idx)); err != nil {
		return err
	}
	// Retain the group's sub-pattern as an anchored matcher for EncodeStrict.
	// The AST re-render bakes flags per node (a `(?i)` literal renders as
	// `(?i:…)`), so the standalone compile preserves the original semantics.
	subPattern := re.Sub[0].String()
	strictRE, err := regexp.Compile(`\A(?:` + subPattern + `)\z`)
	if err != nil {
		// Unreachable in practice — the sub-pattern re-renders from an AST that
		// already parsed — but surfaced defensively rather than panicked,
		// mirroring the parse branch in Encoder().
		return fmt.Errorf("%w: %w", ErrInvalidPattern, err)
	}
	sb.addField(encodeSegment{
		field:        true,
		fieldIndex:   idx,
		name:         re.Name,
		opts:         opts,
		subPattern:   subPattern,
		strictRE:     strictRE,
		ctxSensitive: containsZeroWidthAssertion(re.Sub[0]),
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
// exactly first and then case-insensitively via Unicode simple case folding
// (mirroring matchGroupName). Returns the field's index path, its parsed tag
// options, and true on a match; (nil, nil, false) when no field resolves.
//
// `inline`-promoted embedded structs are searched breadth-first, one embedding
// depth at a time, so a shallower field always wins over a promoted deeper one —
// the same shadowing rule buildDecodePlan applies on the decode side
// (resolvePromotionShadowing). Within one depth the exact pass runs before the
// fold pass, so an exact name never loses to a fold sibling. Below the top
// level the exact pass prefers an explicitly tagged binding over an untagged
// exact one regardless of field order, mirroring the decode side's
// tagged-beats-untagged tiebreak; at the top level, where the decode plan
// keeps every binding of the group, the first exact match in field order picks
// the encode source. Ties the tiebreak cannot resolve need no check here:
// [Compile] already rejects them, and [Decoder.Encoder] is only reachable
// through a compiled Decoder. A visited-types set terminates pointer-embedding
// cycles, mirroring collectDecodeFields' guard.
func resolveEncodeField(rt reflect.Type, name string) ([]int, map[string]string, bool) {
	type level struct {
		path []int
		typ  reflect.Type
	}
	levels := []level{{nil, rt}}
	visited := []reflect.Type{rt}
	for top := true; len(levels) > 0; top = false {
		// Exact pass first so an exact name never loses to an earlier fold
		// sibling at the same depth. Below the top level a tagged exact match
		// beats an untagged one regardless of field order — the decode side's
		// tagged-beats-untagged tiebreak makes the tagged field the decode
		// winner, so it must be the encode source too. At the top level every
		// exact binding stays in the decode plan, so field order decides.
		var untaggedPath []int
		var untaggedOpts map[string]string
		for _, lv := range levels {
			for i := range lv.typ.NumField() {
				sf := lv.typ.Field(i)
				if !sf.IsExported() {
					continue
				}
				candidate, opts, tagged, promoted, skip := fieldCandidateName(sf)
				if skip || promoted {
					continue
				}
				if candidate != name {
					continue
				}
				if tagged || top {
					return appendFieldPath(lv.path, i), opts, true
				}
				if untaggedPath == nil {
					untaggedPath = appendFieldPath(lv.path, i)
					untaggedOpts = opts
				}
			}
		}
		if untaggedPath != nil {
			return untaggedPath, untaggedOpts, true
		}
		// Fold pass: only untagged fields fold. The decode side folds solely the
		// field-name fallback (matchGroupName); an explicit `regex:` tag is matched
		// exactly (subexpIndexes). Folding a tag here would bind a group that the
		// decoder maps back to nothing, silently corrupting the round-trip — e.g. a
		// field `regex:"ID,default=x"` against a `(?P<id>…)` group would Encode via
		// the fold yet Decode to the default. See buildDecodePlan in plan.go.
		for _, lv := range levels {
			for i := range lv.typ.NumField() {
				sf := lv.typ.Field(i)
				if !sf.IsExported() {
					continue
				}
				candidate, opts, tagged, promoted, skip := fieldCandidateName(sf)
				if skip || tagged || promoted {
					continue
				}
				if strings.EqualFold(candidate, name) {
					return appendFieldPath(lv.path, i), opts, true
				}
			}
		}
		// No match at this depth — descend one level into the promoted
		// embedded structs.
		var next []level
		for _, lv := range levels {
			for i := range lv.typ.NumField() {
				sf := lv.typ.Field(i)
				if !sf.IsExported() {
					continue
				}
				if _, _, _, promoted, _ := fieldCandidateName(sf); !promoted {
					continue
				}
				et := inlineStructType(sf)
				if typeOnPath(visited, et) {
					continue
				}
				visited = append(visited, et)
				next = append(next, level{path: appendFieldPath(lv.path, i), typ: et})
			}
		}
		levels = next
	}
	return nil, nil, false
}

// fieldCandidateName returns the name a field is addressable by — its
// `regex:"name"` tag name when set, otherwise its own field name — plus the
// parsed tag options, whether the name came from an explicit tag (tagged),
// whether the field dissolves into `inline` promotion (promoted: a valid
// nameless `inline` flag on an embedded struct, making the field itself not
// addressable by any group), and whether the field is excluded (`regex:"-"`).
// Callers fold only untagged candidates, mirroring the decoder's exact-tag /
// fold-field-name split.
func fieldCandidateName(sf reflect.StructField) (name string, opts map[string]string, tagged, promoted, skip bool) {
	// required is a decode-side presence flag; encoding always emits the field's
	// actual value, so it is irrelevant here.
	tagName, opts, _, inline, skip := parseFieldTag(sf)
	if skip {
		return "", nil, false, false, true
	}
	if inline && tagName == "" && inlineStructType(sf) != nil {
		return "", nil, false, true, false
	}
	if tagName == "" {
		return sf.Name, opts, false, false, false
	}
	return tagName, opts, true, false, false
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
//
// Encode performs no verification that the output re-decodes; see
// [Encoder.EncodeStrict] for the variant that re-matches each encoded value
// against its group's sub-pattern.
func (e *Encoder[T]) Encode(v T) (string, error) {
	return e.encode(v, false, "regextra.Encoder.Encode")
}

// EncodeStrict is like [Encoder.Encode] but additionally verifies that each
// encoded field value re-matches the sub-pattern of the capture group it fills —
// the exact per-group condition the round-trip contract (see [Encoder]) places
// on the caller. A value that would not re-match makes EncodeStrict return an
// [EncodeError] (recover with [errors.As]) naming the field and group, whose
// underlying cause wraps [ErrValueMismatch] and reports the rendered value and
// the sub-pattern it failed.
//
// Each check is a full anchored match (`\A(?:sub)\z`), so a value that only
// partially matches its sub-pattern fails. When the sub-pattern contains a
// zero-width assertion (`\b`, `\B`, a multiline `^`/`$`, `\A`, `\z`), the
// matcher additionally bakes in one rune of the adjacent literal segments so
// assertions at the value's edges evaluate against the characters the emitted
// string actually places there — `X(?P<v>\bfoo)` rejects v = "foo" (no word
// boundary between `X` and `f` in the output) even though `foo` alone matches
// `\bfoo` in isolation. An edge that adjoins another capture with no literal
// between has no known neighbor; it is checked without context, consistent with
// the adjacent-captures carve-out below. The cost is one regexp match per field
// per call; [Encoder.Encode] skips the checks entirely.
//
// EncodeStrict verifies the contract's stated per-group condition, not a full
// re-decode: values that collide with a surrounding literal delimiter, or two
// adjacent captures with no literal between them, remain out of scope (e.g.
// `(?P<a>.+)-(?P<b>.+)` with a = "x-y" passes per-group yet decodes
// differently).
func (e *Encoder[T]) EncodeStrict(v T) (string, error) {
	return e.encode(v, true, "regextra.Encoder.EncodeStrict")
}

// encode is the shared core of [Encoder.Encode] and [Encoder.EncodeStrict]:
// one plan walk, with the strict flag adding the anchored re-match check per
// field segment. entrypoint is the `regextra.<Entrypoint>` prefix the caller
// wraps its errors with.
func (e *Encoder[T]) encode(v T, strict bool, entrypoint string) (string, error) {
	// Reflect on v through its address so the value is addressable and fields
	// with pointer-receiver marshalers dispatch via Addr() — the same reason
	// setFieldValue relies on addressability on the decode side.
	// reflect.ValueOf(v) alone is not addressable; taking &v of the by-value
	// parameter gives the same addressable copy that reflect.New + Set built,
	// without allocating a second T or boxing v in an interface.
	rv := reflect.ValueOf(&v).Elem()

	var b strings.Builder
	for _, seg := range e.segments {
		if !seg.field {
			b.WriteString(seg.literal)
			continue
		}
		var s string
		field, err := encodeFieldPath(rv, seg.fieldIndex)
		if err == nil {
			s, err = encodeFieldValue(field, seg.opts)
		}
		if err == nil && strict {
			// The probe carries the adjacent-context runes baked into strictRE
			// (both empty for the common no-assertion sub-pattern, keeping the
			// fast path concatenation-free).
			probe := s
			if seg.strictPrefix != "" || seg.strictSuffix != "" {
				probe = seg.strictPrefix + s + seg.strictSuffix
			}
			if !seg.strictRE.MatchString(probe) {
				err = fmt.Errorf("%w: value %q does not match sub-pattern `%s`", ErrValueMismatch, s, seg.subPattern)
			}
		}
		if err != nil {
			sf := e.rtype.FieldByIndex(seg.fieldIndex)
			return "", fmt.Errorf("%s: %w", entrypoint, &EncodeError{
				Field: sf.Name,
				Group: seg.name,
				Type:  sf.Type.String(),
				Err:   err,
			})
		}
		b.WriteString(s)
	}
	return b.String(), nil
}

// encodeFieldPath resolves an encode segment's index path against rv (the
// addressable reflect.Value of the source struct), returning the leaf field.
// Intermediate steps are `inline`-promoted embedded fields; a pointer-embedded
// step that is nil fails — the leaf value cannot be read through it, mirroring
// encodeFieldValue's nil-pointer error (every derived slot must render). The
// decode side allocates the intermediate instead (walkFieldPath): an absent
// destination can be created, an absent source cannot. The common flat case
// (len(path) == 1) reduces to a single Field call.
func encodeFieldPath(rv reflect.Value, path []int) (reflect.Value, error) {
	field := rv.Field(path[0])
	for _, i := range path[1:] {
		if field.Kind() == reflect.Ptr {
			if field.IsNil() {
				return reflect.Value{}, fmt.Errorf("cannot encode through nil embedded pointer of type %s", field.Type())
			}
			field = field.Elem()
		}
		field = field.Field(i)
	}
	return field, nil
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
