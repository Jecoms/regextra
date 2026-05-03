# v1.0 API Readiness Review

A focused review of every public surface in `regextra` before stamping v1.0. Each item carries a verdict in one of three buckets:

- **Keep** — ship as-is. Documenting current behavior may still be required.
- **Change before v1** — needs a decision or fix before the stability promise lands. Tracked as a blocker bead.
- **Nice to have** — additive improvement. Can ship pre- or post-v1; will not block the stamp.

The review covers `regextra.go`, `unmarshal.go`, and `decoder.go` as of the `polecat/obsidian/re-dh8` branch.

---

## Functions

### `FindNamed(re *regexp.Regexp, target, groupName string) (string, bool)`

**Verdict: Keep.**

The `(value, ok)` shape is the right call here. Stdlib precedent is split — `regexp.FindStringSubmatch` returns `[]string` (nil = no match), `strconv.ParseBool` returns `(bool, error)` — but `(value, ok)` is the established Go idiom for "lookup that may miss with no further detail" (`map[k]`, type assertions). Returning `error` would force callers into `if err != nil` for a control-flow case that is not actually exceptional.

Two failure modes are conflated into `false`: (a) the group name is not declared on the regex, (b) the regex didn't match. Distinguishing them would require a third return or an error. The cost-benefit doesn't pay off — callers who care about (a) can use `Validate` at startup, and callers who care about (b) can do their own `re.MatchString` check.

### `FindAllNamed(re *regexp.Regexp, target, groupName string) []string`

**Verdict: Keep.**

The `nil` vs `[]string{}` distinction is meaningful and intentional:

- `nil` — group name is not declared on the regex (programmer error)
- `[]string{}` — group is declared but the regex has no matches in the target (data absence)

This lets callers write `if words == nil { /* fix the pattern */ }` separately from `for _, w := range words { /* iterate over zero or more */ }`. The current docstring already documents this, but the README's reference for this function buries it; the README copy is fine for v1, no change needed.

### `NamedGroups(re *regexp.Regexp, target string) map[string]string`

**Verdict: Keep.**

Empty-map-on-no-match is correct. A nil map would also work for callers using `len()` and ranging, but returning an initialized map means callers can immediately do `m["name"]` reads without a nil check or assignment to a fresh map. Matches Go's general "nil-safe but never-nil-by-design" pattern for collection-returning helpers.

Documented behavior already covers the no-match case. No action.

### `AllNamedGroups(re *regexp.Regexp, target string) map[string][]string`

**Verdict: Change before v1 (filed: see "Open blockers" below).**

The function name reads as "all matches across the target", but the implementation calls `FindStringSubmatch` (single match) and the slice values come from duplicate-group-name handling within that one match. The example in the doc comment makes this obvious if you read carefully — `(?P<word>\w+) (?P<word>\w+)` against `"hello world"` yields `{"word": ["hello", "world"]}` because `word` is declared twice in the pattern, not because there are two matches in the string.

This is a v1 concern because:

