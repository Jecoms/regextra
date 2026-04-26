package regextra_test

import (
	"fmt"
	rx "github.com/jecoms/regextra"
	"reflect"
	"regexp"
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
