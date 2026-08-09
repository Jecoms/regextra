package regextra

import (
	"encoding"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

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
	// conv converts one matched (or defaulted) string into the field, resolved
	// once at plan-build time by resolveConverter so the per-decode call skips
	// setFieldValue's type-dispatch probes. Interface-typed fields get a
	// converter that defers to setFieldValue, preserving dynamic dispatch.
	conv func(field reflect.Value, value string) error
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
// decode time, and a stray `layout=` is ignored on non-time fields (their
// converters never consult it). This preserves Unmarshal's historical
// best-effort behavior, so
// buildDecodePlan never returns a non-nil error when strict=false.
//
// An embedded struct (or *struct) field tagged `regex:",inline"` is promoted:
// buildDecodePlan recurses into it and its exported fields join the plan with
// full top-level treatment (tag parse, name fallback, strict validation), each
// carrying the multi-element index path back to its slot. Promotion follows
// encoding/json precedence — a shallower field bound to a group shadows a
// deeper promoted field bound to the same group; when promoted fields bound to
// the same group tie at the shallowest depth, a sole explicitly tagged binding
// wins over field-name-matched ones (encoding/json's tagged-beats-untagged
// tiebreak), and a tie with no tagged binding — or with several — is rejected
// under strict (wrapped [ErrInvalidStruct]) with every binding of the group
// dropped under lenient, mirroring encoding/json's ambiguous-field rule.
// Embedded fields without the flag are not promoted. Strict additionally
// rejects `inline` on a non-embedded or non-struct field and a group name
// combined with `inline` (e.g. `regex:"meta,inline"`); lenient ignores the
// misplaced flag and treats the field as if it were absent from the tag.
func buildDecodePlan(rt reflect.Type, re *regexp.Regexp, strict bool) ([]fieldDecoder, error) {
	// NumField is an upper bound on candidate entries in the flat case — the
	// loop only ever skips fields — which is what makes it the right capacity
	// hint; only promotion can push the count past it.
	entries := make([]decodePlanEntry, 0, rt.NumField())
	if err := collectDecodeFields(rt, re, strict, nil, []reflect.Type{rt}, &entries); err != nil {
		return nil, err
	}
	return resolvePromotionShadowing(rt, entries, strict)
}

// decodePlanEntry is one candidate plan entry during buildDecodePlan's
// collection pass, before promotion shadowing is resolved: the fieldDecoder
// plus the group name it bound and whether that name came from an explicit
// `regex:"name"` tag. group is empty only for an untagged field whose name
// matched no declared group (retained for its `default=` or `required`); a
// tagged field whose group is undeclared keeps its tag name here with empty
// fd.groupIndexes — resolvePromotionShadowing's groupIndexes filter is what
// keeps those out of contention, not an empty group.
type decodePlanEntry struct {
	fd     fieldDecoder
	group  string
	tagged bool
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
		// An explicit tag name binds exactly and, at equal promotion depth,
		// beats field-name-matched bindings (resolvePromotionShadowing's
		// tiebreak) — record its provenance before the name fallback below
		// erases the distinction.
		tagged := groupName != ""
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
				// binding: the embedded field itself binds the group and no
				// promotion happens.
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

		// Resolve the field's converter once, at plan-build time. Entries later
		// dropped by promotion shadowing resolve one too — wasted work, but
		// only on the cold (cached-per-pattern-and-type) plan-build path.
		conv := resolveConverter(sf.Type, opts)

		if strict {
			// Validate `default=` eagerly: try to assign it to a fresh field
			// and surface any conversion error at compile time, not at first
			// request. Reusing the resolved converter keeps the probe's errors
			// identical to what decode time would produce.
			if def, ok := opts["default"]; ok {
				probe := reflect.New(sf.Type).Elem()
				if err := conv(probe, def); err != nil {
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
				conv:         conv,
			},
			group:  groupName,
			tagged: tagged,
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
// group bound by more than one entry, the shallowest depth wins. Every
// top-level field (depth 1) bound to the group stays in the plan and decodes —
// there is no winner notion among top-level bindings — and shadows all
// promoted bindings. When the shallowest binding is itself promoted
// (depth > 1) and unique, it wins alone; when promoted entries tie at the
// shallowest depth, a sole explicitly tagged entry wins over the
// field-name-matched ones (encoding/json's tagged-beats-untagged tiebreak),
// and a tie with no tagged entry — or with several — is ambiguous: strict
// rejects the plan (wrapped [ErrInvalidStruct]) and lenient drops every
// binding of that group, mirroring encoding/json's ambiguous-field rule.
//
// A flat plan (no promotion) skips the per-group map work, but the pass is
// not free even then: the drop mask, the output slice, and each entry's
// heap-allocated index path (appendFieldPath) are new costs relative to the
// pre-promotion plan build (which PR #174 had trimmed). They sit on the cold
// plan-build path — cached per (pattern, type) — so per-decode cost is
// unchanged; future perf work should treat plan build as having regressed by
// those allocations, not as allocation-identical.
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
				// encoding/json's tagged-beats-untagged tiebreak: among the
				// equal-depth promoted winners, a sole explicitly tagged
				// binding wins and the field-name-matched ones drop. No tagged
				// binding — or more than one — leaves the tie ambiguous.
				taggedIdx, taggedCount := -1, 0
				for _, i := range winners {
					if entries[i].tagged {
						taggedIdx, taggedCount = i, taggedCount+1
					}
				}
				if taggedCount == 1 {
					for _, i := range winners {
						if i != taggedIdx {
							drop[i] = true
						}
					}
					continue
				}
				if strict {
					a := rt.FieldByIndex(entries[winners[0]].fd.fieldIndex)
					b := rt.FieldByIndex(entries[winners[1]].fd.fieldIndex)
					return nil, fmt.Errorf("%w: group %q is bound by promoted fields %s and %s at equal depth with no sole tagged binding to break the tie", ErrInvalidStruct, group, a.Name, b.Name)
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
		if err := fd.conv(field, value); err != nil {
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
// Two flag-style tokens (lone tokens, no `=`) are recognized:
//   - required — marks the field's group as mandatory: decode fails with a
//     *[RequiredGroupError] when the group does not participate in a match or
//     matches an empty span and no `default=` supplies a value. The first
//     recognized lone-token flag (the slot the forward-compat rules below
//     reserved).
//   - inline — on an embedded struct (or *struct) field only: promotes the
//     embedded struct's fields into the decode/encode plan with
//     encoding/json-style precedence (see buildDecodePlan). The second
//     recognized lone-token flag.
//
// Forward-compat rules (part of the stability contract since v1 — see the
// package doc's "Tag grammar" section for the full statement and rationale):
//   - Unknown key=value pairs are preserved in the returned map so future
//     option additions don't need to touch the parser; adding a new option
//     key is therefore not a breaking change.
//   - Lone tokens without `=` other than the recognized `required` and `inline`
//     flags are silently ignored today; the slot remains reserved for future
//     flag-style options, so callers must not rely on an unrecognized lone
//     token staying inert.
//
// The two forms differ:
//   - `regex:""` (no tag) signals "no name", returning
//     ("", nil, false, false, false); the caller falls back to matching the
//     field's own name against a group.
//   - `regex:"-"` signals "exclude this field", returning
//     ("", nil, false, false, true); the
//     caller excludes the field entirely, never attempting a name fallback. This
//     mirrors the `-` convention in encoding/json, encoding/xml, and
//     gopkg.in/yaml. Only the bare `-` tag excludes; a leading `-` followed by
//     options (e.g. `regex:"-,default=x"`) parses `-` as the group name, which
//     matches no group since group names are Go identifiers.
func parseFieldTag(field reflect.StructField) (name string, opts map[string]string, required, inline, skip bool) {
	tag := field.Tag.Get("regex")
	if tag == "-" {
		return "", nil, false, false, true
	}
	if tag == "" {
		return "", nil, false, false, false
	}
	// Walk the comma-separated pieces with strings.Cut instead of allocating
	// strings.Split's []string — the pieces are only visited once, in order,
	// so the slice was pure overhead (-1 alloc per tagged field on every plan
	// build; the durable win is the Compile/DeriveEncoder/cache-miss paths).
	first, rest, hasOpts := strings.Cut(tag, ",")
	name = strings.TrimSpace(first)
	if !hasOpts {
		return name, nil, false, false, false
	}
	for more := true; more; {
		var p string
		p, rest, more = strings.Cut(rest, ",")
		p = strings.TrimSpace(p)
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			// No '=': a lone token. `required` and `inline` are the two
			// recognized flags — `required` marks the field's group mandatory
			// (enforced in runDecodePlan); `inline` opts an embedded struct
			// field into promotion (consumed in buildDecodePlan and
			// resolveEncodeField). Any other lone token — including an empty
			// piece from a doubled, leading, or trailing comma — is silently
			// ignored to keep the parser forward-compatible. An empty piece
			// needs no separate guard: strings.Cut("", "=") returns ok=false,
			// so it lands here too.
			switch k {
			case "required":
				required = true
			case "inline":
				inline = true
			}
			continue
		}
		// Allocate the options map lazily — only a key=value pair populates it.
		// A field with just a lone flag (e.g. `name,required`) keeps opts nil,
		// matching the no-options case (no comma in the tag). parseFieldTag
		// runs only inside the at-most-once-per-(pattern, type) plan build, so
		// the win is not per call but per plan entry: nil opts avoids an empty
		// map retained for the life of the process by every cached plan.
		// Consumers already treat nil opts as "no options" (nil-map reads are
		// zero-value). The size hint (comma count = option-piece count, since
		// n commas split into n+1 pieces and the first piece is the name)
		// matches the old len(parts)-1.
		if opts == nil {
			opts = make(map[string]string, strings.Count(tag, ","))
		}
		opts[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return name, opts, required, inline, false
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

// regexUnmarshalerType is the reflect.Type of RegexUnmarshaler, cached so
// the implements-check on every field doesn't pay the reflect.TypeOf cost.
var regexUnmarshalerType = reflect.TypeOf((*RegexUnmarshaler)(nil)).Elem()

// textUnmarshalerType is the reflect.Type of encoding.TextUnmarshaler, cached
// for the same reason as regexUnmarshalerType. Many stdlib and third-party
// types (netip.Addr, big.Int, slog.Level, uuid.UUID, ...) already implement
// it, so honoring it lets those drop into a struct field with no wrapper.
var textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()

// resolveConverter resolves, at plan-build time, the conversion func a field
// of type t executes per decoded value. It mirrors setFieldValue's dispatch
// precedence exactly — RegexUnmarshaler, then the time.Time/time.Duration
// special-cases, then encoding.TextUnmarshaler, then the built-in kind switch —
// but hoists the type probes out of the per-decode path: for a concrete field
// type every probe result is a pure function of t (and the parsed tag opts), so
// the plan can pay them once and the hot path collapses to one indirect call.
// See setFieldValue for the semantics each branch implements; error text is
// built from the same format strings so the two paths stay byte-identical.
//
// Two deliberate exceptions keep behavior unchanged:
//   - Interface-kind fields dispatch on the *dynamic* value stored in the field
//     at decode time, which no plan-time probe of t can know. Those fields get
//     a converter that defers to setFieldValue per call, preserving the
//     dynamic path.
//   - Unsupported types (nested struct, slice, map, ...) return their
//     "unsupported field type" error from the converter at decode time, not as
//     a plan-build failure — Compile does not reject such fields today (they
//     only error when a match actually reaches them), and the lenient
//     Unmarshal path must stay lenient.
//
// The returned converter assumes field is addressable and of exactly type t —
// true everywhere the plan runs (walkFieldPath resolves fields of an
// addressable struct; the strict default= probe uses reflect.New(t).Elem()).
func resolveConverter(t reflect.Type, opts map[string]string) func(field reflect.Value, value string) error {
	// Interface-typed fields: dynamic dispatch preserved (see doc above).
	if t.Kind() == reflect.Interface {
		return func(field reflect.Value, value string) error {
			return setFieldValue(field, value, opts)
		}
	}

	// Pointer fields: allocate the pointee if nil, then either the pointer
	// type's own RegexUnmarshaler or the pointee's converter, resolved here
	// once per indirection level (`**Foo` recurses).
	if t.Kind() == reflect.Ptr {
		elemType := t.Elem() // constant per the exactly-type-t invariant; hoisted out of the closures
		if t.Implements(regexUnmarshalerType) {
			return func(field reflect.Value, value string) error {
				if field.IsNil() {
					field.Set(reflect.New(elemType))
				}
				return field.Interface().(RegexUnmarshaler).UnmarshalRegex(value)
			}
		}
		elemConv := resolveConverter(elemType, opts)
		return func(field reflect.Value, value string) error {
			if field.IsNil() {
				field.Set(reflect.New(elemType))
			}
			return elemConv(field.Elem(), value)
		}
	}

	// RegexUnmarshaler first — caller-defined conversions beat everything.
	// Probing reflect.PointerTo(t) covers value-receiver implementations too
	// (*T's method set includes T's), matching setFieldValue's Addr dispatch.
	if reflect.PointerTo(t).Implements(regexUnmarshalerType) {
		return func(field reflect.Value, value string) error {
			return field.Addr().Interface().(RegexUnmarshaler).UnmarshalRegex(value)
		}
	}

	// time.Time and time.Duration special-cases, ahead of TextUnmarshaler for
	// the reason documented on setFieldValue step 3. The `layout=` option is
	// resolved here so per-decode calls skip the opts lookup entirely.
	switch t {
	case timeTimeType:
		if layout := opts["layout"]; layout != "" {
			return func(field reflect.Value, value string) error {
				tv, err := time.Parse(layout, value)
				if err != nil {
					return fmt.Errorf("cannot convert %q to time.Time using layout %q: %w", value, layout, err)
				}
				field.Set(reflect.ValueOf(tv))
				return nil
			}
		}
		return func(field reflect.Value, value string) error {
			tv, err := parseTime(value)
			if err != nil {
				return fmt.Errorf("cannot convert %q to time.Time: %w", value, err)
			}
			field.Set(reflect.ValueOf(tv))
			return nil
		}
	case timeDurationType:
		return func(field reflect.Value, value string) error {
			d, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("cannot convert %q to time.Duration: %w", value, err)
			}
			field.Set(reflect.ValueOf(d))
			return nil
		}
	}

	// encoding.TextUnmarshaler fallback (below RegexUnmarshaler and the time
	// special-cases, as in setFieldValue).
	if reflect.PointerTo(t).Implements(textUnmarshalerType) {
		return func(field reflect.Value, value string) error {
			if err := field.Addr().Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(value)); err != nil {
				return fmt.Errorf("cannot convert %q to %s: %w", value, t, err)
			}
			return nil
		}
	}

	// Built-in kind switch. Bits() is resolved here so per-decode calls parse
	// straight at the field's width.
	switch t.Kind() {
	case reflect.String:
		return func(field reflect.Value, value string) error {
			field.SetString(value)
			return nil
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		bits := t.Bits()
		return func(field reflect.Value, value string) error {
			intVal, err := strconv.ParseInt(value, 10, bits)
			if err != nil {
				return fmt.Errorf("cannot convert %q to %s: %w", value, t, err)
			}
			field.SetInt(intVal)
			return nil
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		bits := t.Bits()
		return func(field reflect.Value, value string) error {
			uintVal, err := strconv.ParseUint(value, 10, bits)
			if err != nil {
				return fmt.Errorf("cannot convert %q to %s: %w", value, t, err)
			}
			field.SetUint(uintVal)
			return nil
		}

	case reflect.Float32, reflect.Float64:
		bits := t.Bits()
		return func(field reflect.Value, value string) error {
			floatVal, err := strconv.ParseFloat(value, bits)
			if err != nil {
				return fmt.Errorf("cannot convert %q to %s: %w", value, t, err)
			}
			field.SetFloat(floatVal)
			return nil
		}

	case reflect.Bool:
		return func(field reflect.Value, value string) error {
			boolVal, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("cannot convert %q to bool: %w", value, err)
			}
			field.SetBool(boolVal)
			return nil
		}

	default:
		kind := t.Kind()
		return func(reflect.Value, string) error {
			return fmt.Errorf("unsupported field type: %s", kind)
		}
	}
}

// setFieldValue sets the field value with appropriate type conversion.
// `opts` carries per-field tag options parsed from `regex:"name,key=value,..."`.
// Currently consulted: `layout` (for time.Time fields). Pass nil for no opts.
//
// This is the dynamic-dispatch implementation: the decode plan's per-field
// converters (resolveConverter) hoist this function's type probes to
// plan-build time and are what runDecodePlan executes; setFieldValue remains
// as the per-call path behind interface-typed fields' converters, where
// dispatch depends on the dynamic value. The two must stay in lockstep —
// same precedence, same error text.
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

// planCacheKey identifies one cached decode plan: the regexp's source pattern
// plus the destination struct type. Keying on re.String() rather than the
// *regexp.Regexp itself is sound because two Regexps with equal String() have
// identical SubexpNames, and the non-strict buildDecodePlan reads re only
// through SubexpNames/SubexpIndex (via subexpIndexes and matchGroupName) —
// pure functions of the pattern string.
type planCacheKey struct {
	pattern string
	typ     reflect.Type
}

// planCache caches the lenient (strict=false) decode plan per (pattern, struct
// type) pair for the free functions [Unmarshal] and [UnmarshalAll], so only
// the first call for a given pair pays the reflect plan build. Values are
// []fieldDecoder, immutable after construction (runDecodePlan only reads
// them), so sharing one plan across goroutines is safe.
//
// The cache is process-lifetime with no eviction, mirroring encoding/json's
// type-keyed field cache: entries are small (field indexes + parsed tag
// options; the *regexp.Regexp is not retained), and real workloads use a
// bounded set of (pattern, type) pairs. A workload that decodes unboundedly
// many distinct patterns grows the cache without bound — but such a workload
// already pays per-call regexp compilation, which dwarfs plan retention.
var planCache sync.Map // planCacheKey -> []fieldDecoder

// getDecodePlan returns the decode plan for (rt, re) on the lenient free-
// function path, building and caching it on first use. Racing first callers
// may both build; LoadOrStore makes one plan canonical and both return it.
// buildDecodePlan never returns a non-nil error when strict=false; the check
// is kept for forward-safety (see Unmarshal), and an error is never cached.
func getDecodePlan(rt reflect.Type, re *regexp.Regexp) ([]fieldDecoder, error) {
	key := planCacheKey{pattern: re.String(), typ: rt}
	if cached, ok := planCache.Load(key); ok {
		return cached.([]fieldDecoder), nil
	}
	fields, err := buildDecodePlan(rt, re, false)
	if err != nil {
		return nil, err
	}
	plan, _ := planCache.LoadOrStore(key, fields)
	return plan.([]fieldDecoder), nil
}
