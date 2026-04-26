<!--
Use a Conventional Commits subject for the PR title (e.g. `feat: …`, `fix: …`,
`chore: …`, `ci: …`, `docs: …`, `refactor: …`, `test: …`, `perf: …`). Squash-merge
turns the PR title into the commit subject — keep it short and accurate.
-->

## Summary
<!-- What changed and why. One short paragraph or 2–3 bullets. -->

## Type of change
- [ ] Feature (new public API or behavior)
- [ ] Fix (bug correction, no API change)
- [ ] Chore / CI / docs / refactor (no behavior change)
- [ ] Breaking change (semver minor pre-1.0, major post-1.0)

## CHANGELOG
<!-- For features and fixes, paste the entry you added under `## [Unreleased]`. -->

## Test plan
- [ ] `go test ./...` passes locally
- [ ] `go test -race ./...` passes if touching shared state
- [ ] `golangci-lint run` is clean
- [ ] New behavior covered by tests / examples
