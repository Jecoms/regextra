# Roadmap

A high-level view of where `regextra` is heading. The [issue tracker](https://github.com/Jecoms/regextra/issues) is the source of truth for individual work items; this document groups them into themes so adopters can tell what's coming next, what's gating v1.0, and what's filed-but-not-scheduled.

## Current release

**v1.0.0** — the API-stability stamp. Every public surface audited by the [v1-readiness review](./docs/v1-readiness.md) and pinned as v1 contract. No behavior changes vs v0.5.0; the release exists to mark the stability commitment.

What that means for adopters: post-v1, breaking changes ship only in the next major version (`v2.0.0`). Pin `v1` and follow minor/patch updates without re-reading release notes for migration steps. See the [README's Stability section](./README.md#stability) for the precise definition of "breaking".

See the [CHANGELOG](./CHANGELOG.md) for the full history.

## Next minor

Theme: **make the unmarshal contract more expressive without expanding the API surface.**

- Tag-derived required-group validation — let callers mark a struct field as required directly on its `regex:"name,required"` tag, replacing the separate `Validate` call for the common case.
- Possibly: `Encoder[T]` for typed round-trip via template syntax — symmetric counterpart to `Decoder[T]` for callers building strings from typed structs.

Additive nice-to-haves identified by the v1-readiness review (post-v1, non-breaking):

- `ReplaceFirst` for one-match substitution (symmetric to `Replace`).
- Typed `*ValidationError` so callers can `errors.As` instead of parsing message text.
- Sentinel errors for `Compile` / `MustCompile` failure categories.
- `Decoder.Regexp() *regexp.Regexp` accessor for callers that want their own match-finding while reusing the typed decode plan.

These ship when concrete adopter demand surfaces. Specific issues are tracked in the [issue tracker](https://github.com/Jecoms/regextra/issues).

## Backlog

Themes filed in the issue tracker but not on a near-term milestone:

- Performance improvements that aren't on the unmarshal hot path (e.g. micro-optimizing `NamedGroups` allocations).
- Quality-of-life additions to the tag grammar that don't yet have a clear use case.

Filed-but-deferred ≠ rejected. If you have a use case for one of these, comment on the issue — concrete demand pulls items out of the backlog.

## How decisions get made

This is a single-maintainer project. Roadmap items move based on (in order): real adopter pain, work that gates v1.0, and personal interest. The roadmap is reviewed and pruned with each minor release.