1. The name will surprise users who expect `AllNamedGroups` to be the "all matches" counterpart of `NamedGroups`. (`FindAllNamed` is the all-matches counterpart of `FindNamed`; the naming pair would suggest symmetry that doesn't hold.)
2. The "right" all-matches-all-groups function — `[]map[string]string` shape — does not exist yet, and adding it post-v1 would force us to either keep the misleading name or live with both.

**Decision needed pre-v1:** rename to something like `MultiGroupValues` / `RepeatedNamedGroups`, OR add an explicit `AllNamedGroupsAcrossMatches` (or similar) so the asymmetry is visible from the API surface, OR commit to the current name and lock in a docstring rewrite that leads with the duplicate-group-name use case.

Filed as `re-zwj` (see Open blockers).

### `Replace(re *regexp.Regexp, target string, replacements map[string]string) string`

**Verdict: Keep. `ReplaceFirst` is nice-to-have post-v1.**

`Replace` operates on every match, which matches stdlib's `ReplaceAllString` semantics. The natural symmetry would be `ReplaceFirst` for one-match operation, but:

- The single-match case is rare in practice for the use cases this package targets (log redaction, template-style substitution).
- `ReplaceFirst` is purely additive — it can be added post-v1 without a major bump.

No v1 blocker. The behavior of overlapping/nested groups is documented in the docstring (outermost-first wins, inner spans skipped) — that contract is already tight enough to commit to.

### `Validate(re *regexp.Regexp, required ...string) error`

**Verdict: Keep signature. Typed error is nice-to-have post-v1.**

Variadic-input + single-error-output is the right shape. Callers who want structured access today can split on the message, but that's exactly what we want them not to do — error-message text is non-contract (see "Error messages" below).

A future `*ValidationError` type with `Missing []string` would let callers introspect without parsing. That addition is non-breaking — `Validate` keeps its `error` return, the concrete type just becomes addressable via `errors.As`. Defer to post-v1.

### `Unmarshal(re *regexp.Regexp, target string, v any) error`

**Verdict: Keep.**

`v any` matches `encoding/json.Unmarshal`. The reflection-on-target idiom is the right tradeoff for a one-shot helper: callers don't pay a generic-instantiation cost and the API works uniformly across types. The fast path for repeated decode is the typed `Decoder[T]` — that's the whole point of having both shapes.

One behavior to note (already documented in the docstring): no match returns `nil`, not an error. This is intentionally different from `Decoder.One`, which returns `ErrNoMatch`. See "No-match contract" below.

### `UnmarshalAll(re *regexp.Regexp, target string, v any) error`

**Verdict: Keep.**

Same shape and rationale as `Unmarshal`. The slice is replaced wholesale (existing capacity is discarded — a minor perf nit, but not a v1 issue; can be optimized post-v1 without changing contract).

---

## Interface

### `RegexUnmarshaler { UnmarshalRegex(value string) error }`

**Verdict: Keep. Document the deliberate divergence from `encoding.TextUnmarshaler`.**

`encoding.TextUnmarshaler.UnmarshalText(text []byte) error` takes `[]byte`. We take `string`. The choice is deliberate: regexp's `FindStringSubmatch` returns `[]string`, so the matched value is already a Go string at the point we'd hand it to the unmarshaler. Forcing `[]byte` would mean a `[]byte(value)` conversion on every dispatch — cheap individually, but the per-decode work for typed extension is exactly what `Decoder[T]` exists to minimize.

The divergence is worth documenting in the package doc and the README, since users who have already wired up `TextUnmarshaler` for `encoding/json` will reasonably expect symmetry. The current docstring on `RegexUnmarshaler` already explains the ordering inside `setFieldValue` (pointer → `RegexUnmarshaler` → time-types → kind switch); the README has a worked example. Both are sufficient for v1 — no contract change, just lock in the docs.

---

## Tag grammar

### `regex:"name,key=value,key=value"`

**Verdict: Keep grammar. Document the reserved-key and unknown-key rules explicitly.**

The grammar is JSON-encoding-style: first comma-separated piece is the group name; subsequent pieces are `key=value` pairs. Currently recognized:

| Key | Applies to | Effect |
|---|---|---|
| `default=<value>` | Any field type | Substitutes when group is undeclared OR matched empty. Goes through the same type conversion as a real match. |
| `layout=<go-time-layout>` | `time.Time` only | Used exclusively (no fallback list). |

Two forward-compat decisions in the parser that need to be locked in pre-v1:

1. **Unknown keys are kept in the options map.** `parseFieldTag` does not reject unrecognized keys; it stores them so that consumers added in future minor releases (e.g. `required`, `presence`) can read them without a parser change. This is the right design but is currently un-documented as a stability promise. **Action: document in the v1 doc that adding new option keys is non-breaking, and that callers must not rely on the parser rejecting unknown keys.**
2. **Lone tokens without `=` are silently ignored.** Today, `regex:"name,required"` parses as `(name="name", opts=nil)` — the `required` token is dropped. The roadmap commits to a `required` flag in the next minor; that change will need to start *recognizing* lone tokens as flags. The forward-compat path is: continue ignoring them today, and the next minor release adds meaning to specific flag tokens. **Action: document the silent-ignore-then-promote-to-flag plan so callers understand that adding `regex:"name,foo"` today is a no-op but may stop being one in a future minor.**

### `regex:""` and `regex:"-"`

Both signal "no name" — equivalent to "use the field's own name for matching". Already documented in `parseFieldTag`. **Verdict: Keep, document in v1 doc.**

---

## Error messages

### Format and stability

**Verdict: Non-contract. Document explicitly.**

Error messages take the form `"cannot convert %q to int: %w"` (and similar), wrapping the underlying `strconv` / `time.Parse` error via `%w`. The text is for humans; the wrapping is for `errors.Is` / `errors.As`.

The README's stability section already says "do not pattern-match on `err.Error()` strings; compare against the exported sentinels" — reaffirm this in the v1 doc. The wrapping contract is stable: callers can rely on `errors.Is(err, strconv.ErrSyntax)` / `errors.As` against unwrapped concrete types from the stdlib for as long as those types remain in the underlying package.

---

## Behavior

### Empty match handling

**Verdict: Keep, document.**

When a named group's match is the empty string (either because the group didn't participate in the match, or because it matched a zero-length span), `Unmarshal` and `Decoder` both treat it as "no useful value" for default-fallback purposes:

