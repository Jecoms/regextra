package regextra

import (
	"errors"
	"fmt"
	"iter"
	"reflect"
	"regexp"
	"strings"
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
	// groupIndexes holds the submatch index of every occurrence of the
	// field's group name, in declaration order. Go's regexp allows the same
	// group name to appear more than once in a pattern (e.g. across
	// alternation branches), and only one occurrence participates in a given
	// match — decode scans them all rather than trusting SubexpIndex's first
	// occurrence. Empty means "no group declared, use default if present,
	// otherwise skip."
	groupIndexes []int
	// opts is the parsed tag options map (e.g. {"default": "guest", "layout": "..."}).
	// Nil if the field has no options.
	opts map[string]string
	// required is set when the field's tag carries the `required` flag. A
	// required field that yields no value (group absent, non-participating, or
	// an empty span with no default) fails decode with a *RequiredGroupError
	// instead of being skipped.
	required bool
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

	// strict=true: the Decoder validates eagerly so a successful Compile
	// guarantees no tag-related decode errors later.
	fields, err := buildDecodePlan(rt, re, true)
	if err != nil {
		return nil, err
	}

	return &Decoder[T]{
		pattern: pattern,
		re:      re,
		fields:  fields,
	}, nil
}

// buildDecodePlan maps each exported field of rt to its regex capture group and
// parsed tag options, returning the per-field decode plan that runDecodePlan
// executes against a match. It is the single plan-construction path shared by
// the [Decoder] (compiled once via compileDecoder) and the [Unmarshal] /
// [UnmarshalAll] free functions (built fresh per call) — one set of
// field-mapping semantics, so the two paths can't drift again.
//
// strict selects the validation posture. The Decoder passes strict=true and any
// of three checks fails the build, so a successful Compile is fully validated:
//   - a field references a group not declared on the pattern and has no default
//   - a `default=` value does not convert to the field's type
//   - a `layout=` option sits on a non-time.Time field
//
// The Unmarshal path passes strict=false and tolerates all three rather than
// rejecting them — a missing group with no default skips the field, an
// unconvertible default surfaces only if that field is actually reached at
// decode time, and a stray `layout=` is ignored on non-time fields by
// setFieldValue. This preserves Unmarshal's historical best-effort behavior, so
// buildDecodePlan never returns a non-nil error when strict=false.
func buildDecodePlan(rt reflect.Type, re *regexp.Regexp, strict bool) ([]fieldDecoder, error) {
	var fields []fieldDecoder
	for i := range rt.NumField() {
		sf := rt.Field(i)
		if !sf.IsExported() {
			continue
		}

		groupName, opts, required, skip := parseFieldTag(sf)
		if skip {
			// `regex:"-"` excludes the field entirely — it never enters the
			// decode plan and no name fallback is attempted.
			continue
		}
		if groupName == "" {
			// No explicit tag — fall back to matching the field name against a
			// declared group: exact first, then case-insensitively via Unicode
			// simple-fold (see matchGroupName). A field that matches no group
			// and has no default is treated as a typo and, under strict, fails
			// the build below.
			groupName = matchGroupName(re, sf.Name)
		}

		_, hasDefault := opts["default"]
		var groupIdxs []int
		if groupName != "" {
			groupIdxs = subexpIndexes(re, groupName)
			if len(groupIdxs) == 0 && !hasDefault && strict {
				// Missing group with no default IS a typo — fail at compile.
				// With a default, missing group is intentional (the default
				// always fires). The lenient path skips the field below.
				return nil, fmt.Errorf("regextra.Compile: field %s references group %q which is not declared on the pattern", sf.Name, groupName)
			}
		}

		if strict {
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
		}

		// Skip fields that have neither a group mapping nor a default —
		// they'd be no-ops at decode time. A `required` field is retained even
		// with no mapping/default so runDecodePlan can raise a
		// *RequiredGroupError when it yields no value.
		if len(groupIdxs) == 0 && !hasDefault && !required {
			continue
		}

		fields = append(fields, fieldDecoder{
			fieldIndex:   i,
			groupIndexes: groupIdxs,
			opts:         opts,
			required:     required,
		})
	}

	return fields, nil
}

