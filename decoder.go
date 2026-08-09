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

// ErrInvalidPattern and ErrInvalidStruct categorize [Compile] (and therefore
// [MustCompile]) failures so callers can branch on the failure kind with
// errors.Is rather than parsing the message. ErrInvalidPattern wraps a bad
// regular expression; ErrInvalidStruct wraps every destination-shape problem
// (T is not a struct, a field references an undeclared group, a `default=`
// value does not convert, or `layout=` sits on a non-time.Time field). Each
// wrapped error keeps its descriptive detail — and, where one exists, the
// underlying cause — reachable via errors.Is/As. Like ErrNoMatch, these
// sentinels carry the bare `regextra:` prefix reserved for package-level
// sentinels.
var (
	ErrInvalidPattern = errors.New("regextra: invalid pattern")
	ErrInvalidStruct  = errors.New("regextra: invalid struct")
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

// fieldDecoder is the precomputed decode plan for one struct field.
type fieldDecoder struct {
	// fieldIndex is the index path from T to the field, in the shape of
	// [reflect.StructField.Index]: one element for a top-level field, one
	// additional element per `inline`-promoted embedding level. runDecodePlan
	// walks the path with walkFieldPath, allocating nil embedded pointers on
	// the way down.
	fieldIndex []int
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
//   - A field's `regex:"name"` tag references a group not declared on pattern,
//     and the field has no `default=` (a `default=` makes a missing group
//     intentional, so the field compiles and the default always fires)
//   - A field's `regex:",default=<value>"` cannot be converted to the field's type
//   - A field uses `regex:",layout=..."` on a non-time.Time field
//   - The `inline` flag sits on a non-embedded (or non-struct) field, or is
//     combined with a group name (e.g. `regex:"meta,inline"`)
//   - Two `inline`-promoted fields at equal embedding depth bind the same
//     capture group (see [Unmarshal]'s embedded-struct promotion rules)
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

// buildDecodePlan maps each exported field of rt to its regex capture group and
// parsed tag options, returning the per-field decode plan that runDecodePlan
// executes against a match. It is the single plan-construction path shared by
// the [Decoder] (compiled once via compileDecoder) and the [Unmarshal] /
// [UnmarshalAll] free functions (cached per (pattern, struct type) pair via
// getDecodePlan) — one set of field-mapping semantics, so the two paths can't
// drift again.
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
//
// An embedded struct (or *struct) field tagged `regex:",inline"` is promoted:
// buildDecodePlan recurses into it and its exported fields join the plan with
// full top-level treatment (tag parse, name fallback, strict validation), each
// carrying the multi-element index path back to its slot. Promotion follows
// encoding/json precedence — a shallower field bound to a group shadows a
// deeper promoted field bound to the same group, and two promoted fields bound
// to the same group at equal depth are rejected under strict (wrapped
// [ErrInvalidStruct]) and both dropped under lenient, mirroring
// encoding/json's ambiguous-field rule. Untagged embedded fields keep the
// historical non-promoted behavior. Strict additionally rejects `inline` on a
// non-embedded or non-struct field and a group name combined with `inline`
// (e.g. `regex:"meta,inline"`); lenient ignores the misplaced flag and treats
// the field as if it were absent from the tag.
func buildDecodePlan(rt reflect.Type, re *regexp.Regexp, strict bool) ([]fieldDecoder, error) {
	// NumField is a strict lower bound on candidate entries (promotion can add
	// more, but the flat case — the overwhelmingly common one — never grows).
	entries := make([]decodePlanEntry, 0, rt.NumField())
	if err := collectDecodeFields(rt, re, strict, nil, []reflect.Type{rt}, &entries); err != nil {
		return nil, err
	}
	return resolvePromotionShadowing(rt, entries, strict)
}

// decodePlanEntry is one candidate plan entry during buildDecodePlan's
// collection pass, before promotion shadowing is resolved: the fieldDecoder
// plus the group name it bound (empty for a default-/required-only field with
// no declared group).
type decodePlanEntry struct {
	fd    fieldDecoder
	group string
}

// collectDecodeFields walks one struct level of the decode-plan build,
// recursing into `inline`-promoted embedded structs. path is the index prefix
// from the root type to rt (nil at the top level); onPath lists the struct
// types on the current embedding path, guarding against embedding cycles
// (`type A struct{ *B \`regex:",inline"\` }` / `type B struct{ *A … }`): a
// type already on the path is silently not re-entered, terminating the walk —
// the fields reachable before the repeat are still promoted, mirroring
// encoding/json's cycle handling.
func collectDecodeFields(rt reflect.Type, re *regexp.Regexp, strict bool, path []int, onPath []reflect.Type, entries *[]decodePlanEntry) error {
	for i := range rt.NumField() {
		sf := rt.Field(i)
		if !sf.IsExported() {
			continue
		}

		groupName, opts, required, inline, skip := parseFieldTag(sf)
		if skip {
			// `regex:"-"` excludes the field entirely — it never enters the
			// decode plan and no name fallback is attempted. On an embedded
			// field it also suppresses promotion.
			continue
		}
		if inline {
			switch et := inlineStructType(sf); {
			case et == nil:
				// `inline` promotes an embedded struct's fields; on anything
				// else there is nothing to promote — a tag typo. Strict rejects
				// it fail-fast like the other strict checks; lenient ignores
				// the misplaced flag and falls through to normal handling.
				if strict {
					return fmt.Errorf("%w: field %s has `inline` flag but is not an embedded struct", ErrInvalidStruct, sf.Name)
				}
			case groupName != "":
				// A group name binds the embedded field itself; inline
				// dissolves it into its promoted fields. The two are mutually
				// exclusive. Lenient ignores the flag and keeps the name
				// binding, matching the pre-inline behavior of the same tag.
				if strict {
					return fmt.Errorf("%w: field %s combines a group name %q with the `inline` flag", ErrInvalidStruct, sf.Name, groupName)
				}
			default:
				if !typeOnPath(onPath, et) {
					if err := collectDecodeFields(et, re, strict, appendFieldPath(path, i), append(onPath, et), entries); err != nil {
						return err
					}
				}
				continue
			}
		}
		if groupName == "" {
			// No explicit tag — fall back to matching the field name against a
			// declared group: exact first, then case-insensitively via Unicode
			// simple case folding (see matchGroupName). A field that matches
			// no group and has no default is treated as a typo and, under
			// strict, fails the build below.
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
				return fmt.Errorf("%w: field %s references group %q which is not declared on the pattern", ErrInvalidStruct, sf.Name, groupName)
			}
		}

		if strict {
			// Validate `default=` eagerly: try to assign it to a fresh field
			// and surface any conversion error at compile time, not at first
			// request.
			if def, ok := opts["default"]; ok {
				probe := reflect.New(sf.Type).Elem()
				if err := setFieldValue(probe, def, opts); err != nil {
					return fmt.Errorf("%w: field %s default %q does not convert to %v: %w", ErrInvalidStruct, sf.Name, def, sf.Type, err)
				}
			}

			// Validate `layout=` is only on time.Time fields.
			if _, ok := opts["layout"]; ok {
				ft := sf.Type
				if ft.Kind() == reflect.Ptr {
					ft = ft.Elem()
				}
				if ft != timeTimeType {
					return fmt.Errorf("%w: field %s has `layout=` option but is %v, not time.Time", ErrInvalidStruct, sf.Name, sf.Type)
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

		*entries = append(*entries, decodePlanEntry{
			fd: fieldDecoder{
				fieldIndex:   appendFieldPath(path, i),
				groupIndexes: groupIdxs,
				opts:         opts,
				required:     required,
			},
			group: groupName,
		})
	}

	return nil
}

// inlineStructType returns the struct type an `inline`-tagged field promotes —
// the field's own type for an embedded struct, the pointee for an embedded
// *struct — or nil when the field is not a valid promotion target (not
// embedded, or not struct-kinded), which strict rejects and lenient ignores.
func inlineStructType(sf reflect.StructField) reflect.Type {
	if !sf.Anonymous {
		return nil
	}
	t := sf.Type
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	return t
}

// appendFieldPath returns a fresh index path extending path with i. The copy
// matters: entries retain their paths for the life of the plan, so extending
// the caller's scratch slice in place would let sibling recursions alias and
// clobber each other's tails.
func appendFieldPath(path []int, i int) []int {
	return append(append(make([]int, 0, len(path)+1), path...), i)
}

// typeOnPath reports whether t already appears on the current embedding path.
// A linear scan: embedding depth is small in practice, so a map would cost
// more than it saves.
func typeOnPath(onPath []reflect.Type, t reflect.Type) bool {
	for _, p := range onPath {
		if p == t {
			return true
		}
	}
	return false
}

// resolvePromotionShadowing applies encoding/json's field-precedence rules to
// the collected candidate entries when promotion was used: for each declared
// group bound by more than one entry, the shallowest depth wins. Top-level
// bindings (depth 1) keep the pre-promotion semantics — every top-level field
// bound to the group stays in the plan, exactly as before `inline` existed —
// and shadow all promoted bindings. When the shallowest binding is itself
// promoted (depth > 1) and unique, it wins alone; when two promoted entries tie
// at the shallowest depth, strict rejects the plan (wrapped [ErrInvalidStruct])
// and lenient drops every binding of that group, mirroring encoding/json's
// ambiguous-field rule. A flat plan (no promotion) is returned unchanged
// without any per-group work, keeping the common path allocation-identical.
func resolvePromotionShadowing(rt reflect.Type, entries []decodePlanEntry, strict bool) ([]fieldDecoder, error) {
	promoted := false
	for i := range entries {
		if len(entries[i].fd.fieldIndex) > 1 {
			promoted = true
			break
		}
	}
	drop := make([]bool, len(entries))
	if promoted {
		byGroup := make(map[string][]int)
		for i, e := range entries {
			// Only declared-group bindings compete: an undeclared group with a
			// `default=` (or a bare `required`) writes to its own distinct
			// field and never contends for a match value.
			if e.group != "" && len(e.fd.groupIndexes) > 0 {
				byGroup[e.group] = append(byGroup[e.group], i)
			}
		}
		for group, idxs := range byGroup {
			if len(idxs) < 2 {
				continue
			}
			minDepth := len(entries[idxs[0]].fd.fieldIndex)
			for _, i := range idxs[1:] {
				if d := len(entries[i].fd.fieldIndex); d < minDepth {
					minDepth = d
				}
			}
			var winners []int
			for _, i := range idxs {
				if len(entries[i].fd.fieldIndex) == minDepth {
					winners = append(winners, i)
				} else {
					drop[i] = true
				}
			}
			if minDepth > 1 && len(winners) > 1 {
				if strict {
					a := rt.FieldByIndex(entries[winners[0]].fd.fieldIndex)
					b := rt.FieldByIndex(entries[winners[1]].fd.fieldIndex)
					return nil, fmt.Errorf("%w: group %q is bound by promoted fields %s and %s at equal depth", ErrInvalidStruct, group, a.Name, b.Name)
				}
				for _, i := range idxs {
					drop[i] = true
				}
			}
		}
	}
	fields := make([]fieldDecoder, 0, len(entries))
	for i, e := range entries {
		if !drop[i] {
			fields = append(fields, e.fd)
		}
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
// (exactly, then case-insensitively via Unicode simple case folding), or ""
// if no group matches. Used as a fallback when a field has no explicit
// `regex:"..."` tag.
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
// so the name need not be retained per field in the plan (which would cost
// bytes in every plan, including every entry the Unmarshal/UnmarshalAll plan
// cache keeps for the life of the process).
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
	name, _, _, _, _ := parseFieldTag(sf)
	return name
}

// walkFieldPath resolves a fieldDecoder's index path against rv (the
// addressable reflect.Value of the destination struct), returning the leaf
// field. Intermediate steps are `inline`-promoted embedded fields; a
// pointer-embedded step that is nil is allocated on the way down (mirroring
// setFieldValue's nil-pointer allocation), which is why plain
// reflect.Value.FieldByIndex — which panics on a nil embedded pointer — is not
// used. The common flat case (len(path) == 1) reduces to a single Field call.
func walkFieldPath(rv reflect.Value, path []int) reflect.Value {
	field := rv.Field(path[0])
	for _, i := range path[1:] {
		if field.Kind() == reflect.Ptr {
			if field.IsNil() {
				field.Set(reflect.New(field.Type().Elem()))
			}
			field = field.Elem()
		}
		field = field.Field(i)
	}
	return field
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
				sf := rv.Type().FieldByIndex(fd.fieldIndex)
				return &RequiredGroupError{
					Field: sf.Name,
					Group: resolveGroupName(re, sf, fd.groupIndexes),
				}
			}
			continue
		}
		field := walkFieldPath(rv, fd.fieldIndex)
		if err := setFieldValue(field, value, fd.opts); err != nil {
			sf := rv.Type().FieldByIndex(fd.fieldIndex)
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
