package regextra

import (
	"encoding"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Cached reflect.Type values for the time types we special-case.
// Comparing reflect.Type by equality is faster than rebuilding via
// reflect.TypeOf on every field, and the resulting code is also clearer.
var (
	timeTimeType     = reflect.TypeOf(time.Time{})
	timeDurationType = reflect.TypeOf(time.Duration(0))
)

// timeLayouts is the ordered set of layouts tried when parsing a string
// into a time.Time field. The first layout that yields a non-error wins.
// RFC3339 (and its nano variant) come first because they're the most
// common in logs and APIs; the date / datetime / time-only forms cover
// human-readable inputs without time zones.
var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	time.DateTime, // "2006-01-02 15:04:05"
	time.DateOnly, // "2006-01-02"
	time.TimeOnly, // "15:04:05"
}

// parseTime tries each layout in timeLayouts and returns the first success.
func parseTime(value string) (time.Time, error) {
	var firstErr error
	for _, layout := range timeLayouts {
		t, err := time.Parse(layout, value)
		if err == nil {
			return t, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return time.Time{}, firstErr
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

// Unmarshal extracts named capture groups from the target string and assigns them
// to the corresponding fields in the provided struct pointer.
//
// Field mapping rules:
//   - First checks the `regex:"groupname"` struct tag if provided (highest priority)
//   - Falls back to exact field name match with capture group name
//   - Falls back to case-insensitive field name match (Unicode simple case folding)
//   - Supports type conversion for string, bool, all int/uint widths,
//     float32/float64, time.Time, time.Duration, and single-level pointers
//     to any of these; types implementing [RegexUnmarshaler] convert themselves
//   - Unexported fields are ignored
//   - A group that did not participate in the match (e.g. an optional group),
//     or that matched an empty span, leaves the field unchanged — unless the
//     field has a `default=` tag option, which substitutes instead
//
// On no match, Unmarshal returns nil and leaves *v unchanged — no match is data
// absence, not a failure. Callers who need to distinguish "matched" from
// "didn't match" should inspect their struct after the call, or use
// [Decoder.One] which signals no-match via [ErrNoMatch]. See the package doc's
// "No-match behavior" section for the full cross-API contract.
//
// Returns an error if:
//   - v is not a pointer to a struct
//   - Type conversion fails on a matched group
//   - A matched field is a nested struct, slice, or map: these are not
//     flattened, so a group bound to one yields an "unsupported field type"
//     error (unless the type implements [RegexUnmarshaler] or
//     encoding.TextUnmarshaler, which convert themselves)
//
// Example:
//
//	type Person struct {
//	    Name string
//	    Age  int `regex:"age"`
//	}
//	re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
//	var person Person
//	err := regextra.Unmarshal(re, "Alice is 30", &person)
//	// person.Name = "Alice", person.Age = 30
func Unmarshal(re *regexp.Regexp, target string, v any) error {
	// Get reflection value and validate it's a pointer to struct
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("regextra.Unmarshal: requires a non-nil pointer to a struct, got %T", v)
	}

	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		return fmt.Errorf("regextra.Unmarshal: requires a pointer to a struct, got pointer to %s", elem.Kind())
	}

	// Find the match. The Index variant distinguishes non-participating group
	// occurrences from empty matches, which runDecodePlan needs to pick the
	// occurrence that actually participated when a name is reused.
	matches := re.FindStringSubmatchIndex(target)
	if matches == nil {
		return nil // No match, but not an error
	}

	// Build an uncached decode plan for this struct and run it — the same plan
	// build/run the cached Decoder uses, in lenient mode (strict=false) so
	// Unmarshal stays best-effort rather than erroring on undeclared groups or
	// misplaced tag options. buildDecodePlan never returns an error when
	// strict=false; the check is kept for forward-safety.
	fields, err := buildDecodePlan(elem.Type(), re, false)
	if err != nil {
		return fmt.Errorf("regextra.Unmarshal: %w", err)
	}
	if err := runDecodePlan(re, fields, elem, target, matches); err != nil {
		return fmt.Errorf("regextra.Unmarshal: %w", err)
	}
	return nil
}

// UnmarshalAll extracts all occurrences of the regex pattern from the target string
// and unmarshals them into a slice of structs. The slice is cleared before populating.
//
// v must be a pointer to a slice of structs. On no matches, UnmarshalAll returns
// nil and sets the slice length to 0 — no match is data absence, not a failure.
// See the package doc's "No-match behavior" section for the full cross-API
// contract.
//
// Example:
//
//	type Person struct {
//	    Name string
//	    Age  int
//	}
//	re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
//	var people []Person
//	err := regextra.UnmarshalAll(re, "Alice is 30 and Bob is 25", &people)
//	// people = []Person{{Name: "Alice", Age: 30}, {Name: "Bob", Age: 25}}
func UnmarshalAll(re *regexp.Regexp, target string, v any) error {
	// Get reflection value and validate it's a pointer to slice
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("regextra.UnmarshalAll: requires a non-nil pointer to a slice, got %T", v)
	}

	elem := rv.Elem()
	if elem.Kind() != reflect.Slice {
		return fmt.Errorf("regextra.UnmarshalAll: requires a pointer to a slice, got pointer to %s", elem.Kind())
	}

	// Get the slice element type and verify it's a struct
	sliceElemType := elem.Type().Elem()
	if sliceElemType.Kind() != reflect.Struct {
		return fmt.Errorf("regextra.UnmarshalAll: requires a slice of structs, got slice of %s", sliceElemType.Kind())
	}

	// Find all matches. The Index variant distinguishes non-participating group
	// occurrences from empty matches, which runDecodePlan needs to pick the
	// occurrence that actually participated when a name is reused.
	allMatches := re.FindAllStringSubmatchIndex(target, -1)
	if len(allMatches) == 0 {
		// Clear the slice and return (no matches is not an error)
		elem.SetLen(0)
		return nil
	}

	// Build the decode plan once for the whole call, then run it for every match
	// into a pre-sized slice. This replaces the old per-match map build +
	// per-field tag re-parse: the plan (group indexes + parsed options) is
	// computed once and reused, and each match decodes in place — no reflect.New
	// / reflect.Append copy and no map churn per match. strict=false keeps the
	// lenient Unmarshal posture (see Unmarshal); the error is never non-nil here.
	fields, err := buildDecodePlan(sliceElemType, re, false)
	if err != nil {
		return fmt.Errorf("regextra.UnmarshalAll: %w", err)
	}
	newSlice := reflect.MakeSlice(elem.Type(), len(allMatches), len(allMatches))
	for idx, matches := range allMatches {
		if err := runDecodePlan(re, fields, newSlice.Index(idx), target, matches); err != nil {
			return fmt.Errorf("regextra.UnmarshalAll: match %d: %w", idx, err)
		}
	}

	// Set the slice to the new value
	elem.Set(newSlice)
	return nil
}