// subexpIndexes returns the submatch index of every occurrence of the named
// group on re, in declaration order. Unlike re.SubexpIndex, which reports
// only the first occurrence, this captures duplicates so decode can find the
// occurrence that actually participated in a match.
func subexpIndexes(re *regexp.Regexp, name string) []int {
	var idxs []int
	for i, n := range re.SubexpNames() {
		if i != 0 && n == name {
			idxs = append(idxs, i)
		}
	}
	return idxs
}

// matchGroupName returns the declared group name on re that matches fieldName
// (case-insensitive), or "" if no group matches. Used as a fallback when a
// field has no explicit `regex:"..."` tag.
func matchGroupName(re *regexp.Regexp, fieldName string) string {
	if idx := re.SubexpIndex(fieldName); idx != -1 {
		return fieldName
	}
	for _, n := range re.SubexpNames() {
		if n != "" && strings.EqualFold(n, fieldName) {
			return n
		}
	}
	return ""
}

// resolveGroupName reports the capture-group name a field decoded from, for
// DecodeError.Group. It is computed lazily — only when building a DecodeError —
// so the name need not be retained per field in the plan (which would cost bytes
// on every uncached Unmarshal/UnmarshalAll plan build).
//
// When the field mapped to a declared group (groupIndexes non-empty), the name
// is recovered from the regexp's cached SubexpNames without allocating; any
// occurrence of a reused name resolves to the same string. The empty case — a
// default-only field with no declared group — is reached only when a `default=`
// value itself fails to convert on the lenient Unmarshal path; there the name is
// the explicit tag (or "" if untagged), worth re-parsing the tag for in that
// rare case. This matches the name buildDecodePlan resolved at build time.
func resolveGroupName(re *regexp.Regexp, sf reflect.StructField, groupIndexes []int) string {
	if len(groupIndexes) > 0 {
		return re.SubexpNames()[groupIndexes[0]]
	}
	name, _, _, _ := parseFieldTag(sf)
	return name
}

// One returns the result of decoding the first match of d's pattern in target.
// Returns [ErrNoMatch] if there's no match. Other errors indicate a per-field
// conversion failure (a [DecodeError]); in that case the returned T contains
// whatever fields were successfully decoded before the failure. A matched field
// whose type is a nested struct, slice, or map is one such failure: these are
// not flattened, so binding a group to one yields an "unsupported field type"
// error (unless the type implements [RegexUnmarshaler] or
// encoding.TextUnmarshaler, which convert themselves). The same applies to
// [Decoder.All] and [Decoder.Iter], which share One's decode path.
//
// One uses the [ErrNoMatch] sentinel because the (T, error) return shape would
// otherwise make "no match" indistinguishable from "decoded a struct of all
// zero fields". Compare with errors.Is. See the package doc's "No-match
// behavior" section for the full cross-API contract.
func (d *Decoder[T]) One(target string) (T, error) {
	matches := d.re.FindStringSubmatchIndex(target)
	if matches == nil {
		return d.zero, ErrNoMatch
	}
	var v T
	rv := reflect.ValueOf(&v).Elem()
	if err := d.decode(rv, target, matches); err != nil {
		return v, fmt.Errorf("regextra.Decoder.One: %w", err)
	}
	return v, nil
}

