// Package regextra provides extensions to Go's regexp package for easier handling of named capture groups.
//
// The standard library's regexp package requires verbose code to extract named groups:
//
//	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
//	matches := re.FindStringSubmatch("Alice 30")
//	nameIndex := re.SubexpIndex("name")
//	name := matches[nameIndex]  // "Alice"
//
// This package simplifies named group extraction:
//
//	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
//	name, ok := regextra.FindNamed(re, "Alice 30", "name")  // "Alice", true
//	groups := regextra.NamedGroups(re, "Alice 30")          // map[name:Alice age:30]
package regextra

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
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

// FindNamed returns the value of the named capture group in the target string.
// It returns the matched value and true if found, or empty string and false if not found.
//
// Example:
//
//	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
//	name, ok := regextra.FindNamed(re, "Alice 30", "name")
//	// name = "Alice", ok = true
func FindNamed(re *regexp.Regexp, target, groupName string) (string, bool) {
	index := re.SubexpIndex(groupName)
	if index == -1 {
		return "", false
	}

	matches := re.FindStringSubmatch(target)
	if matches == nil {
		return "", false
	}

	return matches[index], true
}

// FindAllNamed returns every value of the named capture group across all
// matches of re in target. Returns nil if the group name is not declared on
// the regex; an empty slice if the group is declared but the regex has no
// matches.
//
// Example:
//
//	re := regexp.MustCompile(`(?P<word>\S+)`)
//	words := regextra.FindAllNamed(re, "alpha beta gamma", "word")
//	// words = []string{"alpha", "beta", "gamma"}
//
// For a single match, prefer [FindNamed] which returns (value, ok).
// To collect every named group's values across all matches, use
// [AllNamedGroups].
func FindAllNamed(re *regexp.Regexp, target, groupName string) []string {
	index := re.SubexpIndex(groupName)
	if index == -1 {
		return nil
	}
	matches := re.FindAllStringSubmatch(target, -1)
	if len(matches) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[index])
	}
	return out
}

// NamedGroups returns a map of all named capture groups and their matched values
// from the target string. If no match is found, it returns an empty map.
//
// Example:
//
//	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
//	groups := regextra.NamedGroups(re, "Alice 30")
//	// groups = map[string]string{"name": "Alice", "age": "30"}
func NamedGroups(re *regexp.Regexp, target string) map[string]string {
	result := make(map[string]string)

	matches := re.FindStringSubmatch(target)
	if matches == nil {
		return result
	}

	for i, name := range re.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = matches[i]
		}
	}

	return result
}

// AllNamedGroups returns a map where each key is a named capture group and the value
// is a slice of all matches for that group. This handles patterns where the same
// group name appears multiple times.
//
// Example:
//
//	re := regexp.MustCompile(`(?P<word>\w+) (?P<word>\w+)`)
//	allGroups := regextra.AllNamedGroups(re, "hello world")
//	// allGroups = map[string][]string{"word": []string{"hello", "world"}}
func AllNamedGroups(re *regexp.Regexp, target string) map[string][]string {
	result := make(map[string][]string)

	matches := re.FindStringSubmatch(target)
	if matches == nil {
		return result
	}

	for i, name := range re.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = append(result[name], matches[i])
		}
	}

	return result
}

// Replace substitutes the matched span of each named capture group in
// target with the value from replacements, leaving non-matching text and
// any groups absent from the map unchanged. Replace operates on every
// match of re, in order.
//
// If a regex declares a named group but replacements has no entry for it,
// the original matched text passes through. Groups that don't participate
// in a match (optional groups returning index -1) are skipped.
//
// When named groups overlap (nesting), the outermost-named group whose
// span is encountered first wins; inner groups inside an already-replaced
// span are not substituted.
//
// Example:
//
//	re := regexp.MustCompile(`(?P<user>\w+)@(?P<domain>[\w.]+)`)
//	out := regextra.Replace(re, "alice@example.com", map[string]string{
//	    "domain": "redacted",
//	})
//	// out = "alice@redacted"
func Replace(re *regexp.Regexp, target string, replacements map[string]string) string {
	if len(replacements) == 0 {
		return target
	}
	matches := re.FindAllStringSubmatchIndex(target, -1)
	if len(matches) == 0 {
		return target
	}
	names := re.SubexpNames()

	type span struct {
		start, end int
		repl       string
	}

	var b strings.Builder
	cursor := 0
	for _, m := range matches {
		spans := make([]span, 0, len(names))
		for i := 1; i < len(names); i++ {
			name := names[i]
			if name == "" {
				continue
			}
			repl, ok := replacements[name]
			if !ok {
				continue
			}
			s, e := m[2*i], m[2*i+1]
			if s < 0 || e < 0 {
				continue
			}
			spans = append(spans, span{start: s, end: e, repl: repl})
		}
		sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
		for _, sp := range spans {
			if sp.start < cursor {
				continue // already covered (overlap with an earlier substitution)
			}
			b.WriteString(target[cursor:sp.start])
			b.WriteString(sp.repl)
			cursor = sp.end
		}
	}
	b.WriteString(target[cursor:])
	return b.String()
}

