

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

**Supported field types:** `string`, `int`, `int8`, `int16`, `int32`, `int64`, `uint`, `uint8`, `uint16`, `uint32`, `uint64`, `float32`, `float64`, `bool`

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