// parseFieldTag parses a `regex:"name,key=value,key=value"` struct tag into
// the group name and an options map. The grammar is JSON-encoding-style: the
// first comma-separated piece is the name; each subsequent piece is a
// `key=value` pair.
//
// Currently recognized option keys (case-sensitive):
//   - default — value substituted when the named group is not declared on the
//     regex or its match is empty.
//   - layout  — for time.Time fields only: a single time.Parse layout used
//     instead of the default fallback list.
//
// The `required` flag (a lone token, no `=`) marks the field's group as
// mandatory: decode fails with a *[RequiredGroupError] when the group does not
// participate in a match or matches an empty span and no `default=` supplies a
// value. It is the first recognized lone-token flag (the slot the forward-compat
// rules below reserved).
//
// Forward-compat rules (locked in as v1 contract — see the package doc's
// "Tag grammar" section for the full statement and rationale):
//   - Unknown key=value pairs are preserved in the returned map so future
//     option additions don't need to touch the parser; adding a new option
//     key is therefore not a breaking change.
//   - Lone tokens without `=` other than the recognized `required` flag are
//     silently ignored today; the slot remains reserved for future flag-style
//     options, so callers must not rely on an unrecognized lone token staying
//     inert.
//
// The two forms differ:
//   - `regex:""` (no tag) signals "no name", returning ("", nil, false); the
//     caller falls back to matching the field's own name against a group.
//   - `regex:"-"` signals "exclude this field", returning ("", nil, true); the
//     caller excludes the field entirely, never attempting a name fallback. This
//     mirrors the `-` convention in encoding/json, encoding/xml, and
//     gopkg.in/yaml. Only the bare `-` tag excludes; a leading `-` followed by
//     options (e.g. `regex:"-,default=x"`) parses `-` as the group name, which
//     matches no group since group names are Go identifiers.
func parseFieldTag(field reflect.StructField) (name string, opts map[string]string, required, skip bool) {
	tag := field.Tag.Get("regex")
	if tag == "-" {
		return "", nil, false, true
	}
	if tag == "" {
		return "", nil, false, false
	}
	parts := strings.Split(tag, ",")
	name = strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return name, nil, false, false
	}
	for _, p := range parts[1:] {
		p = strings.TrimSpace(p)
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			// No '=': a lone token. `required` is the one recognized flag; it
			// marks the field's group mandatory (enforced in runDecodePlan).
			// Any other lone token — including an empty piece from a doubled,
			// leading, or trailing comma — is silently ignored to keep the
			// parser forward-compatible. An empty piece needs no separate guard:
			// strings.Cut("", "=") returns ok=false, so it lands here too.
			if k == "required" {
				required = true
			}
			continue
		}
		// Allocate the options map lazily — only a key=value pair populates it.
		// A field with just a lone flag (e.g. `name,required`) keeps opts nil,
		// matching the no-options case (parts==1) and avoiding a per-call,
		// per-field empty-map allocation on the Unmarshal hot path. Consumers
		// already treat nil opts as "no options" (nil-map reads are zero-value).
		if opts == nil {
			opts = make(map[string]string, len(parts)-1)
		}
		opts[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return name, opts, required, false
}