// Validate returns an error listing every required group name that is not
// declared on re. Useful for init-time assertions in services that compile
// patterns once: catch typos at startup rather than at the first
// (mis-)matched request.
//
// Returns nil when every required name is declared. The error message lists
// the missing names in the order they were passed.
//
// Example:
//
//	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
//	if err := regextra.Validate(re, "name", "age", "missing"); err != nil {
//	    // err: regextra.Validate: missing named groups: missing
//	}
func Validate(re *regexp.Regexp, required ...string) error {
	declared := make(map[string]struct{}, len(re.SubexpNames()))
	for _, n := range re.SubexpNames() {
		if n != "" {
			declared[n] = struct{}{}
		}
	}
	var missing []string
	for _, name := range required {
		if _, ok := declared[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("regextra.Validate: missing named groups: %s", strings.Join(missing, ", "))
}

// Unmarshal extracts named capture groups from the target string and assigns them
// to the corresponding fields in the provided struct pointer.
//
// Field mapping rules:
//   - First checks the `regex:"groupname"` struct tag if provided (highest priority)
//   - Falls back to exact field name match with capture group name
//   - Falls back to case-insensitive field name match
//   - Supports type conversion for int, int64, float64, and bool
//   - Unexported fields are ignored
//
// Returns an error if:
//   - v is not a pointer to a struct
//   - The pattern does not match the target string
//   - Type conversion fails
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

	// Find the match
	matches := re.FindStringSubmatch(target)
	if matches == nil {
		return nil // No match, but not an error
	}

	// Build a map of capture group names to their values
	groupValues := make(map[string]string)
	for i, name := range re.SubexpNames() {
		if i != 0 && name != "" {
			groupValues[name] = matches[i]
		}
	}

	// Populate the struct fields
	return populateStruct(elem, groupValues)
}

// UnmarshalAll extracts all occurrences of the regex pattern from the target string
// and unmarshals them into a slice of structs. The slice is cleared before populating.
//
// v must be a pointer to a slice of structs. If no matches are found, the slice will
// be empty (length 0).
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

	// Find all matches
	allMatches := re.FindAllStringSubmatch(target, -1)
	if len(allMatches) == 0 {
		// Clear the slice and return (no matches is not an error)
		elem.SetLen(0)
		return nil
	}

	// Create a new slice with capacity for all matches
	newSlice := reflect.MakeSlice(elem.Type(), 0, len(allMatches))

	// Process each match
	for _, matches := range allMatches {
		// Build a map of capture group names to their values for this match
		groupValues := make(map[string]string)
		for i, name := range re.SubexpNames() {
			if i != 0 && name != "" {
				groupValues[name] = matches[i]
			}
		}

		// Create a new struct instance
		structValue := reflect.New(sliceElemType).Elem()

		// Populate the struct fields
		if err := populateStruct(structValue, groupValues); err != nil {
			return err
		}

		// Append to the slice
		newSlice = reflect.Append(newSlice, structValue)
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

		// `default=` substitutes when the field has no match OR the match is
		// empty. Empty-match overlap is intentional: regexp returns "" both
		// for non-participating optional groups and for groups that matched a
		// zero-length span; treating both as "no useful value" matches caller
		// expectations.
		if !found || value == "" {
			if def, ok := opts["default"]; ok {
				value = def
				found = true
			}
		}
		if !found {
			continue
		}

		// Set the field value with type conversion
		if err := setFieldValue(field, value, opts); err != nil {
			return fmt.Errorf("regextra: failed to set field %s: %w", fieldType.Name, err)
		}
	}

	return nil
}

// getGroupName extracts the group name from the struct tag, or returns empty string.
// Thin wrapper around parseFieldTag for callers that don't need the options map.
func getGroupName(field reflect.StructField) string {
	name, _ := parseFieldTag(field)
	return name
}

// parseFieldTag parses a `regex:"name,key=value,key=value"` struct tag into
// the group name and an options map. The grammar is intentionally
// json/encoding-style: the first comma-separated piece is the name; each
// subsequent piece is a `key=value` pair (lone tokens without `=` are ignored,
// not promoted to a flag, since regextra's option set is small enough that
// flags-with-no-value haven't been needed).
//
// Currently recognized option keys (case-sensitive):
//   - default — value substituted when the named group is not declared on the
//     regex or its match is empty.
//   - layout  — for time.Time fields only: a single time.Parse layout used
//     instead of the default fallback list.
//
// Unknown keys are preserved in the returned map so future option additions
// don't need to touch the parser. `regex:""` and `regex:"-"` both signal
// "no name", returning "" and a nil options map.
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
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("cannot convert %q to int: %w", value, err)
		}
		field.SetInt(intVal)
		return nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uintVal, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return fmt.Errorf("cannot convert %q to uint: %w", value, err)
		}
		field.SetUint(uintVal)
		return nil

	case reflect.Float32, reflect.Float64:
		floatVal, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("cannot convert %q to float: %w", value, err)
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