// All returns every match of d's pattern in target decoded into a slice.
// Returns an empty slice and nil error when there are no matches. A non-nil
// error indicates a per-field conversion failure on one of the matches; the
// slice up to that point may contain partially-decoded entries.
func (d *Decoder[T]) All(target string) ([]T, error) {
	allMatches := d.re.FindAllStringSubmatchIndex(target, -1)
	if len(allMatches) == 0 {
		return []T{}, nil
	}
	out := make([]T, len(allMatches))
	for i, matches := range allMatches {
		rv := reflect.ValueOf(&out[i]).Elem()
		if err := d.decode(rv, target, matches); err != nil {
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
		allMatches := d.re.FindAllStringSubmatchIndex(target, -1)
		for _, matches := range allMatches {
			var v T
			rv := reflect.ValueOf(&v).Elem()
			err := d.decode(rv, target, matches)
			if err != nil {
				err = fmt.Errorf("regextra.Decoder.Iter: %w", err)
			}
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

// Regexp returns the compiled [*regexp.Regexp] this Decoder built from its
// pattern, so callers can reuse it for their own match-finding (for example
// [regexp.Regexp.FindAllIndex] or custom iteration) without recompiling.
//
// The returned pointer is shared with the Decoder. Its exported methods are
// read-only and safe for concurrent use, but callers must not mutate shared
// state on it — in particular do not call [regexp.Regexp.Longest], which
// changes the receiver's matching semantics and would affect the Decoder too.
func (d *Decoder[T]) Regexp() *regexp.Regexp {
	return d.re
}

// decode walks the precomputed field plan against a single match and writes the
// values into rv (the addressable reflect.Value of a T). It is a thin wrapper
// over the shared runDecodePlan core, which the [Unmarshal] / [UnmarshalAll]
// free functions drive too.
func (d *Decoder[T]) decode(rv reflect.Value, target string, matches []int) error {
	return runDecodePlan(d.re, d.fields, rv, target, matches)
}

// runDecodePlan executes a decode plan (from buildDecodePlan) against a single
// match's group indices, writing the decoded values into rv — the addressable
// reflect.Value of a struct. matches is a FindStringSubmatchIndex-style index
// slice (or one element of FindAllStringSubmatchIndex); target is the string
// those indices slice into. It is the single decode core shared by [Decoder]
// (One/All/Iter) and the [Unmarshal] / [UnmarshalAll] free functions. re is
// used only to resolve a field's group name lazily when building a DecodeError.
func runDecodePlan(re *regexp.Regexp, fields []fieldDecoder, rv reflect.Value, target string, matches []int) error {
	for _, fd := range fields {
		// Pick the value the same way the map-based readers do (see
		// namedGroupValues): the last occurrence that participated in the
		// match wins, even if it matched an empty span. A non-participating
		// occurrence (negative start index) never overwrites a participating
		// one. Index pairs are what make this possible — FindStringSubmatch's
		// strings can't tell a participating-empty group from a
		// non-participating one. The index slice always holds 2*(NumSubexp+1)
		// entries, so 2*gi+1 is always in range.
		var value string
		var found bool
		for _, gi := range fd.groupIndexes {
			start := matches[2*gi]
			if start < 0 {
				continue
			}
			value = target[start:matches[2*gi+1]]
			found = true
		}
		// The skip-or-default contract is shared with the map-based readers via
		// resolveGroupValue (see its doc): default= substitutes when no
		// occurrence participated OR the winning value is empty, otherwise an
		// empty/absent group skips the field rather than feeding "" to the
		// type converter.
		value, ok := resolveGroupValue(value, found, fd.opts)
		if !ok {
			// No usable value. A `required` field fails here (the group did not
			// participate, matched an empty span, or is undeclared, and no
			// default= supplied a substitute); every other field is skipped and
			// left unchanged. Keying on resolveGroupValue's `ok` — not `found` —
			// means a participating-but-empty span also fails required,
			// consistent with the shared "empty span = data absence" contract.
			if fd.required {
				sf := rv.Type().Field(fd.fieldIndex)
				return &RequiredGroupError{
					Field: sf.Name,
					Group: resolveGroupName(re, sf, fd.groupIndexes),
				}
			}
			continue
		}
		field := rv.Field(fd.fieldIndex)
		if err := setFieldValue(field, value, fd.opts); err != nil {
			sf := rv.Type().Field(fd.fieldIndex)
			return &DecodeError{
				Field: sf.Name,
				Group: resolveGroupName(re, sf, fd.groupIndexes),
				Value: value,
				Type:  field.Type().String(),
				Err:   err,
			}
		}
	}
	return nil
}
