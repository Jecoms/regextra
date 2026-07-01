

# regextra

[![Go Reference](https://pkg.go.dev/badge/github.com/jecoms/regextra.svg)](https://pkg.go.dev/github.com/jecoms/regextra)
[![Go Report Card](https://goreportcard.com/badge/github.com/jecoms/regextra)](https://goreportcard.com/report/github.com/jecoms/regextra)
[![Tests](https://github.com/jecoms/regextra/actions/workflows/test.yml/badge.svg)](https://github.com/jecoms/regextra/actions/workflows/test.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Extensions to Go's regexp package for easier handling of named capture groups.

## Installation

```bash
go get github.com/jecoms/regextra@latest
```

## Stability

`regextra` is at v1 and follows strict SemVer:

- Breaking changes ship in the next major version (`v2.0.0`), never in a minor or patch.
- Minor releases (`v1.x.0`) add features.
- Patch releases (`v1.x.y`) are fixes only.

The forward look is tracked in the [issue tracker](https://github.com/Jecoms/regextra/issues).

**What counts as breaking**

- Removing or renaming an exported symbol.
- Changing the signature of an exported function or method.
- Changing observable behavior of an existing call (e.g. a previously-returning call now returns an error).

**What does *not* count as breaking**

- Adding a new exported function, type, or method.
- Adding a new option to the `regex:"..."` struct tag grammar.
- Accepting additional field types in `Unmarshal` / `UnmarshalAll` / `Decoder`.
- Changing the wording of error messages. Do not pattern-match on `err.Error()` strings; compare against the exported sentinels (e.g. `regextra.ErrNoMatch`, `regextra.ErrInvalidPattern`, `regextra.ErrInvalidStruct` with `errors.Is`) or recover the typed `*regextra.DecodeError` with `errors.As` instead.

## Usage

```go
package main

import (
    "fmt"
    "regexp"
    "github.com/jecoms/regextra"
)

func main() {
    re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
    
    // Extract a single named group
    name, ok := regextra.FindNamed(re, "Alice 30", "name")
    if ok {
        fmt.Println("Name:", name) // Output: Name: Alice
    }
    
    // Get all named groups as a map
    groups := regextra.NamedGroups(re, "Alice 30")
    fmt.Println(groups) // Output: map[age:30 name:Alice]
    
    // Unmarshal into a struct with type conversion
    type Person struct {
        Name string
        Age  int
    }
    var person Person
    regextra.Unmarshal(re, "Bob 25", &person)
    fmt.Printf("%s is %d\n", person.Name, person.Age) // Output: Bob is 25
    
    // Unmarshal all matches into a slice
    var people []Person
    regextra.UnmarshalAll(re, "Alice 30 and Bob 25", &people)
    fmt.Println(len(people)) // Output: 2
}
```

## API

### `FindNamed(re *regexp.Regexp, target, groupName string) (string, bool)`

Extract a single named capture group from the target string.

Returns the matched value and `true` if found, or empty string and `false` if not found.

```go
re := regexp.MustCompile(`(?P<price>\$\d+\.\d{2})`)
price, ok := regextra.FindNamed(re, "Total: $19.99", "price")
// price = "$19.99", ok = true
```

### `FindAllNamed(re *regexp.Regexp, target, groupName string) []string`

Extract every value of a single named capture group across all matches.

Returns `nil` if the group name is not declared on the regex; an empty slice if the group is declared but the regex has no matches.

```go
re := regexp.MustCompile(`(?P<word>\S+)`)
words := regextra.FindAllNamed(re, "alpha beta gamma", "word")
// words = []string{"alpha", "beta", "gamma"}
```

For a single match, prefer `FindNamed`. To pull every named group from one match — including patterns where the same name is declared more than once — use `AllNamedGroups`. Despite the "All" prefix, `AllNamedGroups` operates on a single match; it is not the all-matches counterpart of `FindAllNamed`.

### `NamedGroups(re *regexp.Regexp, target string) map[string]string`

Extract all named capture groups as a map. If a group name appears multiple times, only the last match is returned.

Returns an empty map if no match is found.

```go
re := regexp.MustCompile(`(?P<year>\d{4})-(?P<month>\d{2})-(?P<day>\d{2})`)
groups := regextra.NamedGroups(re, "Date: 2025-10-04")
// groups = map[string]string{"year": "2025", "month": "10", "day": "04"}
```

### `AllNamedGroups(re *regexp.Regexp, target string) map[string][]string`

Operates on a **single match** and returns every value of every named capture group, keyed by group name. Each value is a slice because Go's `regexp` allows the same group name to appear more than once in a pattern — `AllNamedGroups` preserves every occurrence in left-to-right order. Groups that appear once still get a one-element slice.

The leading "All" refers to **all named groups in one match** — not to all matches across the target. Use `FindAllNamed` to collect a single named group across every match, or `NamedGroupsPerMatch` to collect every named group across every match as one map per match (`[]map[string]string`); the unmarshal path (`UnmarshalAll`, `Decoder.All`, `Decoder.Iter`) is the typed equivalent.

Returns an empty map if no match is found.

```go
// Duplicate group names — the use case this function exists for:
re := regexp.MustCompile(`(?P<word>\w+) (?P<word>\w+) (?P<word>\w+)`)
allGroups := regextra.AllNamedGroups(re, "one two three")
// allGroups = map[string][]string{"word": []string{"one", "two", "three"}}

// Distinct group names — each slice has one element:
re = regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
allGroups = regextra.AllNamedGroups(re, "Alice 30")
// allGroups = map[string][]string{"name": []string{"Alice"}, "age": []string{"30"}}
```

### `NamedGroupsPerMatch(re *regexp.Regexp, target string) []map[string]string`

The every-match counterpart to `NamedGroups`: returns one map of named-group values per match of `re` in `target`, in match order. Each map follows the same per-match semantics as `NamedGroups` — every declared group is present, a group that did not participate in that match is mapped to `""`, and a reused group name resolves to the last participating occurrence in that match.

Returns an empty (non-nil) slice if there are no matches. For the typed equivalent that decodes each match into a struct, use `UnmarshalAll` / `Decoder.All`.

```go
re := regexp.MustCompile(`(?P<key>\w+)=(?P<value>\w+)`)
all := regextra.NamedGroupsPerMatch(re, "a=1 b=2")
// all = []map[string]string{{"key": "a", "value": "1"}, {"key": "b", "value": "2"}}
```

### `NamedGroupsPerMatchSeq(re *regexp.Regexp, target string) iter.Seq[map[string]string]`

The lazy, range-over-func (Go 1.23+) form of `NamedGroupsPerMatch`: yields one named-group map per match, in match order, without building the intermediate slice. Stopping the range early (`break`) stops the iteration. On no match, the iterator yields zero times. For the typed streaming equivalent, use `Decoder.Iter`.

```go
re := regexp.MustCompile(`(?P<key>\w+)=(?P<value>\w+)`)
for m := range regextra.NamedGroupsPerMatchSeq(re, "a=1 b=2") {
    fmt.Println(m["key"], m["value"])
}
// a 1
// b 2
```

### `Replace(re *regexp.Regexp, target string, replacements map[string]string) string`

Substitute the matched span of each named capture group with the value from `replacements`, leaving non-matching text and any groups absent from the map unchanged. `Replace` operates on every match of `re`, in order.

```go
re := regexp.MustCompile(`(?P<user>\w+)@(?P<domain>[\w.]+)`)
out := regextra.Replace(re, "alice@example.com bob@other.org", map[string]string{
    "domain": "redacted",
})
// out = "alice@redacted bob@redacted"
```

### `ReplaceFirst(re *regexp.Regexp, target string, replacements map[string]string) string`

Like `Replace`, but substitutes named-group spans only within the **first** match of `re`; every later match, and all text outside the first match, passes through byte-for-byte unchanged. Within that first match it follows `Replace`'s rules exactly (groups absent from the map pass through, non-participating groups are skipped, outermost span wins on overlap). On no match, the target is returned unchanged.

```go
re := regexp.MustCompile(`(?P<user>\w+)@(?P<domain>[\w.]+)`)
out := regextra.ReplaceFirst(re, "alice@example.com bob@other.org", map[string]string{
    "domain": "redacted",
})
// out = "alice@redacted bob@other.org"
```

### `ReplaceFunc(re *regexp.Regexp, target string, fn func(group, match string) string) string`

Like `Replace`, but the replacement for each named-group span is computed by a callback over the matched value instead of looked up in a static map. Use it when the substitution depends on what matched — redaction, normalization, and similar. `fn` is called once per substituted named span, left to right, with the group's name and matched text; return the match verbatim to leave a group unchanged. On no match the target is returned unchanged and `fn` is never called. Passing a nil `fn` is a programmer error and panics on the first match, mirroring the standard library's `Regexp.ReplaceAllStringFunc`.

When named groups overlap (nesting), the outermost group whose span is encountered first wins and `fn` is not called for inner groups inside an already-substituted span — the same overlap rule as `Replace`.

```go
// Mask all but the last four digits of a captured card number:
re := regexp.MustCompile(`(?P<card>\d{12,19})`)
out := regextra.ReplaceFunc(re, "card 4111111111111111 ok", func(group, match string) string {
    return strings.Repeat("*", len(match)-4) + match[len(match)-4:]
})
// out = "card ************1111 ok"
```

### `Validate(re *regexp.Regexp, required ...string) error`

Returns an error listing every required group name that is not declared on `re`. Use it for init-time assertions in services that compile patterns once: catch typos at startup rather than at the first (mis-)matched request.

```go
re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)

if err := regextra.Validate(re, "name", "age", "ssn"); err != nil {
    // err: regextra.Validate: missing named groups: ssn
}
```

On failure `Validate` returns an `errors.As`-able `*regextra.MissingNamedGroupsError` whose `Missing` field carries the absent group names (in the order passed), so you can branch on the missing set without parsing the message — see [§Stability](#stability) on comparing types, not strings:

```go
var ve *regextra.MissingNamedGroupsError
if errors.As(err, &ve) {
    // ve.Missing == []string{"ssn"}
}
```

### `Unmarshal(re *regexp.Regexp, target string, v any) error`

Unmarshal regex matches into a struct with automatic type conversion. Similar to `json.Unmarshal`, but for regex patterns.

**Supported field types:** `string`, `int`, `int8`, `int16`, `int32`, `int64`, `uint`, `uint8`, `uint16`, `uint32`, `uint64`, `float32`, `float64`, `bool`, `time.Time`, `time.Duration`. Pointer-to-any-of-the-above is also supported — nil pointers are allocated, non-nil pointers are reused (pointee overwritten). For `time.Time`, several common layouts are tried (RFC3339, RFC3339Nano, `2006-01-02 15:04:05`, `2006-01-02`, `15:04:05`); `time.Duration` is parsed via `time.ParseDuration`. Any field whose type (or pointer-to-type) implements [`encoding.TextUnmarshaler`](https://pkg.go.dev/encoding#TextUnmarshaler) is also supported out of the box — e.g. `netip.Addr`, `math/big.Int`, `log/slog.Level`, `github.com/google/uuid.UUID` — by calling its `UnmarshalText` with the matched value. For caller-defined types, implement [`RegexUnmarshaler`](#regexunmarshaler-interface).

**Field mapping priority:**
1. Struct tag `regex:"groupname"` if provided (highest priority)
2. Exact field name match with capture group name
3. Case-insensitive field name match
- Unexported fields are ignored

Returns an error if the target is not a pointer to a struct, or if type conversion fails.

```go
type Person struct {
    Name string
    Age  int
}

re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
var person Person
err := regextra.Unmarshal(re, "Alice is 30", &person)
// person.Name = "Alice", person.Age = 30
```

**With struct tags:**

```go
type Email struct {
    Username string `regex:"user"`
    Domain   string `regex:"domain"`
}

re := regexp.MustCompile(`(?P<user>\w+)@(?P<domain>[\w.]+)`)
var email Email
err := regextra.Unmarshal(re, "alice@example.com", &email)
// email.Username = "alice", email.Domain = "example.com"
```

**Tag options:**

The `regex:"..."` tag accepts comma-separated `key=value` options after the group name:

| Option | Applies to | Effect |
|---|---|---|
| `default=<value>` | Any field type | Substituted when the named group is not declared on the regex or its match is empty. The default goes through the same type conversion as a real match. |
| `layout=<go-time-layout>` | `time.Time` only | Use the supplied [time.Parse layout](https://pkg.go.dev/time#Parse) exclusively, instead of the default fallback list. Lets you pin the parser to (e.g.) Apache, syslog, or any other non-RFC3339 timestamp shape. |
| `required` *(flag)* | Any field type | Decode fails with an `errors.As`-able `*RequiredGroupError` when the named group does not participate in the match or matches an empty span and no `default=` supplies a value. A `default=` satisfies the requirement. Lets a field declare its mandatory-ness inline instead of a separate `Validate` pass. |

```go
type LogLine struct {
    TS    time.Time `regex:"ts,layout=02/Jan/2006:15:04:05 -0700"`
    Level string    `regex:"level,default=info"`
    User  string    `regex:"user,required"`
}
```

**Excluding a field:** `regex:"-"` excludes a field entirely — it is never populated, even if a declared group happens to share the field's name. This matches the `-` convention in `encoding/json`, `encoding/xml`, and `gopkg.in/yaml`. It differs from an absent tag (`regex:""`), which falls back to matching the field's own name against a group. Only the bare `-` excludes; a leading `-` followed by options (e.g. `regex:"-,default=x"`) parses `-` as the group name, which matches no group since regexp group names are Go identifiers.

**Forward-compat rules (v1 contract):**

- **Unknown `key=value` pairs are preserved, not rejected.** Adding a new option key in a future minor release is not a breaking change. Don't rely on the parser rejecting unknown keys — pin a minor version range if you need a specific recognized set.
- **Lone tokens (no `=`) other than the recognized `required` flag are silently ignored.** Today, `regex:"name,foo"` parses as `(name="name")` — the `foo` token is dropped. The slot is reserved for future flag-style options (`required` claimed the first one — see the options table above); a later minor may start recognizing further lone tokens. Don't rely on an unrecognized lone token remaining inert.

See the package doc's **Tag grammar** section on [pkg.go.dev](https://pkg.go.dev/github.com/jecoms/regextra) for the canonical statement.

### `RegexUnmarshaler` interface

Mirror of `encoding.TextUnmarshaler` for regextra's unmarshal path. When a destination field's pointer type satisfies this interface, `Unmarshal` (and `UnmarshalAll`) call `UnmarshalRegex` with the matched group value instead of running the built-in string/int/uint/float/bool conversion.

```go
type RegexUnmarshaler interface {
    UnmarshalRegex(value string) error
}
```

The extension point for caller-defined types the built-in type switch can't handle (URLs, enums, big numbers, IP addresses, etc.).

**Conversion precedence.** For each field, `Unmarshal` tries in order: (1) `RegexUnmarshaler` — the package-specific hook always wins; (2) the `time.Time` / `time.Duration` special-cases (so the multi-layout fallback and `layout` tag option are preserved — `time.Time` happens to implement `encoding.TextUnmarshaler` but its `UnmarshalText` only accepts RFC3339, so it is *not* routed through the fallback); (3) `encoding.TextUnmarshaler` — for any other type that implements it; (4) the built-in string/int/uint/float/bool conversion. So a type implementing both `RegexUnmarshaler` and `encoding.TextUnmarshaler` dispatches on `UnmarshalRegex`.

```go
type Status int

const (
    StatusUnknown Status = iota
    StatusOpen
    StatusClosed
)

func (s *Status) UnmarshalRegex(value string) error {
    switch value {
    case "open":   *s = StatusOpen
    case "closed": *s = StatusClosed
    default:       return fmt.Errorf("unknown status %q", value)
    }
    return nil
}

type Issue struct {
    ID    int    `regex:"id"`
    State Status `regex:"state"`
}

re := regexp.MustCompile(`#(?P<id>\d+) \[(?P<state>\w+)\]`)
var issue Issue
regextra.Unmarshal(re, "#42 [open]", &issue)
// issue = Issue{ID: 42, State: StatusOpen}
```

### `UnmarshalAll(re *regexp.Regexp, target string, v any) error`

UnmarshalAll finds all occurrences of the regex pattern in the target string and unmarshals them into a slice of structs. The slice is cleared before populating.

v must be a pointer to a slice of structs. If no matches are found, the slice will be empty.

**Supported field types:** Same as `Unmarshal`

**Field mapping priority:** Same as `Unmarshal`

```go
type Person struct {
    Name string
    Age  int
}

re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
var people []Person
err := regextra.UnmarshalAll(re, "Alice is 30 and Bob is 25", &people)
// people = []Person{
//     {Name: "Alice", Age: 30},
//     {Name: "Bob", Age: 25},
// }
```

### `Compile[T any](pattern string) (*Decoder[T], error)` / `MustCompile[T any](pattern string) *Decoder[T]`

Typed, regex-bound unmarshaler that caches the reflect plan for `T`'s fields. **Compile once, decode many times** — eliminates the per-call reflect work that `Unmarshal` does on every invocation.

```go
type Person struct {
    Name string `regex:"name"`
    Age  int    `regex:"age"`
}

// Compile validates pattern + struct tags upfront.
var personDecoder = regextra.MustCompile[Person](`(?P<name>\w+) is (?P<age>\d+)`)

// Hot path — no reflect per call.
p, err := personDecoder.One("Alice is 30")
// p = Person{Name: "Alice", Age: 30}, err = nil

people, _ := personDecoder.All("Alice is 30 and Bob is 25")
// people = []Person{{"Alice", 30}, {"Bob", 25}}
```

**vs `Unmarshal`:** the same simple-struct shape benchmarks at **~270 ns/op (3 allocs)** via `Decoder` versus **~500 ns/op (6 allocs)** via `Unmarshal` on Apple M4 — roughly half the time and half the allocations. Use `Decoder` when you'll decode the same shape many times (log parsers, config readers, request handlers); use `Unmarshal` for one-shot extraction.

**Compile-time validation is strict.** `Compile` returns an error (or `MustCompile` panics) if:
- The pattern is not a valid regex
- `T` is not a struct
- A field's `regex:"name"` tag references a group not declared on the pattern (unless paired with `default=`)
- A `default=` value cannot be converted to its field type
- A `layout=` option is on a non-`time.Time` field

This is the strictness you want for "compile once" — typos fail at startup, not at first request.

Each failure is categorized by a wrapped sentinel so you can branch on the kind with `errors.Is` instead of parsing the message: `regextra.ErrInvalidPattern` for the bad-regex case, and `regextra.ErrInvalidStruct` for the four destination-shape cases. `MustCompile` panics with the same wrapped error. The sentinels are `Compile`-only — the lenient `Unmarshal` / `UnmarshalAll` path never surfaces them.

```go
if _, err := regextra.Compile[Person](pattern); err != nil {
    switch {
    case errors.Is(err, regextra.ErrInvalidPattern):
        // bad regular expression
    case errors.Is(err, regextra.ErrInvalidStruct):
        // struct/tag shape problem
    }
}
```

`Decoder.One` returns `regextra.ErrNoMatch` (compare with `errors.Is`) when there's no match. Other errors indicate per-field conversion failure on a successful match.

**Typed conversion failures.** When a matched group value can't be converted to its field type, every decode entrypoint (`Unmarshal`, `UnmarshalAll`, `Decoder.One`/`All`/`Iter`) returns a `*regextra.DecodeError` carrying the field name, capture group, raw value, target type, and wrapped cause. Recover it with `errors.As` to branch without parsing message text:

```go
var de *regextra.DecodeError
if errors.As(err, &de) {
    log.Printf("field %s (group %s): cannot parse %q as %s", de.Field, de.Group, de.Value, de.Type)
}
```

**Required groups.** A field tagged `regex:",required"` (see the Tag options table under [`Unmarshal`](#unmarshalre-regexpregexp-target-string-v-any-error)) must receive a value: when its group does not participate in the match or matches an empty span and no `default=` supplies one, the same decode entrypoints return a `*regextra.RequiredGroupError` carrying the field name and capture group. It is the per-match presence check, complementing `*DecodeError` (a value that failed conversion) and `*MissingNamedGroupsError` (the static `Validate` check that the pattern declares a group at all):

```go
var rge *regextra.RequiredGroupError
if errors.As(err, &rge) {
    log.Printf("field %s (group %s) is required but had no value", rge.Field, rge.Group)
}
```

Constructed errors are prefixed with the entrypoint that produced them (`regextra.Unmarshal:`, `regextra.Decoder.One:`, …); the bare `regextra:` prefix is reserved for package-level sentinels like `ErrNoMatch`, `ErrInvalidPattern`, and `ErrInvalidStruct`. Treat these prefixes as informational — compare against the sentinel or the `*DecodeError` type, not the string.

**Streaming with `Decoder.Iter`:**

```go
// Pair each match with its decode error so callers can skip individual failures
// without aborting the whole iteration. Break freely to stop early.
for v, err := range personDecoder.Iter(input) {
    if err != nil {
        log.Printf("skipping bad match: %v", err)
        continue
    }
    process(v)
}
```

`Iter` returns an `iter.Seq2[T, error]` (Go 1.23+ range-over-func). Use it for streaming-style consumption (log parsers, scrapers) where you don't want to allocate the full slice up-front. **~37% faster and ~50% fewer allocations** than `UnmarshalAll` on a 100-line corpus, since Iter skips the slice allocation entirely. Match-finding still happens in one regex call (Go's stdlib doesn't expose a streaming-find API), but the per-match decode work IS lazy — `break` in the range body avoids decoding the remaining matches.

**Accessors.** A `Decoder` exposes the pattern it was compiled from, so you don't have to keep a copy alongside it:

- `Pattern() string` returns the regex source string — handy for logging and debugging.
- `Regexp() *regexp.Regexp` returns the underlying compiled `*regexp.Regexp`, so you can reuse it for your own match-finding (e.g. `FindAllIndex`, custom iteration) without recompiling. The returned pointer is shared with the `Decoder`: its exported methods are read-only and safe for concurrent use, but do not mutate shared matcher state on it — in particular don't call `Longest()`, which changes matching semantics for the `Decoder` too.

```go
dec := regextra.MustCompile[Person](`(?P<name>\w+) is (?P<age>\d+)`)
dec.Pattern()          // "(?P<name>\\w+) is (?P<age>\\d+)"
dec.Regexp().FindAllString("Alice is 30 and Bob is 25", -1)
// []string{"Alice is 30", "Bob is 25"}
```

`Decoder` instances are safe for concurrent use.

### `(d *Decoder[T]) Encoder() (*Encoder[T], error)`

The typed inverse of `Decoder`, **derived from the decoder's own compiled pattern** — write the pattern once and get the encoder for free, with no separate template to keep in sync. `Encode` followed by a `Decoder.One` / `Unmarshal` on the same pattern round-trips the original struct.

`Encoder()` parses the decoder's pattern with `regexp/syntax` and inverts the **invertible subset** of the grammar into an ordered encode plan:

- **Literal text** is emitted verbatim (regexp escapes like `\.` are already decoded by the parser).
- **Named capture groups** `(?P<name>…)` become field substitutions: `name` resolves to a struct field with the same rules `Decoder` uses (the field's `regex:"name"` tag if present, otherwise the field's own name, matched case-insensitively; a `regex:"-"` field is excluded). The group's sub-pattern is discarded — the field's value fills the span.
- **Anchors and zero-width assertions** (`^`, `$`, `\A`, `\z`, `\b`, …) match no text and are dropped.
- An **unnamed group** whose body is pure literal text is treated as that literal.

```go
type Person struct {
    Name string `regex:"name"`
    Age  int    `regex:"age"`
}

dec := regextra.MustCompile[Person](`(?P<name>\S+) is (?P<age>\d+)`)
enc, _ := dec.Encoder()

s, _ := enc.Encode(Person{Name: "Alice", Age: 30})   // "Alice is 30"
back, _ := dec.One(s)                                 // Person{Name: "Alice", Age: 30}
```

**Non-invertible patterns fail fast.** Any construct with no single string to emit — an alternation (`|`), a quantifier (`*`, `+`, `?`, `{n,m}`), a character class (`[...]`), an any-character wildcard (`.`), or an unnamed group with non-literal content — appearing **outside** a named capture group makes the pattern non-invertible, and `Encoder()` returns an error wrapping `regextra.ErrNotInvertible` that names the offending construct. (Inside a named capture such constructs are fine: the field's value fills the group.)

**Supported field types:** same set as `Unmarshal` — `string`, all int/uint/float widths, `bool`, `time.Time`, `time.Duration`, and single-level pointers to any of these. `time.Time` encodes as RFC3339Nano by default (the first layout `Decoder` tries, so the output re-parses and sub-second precision survives), or the `layout=` layout when tagged. Any type implementing [`encoding.TextMarshaler`](https://pkg.go.dev/encoding#TextMarshaler) (e.g. `netip.Addr`, `uuid.UUID`) is encoded via `MarshalText`. For caller-defined types, implement `RegexMarshaler` (below).

**Construction-time validation is strict**, mirroring `Compile`: `Encoder()` returns an error if the pattern is not invertible (above), a named group maps to no exported/eligible field, or a mapped field's type can't be encoded (the latter two wrap `regextra.ErrInvalidStruct`). A successful `Encoder()` can only fail at `Encode` time on a runtime value error (a custom marshaler returning an error, or a nil pointer field, which has no string form) — surfaced as a `*regextra.EncodeError` (the encode-side mirror of `DecodeError`).

**Round-trip contract.** `Encode(v)` re-decodes to `v` when each encoded value re-matches the sub-pattern of the group it fills — the caller owns that pairing by writing value-appropriate sub-patterns (a captured word wants `\S+`, not `.*`). The `default=` tag option does not affect encoding (it is a decode-side substitution); `Encode` always emits the field's actual value. Values that collide with a surrounding literal delimiter, or two adjacent captures with no literal between them, have no unambiguous decode boundary and are out of scope. (A future option is to re-match each encoded value against its group's sub-pattern at `Encode` time; that is deliberately not done today.)

`Encoder` instances are safe for concurrent use.

### `RegexMarshaler` interface

The encode-side mirror of `RegexUnmarshaler`. When an `Encoder` field's type satisfies this interface, `Encode` calls `MarshalRegex` instead of the built-in conversion. A type implementing both `RegexMarshaler` and `RegexUnmarshaler` round-trips symmetrically through `Encoder` and `Decoder`.

```go
type RegexMarshaler interface {
    MarshalRegex() (string, error)
}
```

**Conversion precedence** mirrors the decode side: (1) `RegexMarshaler`; (2) the `time.Time` / `time.Duration` special-cases; (3) `encoding.TextMarshaler`; (4) the built-in string/int/uint/float/bool conversion.

```go
func (s Status) MarshalRegex() (string, error) {
    switch s {
    case StatusOpen:   return "open", nil
    case StatusClosed: return "closed", nil
    default:           return "", fmt.Errorf("unknown status: %d", s)
    }
}
```

## Why regextra?

The standard library's `regexp` package requires verbose code to extract named capture groups:

```go
// Standard library approach (verbose)
re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
matches := re.FindStringSubmatch("Alice 30")
if matches != nil {
    nameIndex := re.SubexpIndex("name")
    name := matches[nameIndex]  // "Alice"
}

// regextra approach (simple)
re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
name, ok := regextra.FindNamed(re, "Alice 30", "name")  // "Alice", true
```

## Features

- ✅ **Simple functions** - No wrapper types, works directly with `*regexp.Regexp`
- ✅ **Named group extraction** - Extract groups by name without index juggling
- ✅ **Map-based access** - Get all named groups in one call
- ✅ **Struct unmarshaling** - Type-safe extraction with automatic conversion
- ✅ **Typed round-trip** - `Encoder[T]` renders a struct back to a string via a reversible template
- ✅ **Safe by default** - Built-in nil checks, returns empty values on no match
- ✅ **Zero dependencies** - Only depends on Go's standard library
- ✅ **Pay for what you use** - Unused functions are dead-code-eliminated at link time, so importing the package costs nothing for symbols you don't call

## License

MIT