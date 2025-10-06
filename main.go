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
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
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

// Unmarshal extracts named capture groups from the target string and assigns them
// to the corresponding fields in the provided struct pointer.
//
// Field mapping rules:
//   - First checks the `regex:"groupname"` struct tag if provided (highest priority)
//   - Falls back to exact field name match with capture group name
//   - Falls back to case-insensitive field name match
//   - Supports type conversion for int, int64, float64, and bool
//   - Unexported fields are ignored
//
// Returns an error if:
//   - v is not a pointer to a struct
//   - The pattern does not match the target string
//   - Type conversion fails
//
// Example:
//
//	type Person struct {
//	    Name string
//	    Age  int `regex:"age"`
//	}
//	re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
//	var person Person
//	err := regextra.Unmarshal(re, "Alice is 30", &person)
//	// person.Name = "Alice", person.Age = 30
func Unmarshal(re *regexp.Regexp, target string, v any) error {
	// Get reflection value and validate it's a pointer to struct
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("regextra: Unmarshal requires a non-nil pointer to a struct, got %T", v)
	}

	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		return fmt.Errorf("regextra: Unmarshal requires a pointer to a struct, got pointer to %s", elem.Kind())
	}

	// Find the match
	matches := re.FindStringSubmatch(target)
	if matches == nil {
		return nil // No match, but not an error
	}

	// Build a map of capture group names to their values
	groupValues := make(map[string]string)
	for i, name := range re.SubexpNames() {
		if i != 0 && name != "" {
			groupValues[name] = matches[i]
		}
	}

	// Populate the struct fields
	return populateStruct(elem, groupValues)
}

// UnmarshalAll extracts all occurrences of the regex pattern from the target string
// and unmarshals them into a slice of structs. The slice is cleared before populating.
//
// v must be a pointer to a slice of structs. If no matches are found, the slice will
// be empty (length 0).
//
// Example:
//
//	type Person struct {
//	    Name string
//	    Age  int
//	}
//	re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
//	var people []Person
//	err := regextra.UnmarshalAll(re, "Alice is 30 and Bob is 25", &people)
//	// people = []Person{{Name: "Alice", Age: 30}, {Name: "Bob", Age: 25}}
func UnmarshalAll(re *regexp.Regexp, target string, v any) error {
	// Get reflection value and validate it's a pointer to slice
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("regextra: UnmarshalAll requires a non-nil pointer to a slice, got %T", v)
	}

	elem := rv.Elem()
	if elem.Kind() != reflect.Slice {
		return fmt.Errorf("regextra: UnmarshalAll requires a pointer to a slice, got pointer to %s", elem.Kind())
	}

	// Get the slice element type and verify it's a struct
	sliceElemType := elem.Type().Elem()
	if sliceElemType.Kind() != reflect.Struct {
		return fmt.Errorf("regextra: UnmarshalAll requires a slice of structs, got slice of %s", sliceElemType.Kind())
	}

	// Find all matches
	allMatches := re.FindAllStringSubmatch(target, -1)
	if len(allMatches) == 0 {
		// Clear the slice and return (no matches is not an error)
		elem.Set(reflect.MakeSlice(elem.Type(), 0, 0))
		return nil
	}

	// Create a new slice with capacity for all matches
	newSlice := reflect.MakeSlice(elem.Type(), 0, len(allMatches))

	// Process each match
	for _, matches := range allMatches {
		// Build a map of capture group names to their values for this match
		groupValues := make(map[string]string)
		for i, name := range re.SubexpNames() {
			if i != 0 && name != "" {
				groupValues[name] = matches[i]
			}
		}

		// Create a new struct instance
		structValue := reflect.New(sliceElemType).Elem()

		// Populate the struct fields
		if err := populateStruct(structValue, groupValues); err != nil {
			return err
		}

		// Append to the slice
		newSlice = reflect.Append(newSlice, structValue)
	}

	// Set the slice to the new value
	elem.Set(newSlice)
	return nil
}

// populateStruct fills a struct's fields from a map of capture group values
func populateStruct(structValue reflect.Value, groupValues map[string]string) error {
	structType := structValue.Type()
	for i := 0; i < structValue.NumField(); i++ {
		field := structValue.Field(i)
		fieldType := structType.Field(i)

		// Skip unexported fields
		if !field.CanSet() {
			continue
		}

		// Determine the capture group name for this field
		groupName := getGroupName(fieldType)

		// Try to find the value for this field
		value, found := findGroupValue(groupName, fieldType.Name, groupValues)
		if !found {
			continue
		}

		// Set the field value with type conversion
		if err := setFieldValue(field, value); err != nil {
			return fmt.Errorf("regextra: failed to set field %s: %w", fieldType.Name, err)
		}
	}

	return nil
}

// getGroupName extracts the group name from the struct tag, or returns empty string
func getGroupName(field reflect.StructField) string {
	tag := field.Tag.Get("regex")
	if tag == "" || tag == "-" {
		return ""
	}
	return tag
}

// findGroupValue searches for the value in the group values map
// Priority order: explicit tag > exact field name > case-insensitive field name
func findGroupValue(tagName, fieldName string, groupValues map[string]string) (string, bool) {
	// If there's an explicit tag, use it (highest priority)
	if tagName != "" {
		value, found := groupValues[tagName]
		return value, found
	}

	// Try exact field name match
	if value, found := groupValues[fieldName]; found {
		return value, true
	}

	// Try case-insensitive match
	lowerFieldName := strings.ToLower(fieldName)
	for groupName, value := range groupValues {
		if strings.ToLower(groupName) == lowerFieldName {
			return value, true
		}
	}

	return "", false
}

// setFieldValue sets the field value with appropriate type conversion
func setFieldValue(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
		return nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("cannot convert %q to int: %w", value, err)
		}
		field.SetInt(intVal)
		return nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uintVal, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return fmt.Errorf("cannot convert %q to uint: %w", value, err)
		}
		field.SetUint(uintVal)
		return nil

	case reflect.Float32, reflect.Float64:
		floatVal, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("cannot convert %q to float: %w", value, err)
		}
		field.SetFloat(floatVal)
		return nil

	case reflect.Bool:
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("cannot convert %q to bool: %w", value, err)
		}
		field.SetBool(boolVal)
		return nil

	default:
		return fmt.Errorf("unsupported field type: %s", field.Kind())
	}
}
