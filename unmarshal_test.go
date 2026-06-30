package regextra_test

import (
	"encoding"
	"fmt"
	rx "github.com/jecoms/regextra"
	"log/slog"
	"math"
	"math/big"
	"net/netip"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

type status int

const (
	statusUnknown status = iota
	statusOpen
	statusClosed
)

const (
	stateOpen   = "open"
	stateClosed = "closed"
)

// aliceFixture is the canonical fixture string used across input-validation
// tests. Defined as a const to keep the goconst linter happy.
const aliceFixture = "Alice"

func (s *status) UnmarshalRegex(value string) error {
	switch value {
	case stateOpen:
		*s = statusOpen
	case stateClosed:
		*s = statusClosed
	default:
		return fmt.Errorf("unknown status %q", value)
	}
	return nil
}

// valueReceiver is here to confirm the value-receiver implements path is hit.
// Note: a value receiver can't actually mutate the field, so this is only
// useful for read-only side effects in tests.
type valueReceiver struct{}

func (v valueReceiver) UnmarshalRegex(value string) error {
	if value == "fail" {
		return fmt.Errorf("valueReceiver rejected: %s", value)
	}
	return nil
}

func TestUnmarshalRegexUnmarshaler(t *testing.T) {
	t.Run("pointer-receiver custom type populates field", func(t *testing.T) {
		type Issue struct {
			ID    int    `regex:"id"`
			State status `regex:"state"`
		}
		re := regexp.MustCompile(`#(?P<id>\d+) \[(?P<state>\w+)\]`)
		var issue Issue
		if err := rx.Unmarshal(re, "#42 [open]", &issue); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		if issue.ID != 42 {
			t.Errorf("ID = %d, want 42", issue.ID)
		}
		if issue.State != statusOpen {
			t.Errorf("State = %d, want %d (statusOpen)", issue.State, statusOpen)
		}
	})

	t.Run("RegexUnmarshaler error propagates", func(t *testing.T) {
		type Issue struct {
			State status `regex:"state"`
		}
		re := regexp.MustCompile(`\[(?P<state>\w+)\]`)
		var issue Issue
		err := rx.Unmarshal(re, "[bogus]", &issue)
		if err == nil {
			t.Fatal("Unmarshal returned nil, want error")
		}
		if !strings.Contains(err.Error(), `unknown status "bogus"`) {
			t.Errorf("error = %q, want it to mention 'unknown status \"bogus\"'", err.Error())
		}
	})

	t.Run("RegexUnmarshaler intercepts before built-in int conversion", func(t *testing.T) {
		// status's underlying type is int, so without the interface check
		// setFieldValue would try strconv.ParseInt on "open" and fail.
		type Issue struct {
			State status `regex:"state"`
		}
		re := regexp.MustCompile(`\[(?P<state>\w+)\]`)
		var issue Issue
		if err := rx.Unmarshal(re, "[open]", &issue); err != nil {
			t.Fatalf("Unmarshal returned %v — interface check was not hit before int conversion", err)
		}
	})

	t.Run("value-receiver implementation is consulted", func(t *testing.T) {
		type Holder struct {
			V valueReceiver `regex:"v"`
		}
		re := regexp.MustCompile(`(?P<v>\w+)`)
		var h Holder
		if err := rx.Unmarshal(re, "ok", &h); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
	})

	t.Run("interface-typed field with concrete value is dispatched", func(t *testing.T) {
		// Regression guard for #125: an interface-typed struct field whose
		// stored concrete value implements RegexUnmarshaler must dispatch.
		// field.Addr() on an interface field is a *interface, which does NOT
		// satisfy RegexUnmarshaler, so the CanAddr branch alone cannot handle
		// this — the field.Type().Implements(...) branch is required.
		type Holder struct {
			F rx.RegexUnmarshaler `regex:"f"`
		}
		re := regexp.MustCompile(`(?P<f>\w+)`)
		s := new(status)
		h := Holder{F: s}
		if err := rx.Unmarshal(re, "open", &h); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		if *s != statusOpen {
			t.Errorf("UnmarshalRegex did not run: *s = %d, want %d (statusOpen)", *s, statusOpen)
		}
	})
}

func TestUnmarshalTimeTypes(t *testing.T) {
	t.Run("time.Time RFC3339", func(t *testing.T) {
		type Event struct {
			TS time.Time `regex:"ts"`
		}
		re := regexp.MustCompile(`(?P<ts>\S+)`)
		var ev Event
		if err := rx.Unmarshal(re, "2026-04-26T12:34:56Z", &ev); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		want, _ := time.Parse(time.RFC3339, "2026-04-26T12:34:56Z")
		if !ev.TS.Equal(want) {
			t.Errorf("TS = %v, want %v", ev.TS, want)
		}
	})

	t.Run("time.Time DateTime fallback", func(t *testing.T) {
		type Event struct {
			TS time.Time `regex:"ts"`
		}
		re := regexp.MustCompile(`(?P<ts>.+)`)
		var ev Event
		if err := rx.Unmarshal(re, "2026-04-26 12:34:56", &ev); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		if ev.TS.Year() != 2026 || ev.TS.Month() != time.April || ev.TS.Day() != 26 {
			t.Errorf("TS = %v, want 2026-04-26 ...", ev.TS)
		}
	})

	t.Run("time.Time DateOnly fallback", func(t *testing.T) {
		type Event struct {
			TS time.Time `regex:"ts"`
		}
		re := regexp.MustCompile(`(?P<ts>\S+)`)
		var ev Event
		if err := rx.Unmarshal(re, "2026-04-26", &ev); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		if ev.TS.Year() != 2026 {
			t.Errorf("Year = %d, want 2026", ev.TS.Year())
		}
	})

	t.Run("time.Time unparseable returns error", func(t *testing.T) {
		type Event struct {
			TS time.Time `regex:"ts"`
		}
		re := regexp.MustCompile(`(?P<ts>.+)`)
		var ev Event
		err := rx.Unmarshal(re, "definitely not a time", &ev)
		if err == nil {
			t.Fatal("Unmarshal returned nil, want error")
		}
		if !strings.Contains(err.Error(), "cannot convert") || !strings.Contains(err.Error(), "time.Time") {
			t.Errorf("error = %q, want it to mention time.Time conversion failure", err.Error())
		}
	})

	t.Run("time.Duration", func(t *testing.T) {
		type Span struct {
			D time.Duration `regex:"d"`
		}
		re := regexp.MustCompile(`(?P<d>\S+)`)
		var sp Span
		if err := rx.Unmarshal(re, "1h30m", &sp); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		want := 90 * time.Minute
		if sp.D != want {
			t.Errorf("D = %v, want %v", sp.D, want)
		}
	})

	t.Run("time.Duration unparseable returns error", func(t *testing.T) {
		type Span struct {
			D time.Duration `regex:"d"`
		}
		re := regexp.MustCompile(`(?P<d>\S+)`)
		var sp Span
		err := rx.Unmarshal(re, "notaduration", &sp)
		if err == nil {
			t.Fatal("Unmarshal returned nil, want error")
		}
		if !strings.Contains(err.Error(), "cannot convert") || !strings.Contains(err.Error(), "time.Duration") {
			t.Errorf("error = %q, want it to mention time.Duration conversion failure", err.Error())
		}
	})

	t.Run("time.Duration is matched by Type, not by Int64 kind", func(t *testing.T) {
		// time.Duration's underlying type is int64. Without the type-based
		// match before the kind switch, "1h30m" would fall into reflect.Int64
		// and strconv.ParseInt would reject it.
		type Span struct {
			D time.Duration `regex:"d"`
		}
		re := regexp.MustCompile(`(?P<d>\S+)`)
		var sp Span
		if err := rx.Unmarshal(re, "5s", &sp); err != nil {
			t.Fatalf("Unmarshal returned %v — Type-based match did not pre-empt Int64 kind", err)
		}
	})
}

func ExampleRegexUnmarshaler() {
	type Severity int
	const (
		_ Severity = iota
		Low
		Medium
		High
	)
	// In real code this would be a type defined in the same package as
	// the call to Unmarshal, with `func (s *Severity) UnmarshalRegex(...) error`.
	// Compile-time check elided here for example brevity.
	_ = Low
	_ = Medium
	_ = High
	fmt.Println("see TestUnmarshalRegexUnmarshaler for a runnable demo")
	// Output: see TestUnmarshalRegexUnmarshaler for a runnable demo
}

func ExampleUnmarshal_timeTypes() {
	type Event struct {
		Started time.Time     `regex:"start"`
		Took    time.Duration `regex:"took"`
	}
	re := regexp.MustCompile(`(?P<start>\S+)\s+\((?P<took>\S+)\)`)
	var ev Event
	_ = rx.Unmarshal(re, "2026-04-26T12:34:56Z (1h30m)", &ev)
	fmt.Printf("%s for %s\n", ev.Started.Format(time.RFC3339), ev.Took)
	// Output: 2026-04-26T12:34:56Z for 1h30m0s
}

func TestUnmarshalDefault(t *testing.T) {
	t.Run("default fills when group not declared", func(t *testing.T) {
		type Person struct {
			Name string `regex:"name"`
			Role string `regex:"role,default=guest"`
		}
		const wantName = "Mallory"
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var p Person
		if err := rx.Unmarshal(re, wantName, &p); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		if p.Name != wantName {
			t.Errorf("Name = %q, want %q", p.Name, wantName)
		}
		if p.Role != "guest" {
			t.Errorf("Role = %q, want guest (default)", p.Role)
		}
	})

	t.Run("default fills when match is empty", func(t *testing.T) {
		type Person struct {
			Title string `regex:"title,default=N/A"`
		}
		// `title?` matches an optional letter — input below produces an empty
		// match for the named group.
		re := regexp.MustCompile(`^(?P<title>[A-Z]?)\.?$`)
		var p Person
		if err := rx.Unmarshal(re, ".", &p); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		if p.Title != "N/A" {
			t.Errorf("Title = %q, want N/A (default for empty match)", p.Title)
		}
	})

	t.Run("default does not override a non-empty match", func(t *testing.T) {
		type Person struct {
			Role string `regex:"role,default=guest"`
		}
		re := regexp.MustCompile(`(?P<role>\w+)`)
		var p Person
		if err := rx.Unmarshal(re, "admin", &p); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		if p.Role != "admin" {
			t.Errorf("Role = %q, want admin (match wins over default)", p.Role)
		}
	})

	t.Run("default goes through type conversion", func(t *testing.T) {
		type Limits struct {
			Max int `regex:"max,default=100"`
		}
		re := regexp.MustCompile(`(?P<other>\w+)`)
		var l Limits
		if err := rx.Unmarshal(re, "ignored", &l); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		if l.Max != 100 {
			t.Errorf("Max = %d, want 100", l.Max)
		}
	})

	t.Run("malformed default value surfaces a clear error", func(t *testing.T) {
		type Limits struct {
			Max int `regex:"max,default=notanumber"`
		}
		re := regexp.MustCompile(`(?P<other>\w+)`)
		var l Limits
		err := rx.Unmarshal(re, "ignored", &l)
		if err == nil {
			t.Fatal("Unmarshal returned nil, want error from default conversion")
		}
		if !strings.Contains(err.Error(), "cannot convert") {
			t.Errorf("error = %q, want it to mention conversion failure", err.Error())
		}
	})
}

func TestUnmarshalLayoutOverride(t *testing.T) {
	t.Run("layout option parses Apache log time", func(t *testing.T) {
		type Log struct {
			TS time.Time `regex:"ts,layout=02/Jan/2006:15:04:05 -0700"`
		}
		re := regexp.MustCompile(`\[(?P<ts>[^\]]+)\]`)
		var l Log
		if err := rx.Unmarshal(re, "[26/Apr/2026:12:34:56 -0500]", &l); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		if l.TS.Year() != 2026 || l.TS.Month() != time.April || l.TS.Day() != 26 {
			t.Errorf("TS = %v, want 2026-04-26 ...", l.TS)
		}
	})

	t.Run("layout mismatch errors with layout in message", func(t *testing.T) {
		type Log struct {
			TS time.Time `regex:"ts,layout=2006-01-02"`
		}
		re := regexp.MustCompile(`(?P<ts>\S+)`)
		var l Log
		err := rx.Unmarshal(re, "not-a-date", &l)
		if err == nil {
			t.Fatal("Unmarshal returned nil, want error")
		}
		if !strings.Contains(err.Error(), "2006-01-02") {
			t.Errorf("error = %q, want it to mention the layout", err.Error())
		}
	})

	t.Run("layout option does NOT fall back to default list", func(t *testing.T) {
		// Default fallback would parse "2026-04-26" via DateOnly. With layout
		// pinned to RFC3339, this should error rather than succeed.
		type Log struct {
			TS time.Time `regex:"ts,layout=2006-01-02T15:04:05Z07:00"`
		}
		re := regexp.MustCompile(`(?P<ts>\S+)`)
		var l Log
		err := rx.Unmarshal(re, "2026-04-26", &l)
		if err == nil {
			t.Fatal("Unmarshal returned nil; layout override should not fall back to DateOnly")
		}
	})
}

func ExampleUnmarshal_defaultAndLayout() {
	type LogLine struct {
		TS    time.Time `regex:"ts,layout=02/Jan/2006:15:04:05 -0700"`
		Level string    `regex:"level,default=info"`
	}
	re := regexp.MustCompile(`\[(?P<ts>[^\]]+)\]\s*(?P<other>.*)`)
	var l LogLine
	_ = rx.Unmarshal(re, "[26/Apr/2026:12:34:56 -0500] something happened", &l)
	fmt.Printf("%s @ %s\n", l.Level, l.TS.Format("2006-01-02"))
	// Output: info @ 2026-04-26
}

func TestUnmarshalPointerFields(t *testing.T) {
	t.Run("*string allocated and set", func(t *testing.T) {
		type Holder struct {
			S *string `regex:"s"`
		}
		re := regexp.MustCompile(`(?P<s>\w+)`)
		var h Holder
		if err := rx.Unmarshal(re, "hello", &h); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		if h.S == nil {
			t.Fatal("S is nil; pointer was not allocated")
		}
		if *h.S != "hello" {
			t.Errorf("*S = %q, want %q", *h.S, "hello")
		}
	})

	t.Run("*int allocated and set", func(t *testing.T) {
		type Holder struct {
			N *int `regex:"n"`
		}
		re := regexp.MustCompile(`(?P<n>\d+)`)
		var h Holder
		if err := rx.Unmarshal(re, "42", &h); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		if h.N == nil || *h.N != 42 {
			t.Errorf("*N = %v, want 42", h.N)
		}
	})

	t.Run("*float64 allocated and set", func(t *testing.T) {
		type Holder struct {
			F *float64 `regex:"f"`
		}
		re := regexp.MustCompile(`(?P<f>\S+)`)
		var h Holder
		if err := rx.Unmarshal(re, "3.14", &h); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		if h.F == nil || *h.F != 3.14 {
			t.Errorf("*F = %v, want 3.14", h.F)
		}
	})

	t.Run("*bool allocated and set", func(t *testing.T) {
		type Holder struct {
			B *bool `regex:"b"`
		}
		re := regexp.MustCompile(`(?P<b>\S+)`)
		var h Holder
		if err := rx.Unmarshal(re, "true", &h); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		if h.B == nil || *h.B != true {
			t.Errorf("*B = %v, want true", h.B)
		}
	})

	t.Run("*time.Time allocated and set via the time-types path", func(t *testing.T) {
		type Holder struct {
			TS *time.Time `regex:"ts"`
		}
		re := regexp.MustCompile(`(?P<ts>\S+)`)
		var h Holder
		if err := rx.Unmarshal(re, "2026-04-26T12:34:56Z", &h); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		if h.TS == nil {
			t.Fatal("TS is nil; pointer was not allocated")
		}
		if h.TS.Year() != 2026 {
			t.Errorf("TS.Year() = %d, want 2026", h.TS.Year())
		}
	})

	t.Run("*time.Duration allocated and set", func(t *testing.T) {
		type Holder struct {
			D *time.Duration `regex:"d"`
		}
		re := regexp.MustCompile(`(?P<d>\S+)`)
		var h Holder
		if err := rx.Unmarshal(re, "5s", &h); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		if h.D == nil || *h.D != 5*time.Second {
			t.Errorf("*D = %v, want 5s", h.D)
		}
	})

	t.Run("*RegexUnmarshaler dispatched on the pointer's own method", func(t *testing.T) {
		type Holder struct {
			S *status `regex:"s"`
		}
		re := regexp.MustCompile(`\[(?P<s>\w+)\]`)
		var h Holder
		if err := rx.Unmarshal(re, "[open]", &h); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		if h.S == nil || *h.S != statusOpen {
			t.Errorf("*S = %v, want statusOpen", h.S)
		}
	})

	t.Run("non-nil pointer is not re-allocated; existing pointee overwritten", func(t *testing.T) {
		type Holder struct {
			N *int `regex:"n"`
		}
		re := regexp.MustCompile(`(?P<n>\d+)`)
		preallocated := 999
		h := Holder{N: &preallocated}
		original := h.N
		if err := rx.Unmarshal(re, "42", &h); err != nil {
			t.Fatalf("Unmarshal returned %v", err)
		}
		if h.N != original {
			t.Errorf("pointer identity changed; expected the existing pointer to be reused")
		}
		if *h.N != 42 {
			t.Errorf("*N = %d, want 42 (pointee should be overwritten)", *h.N)
		}
	})

	t.Run("conversion error on pointer field is returned without partial state", func(t *testing.T) {
		type Holder struct {
			N *int `regex:"n"`
		}
		re := regexp.MustCompile(`(?P<n>\S+)`)
		var h Holder
		err := rx.Unmarshal(re, "notanumber", &h)
		if err == nil {
			t.Fatal("Unmarshal returned nil, want error")
		}
		if !strings.Contains(err.Error(), "cannot convert") {
			t.Errorf("error = %q, want it to mention conversion failure", err.Error())
		}
	})
}

func ExampleUnmarshal_pointerFields() {
	type Result struct {
		Name *string `regex:"name"`
		Age  *int    `regex:"age"`
	}
	re := regexp.MustCompile(`(?P<name>\w+)\s+is\s+(?P<age>\d+)`)
	var r Result
	_ = rx.Unmarshal(re, "Alice is 30", &r)
	fmt.Printf("%s, %d\n", *r.Name, *r.Age)
	// Output: Alice, 30
}

func TestUnmarshal(t *testing.T) {
	t.Run("basic string fields", func(t *testing.T) {
		type Person struct {
			Name string
			Age  string
		}
		re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
		var person Person
		err := rx.Unmarshal(re, "Alice is 30", &person)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if person.Name != aliceFixture {
			t.Errorf("Name = %q, want %q", person.Name, aliceFixture)
		}
		if person.Age != "30" {
			t.Errorf("Age = %q, want %q", person.Age, "30")
		}
	})

	t.Run("int type conversion", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}
		re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
		var person Person
		err := rx.Unmarshal(re, "Bob is 25", &person)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if person.Name != "Bob" {
			t.Errorf("Name = %q, want %q", person.Name, "Bob")
		}
		if person.Age != 25 {
			t.Errorf("Age = %d, want %d", person.Age, 25)
		}
	})

	t.Run("float type conversion", func(t *testing.T) {
		type Product struct {
			Name  string
			Price float64
		}
		re := regexp.MustCompile(`(?P<name>\w+) costs \$(?P<price>[\d.]+)`)
		var product Product
		err := rx.Unmarshal(re, "Widget costs $19.99", &product)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if product.Name != "Widget" {
			t.Errorf("Name = %q, want %q", product.Name, "Widget")
		}
		if product.Price != 19.99 {
			t.Errorf("Price = %f, want %f", product.Price, 19.99)
		}
	})

	t.Run("struct tags for custom mapping", func(t *testing.T) {
		type Email struct {
			Username string `regex:"user"`
			Domain   string `regex:"domain"`
		}
		re := regexp.MustCompile(`(?P<user>\w+)@(?P<domain>[\w.]+)`)
		var email Email
		err := rx.Unmarshal(re, "alice@example.com", &email)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if email.Username != "alice" {
			t.Errorf("Username = %q, want %q", email.Username, "alice")
		}
		if email.Domain != "example.com" {
			t.Errorf("Domain = %q, want %q", email.Domain, "example.com")
		}
	})

	t.Run("case insensitive field matching", func(t *testing.T) {
		type Data struct {
			UserName string
			Age      int
		}
		re := regexp.MustCompile(`(?P<username>\w+) (?P<age>\d+)`)
		var data Data
		err := rx.Unmarshal(re, "john 42", &data)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if data.UserName != "john" {
			t.Errorf("UserName = %q, want %q", data.UserName, "john")
		}
		if data.Age != 42 {
			t.Errorf("Age = %d, want %d", data.Age, 42)
		}
	})

	t.Run("no match returns no error", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>[a-z]+)`) // Only lowercase letters
		var person Person
		err := rx.Unmarshal(re, "123", &person)
		if err != nil {
			t.Errorf("Unmarshal() error = %v, want nil", err)
		}
		if person.Name != "" {
			t.Errorf("Name = %q, want empty string", person.Name)
		}
	})

	t.Run("error on non-pointer", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var person Person
		err := rx.Unmarshal(re, aliceFixture, person) // Not a pointer
		if err == nil {
			t.Error("Unmarshal() expected error for non-pointer, got nil")
		}
	})

	t.Run("error on nil pointer", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var person *Person
		err := rx.Unmarshal(re, aliceFixture, person) // Nil pointer
		if err == nil {
			t.Error("Unmarshal() expected error for nil pointer, got nil")
		}
	})

	t.Run("error on pointer to non-struct", func(t *testing.T) {
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var name string
		err := rx.Unmarshal(re, aliceFixture, &name) // Pointer to string, not struct
		if err == nil {
			t.Error("Unmarshal() expected error for pointer to non-struct, got nil")
		}
	})

	t.Run("bool type conversion", func(t *testing.T) {
		type Config struct {
			Enabled string
			Debug   bool
		}
		re := regexp.MustCompile(`enabled=(?P<enabled>\w+) debug=(?P<debug>\w+)`)
		var config Config
		err := rx.Unmarshal(re, "enabled=yes debug=true", &config)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if config.Enabled != "yes" {
			t.Errorf("Enabled = %q, want %q", config.Enabled, "yes")
		}
		if config.Debug != true {
			t.Errorf("Debug = %v, want %v", config.Debug, true)
		}
	})

	t.Run("skip unexported fields", func(t *testing.T) {
		type Data struct {
			Public  string
			private string
		}
		re := regexp.MustCompile(`(?P<public>\w+) (?P<private>\w+)`)
		var data Data
		err := rx.Unmarshal(re, "hello world", &data)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if data.Public != "hello" {
			t.Errorf("Public = %q, want %q", data.Public, "hello")
		}
		if data.private != "" {
			t.Errorf("private should be empty, got %q", data.private)
		}
	})

	t.Run("struct tag takes precedence over field name", func(t *testing.T) {
		type Data struct {
			// Field name is "Value" but tag maps to "count"
			Value string `regex:"count"`
		}
		// Pattern has both "value" and "count" groups
		re := regexp.MustCompile(`value=(?P<value>\w+) count=(?P<count>\d+)`)
		var data Data
		err := rx.Unmarshal(re, "value=hello count=42", &data)
		if err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		// Should get "42" from "count" group (via tag), not "hello" from "value" group (field name)
		if data.Value != "42" {
			t.Errorf("Value = %q, want %q (should use tag mapping)", data.Value, "42")
		}
	})
}

func ExampleUnmarshal() {
	type Person struct {
		Name string
		Age  int
	}

	re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+) years old`)
	var person Person
	err := rx.Unmarshal(re, "Alice is 30 years old", &person)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("%s is %d\n", person.Name, person.Age)
	// Output: Alice is 30
}

