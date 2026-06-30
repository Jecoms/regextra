package regextra_test

import (
	"encoding"
	"math/big"
	"net/netip"
	"regexp"
	"strings"
	"testing"
	"time"

	"log/slog"

	rx "github.com/jecoms/regextra"
)

// These tests lock in the encoding.TextUnmarshaler fallback added for #119.
// Fixtures are stdlib-only (no new module dependency): netip.Addr, math/big.Int
// (pointer receiver), and log/slog.Level (underlying int, must NOT be routed
// through the built-in int conversion).

func TestUnmarshal_TextUnmarshaler_Value(t *testing.T) {
	type rec struct {
		Addr  netip.Addr `regex:"addr"`
		Big   big.Int    `regex:"big"`
		Level slog.Level `regex:"level"`
	}

	re := regexp.MustCompile(`(?P<addr>\S+) (?P<big>\S+) (?P<level>\S+)`)
	var got rec
	if err := rx.Unmarshal(re, "192.168.0.1 123456789012345678901234567890 WARN", &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if want := netip.MustParseAddr("192.168.0.1"); got.Addr != want {
		t.Errorf("Addr = %v, want %v", got.Addr, want)
	}
	wantBig, _ := new(big.Int).SetString("123456789012345678901234567890", 10)
	if got.Big.Cmp(wantBig) != 0 {
		t.Errorf("Big = %v, want %v", &got.Big, wantBig)
	}
	if got.Level != slog.LevelWarn {
		t.Errorf("Level = %v, want %v", got.Level, slog.LevelWarn)
	}
}

func TestUnmarshal_TextUnmarshaler_Pointer(t *testing.T) {
	type rec struct {
		Addr  *netip.Addr `regex:"addr"`
		Big   *big.Int    `regex:"big"`
		Level *slog.Level `regex:"level"`
	}

	re := regexp.MustCompile(`(?P<addr>\S+) (?P<big>\S+) (?P<level>\S+)`)
	var got rec
	if err := rx.Unmarshal(re, "::1 42 ERROR", &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.Addr == nil || *got.Addr != netip.MustParseAddr("::1") {
		t.Errorf("Addr = %v, want ::1", got.Addr)
	}
	if got.Big == nil || got.Big.Int64() != 42 {
		t.Errorf("Big = %v, want 42", got.Big)
	}
	if got.Level == nil || *got.Level != slog.LevelError {
		t.Errorf("Level = %v, want ERROR", got.Level)
	}
}

func TestUnmarshal_TextUnmarshaler_InterfaceField(t *testing.T) {
	// An interface-typed field pre-populated with a concrete implementation is
	// dispatched via Type().Implements, mirroring the RegexUnmarshaler path.
	type rec struct {
		Addr encoding.TextUnmarshaler `regex:"addr"`
	}

	re := regexp.MustCompile(`(?P<addr>\S+)`)
	var ip netip.Addr
	got := rec{Addr: &ip}
	if err := rx.Unmarshal(re, "10.0.0.7", &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if want := netip.MustParseAddr("10.0.0.7"); ip != want {
		t.Errorf("Addr = %v, want %v", ip, want)
	}
}

func TestUnmarshal_TextUnmarshaler_Error(t *testing.T) {
	type rec struct {
		Addr netip.Addr `regex:"addr"`
	}

	re := regexp.MustCompile(`(?P<addr>\S+)`)
	var got rec
	err := rx.Unmarshal(re, "not-an-ip", &got)
	if err == nil {
		t.Fatal("expected error for invalid netip.Addr, got nil")
	}
	if !strings.Contains(err.Error(), "not-an-ip") {
		t.Errorf("error %q should mention the offending value", err)
	}
}

// regexAndText implements BOTH RegexUnmarshaler and encoding.TextUnmarshaler;
// RegexUnmarshaler must win.
type regexAndText struct {
	via string
}

func (r *regexAndText) UnmarshalRegex(value string) error {
	r.via = "regex:" + value
	return nil
}

func (r *regexAndText) UnmarshalText(text []byte) error {
	r.via = "text:" + string(text)
	return nil
}

func TestUnmarshal_RegexUnmarshaler_BeatsTextUnmarshaler(t *testing.T) {
	type rec struct {
		F regexAndText `regex:"f"`
	}
	re := regexp.MustCompile(`(?P<f>\S+)`)
	var got rec
	if err := rx.Unmarshal(re, "hello", &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.F.via != "regex:hello" {
		t.Errorf("dispatched via %q, want RegexUnmarshaler", got.F.via)
	}
}

// TestUnmarshal_TimeBeatsTextUnmarshaler locks the ordering: time.Time itself
// satisfies encoding.TextUnmarshaler with an RFC3339-only UnmarshalText. If the
// TextUnmarshaler check ran before the time special-case, the multi-layout
// fallback and the `layout` tag option below would be dropped and these
// non-RFC3339 inputs would fail to parse.
func TestUnmarshal_TimeBeatsTextUnmarshaler(t *testing.T) {
	type fallback struct {
		TS time.Time `regex:"ts"`
	}
	re := regexp.MustCompile(`(?P<ts>.+)`)
	var got fallback
	if err := rx.Unmarshal(re, "2024-01-15", &got); err != nil {
		t.Fatalf("Unmarshal (fallback list): %v", err)
	}
	if got.TS.Year() != 2024 || got.TS.Month() != time.January || got.TS.Day() != 15 {
		t.Errorf("TS = %v, want 2024-01-15", got.TS)
	}

	type withLayout struct {
		TS time.Time `regex:"ts,layout=02/Jan/2006"`
	}
	reL := regexp.MustCompile(`(?P<ts>.+)`)
	var gotL withLayout
	if err := rx.Unmarshal(reL, "15/Jan/2024", &gotL); err != nil {
		t.Fatalf("Unmarshal (layout opt): %v", err)
	}
	if gotL.TS.Year() != 2024 || gotL.TS.Month() != time.January || gotL.TS.Day() != 15 {
		t.Errorf("TS = %v, want 2024-01-15 via layout", gotL.TS)
	}
}
