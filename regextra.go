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
  - Pull every named group across all matches (map of slices): [AllNamedGroups]
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

# Stability

regextra is pre-v1 and follows SemVer. Patch releases are fixes only. Minor
releases may add features and may include breaking changes (called out in the
CHANGELOG). Post-v1, breaking changes will ship in the next major version.
See ROADMAP.md and the README's Stability section for the precise contract,
including what does and does not count as breaking.

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
