package regextra_test

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
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

// An untagged field whose Go name equals a declared group exactly (same case)
// resolves via matchGroupName's exact-match branch, before the case-insensitive
// fold is consulted. TestCompile_fieldNameFallback exercises only the fold path
// (field Name → group name), so this locks in the exact arm.
func TestCompile_fieldNameExactMatch(t *testing.T) {
	type P struct {
		Name string // no tag — matches group "Name" exactly (not via case-fold)
	}
	d, err := rx.Compile[P](`(?P<Name>\w+)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	p, err := d.One("Alice")
	if err != nil {
		t.Fatalf("One() error = %v", err)
	}
	if p.Name != "Alice" {
		t.Errorf("Name = %q, want %q (exact field-name → group match)", p.Name, "Alice")
	}
}

// Typed-path sibling of TestUnmarshal_unsupportedFieldType: a field whose kind
// setFieldValue can't convert (a nested struct here; a slice or map behaves the
// same) and which implements neither RegexUnmarshaler nor encoding.TextUnmarshaler
// passes Compile (which validates tags, not field kinds) and surfaces the
// "unsupported field type" arm as a *DecodeError at decode time via One.
func TestDecoder_unsupportedFieldType(t *testing.T) {
	type Nested struct{ X string }
	type rec struct {
		Field Nested `regex:"field"`
	}

	d, err := rx.Compile[rec](`(?P<field>\w+)`)
	if err != nil {
		t.Fatalf("Compile() error = %v, want nil (field kinds are validated at decode, not compile)", err)
	}
	_, err = d.One("hello")
	if err == nil {
		t.Fatal("expected an error for a struct-typed field, got nil")
	}
	var de *rx.DecodeError
	if !errors.As(err, &de) {
		t.Fatalf("error %q is not a *DecodeError", err)
	}
	if de.Field != "Field" {
		t.Errorf("DecodeError.Field = %q, want %q", de.Field, "Field")
	}
	if de.Group != "field" {
		t.Errorf("DecodeError.Group = %q, want %q", de.Group, "field")
	}
	if !strings.Contains(de.Unwrap().Error(), "unsupported field type") {
		t.Errorf("underlying error %q, want it to mention %q", de.Unwrap(), "unsupported field type")
	}
}

// Typed-path sibling of TestUnmarshal_untaggedFieldNoMatchingGroupSkipped: an
// untagged exported field matching no declared group, with no default= to fall
// back on, is skipped by the decode plan (buildDecodePlan, shared with Unmarshal)
// rather than failing Compile — only a *tagged* missing-group reference is a
// strict-build error. The field is left at its zero value.
func TestDecoder_untaggedFieldNoMatchingGroupSkipped(t *testing.T) {
	type rec struct {
		Name    string `regex:"name"`
		Missing string // no tag, no "missing"/"Missing" group, no default — skipped
	}

	d, err := rx.Compile[rec](`(?P<name>\w+)`)
	if err != nil {
		t.Fatalf("Compile() error = %v, want nil (unmapped untagged field should be skipped)", err)
	}
	got, err := d.One("Alice")
	if err != nil {
		t.Fatalf("One() error = %v, want nil", err)
	}
	if got.Name != "Alice" {
		t.Errorf("Name = %q, want %q", got.Name, "Alice")
	}
	if got.Missing != "" {
		t.Errorf("Missing = %q, want it left at the zero value %q", got.Missing, "")
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

func TestDecoder_duplicateNames(t *testing.T) {
	type rec struct {
		Word string `regex:"word"`
	}
	dec := rx.MustCompile[rec](`(?:x(?P<word>a)|y(?P<word>b))`)

	t.Run("One finds the participating occurrence in either branch", func(t *testing.T) {
		for input, want := range map[string]string{"xa": "a", "yb": "b"} {
			got, err := dec.One(input)
			if err != nil {
				t.Fatalf("One(%q): %v", input, err)
			}
			if got.Word != want {
				t.Errorf("One(%q): Word = %q, want %q", input, got.Word, want)
			}
		}
	})

	t.Run("All decodes mixed branches", func(t *testing.T) {
		got, err := dec.All("xa yb")
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		if len(got) != 2 || got[0].Word != "a" || got[1].Word != "b" {
			t.Errorf("got %+v, want [{a} {b}]", got)
		}
	})

	t.Run("sequential duplicates agree with Unmarshal (last wins)", func(t *testing.T) {
		const pattern = `(?P<word>\w+) (?P<word>\w+)`
		seqDec := rx.MustCompile[rec](pattern)
		fromDecoder, err := seqDec.One("hello world")
		if err != nil {
			t.Fatalf("One: %v", err)
		}
		var fromUnmarshal rec
		if err := rx.Unmarshal(regexp.MustCompile(pattern), "hello world", &fromUnmarshal); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if fromDecoder.Word != fromUnmarshal.Word {
			t.Errorf("paths disagree: Decoder=%q Unmarshal=%q", fromDecoder.Word, fromUnmarshal.Word)
		}
		if fromDecoder.Word != "world" {
			t.Errorf("Word = %q, want %q", fromDecoder.Word, "world")
		}
	})
}

// Issue-105 follow-up: when a duplicate group name's last occurrence
// participates with an *empty* span, the Decoder and the Unmarshal/NamedGroups
// paths must agree (the Decoder previously read the first occurrence's value).
func TestDecoderUnmarshal_participatingEmptyDuplicate(t *testing.T) {
	const pattern = `(?P<word>a)(?P<word>b*)`
	re := regexp.MustCompile(pattern)
	type rec struct {
		Word string `regex:"word"`
	}

	dec := rx.MustCompile[rec](pattern)
	fromDecoder, err := dec.One("a")
	if err != nil {
		t.Fatalf("One: %v", err)
	}
	fromMap := rx.NamedGroups(re, "a")["word"]
	var fromUnmarshal rec
	if err := rx.Unmarshal(re, "a", &fromUnmarshal); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if fromDecoder.Word != fromMap || fromDecoder.Word != fromUnmarshal.Word {
		t.Errorf("paths disagree: Decoder=%q NamedGroups=%q Unmarshal=%q (want all equal)",
			fromDecoder.Word, fromMap, fromUnmarshal.Word)
	}
	if fromDecoder.Word != "" {
		t.Errorf("Word = %q, want \"\" (the empty b* span is the last participating occurrence)", fromDecoder.Word)
	}
}

// The Decoder shares setFieldValue, so overflow must error there too — both
// at decode time and when validating a default= at Compile time.
func TestDecoder_overflow(t *testing.T) {
	t.Run("One errors on overflow", func(t *testing.T) {
		type rec struct {
			N int8 `regex:"n"`
		}
		dec := rx.MustCompile[rec](`(?P<n>\d+)`)
		if _, err := dec.One("300"); err == nil {
			t.Fatal("expected overflow error, got nil")
		}
	})

	t.Run("Compile rejects out-of-range default", func(t *testing.T) {
		type rec struct {
			N int8 `regex:"n,default=300"`
		}
		if _, err := rx.Compile[rec](`(?P<n>\d+)`); err == nil {
			t.Fatal("expected Compile error for out-of-range default, got nil")
		}
	})
}

func TestDecoder_dashExcludesField(t *testing.T) {
	type Person struct {
		Name string `regex:"name"`
		Age  string `regex:"-"`
	}
	d, err := rx.Compile[Person](`(?P<name>\w+) is (?P<age>\d+)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	t.Run("One", func(t *testing.T) {
		p, err := d.One("Alice is 30")
		if err != nil {
			t.Fatalf("One() error = %v", err)
		}
		if p.Name != "Alice" {
			t.Errorf("Name = %q, want %q", p.Name, "Alice")
		}
		if p.Age != "" {
			t.Errorf("Age = %q, want it left at zero value (excluded by regex:\"-\")", p.Age)
		}
	})

	t.Run("All", func(t *testing.T) {
		ps, err := d.All("Alice is 30 Bob is 25")
		if err != nil {
			t.Fatalf("All() error = %v", err)
		}
		if len(ps) != 2 {
			t.Fatalf("got %d people, want 2", len(ps))
		}
		for i, p := range ps {
			if p.Age != "" {
				t.Errorf("ps[%d].Age = %q, want it left at zero value", i, p.Age)
			}
		}
	})

	t.Run("Iter", func(t *testing.T) {
		var n int
		for p, err := range d.Iter("Alice is 30 Bob is 25") {
			if err != nil {
				t.Fatalf("Iter() error = %v", err)
			}
			if p.Age != "" {
				t.Errorf("people[%d].Age = %q, want it left at zero value", n, p.Age)
			}
			n++
		}
		if n != 2 {
			t.Fatalf("got %d people, want 2", n)
		}
	})
}

// A field tagged regex:"-" must not trigger Compile's undeclared-group error
// even when no group of that name exists — it is excluded before resolution.
func TestCompile_dashFieldNeedsNoGroup(t *testing.T) {
	type Person struct {
		Name     string `regex:"name"`
		Internal string `regex:"-"` // no "internal"/"Internal" group on the pattern
	}
	if _, err := rx.Compile[Person](`(?P<name>\w+)`); err != nil {
		t.Fatalf("Compile() error = %v, want nil (regex:\"-\" field should be excluded)", err)
	}
}

// parseFieldTag feeds the Decoder/Compile path too, so the same forward-compat
// no-ops must hold there. Parsing happens once at compile time and One/All/Iter
// share that result, so a single One probe is sufficient parity coverage.
func TestDecoder_tagForwardCompatRules(t *testing.T) {
	type Person struct {
		Name string `regex:"name,future=42"`              // unknown key: accepted, not rejected
		Role string `regex:"role,required,default=guest"` // lone token: ignored; default applies
	}
	d, err := rx.Compile[Person](`(?P<name>\w+)`) // no "role" group; default fires
	if err != nil {
		t.Fatalf("Compile() error = %v, want nil (forward-compat options must not break compilation)", err)
	}

	p, err := d.One("Alice")
	if err != nil {
		t.Fatalf("One() error = %v", err)
	}
	if p.Name != "Alice" {
		t.Errorf("Name = %q, want %q", p.Name, "Alice")
	}
	if p.Role != "guest" {
		t.Errorf("Role = %q, want %q (lone token ignored; default applies on the Decoder path)", p.Role, "guest")
	}
}

// TestDecoderDecodeError verifies the typed decode path surfaces an
// errors.As-able *DecodeError with Field/Group/Value/Type populated, wrapped
// under each entrypoint's regextra.<Entrypoint>: prefix (asserted loosely per
// the Stability contract).
func TestDecoderDecodeError(t *testing.T) {
	type Person struct {
		Age int `regex:"age"`
	}
	dec := rx.MustCompile[Person](`(?P<age>\w+)`)

	assertDecodeErr := func(t *testing.T, err error, wantPrefix, wantValue string) {
		t.Helper()
		if err == nil {
			t.Fatal("got nil error, want a conversion failure")
		}
		var de *rx.DecodeError
		if !errors.As(err, &de) {
			t.Fatalf("error %q is not a *DecodeError", err)
		}
		if de.Field != "Age" || de.Group != "age" || de.Value != wantValue || de.Type != "int" {
			t.Errorf("DecodeError = %+v, want Field=Age Group=age Value=%q Type=int", de, wantValue)
		}
		if de.Unwrap() == nil {
			t.Error("DecodeError.Unwrap() = nil, want the underlying cause")
		}
		if !strings.HasPrefix(err.Error(), wantPrefix) {
			t.Errorf("error = %q, want prefix %q", err.Error(), wantPrefix)
		}
	}

	t.Run("One", func(t *testing.T) {
		_, err := dec.One("notanumber")
		assertDecodeErr(t, err, "regextra.Decoder.One:", "notanumber")
	})

	t.Run("All", func(t *testing.T) {
		_, err := dec.All("12 bad")
		assertDecodeErr(t, err, "regextra.Decoder.All:", "bad")
	})

	t.Run("Iter", func(t *testing.T) {
		var iterErr error
		for _, err := range dec.Iter("bad") {
			iterErr = err
		}
		assertDecodeErr(t, iterErr, "regextra.Decoder.Iter:", "bad")
	})
}

// ── Shared decode-plan extraction (#108) ──────────────────────────────────────

// Locks the extracted shared decode core against drift. The #108 refactor split
// the Decoder's compile path into compileDecoder→buildDecodePlan and the
// One/All/Iter decode path into Decoder.decode→runDecodePlan, then routed
// Unmarshal/UnmarshalAll through the very same buildDecodePlan/runDecodePlan.
// This test exercises one struct that touches several plan behaviors at once —
// a defaulted field, a duplicate group name (last-participating-occurrence
// wins), and a typed conversion — and asserts every path that runs the shared
// core (Decoder.One, Decoder.All, Decoder.Iter, Unmarshal, UnmarshalAll) decodes
// each match identically. If the extracted compile or decode core diverges for
// any path, this fails.
func TestSharedDecodePlan_AllPathsAgree(t *testing.T) {
	type rec struct {
		Word string `regex:"word"`               // duplicate group name across branches
		Age  int    `regex:"age"`                // typed conversion
		Role string `regex:"role,default=guest"` // default fires (no "role" group)
	}
	// Two branches reuse "word"; only one participates per match. "age" is a
	// plain typed group. There is no "role" group, so the default applies.
	const pattern = `(?:x(?P<word>\w+)|y(?P<word>\w+)) (?P<age>\d+)`
	re := regexp.MustCompile(pattern)
	const input = "xalice 30 ybob 25"

	want := []rec{
		{Word: "alice", Age: 30, Role: "guest"},
		{Word: "bob", Age: 25, Role: "guest"},
	}

	dec := rx.MustCompile[rec](pattern)

	// Decoder.All
	all, err := dec.All(input)
	if err != nil {
		t.Fatalf("Decoder.All: %v", err)
	}
	if !reflect.DeepEqual(all, want) {
		t.Errorf("Decoder.All = %+v, want %+v", all, want)
	}

	// Decoder.One decodes the first match like the first All entry.
	one, err := dec.One(input)
	if err != nil {
		t.Fatalf("Decoder.One: %v", err)
	}
	if !reflect.DeepEqual(one, want[0]) {
		t.Errorf("Decoder.One = %+v, want %+v", one, want[0])
	}

	// Decoder.Iter yields the same sequence as All.
	var iter []rec
	for v, err := range dec.Iter(input) {
		if err != nil {
			t.Fatalf("Decoder.Iter: %v", err)
		}
		iter = append(iter, v)
	}
	if !reflect.DeepEqual(iter, want) {
		t.Errorf("Decoder.Iter = %+v, want %+v", iter, want)
	}

	// Unmarshal (first match) and UnmarshalAll (every match) run the same
	// shared plan and must agree field-for-field with the Decoder paths.
	var u rec
	if err := rx.Unmarshal(re, input, &u); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(u, want[0]) {
		t.Errorf("Unmarshal = %+v, want %+v", u, want[0])
	}

	var ua []rec
	if err := rx.UnmarshalAll(re, input, &ua); err != nil {
		t.Fatalf("UnmarshalAll: %v", err)
	}
	if !reflect.DeepEqual(ua, want) {
		t.Errorf("UnmarshalAll = %+v, want %+v", ua, want)
	}
}
