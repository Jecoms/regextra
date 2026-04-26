package regextra_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	rx "github.com/jecoms/regextra"
)

// ── Compile: happy paths ──────────────────────────────────────────────────────

func TestCompile_simpleStruct(t *testing.T) {
	type P struct {
		Name string `regex:"name"`
		Age  int    `regex:"age"`
	}
	d, err := rx.Compile[P](`(?P<name>\w+) is (?P<age>\d+)`)
	if err != nil {
		t.Fatalf("Compile returned %v", err)
	}
	if d.Pattern() != `(?P<name>\w+) is (?P<age>\d+)` {
		t.Errorf("Pattern() = %q, want the source string", d.Pattern())
	}
}

func TestCompile_fieldNameFallback(t *testing.T) {
	type P struct {
		Name string // no tag — should match group "name" via case-insensitive fallback
		Age  int
	}
	if _, err := rx.Compile[P](`(?P<name>\w+) is (?P<age>\d+)`); err != nil {
		t.Fatalf("Compile returned %v on field-name fallback: %v", err, err)
	}
}

// ── Compile: error paths ──────────────────────────────────────────────────────

func TestCompile_invalidPattern(t *testing.T) {
	type P struct {
		X string `regex:"x"`
	}
	_, err := rx.Compile[P](`[unclosed`)
	if err == nil {
		t.Fatal("Compile returned nil for invalid pattern, want error")
	}
	if !strings.Contains(err.Error(), "invalid pattern") {
		t.Errorf("error = %q, want it to mention 'invalid pattern'", err.Error())
	}
}

func TestCompile_notAStruct(t *testing.T) {
	_, err := rx.Compile[string](`(?P<x>\w+)`)
	if err == nil {
		t.Fatal("Compile returned nil for non-struct T, want error")
	}
	if !strings.Contains(err.Error(), "must be a struct") {
		t.Errorf("error = %q, want it to mention 'must be a struct'", err.Error())
	}
}

func TestCompile_undeclaredGroup(t *testing.T) {
	type P struct {
		Name string `regex:"missing"`
	}
	_, err := rx.Compile[P](`(?P<name>\w+)`)
	if err == nil {
		t.Fatal("Compile returned nil for undeclared group reference, want error")
	}
	if !strings.Contains(err.Error(), "missing") || !strings.Contains(err.Error(), "not declared") {
		t.Errorf("error = %q, want it to name the missing group and 'not declared'", err.Error())
	}
}

func TestCompile_badDefault(t *testing.T) {
	type P struct {
		Age int `regex:"age,default=notanumber"`
	}
	_, err := rx.Compile[P](`(?P<age>\d+)`)
	if err == nil {
		t.Fatal("Compile returned nil for malformed default, want error")
	}
	if !strings.Contains(err.Error(), "default") {
		t.Errorf("error = %q, want it to mention 'default'", err.Error())
	}
}

func TestCompile_layoutOnNonTimeField(t *testing.T) {
	type P struct {
		Name string `regex:"name,layout=2006-01-02"`
	}
	_, err := rx.Compile[P](`(?P<name>\w+)`)
	if err == nil {
		t.Fatal("Compile returned nil for layout on non-time field, want error")
	}
	if !strings.Contains(err.Error(), "layout") || !strings.Contains(err.Error(), "time.Time") {
		t.Errorf("error = %q, want it to mention 'layout' and 'time.Time'", err.Error())
	}
}

func TestCompile_layoutOnPointerTimeField(t *testing.T) {
	// *time.Time should be allowed for layout=
	type P struct {
		TS *time.Time `regex:"ts,layout=2006-01-02"`
	}
	if _, err := rx.Compile[P](`(?P<ts>\S+)`); err != nil {
		t.Fatalf("Compile returned %v on *time.Time + layout=, expected no error", err)
	}
}

// ── MustCompile ───────────────────────────────────────────────────────────────

func TestMustCompile_panicsOnBadPattern(t *testing.T) {
	type P struct {
		X string `regex:"x"`
	}
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustCompile did not panic on invalid pattern")
		}
	}()
	_ = rx.MustCompile[P](`[unclosed`)
}

func TestMustCompile_returnsDecoderOnSuccess(t *testing.T) {
	type P struct {
		Name string `regex:"name"`
	}
	d := rx.MustCompile[P](`(?P<name>\w+)`)
	if d == nil {
		t.Fatal("MustCompile returned nil")
	}
}

// ── One ───────────────────────────────────────────────────────────────────────

func TestDecoder_One_match(t *testing.T) {
	type P struct {
		Name string `regex:"name"`
		Age  int    `regex:"age"`
	}
	d := rx.MustCompile[P](`(?P<name>\w+) is (?P<age>\d+)`)
	got, err := d.One("Alice is 30")
	if err != nil {
		t.Fatalf("One returned %v", err)
	}
	if got.Name != "Alice" || got.Age != 30 {
		t.Errorf("One = %+v, want {Name: Alice, Age: 30}", got)
	}
}

