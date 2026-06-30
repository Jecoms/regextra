# Agent guide

This file is the entry point for AI agents working in this repository.

**[CONTRIBUTING.md](CONTRIBUTING.md) is the authoritative contributor guide —
read it before making changes.** It applies equally to human and agent
contributors. This file does not duplicate its content; it only points you to
it and highlights the conventions that are easiest to violate.

A few that trip agents up most often:

- **One test/benchmark file per source file.** Tests for `foo.go` go in
  `foo_test.go`, benchmarks in `foo_bench_test.go`. Do not create topical
  test files (one per feature/bug/issue) — add to the existing sibling file.
  See CONTRIBUTING.md → "Code Standards" → "Testing".
- **Test the public contract, not internals.** Assert observable behavior
  through the exported API.
- **Conventional Commits.** The PR title becomes the squashed commit subject
  and is CI-validated; match the format documented in CONTRIBUTING.md.
- **Keep coverage above the CI gate** (`MIN_COVERAGE` in
  `.github/workflows/test.yml`).

When in doubt, follow CONTRIBUTING.md.
