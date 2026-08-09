package regextra

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNoMatch is returned by [Decoder.One] when the target string does not
// match the regex. Compare with errors.Is so callers can distinguish "no
// match" from genuine decoding failures.
var ErrNoMatch = errors.New("regextra: no match")

// ErrInvalidPattern and ErrInvalidStruct categorize [Compile] (and therefore
// [MustCompile]) failures so callers can branch on the failure kind with
// errors.Is rather than parsing the message. ErrInvalidPattern wraps a bad
// regular expression; ErrInvalidStruct wraps every destination-shape problem
// (T is not a struct, a field references an undeclared group, a `default=`
// value does not convert, `layout=` sits on a non-time.Time field, the
// `inline` flag sits on a non-embedded or non-struct field or is combined
// with a group name, or `inline`-promoted fields bind the same capture group
// at equal depth with no sole tagged binding to break the tie). Each
// wrapped error keeps its descriptive detail — and, where one exists, the
// underlying cause — reachable via errors.Is/As. Like ErrNoMatch, these
// sentinels carry the bare `regextra:` prefix reserved for package-level
// sentinels.
var (
	ErrInvalidPattern = errors.New("regextra: invalid pattern")
	ErrInvalidStruct  = errors.New("regextra: invalid struct")
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

// ErrValueMismatch categorizes an [Encoder.EncodeStrict] failure where an
// encoded field value does not re-match the sub-pattern of the capture group it
// fills, so the output could decode to something other than the input. Callers
// can branch on the failure kind with errors.Is rather than parsing the message;
// the surrounding [EncodeError] (recover with errors.As) names the field and
// group. Like [ErrNoMatch] and [ErrNotInvertible], it carries the bare
// `regextra:` prefix reserved for package-level sentinels.
var ErrValueMismatch = errors.New("regextra: encoded value does not match group sub-pattern")

// MissingNamedGroupsError reports the required group names that [Validate]
// could not find declared on the pattern. It is returned (wrapped with the
// `regextra.Validate:` prefix) whenever at least one required name is missing.
// Recover it with [errors.As] to branch on the missing set without parsing
// message text:
//
//	var ve *regextra.MissingNamedGroupsError
//	if errors.As(err, &ve) {
//	    log.Printf("pattern is missing groups: %v", ve.Missing)
//	}
//
// Missing lists the absent names in the order they were passed to Validate.
// Unlike [DecodeError] there is no underlying cause to unwrap — Missing is the
// payload.
type MissingNamedGroupsError struct {
	// Missing holds the declared-but-absent required group names, in the order
	// they were passed to Validate.
	Missing []string
}

// Error implements the error interface. Validate prepends its own
// `regextra.Validate:` prefix when wrapping. When Missing is empty (only
// reachable by constructing the value directly — Validate never wraps an
// empty set) it reports "no missing named groups" rather than a message with
// a dangling separator.
func (e *MissingNamedGroupsError) Error() string {
	if len(e.Missing) == 0 {
		return "no missing named groups"
	}
	return fmt.Sprintf("missing named groups: %s", strings.Join(e.Missing, ", "))
}

// DecodeError reports the failure to convert a matched capture-group value
// into its destination struct field. It is returned (wrapped with the calling
// entrypoint's prefix) by [Unmarshal], [UnmarshalAll], [Decoder.One],
// [Decoder.All], and [Decoder.Iter] when a field's type conversion fails on a
// participating match. Recover it with [errors.As] to branch on the failure
// without parsing message text:
//
//	var de *regextra.DecodeError
//	if errors.As(err, &de) {
//	    log.Printf("field %s (group %s) could not parse %q as %s", de.Field, de.Group, de.Value, de.Type)
//	}
//
// Err holds the underlying conversion cause (e.g. a *strconv.NumError or a
// time-parsing error) and is reachable via [errors.Is]/[errors.As] through
// Unwrap. No match is not a DecodeError — [Unmarshal]/[UnmarshalAll] return nil
// and [Decoder.One] returns [ErrNoMatch] in that case.
type DecodeError struct {
	// Field is the destination struct field name.
	Field string
	// Group is the capture group the value was read from: the field's
	// `regex:"..."` tag name when set, otherwise the declared group whose name
	// matches the field name. It is empty only when the field maps to no
	// declared group at all — a default-only field. Unmarshal and Decoder share
	// one decode path, so that empty-Group case differs by strictness, not by
	// API: it is observable only on the lenient [Unmarshal]/[UnmarshalAll] path,
	// where a `default=` value that fails to convert raises a DecodeError at
	// decode time. The strict [Decoder.One]/[Decoder.All]/[Decoder.Iter] path
	// validates such defaults at [Compile], so it never surfaces an empty-Group
	// DecodeError at runtime.
	Group string
	// Value is the raw matched string (or substituted default) that failed to
	// convert.
	Value string
	// Type is the destination field's type, rendered (e.g. "int", "time.Time").
	Type string
	// Err is the underlying conversion error.
	Err error
}

// Error implements the error interface. The calling entrypoint prepends its
// own `regextra.<Entrypoint>:` prefix when wrapping. When Err is nil (only
// reachable by constructing the value directly — the decode path always sets
// an underlying cause) it reports "no decode error" rather than a message with
// a dangling "<nil>" cause.
func (e *DecodeError) Error() string {
	if e.Err == nil {
		return "no decode error"
	}
	return fmt.Sprintf("field %s: %v", e.Field, e.Err)
}

// Unwrap returns the underlying conversion error so [errors.Is]/[errors.As]
// can reach it.
func (e *DecodeError) Unwrap() error { return e.Err }

// RequiredGroupError reports that a field marked `regex:",required"` produced no
// value for a match: its capture group did not participate, or matched an empty
// span, and no `default=` supplied a substitute. It is returned (wrapped with the
// calling entrypoint's prefix) by [Unmarshal], [UnmarshalAll], [Decoder.One],
// [Decoder.All], and [Decoder.Iter]. Recover it with [errors.As] to branch on
// the missing field without parsing message text:
//
//	var rge *regextra.RequiredGroupError
//	if errors.As(err, &rge) {
//	    log.Printf("field %s (group %s) is required but had no value", rge.Field, rge.Group)
//	}
//
// It complements [DecodeError] (a participating value that failed type
// conversion) and [MissingNamedGroupsError] (a static [Validate] check that the
// pattern declares a group at all): RequiredGroupError is the per-match
// presence check. Like MissingNamedGroupsError, there is no underlying cause to
// unwrap — the absent value is the payload.
type RequiredGroupError struct {
	// Field is the destination struct field name.
	Field string
	// Group is the capture group the required value was expected from: the
	// field's `regex:"..."` tag name when set, otherwise the declared group
	// whose name matches the field name. It is empty only when a `required`
	// field maps to no declared group at all.
	Group string
}

// Error implements the error interface. The calling entrypoint prepends its own
// `regextra.<Entrypoint>:` prefix when wrapping. When Field is empty (only
// reachable by constructing the value directly — the decode path always sets the
// field name) it reports "no required group error" rather than a message about a
// nameless field.
func (e *RequiredGroupError) Error() string {
	if e.Field == "" {
		return "no required group error"
	}
	return fmt.Sprintf("field %s: required group %q produced no value", e.Field, e.Group)
}

// EncodeError reports the failure to render a struct field into its capture-group
// slot. It is the encode-side mirror of [DecodeError], returned (wrapped with the
// calling entrypoint's prefix) by [Encoder.Encode] and [Encoder.EncodeStrict]
// when a field's value cannot be converted to a string, and by EncodeStrict
// additionally when an encoded value fails its group's re-match check (Err wraps
// [ErrValueMismatch]). Recover it with [errors.As] to branch on the failure
// without parsing message text:
//
//	var ee *regextra.EncodeError
//	if errors.As(err, &ee) {
//	    log.Printf("field %s (group %s) of type %s could not encode: %v", ee.Field, ee.Group, ee.Type, ee.Err)
//	}
//
// Err holds the underlying cause (e.g. an error from a custom [RegexMarshaler]
// or [encoding.TextMarshaler], a nil-pointer or nil-interface field, or
// EncodeStrict's [ErrValueMismatch]) and is reachable via
// [errors.Is]/[errors.As] through Unwrap.
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