```go
if !found || value == "" {
    if def, ok := opts["default"]; ok {
        value = def
        ...
    }
}
```

For string fields that legitimately accept empty matches, this conflates "no match" with "matched empty". The current docstring acknowledges this is intentional — caller expectation aligns with the conflation. Lock in as v1 contract and document in the v1 doc.

### Pointer field allocation

**Verdict: Keep, document.**

In `setFieldValue`:
- If the field is a nil pointer, `reflect.New(field.Type().Elem())` allocates the pointee.
- If the field is a non-nil pointer, the existing pointee is reused (overwritten).
- Single-level pointers only. `**Foo` works because the recursive call descends one level at a time, but the documented contract is single-level.

This matches the optional-field idiom (`*int`, `*time.Time`) without requiring a `RegexUnmarshaler` wrapper. Document in v1 doc as the contract.

### Default value semantics

**Verdict: Keep, document.**

`default=` fires on either condition:

1. The named group is not declared on the regex.
2. The named group is declared and matched, but the matched span is empty.

Defaults go through the same type-conversion path as real matches — `default=100` on an `int` field produces `int(100)`, not the string `"100"`. `Decoder.Compile` validates this eagerly (probes assignment of the default to a fresh field); `Unmarshal` defers the check to first decode.

### Layout option exclusivity

**Verdict: Keep, document.**

`layout=<format>` on a `time.Time` field replaces the default fallback list (`time.RFC3339Nano`, `time.RFC3339`, `time.DateTime`, `time.DateOnly`, `time.TimeOnly`) — there is no fallback. If the supplied layout doesn't parse the input, decoding fails with `"cannot convert %q to time.Time using layout %q: %w"`.

Rationale: a caller who specifies a layout has stated intent — they know what shape the input is. Falling back silently to RFC3339 would mask bugs. Lock in for v1.

### `setFieldValue` dispatch order

**Verdict: Keep, document the order as contract.**

Order (already asserted by tests):

1. Pointer — allocate-if-nil, then dispatch on the pointer's own `RegexUnmarshaler` or recurse into the pointee.
2. `RegexUnmarshaler` (addressable) — caller-defined conversion wins over stdlib special cases.
3. `RegexUnmarshaler` (value-receiver) — rare but possible.
4. `time.Time` / `time.Duration` — Type-match (not Kind-match) so `time.Duration`'s underlying `int64` doesn't pre-empt `time.ParseDuration`.
5. `reflect.Kind` switch — string, signed ints, unsigned ints, floats, bool.

