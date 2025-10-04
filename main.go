// Package regextra provides extensions to Go's regexp package for easier handling of named capture groups.
//
// The standard library's regexp package requires verbose code to extract named groups:
//
//	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
//	matches := re.FindStringSubmatch("Alice 30")
//	nameIndex := re.SubexpIndex("name")
//	name := matches[nameIndex]  // "Alice"
//
// This package simplifies named group extraction:
//
//	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
//	name, ok := regextra.FindNamed(re, "Alice 30", "name")  // "Alice", true
//	groups := regextra.NamedGroups(re, "Alice 30")          // map[name:Alice age:30]
package regextra

import (
	"regexp"
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
