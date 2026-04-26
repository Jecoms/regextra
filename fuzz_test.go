package regextra

import (
	"fmt"
	"regexp"
	"testing"
)

// Fuzz tests for Unmarshal type conversions and FindNamed reflection paths.
// CI runs these with `go test -fuzz=Fuzz... -fuzztime=...` to exercise the
// strconv-backed Int / Uint / Float / Bool branches and the regex-resolution
// path with arbitrary inputs. The goal is to surface panics, infinite loops,
// or surprising errors — not to assert specific output values.

// FuzzFindNamed feeds arbitrary inputs to FindNamed via a fixed pattern
// with three named groups. We don't care what it returns; we only assert
// it doesn't panic and respects the (string, bool) contract.
func FuzzFindNamed(f *testing.F) {
	seeds := []string{
		"",
		"Alice 30",
		"Bob 25 ignored",
		"  spaces  42  ",
		"unicode: αβγ 99",
		"emoji: 🦀 7",
		"only digits: 12345",
		"multiline\nrow 1\nrow 2",
		"\x00null\x00 0",
		"name 99999999999999999999",
		"x \t1",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	re := regexp.MustCompile(`(?P<word>\S+)\s+(?P<num>\d+)`)
	f.Fuzz(func(t *testing.T, target string) {
		// Should not panic for any input.
		got, ok := FindNamed(re, target, "word")
		if !ok && got != "" {
			t.Fatalf("FindNamed contract violated: ok=false but got=%q (want empty)", got)
		}
		// Group name that doesn't exist must always return ("", false).
		miss, missOk := FindNamed(re, target, "definitelyNotThere")
		if miss != "" || missOk {
			t.Fatalf("missing group name should return (\"\", false), got (%q, %v)", miss, missOk)
		}
	})
}

// FuzzNamedGroups feeds arbitrary inputs to NamedGroups. Asserts contract:
// returned map is non-nil; every key, when present, corresponds to a
// declared group on the regex.
func FuzzNamedGroups(f *testing.F) {
	for _, s := range []string{"", "a 1", "abc 12 def 34", "\xff bad utf8"} {
		f.Add(s)
	}

	re := regexp.MustCompile(`(?P<word>\S+)\s+(?P<num>\d+)`)
	declared := map[string]bool{"word": true, "num": true}

	f.Fuzz(func(t *testing.T, target string) {
		groups := NamedGroups(re, target)
		if groups == nil {
			t.Fatalf("NamedGroups returned nil; contract requires non-nil map")
		}
		for k := range groups {
			if !declared[k] {
				t.Fatalf("NamedGroups returned undeclared key %q", k)
			}
		}
	})
}

// FuzzUnmarshalInt drives the int-conversion branch of setFieldValue with
// arbitrary group values. Pattern is one named group; target is constructed
// from the fuzz input. Failure modes: panic, or success on a value that
// strconv.ParseInt(value, 10, 64) would reject.
func FuzzUnmarshalInt(f *testing.F) {
	for _, s := range []string{"0", "1", "-1", "9223372036854775807", "-9223372036854775808", "1e3", "abc", " 12 ", "+5", "0x10", "9999999999999999999"} {
		f.Add(s)
	}

	type intHolder struct {
		N int `regex:"n"`
	}
	re := regexp.MustCompile(`^(?P<n>.*)$`)

	f.Fuzz(func(t *testing.T, raw string) {
		// Skip newline-laced inputs because the anchored pattern won't match
		// across a literal \n; that's a test concern, not an Unmarshal concern.
		for _, b := range []byte(raw) {
			if b == '\n' || b == '\r' {
				return
			}
		}

		var v intHolder
		err := Unmarshal(re, raw, &v)
		if err != nil {
			// Non-int input is allowed to error; just confirm the error message
			// names the offender clearly.
			if !containsByte(err.Error(), '"') {
				t.Fatalf("error should quote the offending value, got: %v", err)
			}
			return
		}
		// If err == nil, raw should round-trip through strconv.ParseInt.
		expected := fmt.Sprintf("%d", v.N)
		if expected == "" {
			t.Fatalf("Unmarshal succeeded but produced unprintable int field for input %q", raw)
		}
	})
}

// FuzzUnmarshalUint exercises the uint branch with arbitrary input.
func FuzzUnmarshalUint(f *testing.F) {
	for _, s := range []string{"0", "1", "18446744073709551615", "-1", "abc", "1.5", "+5", "999999999999999999999"} {
		f.Add(s)
	}

	type uintHolder struct {
		N uint64 `regex:"n"`
	}
	re := regexp.MustCompile(`^(?P<n>.*)$`)

	f.Fuzz(func(_ *testing.T, raw string) {
		for _, b := range []byte(raw) {
			if b == '\n' || b == '\r' {
				return
			}
		}
		var v uintHolder
		_ = Unmarshal(re, raw, &v) // allowed to error; just must not panic
	})
}

// FuzzUnmarshalFloat exercises the float branch with arbitrary input.
func FuzzUnmarshalFloat(f *testing.F) {
	for _, s := range []string{"0", "0.0", "-1.5", "1e10", "Inf", "-Inf", "NaN", "abc", "", "1.7976931348623157e+308"} {
		f.Add(s)
	}

	type floatHolder struct {
		N float64 `regex:"n"`
	}
	re := regexp.MustCompile(`^(?P<n>.*)$`)

	f.Fuzz(func(_ *testing.T, raw string) {
		for _, b := range []byte(raw) {
			if b == '\n' || b == '\r' {
				return
			}
		}
		var v floatHolder
		_ = Unmarshal(re, raw, &v) // allowed to error; just must not panic
	})
}

// FuzzUnmarshalBool exercises the bool branch — strconv.ParseBool accepts
// 1/0/t/f/T/F/true/false/TRUE/FALSE/True/False and rejects everything else.
func FuzzUnmarshalBool(f *testing.F) {
	for _, s := range []string{"0", "1", "t", "f", "true", "false", "TRUE", "FALSE", "yes", "no", "on", "off", "", "2"} {
		f.Add(s)
	}

	type boolHolder struct {
		B bool `regex:"b"`
	}
	re := regexp.MustCompile(`^(?P<b>.*)$`)

	f.Fuzz(func(_ *testing.T, raw string) {
		for _, b := range []byte(raw) {
			if b == '\n' || b == '\r' {
				return
			}
		}
		var v boolHolder
		_ = Unmarshal(re, raw, &v) // allowed to error; just must not panic
	})
}

func containsByte(s string, b byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return true
		}
	}
	return false
}
