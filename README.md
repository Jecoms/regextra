

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

`regextra` is pre-v1 and follows SemVer. The exact rules:

**Pre-v1 (current)**

- Patch releases (`v0.x.y`) are fixes only — never breaking.
- Minor releases (`v0.x.0`) may add features and may include breaking changes. Breaking changes are called out in the [CHANGELOG](./CHANGELOG.md).
- The path to v1.0 (and what "v1" commits to) is in [ROADMAP.md](./ROADMAP.md).

**Post-v1 (future)**

- Strict SemVer. Breaking changes ship in the next major version, never in a minor or patch.

**What counts as breaking**

- Removing or renaming an exported symbol.
- Changing the signature of an exported function or method.
- Changing observable behavior of an existing call (e.g. a previously-returning call now returns an error).

**What does *not* count as breaking**

- Adding a new exported function, type, or method.
- Adding a new option to the `regex:"..."` struct tag grammar.
- Accepting additional field types in `Unmarshal` / `UnmarshalAll` / `Decoder`.
- Changing the wording of error messages. Do not pattern-match on `err.Error()` strings; compare against the exported sentinels (e.g. `regextra.ErrNoMatch` with `errors.Is`) instead.

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

For a single match, prefer `FindNamed`. To collect every named group's values across all matches, use `AllNamedGroups`.

### `NamedGroups(re *regexp.Regexp, target string) map[string]string`

Extract all named capture groups as a map. If a group name appears multiple times, only the last match is returned.

Returns an empty map if no match is found.

```go
re := regexp.MustCompile(`(?P<year>\d{4})-(?P<month>\d{2})-(?P<day>\d{2})`)
groups := regextra.NamedGroups(re, "Date: 2025-10-04")
// groups = map[string]string{"year": "2025", "month": "10", "day": "04"}
```

### `AllNamedGroups(re *regexp.Regexp, target string) map[string][]string`

Extract all values for each named capture group, handling duplicate group names within a single match.

Returns an empty map if no match is found.

```go
re := regexp.MustCompile(`(?P<word>\w+) (?P<word>\w+) (?P<word>\w+)`)
allGroups := regextra.AllNamedGroups(re, "one two three")
// allGroups = map[string][]string{"word": []string{"one", "two", "three"}}
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

### `Validate(re *regexp.Regexp, required ...string) error`

Returns an error listing every required group name that is not declared on `re`. Use it for init-time assertions in services that compile patterns once: catch typos at startup rather than at the first (mis-)matched request.

```go
re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)

if err := regextra.Validate(re, "name", "age", "ssn"); err != nil {
    // err: regextra.Validate: missing named groups: ssn
}
```

### `Unmarshal(re *regexp.Regexp, target string, v any) error`

Unmarshal regex matches into a struct with automatic type conversion. Similar to `json.Unmarshal`, but for regex patterns.

**Supported field types:** `string`, `int`, `int8`, `int16`, `int32`, `int64`, `uint`, `uint8`, `uint16`, `uint32`, `uint64`, `float32`, `float64`, `bool`, `time.Time`, `time.Duration`. Pointer-to-any-of-the-above is also supported — nil pointers are allocated, non-nil pointers are reused (pointee overwritten). For `time.Time`, several common layouts are tried (RFC3339, RFC3339Nano, `2006-01-02 15:04:05`, `2006-01-02`, `15:04:05`); `time.Duration` is parsed via `time.ParseDuration`. For caller-defined types, implement [`RegexUnmarshaler`](#regexunmarshaler-interface).

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

```go
type LogLine struct {
    TS    time.Time `regex:"ts,layout=02/Jan/2006:15:04:05 -0700"`
    Level string    `regex:"level,default=info"`
}
```

### `RegexUnmarshaler` interface

Mirror of `encoding.TextUnmarshaler` for regextra's unmarshal path. When a destination field's pointer type satisfies this interface, `Unmarshal` (and `UnmarshalAll`) call `UnmarshalRegex` with the matched group value instead of running the built-in string/int/uint/float/bool conversion.

```go
type RegexUnmarshaler interface {
    UnmarshalRegex(value string) error
}
```

The extension point for caller-defined types the built-in type switch can't handle (URLs, enums, big numbers, IP addresses, etc.).

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

`Decoder.One` returns `regextra.ErrNoMatch` (compare with `errors.Is`) when there's no match. Other errors indicate per-field conversion failure on a successful match.

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

`Decoder` instances are safe for concurrent use.

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
- ✅ **Safe by default** - Built-in nil checks, returns empty values on no match
- ✅ **Zero dependencies** - Only depends on Go's standard library

## License

MIT