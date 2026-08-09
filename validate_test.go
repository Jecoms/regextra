package regextra_test

import (
	"errors"
	"fmt"
	rx "github.com/jecoms/regextra/v2"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)

	t.Run("all declared returns nil", func(t *testing.T) {
		if err := rx.Validate(re, "name", "age"); err != nil {
			t.Errorf("Validate returned %v, want nil", err)
		}
	})

	t.Run("subset of declared returns nil", func(t *testing.T) {
		if err := rx.Validate(re, "name"); err != nil {
			t.Errorf("Validate returned %v, want nil", err)
		}
	})

	t.Run("empty required returns nil", func(t *testing.T) {
		if err := rx.Validate(re); err != nil {
			t.Errorf("Validate returned %v, want nil", err)
		}
	})

	t.Run("single missing reports it", func(t *testing.T) {
		err := rx.Validate(re, "name", "ssn")
		if err == nil {
			t.Fatal("Validate returned nil, want error")
		}
		want := "regextra.Validate: missing named groups: ssn"
		if err.Error() != want {
			t.Errorf("Validate error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("multiple missing preserves request order", func(t *testing.T) {
		err := rx.Validate(re, "ssn", "age", "email", "name", "phone")
		if err == nil {
			t.Fatal("Validate returned nil, want error")
		}
		want := "regextra.Validate: missing named groups: ssn, email, phone"
		if err.Error() != want {
			t.Errorf("Validate error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("regex with no named groups, all required missing", func(t *testing.T) {
		bare := regexp.MustCompile(`\w+`)
		err := rx.Validate(bare, "name")
		if err == nil {
			t.Fatal("Validate returned nil, want error")
		}
	})
}

// TestValidateMissingNamedGroupsError verifies that Validate surfaces an
// errors.As-able *MissingNamedGroupsError whose Missing field carries the absent
// required group names in the order they were passed, while keeping the
// prefixed message non-breaking.
func TestValidateMissingNamedGroupsError(t *testing.T) {
	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)

	t.Run("recovers typed error in request order", func(t *testing.T) {
		err := rx.Validate(re, "ssn", "age", "email", "name", "phone")
		if err == nil {
			t.Fatal("Validate returned nil, want error")
		}
		var ve *rx.MissingNamedGroupsError
		if !errors.As(err, &ve) {
			t.Fatalf("error %q is not a *MissingNamedGroupsError", err)
		}
		want := []string{"ssn", "email", "phone"}
		if !reflect.DeepEqual(ve.Missing, want) {
			t.Errorf("MissingNamedGroupsError.Missing = %v, want %v", ve.Missing, want)
		}
		// Message stays prefixed and non-breaking (loose match per §Stability).
		if !strings.HasPrefix(err.Error(), "regextra.Validate:") {
			t.Errorf("error %q lost the regextra.Validate: prefix", err.Error())
		}
		if !strings.Contains(err.Error(), "missing named groups: ssn, email, phone") {
			t.Errorf("error %q missing expected named-groups text", err.Error())
		}
	})

	t.Run("single missing", func(t *testing.T) {
		err := rx.Validate(re, "name", "ssn")
		var ve *rx.MissingNamedGroupsError
		if !errors.As(err, &ve) {
			t.Fatalf("error %q is not a *MissingNamedGroupsError", err)
		}
		if want := []string{"ssn"}; !reflect.DeepEqual(ve.Missing, want) {
			t.Errorf("MissingNamedGroupsError.Missing = %v, want %v", ve.Missing, want)
		}
	})

	t.Run("nil error is not a MissingNamedGroupsError", func(t *testing.T) {
		err := rx.Validate(re, "name", "age")
		var ve *rx.MissingNamedGroupsError
		if errors.As(err, &ve) {
			t.Errorf("errors.As recovered %+v from a nil error, want false", ve)
		}
	})

	// A directly-constructed MissingNamedGroupsError with no Missing names must not
	// render a dangling "missing named groups: " separator. Validate never
	// produces this (it only wraps a non-empty set), but the type is exported.
	t.Run("empty Missing renders clean message", func(t *testing.T) {
		for _, ve := range []*rx.MissingNamedGroupsError{{}, {Missing: []string{}}} {
			if got, want := ve.Error(), "no missing named groups"; got != want {
				t.Errorf("(&MissingNamedGroupsError{Missing:%v}).Error() = %q, want %q", ve.Missing, got, want)
			}
		}
	})
}

// status is a custom type whose pointer satisfies RegexUnmarshaler;
// used by TestUnmarshalRegexUnmarshaler.
func ExampleValidate() {
	re := regexp.MustCompile(`(?P<name>\w+) (?P<age>\d+)`)
	if err := rx.Validate(re, "name", "age", "ssn"); err != nil {
		fmt.Println(err)
	}
	// Output: regextra.Validate: missing named groups: ssn
}