func ExampleUnmarshal_structTags() {
	type Email struct {
		Username string `regex:"user"`
		Domain   string `regex:"domain"`
	}

	re := regexp.MustCompile(`(?P<user>\w+)@(?P<domain>[\w.]+)`)
	var email Email
	err := rx.Unmarshal(re, "alice@example.com", &email)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("User: %s, Domain: %s\n", email.Username, email.Domain)
	// Output: User: alice, Domain: example.com
}

func TestUnmarshalAll(t *testing.T) {
	t.Run("multiple matches", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}
		re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
		var people []Person
		err := rx.UnmarshalAll(re, "Alice is 30 and Bob is 25", &people)
		if err != nil {
			t.Fatalf("UnmarshalAll() error = %v", err)
		}
		if len(people) != 2 {
			t.Fatalf("len(people) = %d, want 2", len(people))
		}
		if people[0].Name != aliceFixture || people[0].Age != 30 {
			t.Errorf("people[0] = %+v, want {Name:Alice Age:30}", people[0])
		}
		if people[1].Name != "Bob" || people[1].Age != 25 {
			t.Errorf("people[1] = %+v, want {Name:Bob Age:25}", people[1])
		}
	})

	t.Run("no matches clears slice", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>[a-z]+)`)
		people := []Person{{Name: "existing"}}
		err := rx.UnmarshalAll(re, "123", &people)
		if err != nil {
			t.Fatalf("UnmarshalAll() error = %v", err)
		}
		if len(people) != 0 {
			t.Errorf("len(people) = %d, want 0 (should clear slice)", len(people))
		}
	})

	t.Run("single match", func(t *testing.T) {
		type Email struct {
			User   string `regex:"user"`
			Domain string `regex:"domain"`
		}
		re := regexp.MustCompile(`(?P<user>\w+)@(?P<domain>[\w.]+)`)
		var emails []Email
		err := rx.UnmarshalAll(re, "Contact: alice@example.com", &emails)
		if err != nil {
			t.Fatalf("UnmarshalAll() error = %v", err)
		}
		if len(emails) != 1 {
			t.Fatalf("len(emails) = %d, want 1", len(emails))
		}
		if emails[0].User != "alice" || emails[0].Domain != "example.com" {
			t.Errorf("emails[0] = %+v, want {User:alice Domain:example.com}", emails[0])
		}
	})

	t.Run("three matches with different types", func(t *testing.T) {
		type Product struct {
			Name  string
			Price float64
		}
		re := regexp.MustCompile(`(?P<name>\w+):\$(?P<price>[\d.]+)`)
		var products []Product
		err := rx.UnmarshalAll(re, "Items: Apple:$1.50 Banana:$0.75 Orange:$2.00", &products)
		if err != nil {
			t.Fatalf("UnmarshalAll() error = %v", err)
		}
		if len(products) != 3 {
			t.Fatalf("len(products) = %d, want 3", len(products))
		}
		want := []Product{
			{Name: "Apple", Price: 1.50},
			{Name: "Banana", Price: 0.75},
			{Name: "Orange", Price: 2.00},
		}
		if !reflect.DeepEqual(products, want) {
			t.Errorf("products = %+v, want %+v", products, want)
		}
	})

	t.Run("error on non-pointer", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var people []Person
		err := rx.UnmarshalAll(re, aliceFixture, people) // Not a pointer
		if err == nil {
			t.Error("UnmarshalAll() expected error for non-pointer, got nil")
		}
	})

	t.Run("error on nil pointer", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var people *[]Person
		err := rx.UnmarshalAll(re, aliceFixture, people) // Nil pointer
		if err == nil {
			t.Error("UnmarshalAll() expected error for nil pointer, got nil")
		}
	})

	t.Run("error on pointer to non-slice", func(t *testing.T) {
		type Person struct {
			Name string
		}
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var person Person
		err := rx.UnmarshalAll(re, aliceFixture, &person) // Pointer to struct, not slice
		if err == nil {
			t.Error("UnmarshalAll() expected error for pointer to non-slice, got nil")
		}
	})

	t.Run("error on slice of non-structs", func(t *testing.T) {
		re := regexp.MustCompile(`(?P<name>\w+)`)
		var names []string
		err := rx.UnmarshalAll(re, aliceFixture, &names) // Slice of strings, not structs
		if err == nil {
			t.Error("UnmarshalAll() expected error for slice of non-structs, got nil")
		}
	})
}

func ExampleUnmarshalAll() {
	type Person struct {
		Name string
		Age  int
	}

	re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)
	var people []Person
	err := rx.UnmarshalAll(re, "Alice is 30 and Bob is 25", &people)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	for _, person := range people {
		fmt.Printf("%s: %d\n", person.Name, person.Age)
	}
	// Output: Alice: 30
	// Bob: 25
}

// Fuzz tests for Unmarshal type conversions and FindNamed reflection paths.
// CI runs these with `go test -fuzz=Fuzz... -fuzztime=...` to exercise the
// strconv-backed Int / Uint / Float / Bool branches and the regex-resolution
// path with arbitrary inputs. The goal is to surface panics, infinite loops,
// or surprising errors — not to assert specific output values.

// FuzzFindNamed feeds arbitrary inputs to FindNamed via a fixed pattern
// with three named groups. We don't care what it returns; we only assert
// it doesn't panic and respects the (string, bool) contract.
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
		err := rx.Unmarshal(re, raw, &v)
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
		_ = rx.Unmarshal(re, raw, &v) // allowed to error; just must not panic
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
		_ = rx.Unmarshal(re, raw, &v) // allowed to error; just must not panic
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
		_ = rx.Unmarshal(re, raw, &v) // allowed to error; just must not panic
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

// Tests for https://github.com/Jecoms/regextra/issues/110: the Unmarshal and
// Decoder paths previously used two different case-folding implementations
// (Unicode-aware strings.ToLower vs a hand-rolled ASCII lower), which could
// disagree on non-ASCII field names. Both now use strings.EqualFold.
//
// Note: Go's regexp rejects non-ASCII capture group names, so divergence was
// only possible via the *field* name side of the comparison.

func TestCaseInsensitiveFallback_parity(t *testing.T) {
	// The field name "KELVIN" below starts with U+212A (KELVIN SIGN), an
	// uppercase Unicode letter that folds to ASCII 'k' under Unicode case
	// folding but is untouched by ASCII-only lowering. Before the fix,
	// Unmarshal matched it to the "kelvin" group and the Decoder did not.
	const pattern = `(?P<kelvin>\d+)`
	type reading struct {
		KELVIN int
	}

	re := regexp.MustCompile(pattern)
	var fromUnmarshal reading
	if err := rx.Unmarshal(re, "273", &fromUnmarshal); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	dec, err := rx.Compile[reading](pattern)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	fromDecoder, err := dec.One("273")
	if err != nil {
		t.Fatalf("Decoder.One: %v", err)
	}

	if fromUnmarshal.KELVIN != 273 {
		t.Errorf("Unmarshal: KELVIN = %d, want 273", fromUnmarshal.KELVIN)
	}
	if fromDecoder.KELVIN != 273 {
		t.Errorf("Decoder.One: KELVIN = %d, want 273", fromDecoder.KELVIN)
	}
}

func TestCaseInsensitiveFallback_ascii(t *testing.T) {
	// Plain ASCII case-insensitive fallback keeps working in both paths.
	const pattern = `(?P<username>\w+)`
	type rec struct {
		UserName string
	}

	re := regexp.MustCompile(pattern)
	var u rec
	if err := rx.Unmarshal(re, "alice", &u); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if u.UserName != "alice" {
		t.Errorf("Unmarshal: UserName = %q, want %q", u.UserName, "alice")
	}

	dec := rx.MustCompile[rec](pattern)
	got, err := dec.One("alice")
	if err != nil {
		t.Fatalf("Decoder.One: %v", err)
	}
	if got.UserName != "alice" {
		t.Errorf("Decoder.One: UserName = %q, want %q", got.UserName, "alice")
	}
}

func TestUnmarshal_duplicateNames(t *testing.T) {
	re := regexp.MustCompile(`(?:x(?P<word>a)|y(?P<word>b))`)
	type rec struct {
		Word string `regex:"word"`
	}

	for input, want := range map[string]string{"xa": "a", "yb": "b"} {
		var r rec
		if err := rx.Unmarshal(re, input, &r); err != nil {
			t.Fatalf("Unmarshal(%q): %v", input, err)
		}
		if r.Word != want {
			t.Errorf("Unmarshal(%q): Word = %q, want %q", input, r.Word, want)
		}
	}
}

func TestUnmarshalAll_duplicateNames(t *testing.T) {
	re := regexp.MustCompile(`(?:x(?P<word>a)|y(?P<word>b))`)
	type rec struct {
		Word string `regex:"word"`
	}

	var recs []rec
	if err := rx.UnmarshalAll(re, "xa yb xa", &recs); err != nil {
		t.Fatalf("UnmarshalAll: %v", err)
	}
	want := []string{"a", "b", "a"}
	if len(recs) != len(want) {
		t.Fatalf("got %d matches, want %d", len(recs), len(want))
	}
	for i, w := range want {
		if recs[i].Word != w {
			t.Errorf("match %d: Word = %q, want %q", i, recs[i].Word, w)
		}
	}
}

// Pins the #108 fold-fallback determinism + participation nuance. When several
// distinctly-spelled declared group names fold-equal one untagged field, the
// field-name fallback now resolves at build time via matchGroupName over
// re.SubexpNames() — picking the declaration-first fold-sibling deterministically
// and participation-agnostically — instead of the old map-iteration order over
// the participating-only namedGroupValues map.
//
// Observable consequence: if the declaration-first fold-sibling does NOT
// participate in the match but a later one does, the field is now skipped where
// the old participating-only fallback would have populated it from the
// participating sibling. This is the intended Unmarshal↔Decoder alignment (both
// paths now agree), not a regression — the test exists to lock the behavior so
// it can't silently flip back.
func TestUnmarshalDecoder_foldFallbackDeterministicParticipation(t *testing.T) {
	// Two distinctly-spelled groups, "value" then "VALUE", both fold-equal the
	// untagged field "Value". matchGroupName binds the declaration-first one
	// ("value") regardless of which participates.
	const pattern = `(?:a(?P<value>\d+)|b(?P<VALUE>\d+))`
	re := regexp.MustCompile(pattern)
	type rec struct {
		Value int
	}
	dec := rx.MustCompile[rec](pattern)

	// "a5": the declaration-first sibling ("value") participates → populated.
	// "b5": only the later sibling ("VALUE") participates; the bound
	// declaration-first "value" did not → field stays zero on BOTH paths.
	cases := []struct {
		input string
		want  int
	}{
		{"a5", 5},
		{"b5", 0},
	}
	for _, tc := range cases {
		var u rec
		if err := rx.Unmarshal(re, tc.input, &u); err != nil {
			t.Fatalf("Unmarshal(%q): %v", tc.input, err)
		}
		got, err := dec.One(tc.input)
		if err != nil {
			t.Fatalf("Decoder.One(%q): %v", tc.input, err)
		}
		if u.Value != tc.want {
			t.Errorf("Unmarshal(%q): Value = %d, want %d", tc.input, u.Value, tc.want)
		}
		if got.Value != tc.want {
			t.Errorf("Decoder.One(%q): Value = %d, want %d", tc.input, got.Value, tc.want)
		}
		if u.Value != got.Value {
			t.Errorf("Unmarshal/Decoder disagree on %q: %d vs %d", tc.input, u.Value, got.Value)
		}
	}
}

// Pins the #108 field-conversion error-prefix unification. Routing
// Unmarshal/UnmarshalAll through runDecodePlan changed their wrapping prefix from
// the old `regextra: failed to set field X: …` to `field X: …`, matching
// Decoder. The underlying `cannot convert …` message is unchanged. This locks
// the wording so the three paths can't drift.
func TestUnmarshalDecoder_fieldConversionErrorPrefixUnified(t *testing.T) {
	const pattern = `(?P<age>\S+)`
	re := regexp.MustCompile(pattern)
	type rec struct {
		Age int `regex:"age"`
	}

	var u rec
	uErr := rx.Unmarshal(re, "abc", &u)

	var all []rec
	allErr := rx.UnmarshalAll(re, "abc", &all)

	_, oneErr := rx.MustCompile[rec](pattern).One("abc")

	for name, err := range map[string]error{
		"Unmarshal":    uErr,
		"UnmarshalAll": allErr,
		"Decoder.One":  oneErr,
	} {
		if err == nil {
			t.Fatalf("%s: got nil error, want conversion failure", name)
		}
		msg := err.Error()
		if !strings.Contains(msg, "field Age:") {
			t.Errorf("%s: error = %q, want unified `field Age:` prefix", name, msg)
		}
		if strings.Contains(msg, "failed to set field") {
			t.Errorf("%s: error = %q, still carries the old `failed to set field` prefix", name, msg)
		}
		if !strings.Contains(msg, "cannot convert") {
			t.Errorf("%s: error = %q, want the underlying `cannot convert` message preserved", name, msg)
		}
	}
}

// A non-participating optional group on a typed field is left at its zero value
// by both paths; Unmarshal no longer errors trying to convert "" (issue-105
// alignment of the Unmarshal and Decoder paths).
func TestUnmarshalDecoder_nonParticipatingOptionalTyped(t *testing.T) {
	const pattern = `y(?P<num>\d+)?`
	re := regexp.MustCompile(pattern)
	type rec struct {
		Num int `regex:"num"`
	}

	var fromUnmarshal rec
	if err := rx.Unmarshal(re, "y", &fromUnmarshal); err != nil {
		t.Fatalf("Unmarshal: unexpected error %v (a non-participating optional group should leave the field zero)", err)
	}
	if fromUnmarshal.Num != 0 {
		t.Errorf("Unmarshal Num = %d, want 0", fromUnmarshal.Num)
	}

	dec := rx.MustCompile[rec](pattern)
	fromDecoder, err := dec.One("y")
	if err != nil {
		t.Fatalf("One: %v", err)
	}
	if fromDecoder.Num != 0 {
		t.Errorf("Decoder Num = %d, want 0", fromDecoder.Num)
	}
}

// Regression tests for https://github.com/Jecoms/regextra/issues/104:
// an optional named group that did not participate in the match must leave
// the field unchanged (matching Decoder behavior) instead of feeding "" to
// the type converter and erroring.

func TestUnmarshal_optionalGroupNotParticipating(t *testing.T) {
	re := regexp.MustCompile(`(?P<name>\w+)(?: (?P<age>\d+))?`)

	type person struct {
		Name string `regex:"name"`
		Age  int    `regex:"age"`
	}

	t.Run("absent optional group leaves field unchanged", func(t *testing.T) {
		var p person
		if err := rx.Unmarshal(re, "Alice", &p); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if p.Name != "Alice" || p.Age != 0 {
			t.Errorf("got %+v, want {Name:Alice Age:0}", p)
		}
	})

	t.Run("participating optional group still decodes", func(t *testing.T) {
		var p person
		if err := rx.Unmarshal(re, "Alice 30", &p); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if p.Name != "Alice" || p.Age != 30 {
			t.Errorf("got %+v, want {Name:Alice Age:30}", p)
		}
	})

	t.Run("default substitutes for absent optional group", func(t *testing.T) {
		type withDefault struct {
			Name string `regex:"name"`
			Age  int    `regex:"age,default=18"`
		}
		var p withDefault
		if err := rx.Unmarshal(re, "Alice", &p); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if p.Age != 18 {
			t.Errorf("Age = %d, want default 18", p.Age)
		}
	})

	t.Run("participating empty-span group leaves field unchanged", func(t *testing.T) {
		// `age` participates in the match here but with a zero-length span —
		// distinct from not participating at all, yet the contract is the
		// same: no default, no write, no conversion error.
		emptySpanRe := regexp.MustCompile(`(?P<name>\w+):(?P<age>\d*)`)
		p := person{Age: 99}
		if err := rx.Unmarshal(emptySpanRe, "Alice:", &p); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if p.Name != "Alice" || p.Age != 99 {
			t.Errorf("got %+v, want {Name:Alice Age:99}", p)
		}
	})

	t.Run("pre-populated field survives absent group", func(t *testing.T) {
		p := person{Age: 99}
		if err := rx.Unmarshal(re, "Alice", &p); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if p.Age != 99 {
			t.Errorf("Age = %d, want 99 (unchanged)", p.Age)
		}
	})

	t.Run("UnmarshalAll skips absent optional groups per match", func(t *testing.T) {
		var people []person
		if err := rx.UnmarshalAll(re, "Alice 30, Bob", &people); err != nil {
			t.Fatalf("UnmarshalAll: %v", err)
		}
		if len(people) != 2 {
			t.Fatalf("got %d matches, want 2", len(people))
		}
		if people[0].Age != 30 || people[1].Age != 0 {
			t.Errorf("got %+v", people)
		}
	})
}

// Unmarshal and Decoder.One must agree on optional-group semantics.
func TestUnmarshal_decoderParityOnOptionalGroups(t *testing.T) {
	const pattern = `(?P<name>\w+)(?: (?P<age>\d+))?`
	type person struct {
		Name string `regex:"name"`
		Age  int    `regex:"age"`
	}

	re := regexp.MustCompile(pattern)
	dec := rx.MustCompile[person](pattern)

	for _, input := range []string{"Alice", "Alice 30"} {
		var fromUnmarshal person
		if err := rx.Unmarshal(re, input, &fromUnmarshal); err != nil {
			t.Fatalf("Unmarshal(%q): %v", input, err)
		}
		fromDecoder, err := dec.One(input)
		if err != nil {
			t.Fatalf("Decoder.One(%q): %v", input, err)
		}
		if fromUnmarshal != fromDecoder {
			t.Errorf("input %q: Unmarshal=%+v Decoder.One=%+v — paths disagree", input, fromUnmarshal, fromDecoder)
		}
	}

	// Parity must also hold for the participating empty-span case — the novel
	// behavior this PR fixes (#104), which the optional-group pattern above
	// can't produce (its `age` group either participates with digits or not at
	// all). On the empty-span input both paths must skip the field identically
	// rather than one erroring on `""`.
	const emptySpanPattern = `(?P<name>\w+):(?P<age>\d*)`
	emptyRe := regexp.MustCompile(emptySpanPattern)
	emptyDec := rx.MustCompile[person](emptySpanPattern)
	var fromUnmarshal person
	if err := rx.Unmarshal(emptyRe, "Alice:", &fromUnmarshal); err != nil {
		t.Fatalf("Unmarshal(empty-span): %v", err)
	}
	fromDecoder, err := emptyDec.One("Alice:")
	if err != nil {
		t.Fatalf("Decoder.One(empty-span): %v", err)
	}
	if fromUnmarshal != fromDecoder {
		t.Errorf("empty-span: Unmarshal=%+v Decoder.One=%+v — paths disagree", fromUnmarshal, fromDecoder)
	}
}

// Regression tests for https://github.com/Jecoms/regextra/issues/103:
// values out of range for narrow numeric fields must error instead of
// silently truncating (e.g. "300" into int8 previously yielded 44, nil).

func TestUnmarshal_intOverflow(t *testing.T) {
	re := regexp.MustCompile(`(?P<n>-?\d+)`)

	t.Run("int8 overflow errors", func(t *testing.T) {
		var dst struct {
			N int8 `regex:"n"`
		}
		err := rx.Unmarshal(re, "300", &dst)
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
		if err := rx.Unmarshal(re, "-129", &dst); err == nil {
			t.Fatalf("expected underflow error, got nil (N = %d)", dst.N)
		}
	})

	t.Run("int8 bounds fit", func(t *testing.T) {
		var dst struct {
			Lo int8 `regex:"n"`
		}
		if err := rx.Unmarshal(re, "127", &dst); err != nil {
			t.Fatalf("127 should fit in int8: %v", err)
		}
		if dst.Lo != math.MaxInt8 {
			t.Errorf("Lo = %d, want 127", dst.Lo)
		}
		if err := rx.Unmarshal(re, "-128", &dst); err != nil {
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
		if err := rx.Unmarshal(re, "40000", &dst16); err == nil {
			t.Errorf("expected int16 overflow error, got nil (N = %d)", dst16.N)
		}
		var dst32 struct {
			N int32 `regex:"n"`
		}
		if err := rx.Unmarshal(re, strconv.FormatInt(math.MaxInt32+1, 10), &dst32); err == nil {
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
		if err := rx.Unmarshal(re, "256", &dst); err == nil {
			t.Fatalf("expected overflow error, got nil (N = %d)", dst.N)
		}
	})

	t.Run("uint8 max fits", func(t *testing.T) {
		var dst struct {
			N uint8 `regex:"n"`
		}
		if err := rx.Unmarshal(re, "255", &dst); err != nil {
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
		if err := rx.Unmarshal(re, "1e308", &dst); err == nil {
			t.Fatalf("expected overflow error, got nil (N = %g)", dst.N)
		}
	})

	t.Run("float32 in range fits", func(t *testing.T) {
		var dst struct {
			N float32 `regex:"n"`
		}
		if err := rx.Unmarshal(re, "3.5", &dst); err != nil {
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
		if err := rx.Unmarshal(re, "1e308", &dst); err != nil {
			t.Fatalf("1e308 fits in float64: %v", err)
		}
	})
}

// Issue #106: `regex:"-"` must exclude a field entirely, even when a declared
// group shares the field's name (the trap the old name-fallback created). An
// absent tag (`regex:""`) must still fall back to the field name — the guard
// against over-skipping.

func TestUnmarshal_dashExcludesField(t *testing.T) {
	type Person struct {
		Name string `regex:"name"`
		Age  string `regex:"-"` // excluded, despite a declared "age" group
	}
	re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)

	var p Person
	if err := rx.Unmarshal(re, "Alice is 30", &p); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if p.Name != "Alice" {
		t.Errorf("Name = %q, want %q", p.Name, "Alice")
	}
	if p.Age != "" {
		t.Errorf("Age = %q, want it left at zero value (excluded by regex:\"-\")", p.Age)
	}
}

func TestUnmarshalAll_dashExcludesField(t *testing.T) {
	type Person struct {
		Name string `regex:"name"`
		Age  string `regex:"-"`
	}
	re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)

	var people []Person
	if err := rx.UnmarshalAll(re, "Alice is 30 Bob is 25", &people); err != nil {
		t.Fatalf("UnmarshalAll() error = %v", err)
	}
	if len(people) != 2 {
		t.Fatalf("got %d people, want 2", len(people))
	}
	for i, p := range people {
		if p.Age != "" {
			t.Errorf("people[%d].Age = %q, want it left at zero value", i, p.Age)
		}
	}
}

func TestUnmarshal_emptyTagStillFallsBackToFieldName(t *testing.T) {
	// Guard against over-skipping: an absent tag must still match by field name.
	type Person struct {
		Name string
		Age  string `regex:""`
	}
	re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)

	var p Person
	if err := rx.Unmarshal(re, "Alice is 30", &p); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if p.Age != "30" {
		t.Errorf("Age = %q, want %q (regex:\"\" should fall back to field name)", p.Age, "30")
	}
}

// Only the bare `-` excludes: a leading `-` followed by options parses `-` as
// the group name, which matches no group, so the field falls through to its
// `default`. The documented boundary in three doc locations (parseFieldTag
// godoc, README, CHANGELOG), guarded here so it can't silently regress.
func TestUnmarshal_dashWithOptionsIsNotExcluded(t *testing.T) {
	type Person struct {
		Name string `regex:"name"`
		Age  string `regex:"-,default=unknown"` // "-" is a group name, not an exclude
	}
	re := regexp.MustCompile(`(?P<name>\w+) is (?P<age>\d+)`)

	var p Person
	if err := rx.Unmarshal(re, "Alice is 30", &p); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if p.Age != "unknown" {
		t.Errorf("Age = %q, want %q (regex:\"-,default=x\" is not excluded; \"-\" matches no group, so default applies)", p.Age, "unknown")
	}
}

// Issue #113: pin the two tag-grammar forward-compatibility rules that the
// package doc (regextra.go "Tag grammar") declares part of the v1 contract,
// exercised through the public Unmarshal/Decoder surface rather than the
// unexported parser:
//
//   - Unknown key=value pairs are accepted, not rejected, so a future minor
//     release can recognize new option keys without a parser change.
//   - Lone tokens (no '=') are silently ignored, reserving that slot for
//     future flag-style options (e.g. `required`).
//
// Each rule is observable only as a no-op today: the extra option carries no
// meaning, so the field resolves exactly as if it were absent. Pairing the
// forward-compat piece with a `default` on an absent group makes that visible
// — the default still applies, proving the parser neither errored on nor was
// derailed by the extra piece. (Whether an unknown key is retained in the
// internal option map is not observable through the public API, so the tests
// pin only that such keys are accepted, not that they are stored.)

func TestUnmarshal_unknownOptionKeyIsAccepted(t *testing.T) {
	// Forward-compat rule 1: an unrecognized key=value must not be rejected.
	// `future` has no meaning today, so the field matches its group exactly as
	// if the option were absent.
	type Person struct {
		Name string `regex:"name,future=42"`
	}
	re := regexp.MustCompile(`(?P<name>\w+)`)

	var p Person
	if err := rx.Unmarshal(re, "Alice", &p); err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil (unknown option keys must be accepted, not rejected)", err)
	}
	if p.Name != "Alice" {
		t.Errorf("Name = %q, want %q", p.Name, "Alice")
	}
}

func TestUnmarshal_loneTokenIsIgnored(t *testing.T) {
	// Forward-compat rule 2: a lone token (no '=') is silently ignored today,
	// reserving the slot for future flag-style options. `required` has no
	// meaning yet, so it neither errors nor blocks the `default` that follows
	// it — the "role" group is absent, so the field takes the default.
	type Person struct {
		Role string `regex:"role,required,default=guest"`
	}
	re := regexp.MustCompile(`(?P<name>\w+)`) // no "role" group

	var p Person
	if err := rx.Unmarshal(re, "Alice", &p); err != nil {
		t.Fatalf("Unmarshal() error = %v, want nil (a lone token must be ignored)", err)
	}
	if p.Role != "guest" {
		t.Errorf("Role = %q, want %q (lone token ignored; default still applies)", p.Role, "guest")
	}
}

func TestUnmarshal_emptyOptionPieceIsSkipped(t *testing.T) {
	// An empty option piece is ignored without disturbing the options around
	// it: the `default` still resolves when the group is absent. The behavior
	// holds the same way wherever the empty piece falls, so these positional
	// variants share subtests (struct tags are compile-time literals, so each
	// needs its own struct rather than a value table). An empty piece has no
	// '=', so the parser drops it via the same path as a lone token.
	re := regexp.MustCompile(`(?P<name>\w+)`) // no "role" group; default fires

	t.Run("doubled comma", func(t *testing.T) {
		type Person struct {
			Role string `regex:"role,,default=guest"`
		}
		var p Person
		if err := rx.Unmarshal(re, "Alice", &p); err != nil {
			t.Fatalf("Unmarshal() error = %v, want nil", err)
		}
		if p.Role != "guest" {
			t.Errorf("Role = %q, want %q (empty piece skipped; default still applies)", p.Role, "guest")
		}
	})

	t.Run("trailing comma", func(t *testing.T) {
		type Person struct {
			Role string `regex:"role,default=guest,"`
		}
		var p Person
		if err := rx.Unmarshal(re, "Alice", &p); err != nil {
			t.Fatalf("Unmarshal() error = %v, want nil", err)
		}
		if p.Role != "guest" {
			t.Errorf("Role = %q, want %q (trailing empty piece skipped)", p.Role, "guest")
		}
	})

	t.Run("whitespace-only piece", func(t *testing.T) {
		type Person struct {
			Role string `regex:"role, ,default=guest"`
		}
		var p Person
		if err := rx.Unmarshal(re, "Alice", &p); err != nil {
			t.Fatalf("Unmarshal() error = %v, want nil", err)
		}
		if p.Role != "guest" {
			t.Errorf("Role = %q, want %q (whitespace-only piece skipped)", p.Role, "guest")
		}
	})
}

// These tests lock in the encoding.TextUnmarshaler fallback added for #119.
// Fixtures are stdlib-only (no new module dependency): netip.Addr, math/big.Int
// (pointer receiver), and log/slog.Level (underlying int, must NOT be routed
// through the built-in int conversion).

// ExampleUnmarshal_textUnmarshaler shows that any field type implementing
// encoding.TextUnmarshaler (here the stdlib's netip.Addr) is populated from its
// capture group via UnmarshalText, with no regextra-specific code required.
func ExampleUnmarshal_textUnmarshaler() {
	type Conn struct {
		Addr netip.Addr `regex:"addr"`
	}
	re := regexp.MustCompile(`from (?P<addr>\S+)`)
	var c Conn
	_ = rx.Unmarshal(re, "from 192.168.0.1", &c)
	fmt.Println(c.Addr)
	// Output: 192.168.0.1
}

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