Document this as the v1 dispatch contract. Adding more types post-v1 (as additional Kind cases or Type matches) is non-breaking; reordering is breaking.

---

## No-match contract (cross-cutting)

**Verdict: Keep the asymmetry, document it explicitly.**

Three different no-match behaviors exist across the API:

| Function | No-match return |
|---|---|
| `FindNamed` | `("", false)` |
| `FindAllNamed` | `[]string{}` (or `nil` if the group is undeclared) |
| `NamedGroups` / `AllNamedGroups` | empty map |
| `Replace` | `target` returned unchanged |
| `Validate` | unrelated — checks declarations, not matches |
| `Unmarshal` / `UnmarshalAll` | `nil` error, struct/slice left unchanged or cleared |
| `Decoder.One` | `ErrNoMatch` (sentinel) |
| `Decoder.All` | `[]T{}, nil` |
| `Decoder.Iter` | yields zero times |

The `Decoder.One` ErrNoMatch sentinel exists because `One` returns a `(T, error)` pair — a zero-value `T` with `nil` error would be ambiguous with "decoded a struct of all zero fields". `Unmarshal` doesn't have this problem because the caller passes the destination — the caller can inspect their own struct after the call.

This asymmetry is intentional but is not currently spelled out in any single place. Lock it in for v1 and document the rationale in the v1 doc and the package doc.

---

## Partial-result-on-error contract

**Verdict: Document, no behavior change.**

- `Decoder.One` returns the partially-populated `T` plus the error (`return v, err`). Caller can inspect which fields decoded before the failure.
- `Decoder.All` returns `out[:i+1]` plus the wrapped error — slice contains all fully-decoded entries plus the partially-decoded failing one.
- `Decoder.Iter` yields the partially-decoded `(T, error)` pair for the failing match and continues iteration.

Document this so callers know they can do best-effort decoding (`Iter`) vs strict (`All` aborts after first failure but you keep what you have).

---

## Additional surfaces

### `ErrNoMatch`

**Verdict: Keep.** Exported sentinel, comparable with `errors.Is`. Only returned by `Decoder.One`. Adding it to `Unmarshal` would be breaking (callers currently rely on `nil` for no-match). Don't change.

### `Decoder.Pattern() string`

**Verdict: Keep. `Decoder.Regexp() *regexp.Regexp` is nice-to-have post-v1.**

The `Pattern()` accessor is enough for logging and debugging. Exposing the underlying `*regexp.Regexp` would let callers do their own match-finding while reusing the typed decode plan, but no concrete demand has surfaced. Defer.

### `Compile` / `MustCompile` errors

**Verdict: Keep. Sentinel errors are nice-to-have post-v1.**

Errors today are formatted strings prefixed `"regextra.Compile: ..."`. No `errors.Is`-friendly sentinels for the four failure categories (invalid pattern, non-struct T, undeclared group, malformed default, layout-on-non-time). Adding sentinels later is additive — defer to post-v1 unless a real adopter pain surfaces.

---

## Open blockers (filed as separate beads)

These are the items where v1.0 cannot ship until a decision lands. Filed against v0.5.x or v0.6.x — not v1.0 itself, per the bead's acceptance criteria.

1. **`AllNamedGroups` naming clarity** (`re-zwj`) — rename or unambiguous doc rewrite. See Functions section above.
2. **Tag-grammar reservation policy** (`re-c4f`) — explicit pre-v1 documentation that unknown keys are preserved (forward-compat) and lone tokens are silently ignored (reserved for future flag promotion). See Tag grammar section.
3. **No-match contract documentation** (`re-ne3`) — single canonical doc page (or expanded package doc) covering the asymmetry. See No-match contract section.

The remaining "Document" items in this review are pure docs work and can be folded into the v1.0 release prep without a separate bead each — they do not change behavior.

---

## Acceptance recap

Per the originating bead `re-dh8`:

- [x] Every public symbol examined.
- [x] Each item has a documented verdict.
- [x] Blocker beads filed for "change before v1" items, assigned to pre-v1 milestones.