func TestDecoder_One_noMatchReturnsSentinel(t *testing.T) {
	type P struct {
		Name string `regex:"name"`
	}
	d := rx.MustCompile[P](`(?P<name>\d+)`)
	got, err := d.One("nodigits")
	if !errors.Is(err, rx.ErrNoMatch) {
		t.Errorf("One error = %v, want ErrNoMatch (or wrapping it)", err)
	}
	var zero P
	if got != zero {
		t.Errorf("One = %+v, want zero value on no-match", got)
	}
}

func TestDecoder_One_conversionError(t *testing.T) {
	// Force a conversion error by matching a non-numeric value into an int.
	type P struct {
		Age int `regex:"age"`
	}
	d := rx.MustCompile[P](`(?P<age>\S+)`)
	_, err := d.One("abc")
	if err == nil {
		t.Fatal("One returned nil error, want conversion failure")
	}
	if !strings.Contains(err.Error(), "Age") {
		t.Errorf("error = %q, want it to name the field 'Age'", err.Error())
	}
}

// ── All ───────────────────────────────────────────────────────────────────────

func TestDecoder_All_multipleMatches(t *testing.T) {
	type Entry struct {
		Name string `regex:"name"`
		Age  int    `regex:"age"`
	}
	d := rx.MustCompile[Entry](`(?P<name>\w+) is (?P<age>\d+)`)
	got, err := d.All("Alice is 30 and Bob is 25 and Carol is 40")
	if err != nil {
		t.Fatalf("All returned %v", err)
	}
	want := []Entry{{"Alice", 30}, {"Bob", 25}, {"Carol", 40}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("All = %+v, want %+v", got, want)
	}
}

func TestDecoder_All_noMatchesReturnsEmptyNotNil(t *testing.T) {
	type E struct {
		Name string `regex:"name"`
	}
	d := rx.MustCompile[E](`(?P<name>\d+)`)
	got, err := d.All("nodigits here")
	if err != nil {
		t.Fatalf("All returned %v", err)
	}
	if got == nil {
		t.Error("All returned nil, want empty non-nil slice")
	}
	if len(got) != 0 {
		t.Errorf("All returned %v, want length 0", got)
	}
}

func TestDecoder_All_conversionErrorMidIteration(t *testing.T) {
	type E struct {
		Age int `regex:"age"`
	}
	d := rx.MustCompile[E](`age=(?P<age>\S+)`)
	got, err := d.All("age=10 age=bad age=20")
	if err == nil {
		t.Fatal("All returned nil error, want conversion failure on second match")
	}
	if !strings.Contains(err.Error(), "match 1") {
		t.Errorf("error = %q, want it to indicate which match failed", err.Error())
	}
	// First match should be in the result; the failed match is also there.
	if len(got) != 2 || got[0].Age != 10 {
		t.Errorf("got = %+v, want first match decoded plus failure entry", got)
	}
}

// ── Default + layout interaction (proves Decoder reuses setFieldValue) ──────

func TestDecoder_default(t *testing.T) {
	type P struct {
		Name string `regex:"name"`
		Role string `regex:"role,default=guest"`
	}
	d := rx.MustCompile[P](`(?P<name>\w+)`)
	got, err := d.One("Mallory")
	if err != nil {
		t.Fatalf("One returned %v", err)
	}
	if got.Role != "guest" {
		t.Errorf("Role = %q, want guest (default applied at decode)", got.Role)
	}
}

func TestDecoder_layout(t *testing.T) {
	type Log struct {
		TS time.Time `regex:"ts,layout=02/Jan/2006:15:04:05 -0700"`
	}
	d := rx.MustCompile[Log](`\[(?P<ts>[^\]]+)\]`)
	got, err := d.One("[26/Apr/2026:12:34:56 -0500]")
	if err != nil {
		t.Fatalf("One returned %v", err)
	}
	if got.TS.Year() != 2026 || got.TS.Month() != time.April {
		t.Errorf("TS = %v, want 2026-04-26 ...", got.TS)
	}
}

// ── Pointer + RegexUnmarshaler reuse ─────────────────────────────────────────

func TestDecoder_pointerFields(t *testing.T) {
	type P struct {
		Name *string `regex:"name"`
	}
	d := rx.MustCompile[P](`(?P<name>\w+)`)
	got, err := d.One("Alice")
	if err != nil {
		t.Fatalf("One returned %v", err)
	}
	if got.Name == nil || *got.Name != "Alice" {
		t.Errorf("*Name = %v, want Alice", got.Name)
	}
}

type decoderStatus int

const (
	decoderStatusUnknown decoderStatus = iota
	decoderStatusOpen
	decoderStatusClosed
)

func (s *decoderStatus) UnmarshalRegex(value string) error {
	switch value {
	case "open":
		*s = decoderStatusOpen
	case "closed":
		*s = decoderStatusClosed
	default:
		return fmt.Errorf("unknown status %q", value)
	}
	return nil
}

