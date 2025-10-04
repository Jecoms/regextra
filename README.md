

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
    
    // Get all named groups from first match as a map
    groups := regextra.NamedGroups(re, "Alice 30")
    fmt.Println(groups) // Output: map[age:30 name:Alice]
    
    // Get all values for duplicate group names
    re2 := regexp.MustCompile(`(?P<word>\w+) (?P<word>\w+)`)
    allGroups := regextra.AllNamedGroups(re2, "hello world")
    fmt.Println(allGroups) // Output: map[word:[hello world]]
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

Extract all named capture groups from the first match as a map.

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
- ✅ **Safe by default** - Built-in nil checks, returns empty values on no match
- ✅ **Zero dependencies** - Only depends on Go's standard library

## License

MIT