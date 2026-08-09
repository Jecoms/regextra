package regextra

import (
	"fmt"
	"regexp"
)

// Validate returns an error listing every required group name that is not
// declared on re. Useful for init-time assertions in services that compile
// patterns once: catch typos at startup rather than at the first
// (mis-)matched request.
//
// Returns nil when every required name is declared. On failure it returns an
// [errors.As]-able *[MissingNamedGroupsError] whose Missing field lists the missing
// names in the order they were passed.
//
// Example:
//
//	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
//	if err := regextra.Validate(re, "name", "age", "missing"); err != nil {
//	    // err: regextra.Validate: missing named groups: missing
//	}
func Validate(re *regexp.Regexp, required ...string) error {
	names := re.SubexpNames()
	declared := make(map[string]struct{}, len(names))
	for _, n := range names {
		if n != "" {
			declared[n] = struct{}{}
		}
	}
	var missing []string
	for _, name := range required {
		if _, ok := declared[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("regextra.Validate: %w", &MissingNamedGroupsError{Missing: missing})
}
