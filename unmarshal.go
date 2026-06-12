package regextra

import (
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

// Unmarshal extracts named capture groups from the target string and assigns them
// to the corresponding fields in the provided struct pointer.
//
// Field mapping rules:
//   - First checks the `regex:"groupname"` struct tag if provided (highest priority)
//   - Falls back to exact field name match with capture group name
//   - Falls back to case-insensitive field name match
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
		return fmt.Errorf("regextra: Unmarshal requires a non-nil pointer to a struct, got %T", v)
	}

	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		return fmt.Errorf("regextra: Unmarshal requires a pointer to a struct, got pointer to %s", elem.Kind())
	}

	// Find the match. The Index variant distinguishes non-participating
	// group occurrences from empty matches, which namedGroupValues needs to
	// handle duplicate group names correctly.
	matches := re.FindStringSubmatchIndex(target)
	if matches == nil {
		return nil // No match, but not an error
	}

	// Populate the struct fields. includeNonParticipating=false: a declared
	// optional group that did not participate in the match is omitted rather
	// than recorded as "", so populateStruct leaves the field at its zero
	// value instead of feeding "" to a typed conversion (which would error).
	return populateStruct(elem, namedGroupValues(re, target, matches, false))
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
		return fmt.Errorf("regextra: UnmarshalAll requires a non-nil pointer to a slice, got %T", v)
	}

	elem := rv.Elem()
	if elem.Kind() != reflect.Slice {
		return fmt.Errorf("regextra: UnmarshalAll requires a pointer to a slice, got pointer to %s", elem.Kind())
	}

	// Get the slice element type and verify it's a struct
	sliceElemType := elem.Type().Elem()
	if sliceElemType.Kind() != reflect.Struct {
		return fmt.Errorf("regextra: UnmarshalAll requires a slice of structs, got slice of %s", sliceElemType.Kind())
	}

	// Find all matches. The Index variant distinguishes non-participating
	// group occurrences from empty matches, which namedGroupValues needs to
	// handle duplicate group names correctly.
	allMatches := re.FindAllStringSubmatchIndex(target, -1)
	if len(allMatches) == 0 {
		// Clear the slice and return (no matches is not an error)
		elem.SetLen(0)
		return nil
	}

	// SubexpNames is identical for every match, so fetch it once instead of per
	// match. Reuse a single groupValues map (cleared each iteration) rather than
	// allocating one per match, and size the result slice up front so each match
	// decodes into place — avoiding the reflect.New + reflect.Append copy the
	// append-grow loop paid per match.
	names := re.SubexpNames()
	newSlice := reflect.MakeSlice(elem.Type(), len(allMatches), len(allMatches))
	groupValues := make(map[string]string, len(names))
	for idx, matches := range allMatches {
		// includeNonParticipating=false (see Unmarshal): a non-participating
		// optional group is omitted so its typed field stays zero, and the
		// participating occurrence of a reused name wins.
		clear(groupValues)
		fillNamedGroupValues(groupValues, names, target, matches, false)
		if err := populateStruct(newSlice.Index(idx), groupValues); err != nil {
			return err
		}
	}

	// Set the slice to the new value
	elem.Set(newSlice)
	return nil
}

// populateStruct fills a struct's fields from a map of capture group values
func populateStruct(structValue reflect.Value, groupValues map[string]string) error {
	structType := structValue.Type()
	for i := range structValue.NumField() {
		field := structValue.Field(i)
		fieldType := structType.Field(i)

		// Skip unexported fields
		if !field.CanSet() {
			continue
		}

		// Determine the capture group name and any per-field options for this field
		groupName, opts := parseFieldTag(fieldType)

		// Try to find the value for this field
		value, found := findGroupValue(groupName, fieldType.Name, groupValues)
		value, ok := resolveGroupValue(value, found, opts)
		if !ok {
			continue
		}

		// Set the field value with type conversion
		if err := setFieldValue(field, value, opts); err != nil {
			return fmt.Errorf("regextra: failed to set field %s: %w", fieldType.Name, err)
		}
	}

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
// Forward-compat rules (locked in as v1 contract — see the package doc's
// "Tag grammar" section for the full statement and rationale):
//   - Unknown key=value pairs are preserved in the returned map so future
//     option additions don't need to touch the parser; adding a new option
//     key is therefore not a breaking change.
//   - Lone tokens without `=` are silently ignored today; the slot is
//     reserved for future flag-style options (e.g. `required`), so callers
//     must not rely on lone tokens remaining inert.
//
// `regex:""` and `regex:"-"` both signal "no name", returning "" and a nil
// options map.
func parseFieldTag(field reflect.StructField) (name string, opts map[string]string) {
	tag := field.Tag.Get("regex")
	if tag == "" || tag == "-" {
		return "", nil
	}
	parts := strings.Split(tag, ",")
	name = strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return name, nil
	}
	opts = make(map[string]string, len(parts)-1)
	for _, p := range parts[1:] {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			// Lone token without '='. Reserved for future flag-style options;
			// silently ignored today rather than rejected to keep the parser
			// forward-compatible.
			continue
		}
		opts[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return name, opts
}

// resolveGroupValue decides what a field receives given its group's raw match
// state: the matched value when one is usable, the `default=` option when not,
// or nothing (ok=false, skip the field, leaving it unchanged).
//
// This is the single source of truth for the skip-or-default contract shared
// by Unmarshal (populateStruct) and Decoder.decode — the two paths drifted on
// exactly this logic once (issue #104), so it lives in one place now.
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

// findGroupValue searches for the value in the group values map
// Priority order: explicit tag > exact field name > case-insensitive field name
func findGroupValue(tagName, fieldName string, groupValues map[string]string) (string, bool) {
	// If there's an explicit tag, use it (highest priority)
	if tagName != "" {
		value, found := groupValues[tagName]
		return value, found
	}

	// Try exact field name match
	if value, found := groupValues[fieldName]; found {
		return value, true
	}

	// Try case-insensitive match
	lowerFieldName := strings.ToLower(fieldName)
	for groupName, value := range groupValues {
		if strings.ToLower(groupName) == lowerFieldName {
			return value, true
		}
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
	if field.CanAddr() {
		if u, ok := field.Addr().Interface().(RegexUnmarshaler); ok {
			return u.UnmarshalRegex(value)
		}
	}
	if field.Type().Implements(regexUnmarshalerType) {
		// Value-receiver method. Rare but possible.
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
