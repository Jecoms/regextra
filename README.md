# regextra

[![Go Reference](https://pkg.go.dev/badge/github.com/jecoms/regextra/v2.svg)](https://pkg.go.dev/github.com/jecoms/regextra/v2)
[![Tests](https://github.com/jecoms/regextra/actions/workflows/test.yml/badge.svg)](https://github.com/jecoms/regextra/actions/workflows/test.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Extensions to Go's regexp package for easier handling of named capture groups:
name-based extraction, `json.Unmarshal`-style decoding into structs, a typed
compiled decoder for hot paths, and a derived encoder that renders structs back
into strings. The simple functions work directly with `*regexp.Regexp` — no
wrapper types required — and there are no dependencies outside the standard
library.

The full per-symbol reference lives in the
[package documentation on pkg.go.dev](https://pkg.go.dev/github.com/jecoms/regextra/v2);
the table below links each symbol to it.

## Installation

```bash
go get github.com/jecoms/regextra/v2@latest
```

## Usage

```go
package main

import (
    "fmt"
    "regexp"
    "github.com/jecoms/regextra/v2"
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

| Symbol | What it does |
|---|---|
| [`FindNamed`](https://pkg.go.dev/github.com/jecoms/regextra/v2#FindNamed) | Pull one named group from the first match |
| [`FindAllNamed`](https://pkg.go.dev/github.com/jecoms/regextra/v2#FindAllNamed) | Pull one named group across all matches |
| [`NamedGroups`](https://pkg.go.dev/github.com/jecoms/regextra/v2#NamedGroups) | All named groups of one match, as a map |
| [`NamedGroupOccurrences`](https://pkg.go.dev/github.com/jecoms/regextra/v2#NamedGroupOccurrences) | Every value of every named group in the first match — one slice element per occurrence of a reused name |
| [`AllNamedGroups`](https://pkg.go.dev/github.com/jecoms/regextra/v2#AllNamedGroups) | Deprecated alias of `NamedGroupOccurrences` |
| [`NamedGroupsPerMatch`](https://pkg.go.dev/github.com/jecoms/regextra/v2#NamedGroupsPerMatch) | One named-group map per match, across all matches |
| [`NamedGroupsPerMatchSeq`](https://pkg.go.dev/github.com/jecoms/regextra/v2#NamedGroupsPerMatchSeq) | Lazy (Go 1.23+ range-over-func) form of `NamedGroupsPerMatch` |
| [`Replace`](https://pkg.go.dev/github.com/jecoms/regextra/v2#Replace) | Substitute named-group spans by name, in every match |
| [`ReplaceFirst`](https://pkg.go.dev/github.com/jecoms/regextra/v2#ReplaceFirst) | Substitute named-group spans in the first match only |
| [`ReplaceFunc`](https://pkg.go.dev/github.com/jecoms/regextra/v2#ReplaceFunc) | Substitute named-group spans via a callback over the matched value |
| [`ReplaceFuncFirst`](https://pkg.go.dev/github.com/jecoms/regextra/v2#ReplaceFuncFirst) | Substitute named-group spans via a callback, in the first match only |
| [`Validate`](https://pkg.go.dev/github.com/jecoms/regextra/v2#Validate) | Assert at startup that required group names are declared on a pattern |
| [`Unmarshal`](https://pkg.go.dev/github.com/jecoms/regextra/v2#Unmarshal) | Decode the first match into a struct, with type conversion and `regex:"..."` tags |
| [`UnmarshalAll`](https://pkg.go.dev/github.com/jecoms/regextra/v2#UnmarshalAll) | Decode every match into a slice of structs |
| [`Compile`](https://pkg.go.dev/github.com/jecoms/regextra/v2#Compile) / [`MustCompile`](https://pkg.go.dev/github.com/jecoms/regextra/v2#MustCompile) | Build a [`Decoder[T]`](https://pkg.go.dev/github.com/jecoms/regextra/v2#Decoder) with strict upfront validation — compile once, decode many times |
| [`Decoder.One`](https://pkg.go.dev/github.com/jecoms/regextra/v2#Decoder.One) / [`Decoder.All`](https://pkg.go.dev/github.com/jecoms/regextra/v2#Decoder.All) / [`Decoder.Iter`](https://pkg.go.dev/github.com/jecoms/regextra/v2#Decoder.Iter) | Typed decode of the first match / every match / a lazy match stream |
| [`Decoder.Pattern`](https://pkg.go.dev/github.com/jecoms/regextra/v2#Decoder.Pattern) / [`Decoder.Regexp`](https://pkg.go.dev/github.com/jecoms/regextra/v2#Decoder.Regexp) | Accessors for the decoder's pattern source and its compiled `*regexp.Regexp` |
| [`Decoder.Encoder`](https://pkg.go.dev/github.com/jecoms/regextra/v2#Decoder.Encoder) / [`Decoder.MustEncoder`](https://pkg.go.dev/github.com/jecoms/regextra/v2#Decoder.MustEncoder) / [`Encoder.Encode`](https://pkg.go.dev/github.com/jecoms/regextra/v2#Encoder.Encode) / [`Encoder.EncodeStrict`](https://pkg.go.dev/github.com/jecoms/regextra/v2#Encoder.EncodeStrict) | Derive an [`Encoder[T]`](https://pkg.go.dev/github.com/jecoms/regextra/v2#Encoder) — the typed inverse built by inverting the decoder's own pattern (`MustEncoder` panics, for package-level vars) — and render a struct back into a string (`EncodeStrict` additionally re-matches each value against its group's sub-pattern) |
| [`RegexUnmarshaler`](https://pkg.go.dev/github.com/jecoms/regextra/v2#RegexUnmarshaler) / [`RegexMarshaler`](https://pkg.go.dev/github.com/jecoms/regextra/v2#RegexMarshaler) | Extension points for caller-defined types on the decode / encode side |
| [`ErrNoMatch`](https://pkg.go.dev/github.com/jecoms/regextra/v2#ErrNoMatch), [`ErrInvalidPattern`](https://pkg.go.dev/github.com/jecoms/regextra/v2#ErrInvalidPattern), [`ErrInvalidStruct`](https://pkg.go.dev/github.com/jecoms/regextra/v2#ErrInvalidStruct), [`ErrNotInvertible`](https://pkg.go.dev/github.com/jecoms/regextra/v2#ErrNotInvertible), [`ErrValueMismatch`](https://pkg.go.dev/github.com/jecoms/regextra/v2#ErrValueMismatch) | Sentinel errors — compare with `errors.Is` |
| [`DecodeError`](https://pkg.go.dev/github.com/jecoms/regextra/v2#DecodeError), [`EncodeError`](https://pkg.go.dev/github.com/jecoms/regextra/v2#EncodeError), [`RequiredGroupError`](https://pkg.go.dev/github.com/jecoms/regextra/v2#RequiredGroupError), [`MissingNamedGroupsError`](https://pkg.go.dev/github.com/jecoms/regextra/v2#MissingNamedGroupsError) | Typed errors — recover with `errors.As` |

## Stability

`regextra` is at v2 and follows strict SemVer. Import it at the `/v2` module path (`github.com/jecoms/regextra/v2`); the v1 line is frozen at the `v1.x` tags.

- Breaking changes ship in the next major version (`v3.0.0`), never in a minor or patch.
- Minor releases (`v2.x.0`) add features.
- Patch releases (`v2.x.y`) are fixes only.

v2.0.0 collects the behavior changes made since v1.0.0; see [CHANGELOG.md](./CHANGELOG.md) for the exhaustive list, with every breaking entry marked.

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

## License

MIT
