package regextra

import (
	"fmt"
	"reflect"
	"regexp"
)

// Unmarshal extracts named capture groups from the target string and assigns them
// to the corresponding fields in the provided struct pointer.
//
// Field mapping rules:
//   - First checks the `regex:"groupname"` struct tag if provided (highest priority)
//   - Falls back to exact field name match with capture group name
//   - Falls back to case-insensitive field name match (Unicode simple case folding)
//   - Supports type conversion for string, bool, all int/uint widths,
//     float32/float64, time.Time, time.Duration, and pointers (any depth)
//     to any of these; types implementing [RegexUnmarshaler] or
//     encoding.TextUnmarshaler convert themselves (see [RegexUnmarshaler]
//     for the full conversion precedence)
//   - A nil pointer field is allocated; a non-nil pointer is reused, its
//     pointee overwritten
//   - A time.Time field tries these layouts in order: RFC3339Nano, RFC3339,
//     DateTime ("2006-01-02 15:04:05"), DateOnly ("2006-01-02"), TimeOnly
//     ("15:04:05") — unless a `layout=` tag option pins one exclusively;
//     time.Duration is parsed with [time.ParseDuration]
//   - Unexported fields are ignored
//   - A group that did not participate in the match (e.g. an optional group),
//     or that matched an empty span, leaves the field unchanged — unless the
//     field has a `default=` tag option, which substitutes instead
//   - An embedded struct (or *struct) field tagged `regex:",inline"` is
//     promoted: its exported fields join the mapping as if declared on the
//     outer struct, under the same rules (tags, name fallback, `default=`,
//     `required`), recursively through nested `inline` tags. A nil embedded
//     pointer is allocated when a promoted field beneath it decodes.
//     Precedence follows encoding/json: a shallower field bound to a group
//     shadows a deeper promoted field bound to the same group; when promoted
//     fields binding the same group tie at the shallowest depth, a sole
//     binding whose group name came from an explicit `regex:"name"` tag wins
//     over the field-name-matched ones (json's tagged-beats-untagged
//     tiebreak), and a tie with no tagged binding — or with several — is
//     ambiguous: every binding of the group is dropped here (best-effort
//     posture), while the strict [Compile] rejects the struct. An embedded
//     field without the flag is not promoted; `regex:"-"` excludes it entirely
//
// On no match, Unmarshal returns nil and leaves *v unchanged — no match is data
// absence, not a failure. Callers who need to distinguish "matched" from
// "didn't match" should inspect their struct after the call, or use
// [Decoder.One] which signals no-match via [ErrNoMatch]. See the package doc's
// "No-match behavior" section for the full cross-API contract.
//
// Returns an error if:
//   - v is not a pointer to a struct
//   - Type conversion fails on a matched group (a [DecodeError])
//   - A `required` field's group produced no value (a [RequiredGroupError]): it
//     did not participate in the match or matched an empty span, and no
//     `default=` supplied a substitute
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

	// Fetch the decode plan for this (pattern, struct type) pair — cached in
	// planCache after first use — and run it. It is the same plan build/run
	// the Decoder uses, in lenient mode (strict=false) so Unmarshal stays
	// best-effort rather than erroring on undeclared groups or misplaced tag
	// options. buildDecodePlan never returns an error when strict=false; the
	// check is kept for forward-safety.
	fields, err := getDecodePlan(elem.Type(), re)
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
// Returns an error if v is not a pointer to a slice of structs, or if any match
// fails to decode — a per-field conversion failure (a [DecodeError]) or a
// `required` group that produced no value (a [RequiredGroupError]). The first
// failing match stops the call and returns that error, prefixed with its match
// index.
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

	// Fetch the decode plan — cached in planCache per (pattern, struct type)
	// pair, so it is built at most once per process — then run it for every
	// match into a pre-sized slice. The plan (group indexes + parsed options)
	// replaced the old per-match map build + per-field tag re-parse, and each
	// match decodes in place — no reflect.New / reflect.Append copy and no map
	// churn per match. strict=false keeps the lenient Unmarshal posture (see
	// Unmarshal); the error is never non-nil here.
	fields, err := getDecodePlan(sliceElemType, re)
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
// Conversion precedence: for each field, the decode path tries in order
//
//  1. RegexUnmarshaler — the package-specific hook always wins;
//  2. the time.Time / time.Duration special-cases, so the multi-layout
//     fallback and the `layout=` tag option are preserved (time.Time
//     implements [encoding.TextUnmarshaler], but its UnmarshalText accepts
//     only RFC3339, so it is deliberately not routed through step 3);
//  3. [encoding.TextUnmarshaler] — for any other type that implements it
//     (e.g. netip.Addr, math/big.Int, log/slog.Level);
//  4. the built-in string/int/uint/float/bool conversion.
//
// A type implementing both RegexUnmarshaler and [encoding.TextUnmarshaler]
// therefore dispatches on UnmarshalRegex.
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
