package regextra

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
)

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
// For template-style replacement — building a new string from a template with
// $name placeholders, rather than substituting group spans in place — use the
// standard library's [regexp.Regexp.Expand] or [regexp.Regexp.ReplaceAllString];
// see the package doc's "When the standard library already does it" section.
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
	return replaceNamed(re, target, -1, func(name, _ string) (string, bool) {
		repl, ok := replacements[name]
		return repl, ok
	})
}

// ReplaceFirst is like [Replace] but substitutes named-group spans only within
// the first match of re in target; every later match, and all text outside the
// first match, passes through byte-for-byte unchanged. Use it when only the
// leading occurrence should be rewritten — e.g. redacting the first token on a
// line while leaving the rest intact.
//
// Within that first match it follows [Replace]'s rules exactly: a group absent
// from replacements passes through, non-participating groups are skipped, and on
// overlap the outermost span encountered first wins.
//
// On no match, returns target unchanged. See the package doc's "No-match
// behavior" section for the full cross-API contract.
//
// Example:
//
//	re := regexp.MustCompile(`(?P<user>\w+)@(?P<domain>[\w.]+)`)
//	out := regextra.ReplaceFirst(re, "alice@example.com bob@other.org", map[string]string{
//	    "domain": "redacted",
//	})
//	// out = "alice@redacted bob@other.org"
func ReplaceFirst(re *regexp.Regexp, target string, replacements map[string]string) string {
	if len(replacements) == 0 {
		return target
	}
	return replaceNamed(re, target, 1, func(name, _ string) (string, bool) {
		repl, ok := replacements[name]
		return repl, ok
	})
}

// ReplaceFunc substitutes the matched span of each named capture group in
// target with the string returned by fn, which is called with the group's name
// and the text it matched. Like [Replace] it operates on every match of re, in
// order — but the replacement is computed from the matched value rather than
// looked up in a static map. Use it for substitutions that depend on what
// matched: redaction (mask all but the last four digits of a card number),
// normalization (lowercase a captured host), and similar.
//
// fn runs once per substituted named span, left to right. To leave a group
// unchanged, return its match verbatim. This differs from [Replace], which
// passes a group through when it is absent from the map: every participating
// named group reaches fn, so fn itself decides what to keep. Passing a nil fn
// is a programmer error — it panics on the first match, mirroring the standard
// library's [regexp.Regexp.ReplaceAllStringFunc].
//
// When named groups overlap (nesting), the outermost named group whose span is
// encountered first wins; fn is not called for inner groups inside an
// already-substituted span — matching [Replace]'s overlap rule.
//
// On no match, returns target unchanged and never calls fn. See the package
// doc's "No-match behavior" section for the full cross-API contract.
//
// Example — mask all but the last four digits of a captured card number:
//
//	re := regexp.MustCompile(`(?P<card>\d{12,19})`)
//	out := regextra.ReplaceFunc(re, "card 4111111111111111 ok", func(group, match string) string {
//	    return strings.Repeat("*", len(match)-4) + match[len(match)-4:]
//	})
//	// out = "card ************1111 ok"
func ReplaceFunc(re *regexp.Regexp, target string, fn func(group, match string) string) string {
	return replaceNamed(re, target, -1, func(name, matched string) (string, bool) {
		return fn(name, matched), true
	})
}

// ReplaceFuncFirst is like [ReplaceFunc] but substitutes named-group spans only
// within the first match of re in target; every later match, and all text
// outside the first match, passes through byte-for-byte unchanged. Use it when
// only the leading occurrence should be rewritten from its matched value —
// e.g. masking the first card number on a line while leaving the rest intact.
//
// Within that first match it follows [ReplaceFunc]'s rules exactly: fn runs
// once per substituted named span, left to right; to leave a group unchanged,
// return its match verbatim; on overlap (nesting) the outermost span
// encountered first wins, and fn is not called for inner groups inside an
// already-substituted span. Passing a nil fn is a programmer error — it panics
// on the first match, mirroring the standard library's
// [regexp.Regexp.ReplaceAllStringFunc].
//
// On no match, returns target unchanged and never calls fn. See the package
// doc's "No-match behavior" section for the full cross-API contract.
//
// Example — mask only the first captured card number:
//
//	re := regexp.MustCompile(`(?P<card>\d{12,19})`)
//	out := regextra.ReplaceFuncFirst(re, "4111111111111111 then 4242424242424242", func(group, match string) string {
//	    return strings.Repeat("*", len(match)-4) + match[len(match)-4:]
//	})
//	// out = "************1111 then 4242424242424242"
func ReplaceFuncFirst(re *regexp.Regexp, target string, fn func(group, match string) string) string {
	return replaceNamed(re, target, 1, func(name, matched string) (string, bool) {
		return fn(name, matched), true
	})
}

// replaceNamed is the shared substitution engine behind [Replace],
// [ReplaceFirst], [ReplaceFunc] and [ReplaceFuncFirst]. It walks up to limit
// matches of re over target and, for each participating named group, asks
// replFor for the replacement; replFor reports false to pass the group's
// matched text through unchanged. limit is passed straight to
// FindAllStringSubmatchIndex: -1 processes every match
// ([Replace]/[ReplaceFunc]), 1 processes only the first
// ([ReplaceFirst]/[ReplaceFuncFirst]). Text after the last processed match is
// copied through verbatim by the trailing cursor write, so a capped limit
// leaves the remainder untouched for free.
//
// replFor is resolved at apply time and only for spans that actually win the
// overlap rule, so a group skipped by an enclosing outermost span never reaches
// it — [ReplaceFunc] therefore never invokes its callback for an inner group it
// suppresses, and [Replace] does its map lookup only where it matters.
func replaceNamed(re *regexp.Regexp, target string, limit int, replFor func(name, matched string) (string, bool)) string {
	matches := re.FindAllStringSubmatchIndex(target, limit)
	if len(matches) == 0 {
		return target
	}
	names := re.SubexpNames()

	type span struct {
		start, end int
		name       string
	}

	var b strings.Builder
	spans := make([]span, 0, len(names)) // reused across matches
	cursor := 0
	for _, m := range matches {
		spans = spans[:0]
		for i := 1; i < len(names); i++ {
			name := names[i]
			if name == "" {
				continue
			}
			s, e := m[2*i], m[2*i+1]
			if s < 0 || e < 0 {
				continue
			}
			spans = append(spans, span{start: s, end: e, name: name})
		}
		// Sort by start ascending, then end descending: when named groups share
		// a start offset (nested groups), the outermost span — the one with the
		// larger end — sorts first, claims the cursor, and the inner spans it
		// encloses are skipped, making the documented "outermost wins" tie-break
		// deterministic. The stable sort preserves declaration order for spans
		// that are otherwise equal. (Fixes #107: the previous sort.Slice was
		// unstable, so the tie-break was not guaranteed.)
		slices.SortStableFunc(spans, func(a, c span) int {
			if a.start != c.start {
				return cmp.Compare(a.start, c.start)
			}
			return cmp.Compare(c.end, a.end)
		})
		for _, sp := range spans {
			if sp.start < cursor {
				continue // already covered (overlap with an earlier substitution)
			}
			repl, ok := replFor(sp.name, target[sp.start:sp.end])
			if !ok {
				continue // group passes through unchanged; cursor not advanced
			}
			b.WriteString(target[cursor:sp.start])
			b.WriteString(repl)
			cursor = sp.end
		}
	}
	b.WriteString(target[cursor:])
	return b.String()
}
