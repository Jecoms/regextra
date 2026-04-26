package regextra

import (
	"errors"
	"fmt"
	"iter"
	"reflect"
	"regexp"
)

// ErrNoMatch is returned by [Decoder.One] when the target string does not
// match the regex. Compare with errors.Is so callers can distinguish "no
// match" from genuine decoding failures.
var ErrNoMatch = errors.New("regextra: no match")

// Decoder is a typed, regex-bound unmarshaler that caches the reflect plan
// for T's fields. Compile once, decode many times — eliminates the per-call
// reflect work that [Unmarshal] does on every invocation.
//
// The decode plan is computed during [Compile]: each exported field of T is
// mapped to its regex capture group, its tag options are parsed, and any
// default value is type-checked. Runtime [Decoder.One] / [Decoder.All] calls
// walk the precomputed plan against the regex match indices, never running
// reflect on the destination type.
//
// Decoders are safe for concurrent use — no shared mutable state after
// Compile. Reuse one Decoder per goroutine pool / per request handler.
//
// Use [Compile] (returns error) or [MustCompile] (panics) to construct.
type Decoder[T any] struct {
	pattern string
	re      *regexp.Regexp

	// fields is the precomputed decode plan, one entry per exported struct
	// field that maps to a regex group. Unmapped or unexported fields are
	// not represented.
	fields []fieldDecoder

	// zero is a cached reflect.Value of T's zero value, used by One when no
	// match is found.
	zero T
}

// fieldDecoder is the precomputed decode plan for one struct field.
type fieldDecoder struct {
	// fieldIndex is the index into T's struct fields (StructField.Index[0]).
	fieldIndex int
	// groupIndex is the regex SubexpIndex for the named group; -1 means
	// "no group declared, use default if present, otherwise skip."
	groupIndex int
	// opts is the parsed tag options map (e.g. {"default": "guest", "layout": "..."}).
	// Nil if the field has no options.
	opts map[string]string
}

// Compile parses pattern and validates T's struct tags against it.
//
// Returns an error if:
//   - pattern is not a valid regular expression
//   - T is not a struct type
//   - A field's `regex:"name"` tag references a group not declared on pattern
//   - A field's `regex:",default=<value>"` cannot be converted to the field's type
//   - A field uses `regex:",layout=..."` on a non-time.Time field
//
// Once Compile returns nil, the resulting Decoder is fully validated and
// guaranteed not to produce tag-related errors at decode time.
func Compile[T any](pattern string) (*Decoder[T], error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("regextra.Compile: invalid pattern: %w", err)
	}
	return compileDecoder[T](pattern, re)
}

// MustCompile is like [Compile] but panics on error. Intended for
// package-level vars where startup-time failure is the right behavior:
//
//	var personDecoder = regextra.MustCompile[Person](`(?P<name>\w+) is (?P<age>\d+)`)
func MustCompile[T any](pattern string) *Decoder[T] {
	d, err := Compile[T](pattern)
	if err != nil {
		panic(err)
	}
	return d
}

func compileDecoder[T any](pattern string, re *regexp.Regexp) (*Decoder[T], error) {
	var zero T
	rt := reflect.TypeOf(zero)
	if rt == nil || rt.Kind() != reflect.Struct {
		return nil, fmt.Errorf("regextra.Compile: T must be a struct type, got %v", rt)
	}

	d := &Decoder[T]{
		pattern: pattern,
		re:      re,
	}

	for i := range rt.NumField() {
		sf := rt.Field(i)
		if !sf.IsExported() {
			continue
		}

		groupName, opts := parseFieldTag(sf)
		if groupName == "" {
			// No explicit tag — try field name match (exact or
			// case-insensitive). For the decoder we only consider exact
			// matches against declared groups; loose matching at compile
			// time would mask typos.
			groupName = matchGroupName(re, sf.Name)
		}

		_, hasDefault := opts["default"]
		groupIdx := -1
		if groupName != "" {
			groupIdx = re.SubexpIndex(groupName)
			if groupIdx == -1 && !hasDefault {
				// Missing group with no default IS a typo — fail at compile.
				// With a default, missing group is intentional (the default
				// always fires).
				return nil, fmt.Errorf("regextra.Compile: field %s references group %q which is not declared on the pattern", sf.Name, groupName)
			}
		}

		// Validate `default=` eagerly: try to assign it to a fresh field
		// and surface any conversion error at compile time, not at first
		// request.
		if def, ok := opts["default"]; ok {
			probe := reflect.New(sf.Type).Elem()
			if err := setFieldValue(probe, def, opts); err != nil {
				return nil, fmt.Errorf("regextra.Compile: field %s default %q does not convert to %v: %w", sf.Name, def, sf.Type, err)
			}
		}

		// Validate `layout=` is only on time.Time fields.
		if _, ok := opts["layout"]; ok {
			ft := sf.Type
			if ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			if ft != timeTimeType {
				return nil, fmt.Errorf("regextra.Compile: field %s has `layout=` option but is %v, not time.Time", sf.Name, sf.Type)
			}
		}

		// Skip fields that have neither a group mapping nor a default —
		// they'd be no-ops at decode time.
		if groupIdx == -1 && !hasDefault {
			continue
		}

		d.fields = append(d.fields, fieldDecoder{
			fieldIndex: i,
			groupIndex: groupIdx,
			opts:       opts,
		})
	}

	return d, nil
}

