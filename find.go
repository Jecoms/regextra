package regextra

import (
	"iter"
	"regexp"
)

// FindNamed returns the value of the named capture group in the target string.
// It returns the matched value and true if found, or empty string and false if not found.
//
// When the pattern reuses a group name (e.g. across alternation branches),
// FindNamed returns the value of the last occurrence that participated in the
// match — it does not trust re.SubexpIndex, which reports only the first
// occurrence and would return the wrong value when a later branch is the one
// that matched.
//
// Example:
//
//	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
//	name, ok := regextra.FindNamed(re, "Alice 30", "name")
//	// name = "Alice", ok = true
func FindNamed(re *regexp.Regexp, target, groupName string) (string, bool) {
	// Scan SubexpNames directly instead of materializing subexpIndexes' []int
	// — the occurrence indices are only ever walked in declaration order, so
	// two passes over the (already-cached) names slice make FindNamed
	// allocation-free apart from the regexp engine's own work.
	names := re.SubexpNames()
	declared := false
	for i := 1; i < len(names); i++ {
		if names[i] == groupName {
			declared = true
			break
		}
	}
	if !declared {
		return "", false
	}

	// FindStringSubmatchIndex returns only the match offsets ([]int), so we
	// slice the participating group out of target directly. FindStringSubmatch
	// would additionally allocate a []string for every group just to discard
	// all but one.
	loc := re.FindStringSubmatchIndex(target)
	if loc == nil {
		return "", false
	}
	// A pattern may reuse a group name (e.g. across alternation branches); the
	// last occurrence that participated in the match wins, and a
	// non-participating occurrence is skipped. If no occurrence participated,
	// value stays "" — matching FindStringSubmatch's "" for a declared but
	// non-participating group.
	value := ""
	for i := 1; i < len(names); i++ {
		if names[i] != groupName {
			continue
		}
		if start := loc[2*i]; start >= 0 {
			value = target[start:loc[2*i+1]]
		}
	}
	return value, true
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
// use [NamedGroupOccurrences].
//
// When the pattern reuses a group name, each match contributes the value of
// the occurrence that participated in that match, not blindly re.SubexpIndex's
// first occurrence.
func FindAllNamed(re *regexp.Regexp, target, groupName string) []string {
	// Collect the name's occurrence indices into a stack-backed buffer instead
	// of subexpIndexes' heap slice — the indices never outlive this call, so
	// the array does not escape. Four covers any realistic duplicate-name
	// count; a pattern reusing one name more than four times spills to a heap
	// append, trading back the one alloc this avoids.
	var buf [4]int
	idxs := buf[:0]
	for i, n := range re.SubexpNames() {
		if i != 0 && n == groupName {
			idxs = append(idxs, i)
		}
	}
	if len(idxs) == 0 {
		return nil
	}
	// Index form returns []int offsets per match; we slice the participating
	// group out of target rather than have FindAllStringSubmatch build a
	// [][]string of every group across every match only to read one column.
	locs := re.FindAllStringSubmatchIndex(target, -1)
	if len(locs) == 0 {
		return []string{}
	}
	out := make([]string, len(locs))
	for i, loc := range locs {
		// Last participating occurrence of the name in this match wins; a
		// non-participating occurrence is skipped, leaving out[i] == "".
		for _, idx := range idxs {
			if start := loc[2*idx]; start >= 0 {
				out[i] = target[start:loc[2*idx+1]]
			}
		}
	}
	return out
}

// NamedGroups returns a map of all named capture groups and their matched values
// from the target string. If no match is found, it returns an empty map.
//
// When the pattern reuses a group name (e.g. across alternation branches),
// the value of the last occurrence that participated in the match wins; an
// occurrence that did not participate never overwrites a participating
// occurrence's value. A declared group that did not participate at all is
// still present in the map, mapped to "".
//
// To see every occurrence rather than just the winning one, use
// [NamedGroupOccurrences] — but note it reports a non-participating occurrence
// and an occurrence that matched an empty span identically (both as ""), so it
// cannot be used to tell those two cases apart.
//
// Example:
//
//	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
//	groups := regextra.NamedGroups(re, "Alice 30")
//	// groups = map[string]string{"name": "Alice", "age": "30"}
func NamedGroups(re *regexp.Regexp, target string) map[string]string {
	m := re.FindStringSubmatchIndex(target)
	if m == nil {
		return make(map[string]string)
	}
	// includeNonParticipating=true: NamedGroups surfaces every declared group,
	// including ones that did not participate in the match (mapped to "").
	return namedGroupValues(re, target, m, true)
}

// NamedGroupsPerMatch returns one map of named-group values per match of re in
// target, in match order — the every-match counterpart to [NamedGroups]. Each
// map follows the same per-match semantics as [NamedGroups]: every declared
// group is present, a group that did not participate in that match is mapped to
// "", and when the pattern reuses a group name the last participating
// occurrence in that match wins.
//
// On no match, returns an empty (non-nil) slice. See the package doc's
// "No-match behavior" section for the full cross-API contract. For the lazy,
// allocation-light streaming form, use [NamedGroupsPerMatchSeq]. For the typed
// equivalent that decodes each match into a struct, use [UnmarshalAll] /
// [Decoder.All].
//
// Example:
//
//	re := regexp.MustCompile(`(?P<key>\w+)=(?P<value>\w+)`)
//	all := regextra.NamedGroupsPerMatch(re, "a=1 b=2")
//	// all = []map[string]string{{"key": "a", "value": "1"}, {"key": "b", "value": "2"}}
func NamedGroupsPerMatch(re *regexp.Regexp, target string) []map[string]string {
	locs := re.FindAllStringSubmatchIndex(target, -1)
	if len(locs) == 0 {
		return []map[string]string{}
	}
	names := re.SubexpNames()
	out := make([]map[string]string, len(locs))
	for i, m := range locs {
		result := make(map[string]string, len(names))
		fillNamedGroupValues(result, names, target, m, true)
		out[i] = result
	}
	return out
}

// NamedGroupsPerMatchSeq is the range-over-func (Go 1.23+) form of
// [NamedGroupsPerMatch]: it yields one named-group map per match of re in
// target, in match order, without building the intermediate slice. Each yielded
// map follows the same per-match semantics as [NamedGroups]. Stopping the range
// early stops the iteration.
//
// On no match, the iterator yields zero times. See the package doc's "No-match
// behavior" section for the full cross-API contract. For the typed streaming
// equivalent, use [Decoder.Iter].
//
// Example:
//
//	re := regexp.MustCompile(`(?P<key>\w+)=(?P<value>\w+)`)
//	for m := range regextra.NamedGroupsPerMatchSeq(re, "a=1 b=2") {
//	    fmt.Println(m["key"], m["value"])
//	}
//	// Output:
//	// a 1
//	// b 2
func NamedGroupsPerMatchSeq(re *regexp.Regexp, target string) iter.Seq[map[string]string] {
	return func(yield func(map[string]string) bool) {
		locs := re.FindAllStringSubmatchIndex(target, -1)
		if len(locs) == 0 {
			return
		}
		names := re.SubexpNames()
		for _, m := range locs {
			result := make(map[string]string, len(names))
			fillNamedGroupValues(result, names, target, m, true)
			if !yield(result) {
				return
			}
		}
	}
}

// namedGroupValues builds the group-name→value map for one match, given the
// match's index pairs from FindStringSubmatchIndex (or one element of
// FindAllStringSubmatchIndex). Index pairs distinguish "did not participate"
// (negative indices) from "matched an empty span", so when a pattern reuses a
// group name a participating occurrence always wins and a non-participating
// occurrence never clobbers it.
//
// includeNonParticipating controls what happens to a group name that never
// participated in the match:
//   - true (the [NamedGroups] family — its only callers today): the name is
//     recorded as "" so callers see every declared group.
//   - false: the name is omitted entirely, so a non-participating optional
//     group reads as "no value" rather than an empty match. A duplicate that
//     participates elsewhere still sets the key, so omitting never drops a real
//     value. The typed [Unmarshal] / [UnmarshalAll] path no longer routes
//     through this map — it shares the [Decoder]'s index-based decode plan (see
//     buildDecodePlan / runDecodePlan in plan.go) — but the flag is kept for
//     the omit-vs-empty distinction.
func namedGroupValues(re *regexp.Regexp, target string, m []int, includeNonParticipating bool) map[string]string {
	names := re.SubexpNames()
	result := make(map[string]string, len(names))
	fillNamedGroupValues(result, names, target, m, includeNonParticipating)
	return result
}

// fillNamedGroupValues writes one match's group-name→value pairs into dst,
// which the caller has already cleared. names is re.SubexpNames(), passed in so
// a caller decoding many matches (UnmarshalAll) fetches it once and reuses one
// map instead of allocating per match. m is the match's index pairs; see
// [namedGroupValues] for the participation semantics and includeNonParticipating.
func fillNamedGroupValues(dst map[string]string, names []string, target string, m []int, includeNonParticipating bool) {
	for i, name := range names {
		if i == 0 || name == "" {
			continue
		}
		start, end := m[2*i], m[2*i+1]
		if start < 0 {
			if includeNonParticipating {
				if _, ok := dst[name]; !ok {
					dst[name] = ""
				}
			}
			continue
		}
		dst[name] = target[start:end]
	}
}

// NamedGroupOccurrences returns every value of every named capture group in
// the first match, keyed by group name — one slice element per occurrence of
// the name in the pattern. Each value is a slice because Go's regexp package
// allows the same group name to appear more than once in a pattern;
// NamedGroupOccurrences preserves every occurrence in left-to-right order.
// Groups that appear once still get a one-element slice.
//
// Only the first match contributes values. To collect every value of a single
// named group across every match in the target, use [FindAllNamed]. To
// collect every named group across every match as one map per match, use
// [NamedGroupsPerMatch] (or [NamedGroupsPerMatchSeq] for the lazy form); the
// unmarshal path ([UnmarshalAll], [Decoder.All], [Decoder.Iter]) is the typed
// equivalent.
//
// On no match, returns an empty (non-nil) map. See the package doc's
// "No-match behavior" section for the full cross-API contract.
//
// Example — duplicate group names (the use case this function exists for):
//
//	re := regexp.MustCompile(`(?P<word>\w+) (?P<word>\w+)`)
//	occurrences := regextra.NamedGroupOccurrences(re, "hello world")
//	// occurrences = map[string][]string{"word": []string{"hello", "world"}}
//
// Example — distinct group names (each slice has one element):
//
//	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
//	occurrences := regextra.NamedGroupOccurrences(re, "Alice 30")
//	// occurrences = map[string][]string{"name": []string{"Alice"}, "age": []string{"30"}}
func NamedGroupOccurrences(re *regexp.Regexp, target string) map[string][]string {
	names := re.SubexpNames()
	// No map size hint: len(names) counts slots, not distinct names, so it
	// over-allocates when a pattern reuses one name across many occurrences.
	result := make(map[string][]string)

	// Index form returns only the match offsets ([]int); we slice each
	// participating group out of target directly. FindStringSubmatch would
	// additionally allocate a []string of every group just to read it once.
	loc := re.FindStringSubmatchIndex(target)
	if loc == nil {
		return result
	}

	for i, name := range names {
		if i == 0 || name == "" {
			continue
		}
		// A non-participating group has start < 0; preserve
		// FindStringSubmatch's behavior of contributing "" for it.
		value := ""
		if start := loc[2*i]; start >= 0 {
			value = target[start:loc[2*i+1]]
		}
		result[name] = append(result[name], value)
	}

	return result
}

// AllNamedGroups returns every value of every named capture group in the
// first match, keyed by group name.
//
// Deprecated: Use [NamedGroupOccurrences] instead; identical behavior. The
// name survives as an alias because its "All" prefix reads as "all matches"
// when it actually means "all occurrences within one match"; removal is
// parked for a hypothetical v3 (issue #206).
func AllNamedGroups(re *regexp.Regexp, target string) map[string][]string {
	return NamedGroupOccurrences(re, target)
}
