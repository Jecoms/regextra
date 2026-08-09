package regextra

import (
	"fmt"
	"iter"
	"reflect"
	"regexp"
)

// Decoder is a typed, regex-bound unmarshaler that carries the reflect plan
// for T's fields. Compile once, decode many times — [Unmarshal] caches its
// decode plan per (pattern, struct type) pair too, so the Decoder's remaining
// edge is skipping the per-call cache lookup, plus the strict compile-time
// validation below.
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

// Compile parses pattern and validates T's struct tags against it.
//
// Returns an error if:
//   - pattern is not a valid regular expression
//   - T is not a struct type
//   - A field's `regex:"name"` tag references a group not declared on pattern,
//     and the field has no `default=` (a `default=` makes a missing group
//     intentional, so the field compiles and the default always fires)
//   - A field's `regex:",default=<value>"` cannot be converted to the field's type
//   - A field uses `regex:",layout=..."` on a non-time.Time field
//   - The `inline` flag sits on a non-embedded (or non-struct) field, or is
//     combined with a group name (e.g. `regex:"meta,inline"`)
//   - Two `inline`-promoted fields at equal embedding depth bind the same
//     capture group and the tie is not broken by a sole explicitly tagged
//     binding (see [Unmarshal]'s embedded-struct promotion rules)
//
// Once Compile returns nil, the resulting Decoder is fully validated and
// guaranteed not to produce tag-related errors at decode time.
//
// Failures are categorized by wrapped sentinel: the first cause above wraps
// [ErrInvalidPattern] and the rest wrap [ErrInvalidStruct], so
// callers can branch on the failure kind with errors.Is instead of parsing the
// message. [MustCompile] panics with the same wrapped error.
func Compile[T any](pattern string) (*Decoder[T], error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidPattern, err)
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
		return nil, fmt.Errorf("%w: T must be a struct type, got %v", ErrInvalidStruct, rt)
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

// One returns the result of decoding the first match of d's pattern in target.
// Returns [ErrNoMatch] if there's no match. Other errors indicate either a
// per-field conversion failure (a [DecodeError]) or a `required` group that
// produced no value (a [RequiredGroupError]); in that case the returned T
// contains whatever fields were successfully decoded before the failure. A matched field
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
// error indicates a per-field conversion failure (a [DecodeError]) or a
// `required` group that produced no value (a [RequiredGroupError]) on one of the
// matches; the slice up to that point may contain partially-decoded entries.
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
// nil error means decoded successfully; non-nil means a per-field conversion
// failed (a [DecodeError]) or a `required` group produced no value (a
// [RequiredGroupError]) on that match. Iteration continues past errors so
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
// On a fully-consumed input, throughput is roughly comparable to
// [UnmarshalAll] — Iter's advantage is streaming (no
// materialized result slice, so lower peak memory on large inputs, plus the
// lazy decode above), not raw speed.
//
// For a slice of all results with a single error, prefer [Decoder.All].
// For a single match with a sentinel ErrNoMatch, prefer [Decoder.One].
func (d *Decoder[T]) Iter(target string) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		allMatches := d.re.FindAllStringSubmatchIndex(target, -1)
		if len(allMatches) == 0 {
			// Early return before declaring the scratch value: &v escapes to
			// the heap, and this guard keeps that alloc off the no-match path.
			return
		}
		// One scratch T reused across matches instead of a fresh heap
		// allocation per match. The zero-reset below is what makes the reuse
		// sound: yield receives v by value, so a copy is handed out, and the
		// reset guarantees the scratch holds no references (pointer fields,
		// etc.) into a previously yielded copy — decode allocates fresh
		// pointees for each match rather than writing through stale ones.
		var v T
		rv := reflect.ValueOf(&v).Elem()
		for _, matches := range allMatches {
			v = d.zero
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
