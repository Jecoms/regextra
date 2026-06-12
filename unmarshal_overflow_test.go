package regextra

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Regression tests for https://github.com/Jecoms/regextra/issues/103:
// values out of range for narrow numeric fields must error instead of
// silently truncating (e.g. "300" into int8 previously yielded 44, nil).

func TestUnmarshal_intOverflow(t *testing.T) {
	re := regexp.MustCompile(`(?P<n>-?\d+)`)

	t.Run("int8 overflow errors", func(t *testing.T) {
		var dst struct {
			N int8 `regex:"n"`
		}
		err := Unmarshal(re, "300", &dst)
		if err == nil {
			t.Fatalf("expected overflow error, got nil (N = %d)", dst.N)
		}
		if !strings.Contains(err.Error(), "cannot convert") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("int8 underflow errors", func(t *testing.T) {
		var dst struct {
			N int8 `regex:"n"`
		}
		if err := Unmarshal(re, "-129", &dst); err == nil {
			t.Fatalf("expected underflow error, got nil (N = %d)", dst.N)
		}
	})

	t.Run("int8 bounds fit", func(t *testing.T) {
		var dst struct {
			Lo int8 `regex:"n"`
		}
		if err := Unmarshal(re, "127", &dst); err != nil {
			t.Fatalf("127 should fit in int8: %v", err)
		}
		if dst.Lo != math.MaxInt8 {
			t.Errorf("Lo = %d, want 127", dst.Lo)
		}
		if err := Unmarshal(re, "-128", &dst); err != nil {
			t.Fatalf("-128 should fit in int8: %v", err)
		}
		if dst.Lo != math.MinInt8 {
			t.Errorf("Lo = %d, want -128", dst.Lo)
		}
	})

	t.Run("int16 and int32 overflow errors", func(t *testing.T) {
		var dst16 struct {
			N int16 `regex:"n"`
		}
		if err := Unmarshal(re, "40000", &dst16); err == nil {
			t.Errorf("expected int16 overflow error, got nil (N = %d)", dst16.N)
		}
		var dst32 struct {
			N int32 `regex:"n"`
		}
		if err := Unmarshal(re, strconv.FormatInt(math.MaxInt32+1, 10), &dst32); err == nil {
			t.Errorf("expected int32 overflow error, got nil (N = %d)", dst32.N)
		}
	})
}

func TestUnmarshal_uintOverflow(t *testing.T) {
	re := regexp.MustCompile(`(?P<n>\d+)`)

	t.Run("uint8 overflow errors", func(t *testing.T) {
		var dst struct {
			N uint8 `regex:"n"`
		}
		if err := Unmarshal(re, "256", &dst); err == nil {
			t.Fatalf("expected overflow error, got nil (N = %d)", dst.N)
		}
	})

	t.Run("uint8 max fits", func(t *testing.T) {
		var dst struct {
			N uint8 `regex:"n"`
		}
		if err := Unmarshal(re, "255", &dst); err != nil {
			t.Fatalf("255 should fit in uint8: %v", err)
		}
		if dst.N != math.MaxUint8 {
			t.Errorf("N = %d, want 255", dst.N)
		}
	})
}

func TestUnmarshal_floatOverflow(t *testing.T) {
	re := regexp.MustCompile(`(?P<n>[\d.e+]+)`)

	t.Run("float32 overflow errors", func(t *testing.T) {
		var dst struct {
			N float32 `regex:"n"`
		}
		if err := Unmarshal(re, "1e308", &dst); err == nil {
			t.Fatalf("expected overflow error, got nil (N = %g)", dst.N)
		}
	})

	t.Run("float32 in range fits", func(t *testing.T) {
		var dst struct {
			N float32 `regex:"n"`
		}
		if err := Unmarshal(re, "3.5", &dst); err != nil {
			t.Fatalf("3.5 should fit in float32: %v", err)
		}
		if dst.N != 3.5 {
			t.Errorf("N = %g, want 3.5", dst.N)
		}
	})

	t.Run("float64 still accepts large values", func(t *testing.T) {
		var dst struct {
			N float64 `regex:"n"`
		}
		if err := Unmarshal(re, "1e308", &dst); err != nil {
			t.Fatalf("1e308 fits in float64: %v", err)
		}
	})
}

// The Decoder shares setFieldValue, so overflow must error there too — both
// at decode time and when validating a default= at Compile time.
func TestDecoder_overflow(t *testing.T) {
	t.Run("One errors on overflow", func(t *testing.T) {
		type rec struct {
			N int8 `regex:"n"`
		}
		dec := MustCompile[rec](`(?P<n>\d+)`)
		if _, err := dec.One("300"); err == nil {
			t.Fatal("expected overflow error, got nil")
		}
	})

	t.Run("Compile rejects out-of-range default", func(t *testing.T) {
		type rec struct {
			N int8 `regex:"n,default=300"`
		}
		if _, err := Compile[rec](`(?P<n>\d+)`); err == nil {
			t.Fatal("expected Compile error for out-of-range default, got nil")
		}
	})
}
