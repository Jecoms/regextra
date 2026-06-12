/*
Package regextra adds the convenience layer the standard library's regexp
package leaves out: name-based access to capture groups, struct unmarshaling,
and a typed cached decoder for repeated patterns.

# The pain it solves

Extracting a named group with stdlib regexp is a three-step dance:

	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
	matches := re.FindStringSubmatch("Alice 30")
	nameIndex := re.SubexpIndex("name")
	name := matches[nameIndex]  // "Alice"

regextra collapses that to one call, and goes further with map-based access
and json.Unmarshal-style decoding into structs.

# Quick start

	re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)

	// Single named group:
	name, ok := regextra.FindNamed(re, "Alice is 30", "name")  // "Alice", true

	// All named groups as a map:
	m := regextra.NamedGroups(re, "Alice is 30")  // map[name:Alice age:30]

	// Decode into a typed struct:
	type Person struct {
	    Name string
	    Age  int
	}
	var p Person
	regextra.Unmarshal(re, "Alice is 30", &p)  // p = {Name: "Alice", Age: 30}

# API at a glance

By use case:

  - Pull one named group from one match: [FindNamed]
  - Pull one named group across all matches: [FindAllNamed]
  - Pull every named group from one match (map): [NamedGroups]
  - Pull every named group from one match, keeping every value when a group
    name is reused inside the pattern (map of slices): [AllNamedGroups]
  - Substitute named-group spans by name: [Replace]
  - Assert at startup that required groups are declared: [Validate]
  - Decode one match into a struct: [Unmarshal]
  - Decode all matches into a slice of structs: [UnmarshalAll]
  - Decode the same shape repeatedly with cached reflect work: [Compile], [MustCompile], [Decoder]
  - Stream matches lazily (Go 1.23+ range-over-func): [Decoder.Iter]
  - Plug in caller-defined types in the unmarshal path: [RegexUnmarshaler]
  - Compare against the no-match sentinel: [ErrNoMatch]

# Performance

For one-shot extraction, [Unmarshal] does its reflect work per call. For repeated
decode of the same shape (log parsers, request handlers, config readers), use
[Compile] / [Decoder] — it caches the per-field plan and benchmarks at roughly
half the time and half the allocations of [Unmarshal] on equivalent input.
[Decoder.Iter] further skips the slice allocation entirely for streaming
consumers.

# No-match behavior

Functions in this package handle "the regex did not match the target" differently
depending on their return shape. The asymmetry is intentional — each call returns
the no-match form that lets the caller continue without a special-case branch.

	Function                                  No-match return
	----------------------------------------------------------------------------
	FindNamed                                 ("", false)
	FindAllNamed                              []string{} (or nil if the group
	                                          name is not declared on the regex)
	NamedGroups, AllNamedGroups               empty map (initialized, not nil)
	Replace                                   target returned unchanged
	Validate                                  unrelated — checks declarations,
	                                          not matches against a target
	Unmarshal                                 nil error; destination struct left
	                                          unchanged
	UnmarshalAll                              nil error; destination slice
	                                          length set to 0
	Decoder.One                               zero T, [ErrNoMatch]
	Decoder.All                               []T{}, nil
	Decoder.Iter                              iterator yields zero times

The contrast worth understanding is between [Unmarshal] and [Decoder.One]:

  - [Unmarshal] returns nil on no match. The caller passes the destination, so
    they can inspect their own struct after the call to detect "did anything
    decode?" — no sentinel needed, and reserving error for genuine failures
    (bad pointer, type-conversion failure) keeps `if err != nil` meaningful.

  - [Decoder.One] returns [ErrNoMatch]. It returns (T, error), constructing
    the value itself; a zero-value T paired with nil error would be
    indistinguishable from "successfully decoded a struct of all zero fields".
    The sentinel disambiguates. Compare with errors.Is so the check survives
    wrapping.

Example of the [Decoder.One] no-match check:

	v, err := dec.One(input)
	if errors.Is(err, regextra.ErrNoMatch) {
	    // no match — handle as data absence, not failure
	}

[Decoder.All] and [Decoder.Iter] don't have the ambiguity problem — an empty
slice and a zero-iteration range are unambiguous — so they follow the same
"no match is not an error" convention as [UnmarshalAll].

For [FindAllNamed], the nil-vs-empty-slice split is a separate signal:

  - nil — the group name is not declared on the regex (likely a typo; consider
    [Validate] at startup to catch this).
  - []string{} — the group is declared but the regex has no matches in the
    target (data absence — iterate over zero or more).

This distinction is the only place in the no-match table where a single function
returns two different no-match shapes; everywhere else the no-match form is
fixed regardless of why the match failed.

# Tag grammar

The `regex:"..."` struct tag uses a JSON-encoding-style grammar: the first
comma-separated piece is the group name; each subsequent piece is a key=value
option. Currently recognized keys:

	default=<value>           Any field type. Substituted when the named
	                          group is undeclared on the regex or its match
	                          is empty. Goes through the same type
	                          conversion as a real match.
	layout=<go-time-layout>   time.Time only. Used exclusively, instead of
	                          the default RFC3339-and-friends fallback list.

`regex:""` and `regex:"-"` both signal "no name" — fall back to the field's
own name for matching.

Two forward-compatibility rules in [parseFieldTag] are part of the v1 contract:

  - Unknown key=value pairs are preserved, not rejected. The parser stores
    every key=value pair regardless of whether the key is currently
    recognized, so a future minor release can introduce additional option
    keys without a parser change. Adding a new option key is therefore not
    a breaking change. Callers must not rely on the parser rejecting
    unknown keys; pin a minor version range if you need a specific
    recognized set.

  - Lone tokens (no `=`) are silently ignored. Today, `regex:"name,foo"`
    parses as (name="name", opts=nil) — the `foo` token is dropped. This
    slot is reserved for future flag-style options (e.g. `required`; see
    ROADMAP.md). A later minor release may start recognizing specific lone
    tokens and giving them meaning, so adding `regex:"name,foo"` today is a
    no-op but may stop being one. Callers must not rely on lone tokens
    remaining inert.

The two rules together are how the tag grammar grows compatibly: a new
option ships as either an additional key=value pair (orthogonal to today's
grammar) or a recognized flag token (claiming a previously-ignored slot).

# Stability

regextra is at v1 and follows strict SemVer. Patch releases are fixes only.
Minor releases add features without breaking changes. Breaking changes ship
in the next major version, never in a minor or patch. See ROADMAP.md and the
README's Stability section for the precise contract, including what does and
does not count as breaking.

# More

The package README has full per-function reference with runnable examples for
each function.
*/
package regextra

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

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
// To pull every named group from one match (with duplicate-name handling),
// use [AllNamedGroups]. Despite the "All" prefix, AllNamedGroups operates on
// a single match — it does not iterate matches across the target the way
// FindAllNamed does.
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

