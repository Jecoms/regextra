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
    name is reused inside the pattern (map of slices): [NamedGroupOccurrences]
  - Pull every named group across all matches (one map per match):
    [NamedGroupsPerMatch], or lazily [NamedGroupsPerMatchSeq]
  - Substitute named-group spans by name: [Replace]
  - Substitute named-group spans in the first match only: [ReplaceFirst]
  - Substitute named-group spans with a callback over the matched value: [ReplaceFunc]
  - Substitute named-group spans with a callback, first match only: [ReplaceFuncFirst]
  - Assert at startup that required groups are declared: [Validate]
  - Decode one match into a struct: [Unmarshal]
  - Decode all matches into a slice of structs: [UnmarshalAll]
  - Decode the same shape repeatedly with cached reflect work: [Compile], [MustCompile], [Decoder]
  - Stream matches lazily (Go 1.23+ range-over-func): [Decoder.Iter]
  - Render a struct back into a string by inverting the decoder's own compiled
    pattern (the typed inverse of [Decoder]): [Decoder.Encoder] (or
    [Decoder.MustEncoder] for package-level vars), [Encoder]
  - Plug in caller-defined types in the unmarshal path: [RegexUnmarshaler]
  - Plug in caller-defined types in the encode path: [RegexMarshaler]
  - Compare against the no-match sentinel: [ErrNoMatch]

# When the standard library already does it

regextra composes with stdlib regexp rather than wrapping it, so two
capabilities are deliberately not functions here:

Template-style replacement ($name expansion). Building a new string from a
replacement template with $name placeholders is [regexp.Regexp.Expand] /
[regexp.Regexp.ExpandString], or [regexp.Regexp.ReplaceAllString] with $name
references in the replacement. regextra's [Replace] family is the other
direction: it keeps the target string and substitutes the matched spans of
named groups in place, keyed by group name. Reach for stdlib when the
template is the output shape; reach for [Replace] when the input is the
output shape and only the captured spans change.

Streaming input (io.Reader, log tailing). Go's regexp has no streaming
submatch extraction — its reader-based forms ([regexp.Regexp.MatchReader],
[regexp.Regexp.FindReaderSubmatchIndex]) return indices only, not the
matched text — so any streaming decode API here would be a line scanner in
disguise. Compose one directly: read lines with a [bufio.Scanner] and decode
each with [Decoder.One], skipping non-matches via [ErrNoMatch] — see the
ExampleDecoder_One_scanner example. This streams the input; [Decoder.Iter]
is the other sense of streaming, lazily yielding results from a string
already in memory.

# Performance

Every decode path caches its field-mapping reflect work. [Unmarshal] /
[UnmarshalAll] build the per-field decode plan on first use of a (pattern,
struct type) pair and reuse it from an internal package-level cache on later
calls, so repeated free-function decode pays only a cache lookup on top of
the match itself. The cache lives for the process and is never evicted — the
same trade encoding/json makes for its field cache: entries are small (they
do not retain the compiled regexp), and real workloads use a bounded set of
patterns and types. [Compile] / [Decoder] remains the right tool for repeated decode
of the same shape (log parsers, request handlers, config readers): the
Decoder carries its own plan, so it skips even the cache lookup, and its
strict compile-time validation surfaces tag typos at startup rather than
never (the lenient free functions skip misbound fields silently).
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
	NamedGroups, NamedGroupOccurrences        empty map (initialized, not nil)
	AllNamedGroups (deprecated alias)         same as NamedGroupOccurrences
	NamedGroupsPerMatch                       []map[string]string{} (empty, not nil)
	NamedGroupsPerMatchSeq                    iterator yields zero times
	Replace                                   target returned unchanged
	ReplaceFirst                              target returned unchanged
	ReplaceFunc                               target returned unchanged (fn
	                                          never called)
	ReplaceFuncFirst                          target returned unchanged (fn
	                                          never called)
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

The grammar also recognizes two flag-style tokens (no `=`):

	required                  Decode fails with a *RequiredGroupError when
	                          the named group does not participate in the
	                          match or matches an empty span and no default=
	                          supplies a value. A default= satisfies the
	                          requirement, since it always yields a value.
	inline                    Embedded struct (or *struct) fields only, with
	                          no group name (`regex:",inline"`). Promotes the
	                          embedded struct's exported fields into the
	                          decode and encode plans as if declared on the
	                          outer struct, with encoding/json precedence: a
	                          shallower field bound to a group shadows a
	                          deeper promoted one; promoted fields tying at
	                          equal depth resolve to a sole explicitly
	                          tagged binding (json's tagged-beats-untagged
	                          tiebreak), and an unresolved tie is rejected
	                          by [Compile] and dropped by [Unmarshal].
	                          Without the flag an embedded field is not
	                          promoted. See [Unmarshal] for the full rules.

The two "empty" forms differ, matching the convention in encoding/json,
encoding/xml, and gopkg.in/yaml:

	regex:""    No tag. Fall back to the field's own name for matching.
	regex:"-"   Exclude the field entirely. It is never populated, even if a
	            declared group happens to share its name.

Only the bare `-` tag excludes. A leading `-` followed by options
(e.g. `regex:"-,default=x"`) parses `-` as the group name, which matches no
group since regexp group names are Go identifiers.

Two forward-compatibility rules in the tag parser are part of the
stability contract since v1:

  - Unknown key=value pairs are preserved, not rejected. The parser stores
    every key=value pair regardless of whether the key is currently
    recognized, so a future minor release can introduce additional option
    keys without a parser change. Adding a new option key is therefore not
    a breaking change. Callers must not rely on the parser rejecting
    unknown keys; pin a minor version range if you need a specific
    recognized set.

  - Lone tokens (no `=`) other than the recognized `required` and `inline`
    flags are silently ignored. Today, `regex:"name,foo"` is a no-op — the
    `foo` token is dropped, so the field resolves exactly as `regex:"name"`
    would. This slot is reserved for future flag-style options (`required`
    claimed the first one and `inline` the second; see the issue tracker at
    https://github.com/Jecoms/regextra/issues). A later minor release may start
    recognizing further lone tokens and giving them meaning, so adding
    `regex:"name,foo"` today is a no-op but may stop being one. Callers must not
    rely on an unrecognized lone token remaining inert.

The two rules together are how the tag grammar grows compatibly: a new
option ships as either an additional key=value pair (orthogonal to today's
grammar) or a recognized flag token (claiming a previously-ignored slot).

# Stability

regextra is at v2 and follows strict SemVer. Patch releases are fixes only.
Minor releases add features without breaking changes. Breaking changes ship
in the next major version, never in a minor or patch. See the README's
Stability section for the precise contract, including what does and
does not count as breaking.

# More

This package documentation is the canonical reference for per-symbol
contracts — each exported symbol's doc comment states the behavior a caller
may rely on. The repository README is a front page (installation, a quick
usage tour, an API table linking here, and the stability policy), not a
second copy of the reference.
*/
package regextra