func TestDecoder_regexUnmarshaler(t *testing.T) {
	type Issue struct {
		State decoderStatus `regex:"state"`
	}
	d := rx.MustCompile[Issue](`\[(?P<state>\w+)\]`)
	got, err := d.One("[open]")
	if err != nil {
		t.Fatalf("One returned %v", err)
	}
	if got.State != decoderStatusOpen {
		t.Errorf("State = %d, want decoderStatusOpen (%d)", got.State, decoderStatusOpen)
	}
}

// ── Concurrency safety ───────────────────────────────────────────────────────

func TestDecoder_concurrentUseSafe(t *testing.T) {
	type P struct {
		Name string `regex:"name"`
		Age  int    `regex:"age"`
	}
	d := rx.MustCompile[P](`(?P<name>\w+) is (?P<age>\d+)`)

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			input := fmt.Sprintf("user%d is %d", seed, seed*2)
			got, err := d.One(input)
			if err != nil {
				t.Errorf("goroutine %d: One returned %v", seed, err)
				return
			}
			if got.Age != seed*2 {
				t.Errorf("goroutine %d: Age = %d, want %d", seed, got.Age, seed*2)
			}
		}(i)
	}
	wg.Wait()
}

// ── Examples ────────────────────────────────────────────────────────────────

func ExampleCompile() {
	type Person struct {
		Name string `regex:"name"`
		Age  int    `regex:"age"`
	}
	dec, err := rx.Compile[Person](`(?P<name>\w+) is (?P<age>\d+)`)
	if err != nil {
		fmt.Println(err)
		return
	}
	p, err := dec.One("Alice is 30")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%s, %d\n", p.Name, p.Age)
	// Output: Alice, 30
}

func ExampleMustCompile() {
	type Person struct {
		Name string `regex:"name"`
	}
	// Package-level vars commonly use MustCompile so a typo fails the build.
	var dec = rx.MustCompile[Person](`(?P<name>\w+)`)
	p, _ := dec.One("Alice")
	fmt.Println(p.Name)
	// Output: Alice
}

func ExampleDecoder_All() {
	type Entry struct {
		Name string `regex:"name"`
		Age  int    `regex:"age"`
	}
	dec := rx.MustCompile[Entry](`(?P<name>\w+) is (?P<age>\d+)`)
	people, _ := dec.All("Alice is 30 and Bob is 25")
	for _, p := range people {
		fmt.Printf("%s/%d\n", p.Name, p.Age)
	}
	// Output:
	// Alice/30
	// Bob/25
}

func ExampleDecoder_Iter() {
	type Entry struct {
		Name string `regex:"name"`
		Age  int    `regex:"age"`
	}
	dec := rx.MustCompile[Entry](`(?P<name>\w+) is (?P<age>\d+)`)
	for p, err := range dec.Iter("Alice is 30 and Bob is 25") {
		if err != nil {
			continue
		}
		fmt.Printf("%s/%d\n", p.Name, p.Age)
	}
	// Output:
	// Alice/30
	// Bob/25
}

// ── Iter ──────────────────────────────────────────────────────────────────────

func TestDecoder_Iter_yieldsEveryMatch(t *testing.T) {
	type E struct {
		Name string `regex:"name"`
		Age  int    `regex:"age"`
	}
	dec := rx.MustCompile[E](`(?P<name>\w+) is (?P<age>\d+)`)
	var got []E
	for v, err := range dec.Iter("Alice is 30 and Bob is 25 and Carol is 40") {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got = append(got, v)
	}
	want := []E{{"Alice", 30}, {"Bob", 25}, {"Carol", 40}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Iter collected %+v, want %+v", got, want)
	}
}

func TestDecoder_Iter_noMatchesYieldsNothing(t *testing.T) {
	type E struct {
		Name string `regex:"name"`
	}
	dec := rx.MustCompile[E](`(?P<name>\d+)`)
	count := 0
	for range dec.Iter("nodigits here") {
		count++
	}
	if count != 0 {
		t.Errorf("Iter yielded %d times on no-match input, want 0", count)
	}
}

func TestDecoder_Iter_breakStopsEarly(t *testing.T) {
	type E struct {
		Name string `regex:"name"`
	}
	dec := rx.MustCompile[E](`(?P<name>\w+)`)
	count := 0
	for v := range dec.Iter("alpha beta gamma delta") {
		count++
		if v.Name == "beta" {
			break
		}
	}
	if count != 2 {
		t.Errorf("Iter yielded %d times before break, want 2", count)
	}
}

func TestDecoder_Iter_continuesPastErrors(t *testing.T) {
	type E struct {
		Age int `regex:"age"`
	}
	dec := rx.MustCompile[E](`age=(?P<age>\S+)`)
	var oks []E
	var errs int
	for v, err := range dec.Iter("age=10 age=bad age=20") {
		if err != nil {
			errs++
			continue
		}
		oks = append(oks, v)
	}
	if errs != 1 {
		t.Errorf("got %d errors, want 1 (the bad middle match)", errs)
	}
	want := []E{{Age: 10}, {Age: 20}}
	if !reflect.DeepEqual(oks, want) {
		t.Errorf("got %+v, want %+v", oks, want)
	}
}