// resolveGroupValue decides what a field receives given its group's raw match
// state: the matched value when one is usable, the `default=` option when not,
// or nothing (ok=false, skip the field, leaving it unchanged).
//
// This is the single source of truth for the skip-or-default contract that
// runDecodePlan applies for both the Unmarshal and Decoder paths — the two
// paths drifted on exactly this logic once (issue #104), so it lives in one
// place now.
//
// `default=` substitutes when the field has no match OR the match is empty.
// Empty-match overlap is intentional: regexp returns "" both for
// non-participating optional groups and for groups that matched a zero-length
// span; treating both as "no useful value" matches caller expectations. With
// no default, an empty value skips the field entirely rather than feeding ""
// to the type converter — an optional group that didn't participate is data
// absence, not a conversion failure.
func resolveGroupValue(value string, found bool, opts map[string]string) (string, bool) {
	if found && value != "" {
		return value, true
	}
	if def, ok := opts["default"]; ok {
		return def, true
	}
	return "", false
}

// RegexUnmarshaler is the interface implemented by types that know how to
// initialize themselves from a regex group's matched string. It mirrors
// [encoding.TextUnmarshaler] for the regextra unmarshal path: when a
// destination field's pointer type satisfies this interface, [Unmarshal]
// (and [UnmarshalAll]) call UnmarshalRegex with the matched group value
// instead of running the built-in string/int/uint/float/bool conversion.
//
// This is the extension point for caller-defined types that the built-in
// type switch can't handle (URLs, enums, big numbers, IP addresses, etc.).
//
// Example:
//
//	type Status int
//
//	func (s *Status) UnmarshalRegex(value string) error {
//	    switch value {
//	    case "open":   *s = StatusOpen
//	    case "closed": *s = StatusClosed
//	    default:       return fmt.Errorf("unknown status: %q", value)
//	    }
//	    return nil
//	}
type RegexUnmarshaler interface {
	UnmarshalRegex(value string) error
}

// regexUnmarshalerType is the reflect.Type of RegexUnmarshaler, cached so
// the implements-check on every field doesn't pay the reflect.TypeOf cost.
var regexUnmarshalerType = reflect.TypeOf((*RegexUnmarshaler)(nil)).Elem()

// textUnmarshalerType is the reflect.Type of encoding.TextUnmarshaler, cached
// for the same reason as regexUnmarshalerType. Many stdlib and third-party
// types (netip.Addr, big.Int, slog.Level, uuid.UUID, ...) already implement
// it, so honoring it lets those drop into a struct field with no wrapper.
var textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()