// matchGroupName returns the declared group name on re that matches fieldName
// (case-insensitive), or "" if no group matches. Used as a fallback when a
// field has no explicit `regex:"..."` tag.
func matchGroupName(re *regexp.Regexp, fieldName string) string {
	if idx := re.SubexpIndex(fieldName); idx != -1 {
		return fieldName
	}
	lowerField := lower(fieldName)
	for _, n := range re.SubexpNames() {
		if n != "" && lower(n) == lowerField {
			return n
		}
	}
	return ""
}

// lower is strings.ToLower without the import dependency for this file.
// Cheap inline since we only call it during Compile, not in the hot path.
func lower(s string) string {
	out := make([]byte, len(s))
	for i := range len(s) {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

// One returns the result of decoding the first match of d's pattern in target.
// Returns [ErrNoMatch] if there's no match. Other errors indicate a per-field
// conversion failure; in that case the returned T contains whatever fields
// were successfully decoded before the failure.
func (d *Decoder[T]) One(target string) (T, error) {
	matches := d.re.FindStringSubmatch(target)
	if matches == nil {
		return d.zero, ErrNoMatch
	}
	var v T
	rv := reflect.ValueOf(&v).Elem()
	if err := d.decode(rv, matches); err != nil {
		return v, err
	}
	return v, nil
}

// All returns every match of d's pattern in target decoded into a slice.
// Returns an empty slice and nil error when there are no matches. A non-nil
// error indicates a per-field conversion failure on one of the matches; the
// slice up to that point may contain partially-decoded entries.
func (d *Decoder[T]) All(target string) ([]T, error) {
	allMatches := d.re.FindAllStringSubmatch(target, -1)
	if len(allMatches) == 0 {
		return []T{}, nil
	}
	out := make([]T, len(allMatches))
	for i, matches := range allMatches {
		rv := reflect.ValueOf(&out[i]).Elem()
		if err := d.decode(rv, matches); err != nil {
			return out[:i+1], fmt.Errorf("regextra.Decoder.All: match %d: %w", i, err)
		}
	}
	return out, nil
}

// Iter returns a range-over-func iterator that decodes each match of d's
// pattern in target into a T, yielded with the per-match decode error.
// nil error means decoded successfully; non-nil means a per-field
// conversion failed on that match. Iteration continues past errors so
// callers can collect or skip individual failures:
//
//	for v, err := range dec.Iter(input) {
//	    if err != nil {
//	        log.Printf("skipping bad match: %v", err)
//	        continue
//	    }
//	    process(v)
//	}
//
// Break out of the range body to stop iteration early (e.g. after the first
// match). The match-finding step is not lazy — Go's regexp package
// pre-computes all match positions in one call — but the decode step IS
// lazy, so breaking early avoids the per-match reflect work for the
// remaining matches.
//
// For a slice of all results with a single error, prefer [Decoder.All].
// For a single match with a sentinel ErrNoMatch, prefer [Decoder.One].
func (d *Decoder[T]) Iter(target string) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		allMatches := d.re.FindAllStringSubmatch(target, -1)
		for _, matches := range allMatches {
			var v T
			rv := reflect.ValueOf(&v).Elem()
			err := d.decode(rv, matches)
			if !yield(v, err) {
				return
			}
		}
	}
}

// Pattern returns the regex source pattern this Decoder was compiled from.
// Useful for logging and debugging.
func (d *Decoder[T]) Pattern() string {
	return d.pattern
}

// decode walks the precomputed field plan against a single match's group
// values and writes them into rv (the addressable reflect.Value of a T).
func (d *Decoder[T]) decode(rv reflect.Value, matches []string) error {
	for _, fd := range d.fields {
		var value string
		var found bool
		if fd.groupIndex >= 0 && fd.groupIndex < len(matches) {
			value = matches[fd.groupIndex]
			// regexp returns "" for non-participating groups; treat as not-found
			// so the default-fallback logic below kicks in.
			found = value != "" || fd.groupIndex == 0
		}
		if !found || value == "" {
			if def, ok := fd.opts["default"]; ok {
				value = def
				found = true
			}
		}
		if !found {
			continue
		}
		field := rv.Field(fd.fieldIndex)
		if err := setFieldValue(field, value, fd.opts); err != nil {
			fieldName := rv.Type().Field(fd.fieldIndex).Name
			return fmt.Errorf("field %s: %w", fieldName, err)
		}
	}
	return nil
}