// AllNamedGroups operates on a single match and returns every value of every
// named capture group, keyed by group name. Each value is a slice because Go's
// regexp package allows the same group name to appear more than once in a
// pattern; AllNamedGroups preserves every occurrence in left-to-right order.
// Groups that appear once still get a one-element slice.
//
// The leading "All" refers to all named groups in one match — not to all
// matches across the target. Internally the function calls FindStringSubmatch,
// so only the first match contributes values. To collect every value of a
// single named group across every match in the target, use [FindAllNamed].
// There is no current function that returns every named group across every
// match (i.e. []map[string]string); the unmarshal path ([UnmarshalAll],
// [Decoder.All], [Decoder.Iter]) is the typed equivalent.
//
// On no match, returns an empty (non-nil) map. See the package doc's
// "No-match behavior" section for the full cross-API contract.
//
// Example — duplicate group names (the use case this function exists for):
//
//	re := regexp.MustCompile(`(?P<word>\w+) (?P<word>\w+)`)
//	allGroups := regextra.AllNamedGroups(re, "hello world")
//	// allGroups = map[string][]string{"word": []string{"hello", "world"}}
//
// Example — distinct group names (each slice has one element):
//
//	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
//	allGroups := regextra.AllNamedGroups(re, "Alice 30")
//	// allGroups = map[string][]string{"name": []string{"Alice"}, "age": []string{"30"}}
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
// On no match, returns target unchanged. See the package doc's
// "No-match behavior" section for the full cross-API contract.
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