// setFieldValue sets the field value with appropriate type conversion.
// `opts` carries per-field tag options parsed from `regex:"name,key=value,..."`.
// Currently consulted: `layout` (for time.Time fields). Pass nil for no opts.
func setFieldValue(field reflect.Value, value string, opts map[string]string) error {
	// 0. Pointer fields: allocate the pointee if nil, then either dispatch
	//    on the pointer's own RegexUnmarshaler (the common case for
	//    pointer-receiver methods) or recurse into the pointee for the
	//    built-in type conversions. Single-level pointers only —
	//    `**Foo` falls through to the recursive call which handles each
	//    level of indirection until the base case.
	if field.Kind() == reflect.Ptr {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		if u, ok := field.Interface().(RegexUnmarshaler); ok {
			return u.UnmarshalRegex(value)
		}
		return setFieldValue(field.Elem(), value, opts)
	}

	// 1. RegexUnmarshaler comes first for non-pointer fields — caller-defined
	//    conversions beat everything, including the stdlib special-cases
	//    below. A type `type MyTime time.Time` with its own UnmarshalRegex
	//    must NOT be pre-empted by the time.Time fast path.
	//
	//    Values reaching this function are addressable (struct fields via a
	//    pointer's Elem, reflect.New(...).Elem(), or a recursive Elem of a
	//    pointer field), and *T's method set includes T's value-receiver
	//    methods, so for concrete field types this check dispatches both
	//    pointer-receiver and value-receiver implementations.
	if field.CanAddr() {
		if u, ok := field.Addr().Interface().(RegexUnmarshaler); ok {
			return u.UnmarshalRegex(value)
		}
	}
	// The CanAddr check above does NOT cover interface-typed fields: an
	// interface field's Addr() is a *interface, which does not satisfy
	// RegexUnmarshaler even when the stored concrete value does. Dispatch
	// those on the field's own type/value (e.g. `F RegexUnmarshaler`
	// pre-populated with a concrete implementation).
	if field.Type().Implements(regexUnmarshalerType) {
		if u, ok := field.Interface().(RegexUnmarshaler); ok {
			return u.UnmarshalRegex(value)
		}
	}

	// 2. time.Time and time.Duration. Stdlib types we can't extend with
	//    RegexUnmarshaler, but they dominate real-world parsing needs. Caught
	//    by Type before the Kind switch because time.Duration's underlying
	//    Kind is reflect.Int64.
	switch field.Type() {
	case timeTimeType:
		var t time.Time
		var err error
		if layout, ok := opts["layout"]; ok && layout != "" {
			// Caller-supplied layout wins exclusively — no fallback list,
			// because if you specified a layout you want exactly that one.
			t, err = time.Parse(layout, value)
			if err != nil {
				return fmt.Errorf("cannot convert %q to time.Time using layout %q: %w", value, layout, err)
			}
		} else {
			t, err = parseTime(value)
			if err != nil {
				return fmt.Errorf("cannot convert %q to time.Time: %w", value, err)
			}
		}
		field.Set(reflect.ValueOf(t))
		return nil
	case timeDurationType:
		d, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("cannot convert %q to time.Duration: %w", value, err)
		}
		field.Set(reflect.ValueOf(d))
		return nil
	}

	// 3. encoding.TextUnmarshaler fallback. Ranks below RegexUnmarshaler (the
	//    package-specific extension point keeps priority) AND below the
	//    time.Time/time.Duration special-cases above: time.Time itself
	//    satisfies TextUnmarshaler but its UnmarshalText only accepts RFC3339,
	//    so checking it earlier would silently drop the multi-layout parseTime
	//    fallback and the `layout` tag option. Mirrors the RegexUnmarshaler
	//    dispatch: addressable fields via Addr(), interface-typed fields via
	//    Implements. Pointer fields are handled in step 0 (recurse into Elem,
	//    where the pointee is addressable and caught here).
	if field.CanAddr() {
		if u, ok := field.Addr().Interface().(encoding.TextUnmarshaler); ok {
			if err := u.UnmarshalText([]byte(value)); err != nil {
				return fmt.Errorf("cannot convert %q to %s: %w", value, field.Type(), err)
			}
			return nil
		}
	}
	if field.Type().Implements(textUnmarshalerType) {
		if u, ok := field.Interface().(encoding.TextUnmarshaler); ok {
			if err := u.UnmarshalText([]byte(value)); err != nil {
				return fmt.Errorf("cannot convert %q to %s: %w", value, field.Type(), err)
			}
			return nil
		}
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
		return nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// Parse at the field's actual bit width so out-of-range values error
		// instead of silently truncating on SetInt (same approach as
		// encoding/json).
		intVal, err := strconv.ParseInt(value, 10, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("cannot convert %q to %s: %w", value, field.Type(), err)
		}
		field.SetInt(intVal)
		return nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uintVal, err := strconv.ParseUint(value, 10, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("cannot convert %q to %s: %w", value, field.Type(), err)
		}
		field.SetUint(uintVal)
		return nil

	case reflect.Float32, reflect.Float64:
		floatVal, err := strconv.ParseFloat(value, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("cannot convert %q to %s: %w", value, field.Type(), err)
		}
		field.SetFloat(floatVal)
		return nil

	case reflect.Bool:
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("cannot convert %q to bool: %w", value, err)
		}
		field.SetBool(boolVal)
		return nil

	default:
		return fmt.Errorf("unsupported field type: %s", field.Kind())
	}
}
