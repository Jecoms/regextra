# Roadmap

A high-level view of where `regextra` is heading. The [issue tracker](https://github.com/Jecoms/regextra/issues) is the source of truth for individual work items; this document groups them into themes so adopters can tell what's coming next, what's gating v1.0, and what's filed-but-not-scheduled.

## Current release

**v0.5.0** — typed cached decode (`Compile[T]` / `Decoder[T]`) with streaming via `Decoder.Iter` (range-over-func). Roughly half the time and half the allocations of `Unmarshal` for repeated decode of the same shape; `Iter` skips slice allocation entirely for streaming consumers.

See the [CHANGELOG](./CHANGELOG.md) for the full history.

## Next minor

Theme: **make the unmarshal contract more expressive without expanding the API surface.**

- Tag-derived required-group validation — let callers mark a struct field as required directly on its `regex:"name,required"` tag, replacing the separate `Validate` call for the common case.
- Possibly: `Encoder[T]` for typed round-trip via template syntax — symmetric counterpart to `Decoder[T]` for callers building strings from typed structs.

Specific issues are tracked in the [issue tracker](https://github.com/Jecoms/regextra/issues).

## Working toward v1.0

v1.0 is the API-stability promise: post-v1, breaking changes ship as v2. The gating work:

- **API stability review** — full audit of the public surface (function signatures, exported types, tag grammar, error sentinels) for anything we'd regret committing to. This is the single hard gate on v1.0.
- **Documentation polish** — package doc on pkg.go.dev as the canonical API reference, README slimmed to quick-start + showcase, examples folder.

What this means for adopters: pre-v1 the package follows SemVer with breaking changes signaled by minor bumps. After v1.0, breaking changes signal a major. The README's Stability section will spell out the precise definition of "breaking" once it lands as part of the v1.0-readiness docs.

## Backlog

Themes filed in the issue tracker but not on a near-term milestone:

- Performance improvements that aren't on the unmarshal hot path (e.g. micro-optimizing `NamedGroups` allocations).
- Quality-of-life additions to the tag grammar that don't yet have a clear use case.

Filed-but-deferred ≠ rejected. If you have a use case for one of these, comment on the issue — concrete demand pulls items out of the backlog.

## How decisions get made

This is a single-maintainer project. Roadmap items move based on (in order): real adopter pain, work that gates v1.0, and personal interest. The roadmap is reviewed and pruned with each minor release.
