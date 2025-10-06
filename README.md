

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

### `Unmarshal(re *regexp.Regexp, target string, v interface{}) error`

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