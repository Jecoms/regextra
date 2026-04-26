# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.2] - 2026-04-26

### Added
- Fuzz harnesses (`fuzz_test.go`) covering the public API surface: `FuzzFindNamed` and `FuzzNamedGroups` exercise the regex-resolution paths and contract; `FuzzUnmarshalInt`, `FuzzUnmarshalUint`, `FuzzUnmarshalFloat`, `FuzzUnmarshalBool` drive each `strconv`-backed branch of the Unmarshal type-conversion code with arbitrary inputs. Seed corpus covers stdlib edge cases (sign-prefixed, scientific notation, MAX/MIN bounds, NaN/Inf, locale-flavored bool variants, unicode, NUL bytes).

### Changed
- CI test matrix expanded from `['1.24']` to `['1.24', '1.25', '1.26']` so regressions on newer toolchains surface in CI without raising the consumer floor (`go.mod` stays at 1.24). `fail-fast: false` lets all three legs finish so triage shows which version broke first. Lint and vet jobs stay on 1.24.
- CI runs each fuzz target for 10 seconds on the floor matrix leg (Go 1.24) — adds ~60 seconds to one job leg, doesn't multiply across the matrix. Seed corpus runs unconditionally on every leg via the regular `go test` step.

### Fixed
- `auto-tag.yml` now publishes the GitHub Release in the same job that pushes the tag, sidestepping the `GITHUB_TOKEN` cascade limitation that previously prevented `release.yml` from firing on automated tag pushes (every release since v0.3.0 had a tag but no Release until manual `gh release create`). The "Delete release branch" step is now best-effort: it checks `git ls-remote` first and prints an info line if the branch was already auto-deleted by the repo's merge-cleanup setting, instead of exiting non-zero.

### Removed
- `release.yml` workflow — its tag-listening / test-running / release-creation steps are now folded into `auto-tag.yml`.

## [0.3.1] - 2026-04-26

### Added
- Issue and pull request templates: `.github/ISSUE_TEMPLATE/{bug_report,feature_request}.md` plus `.github/PULL_REQUEST_TEMPLATE.md`. Issue templates ask for the regex pattern, input string, expected vs actual, and Go / regextra / OS versions; PR template prompts for type-of-change, the CHANGELOG entry, and a test plan. `.github/ISSUE_TEMPLATE/config.yml` routes open-ended questions to GitHub Discussions and disables blank issues.

### Security
- New CodeQL workflow (`.github/workflows/codeql.yml`) — runs Go static analysis on every PR, push to `main`, and weekly on Monday 09:00 UTC. Findings surface in the Security tab.
- New govulncheck workflow (`.github/workflows/govulncheck.yml`) — installs `golang.org/x/vuln/cmd/govulncheck` and scans on every PR, push to `main`, and weekly. Catches stdlib + transitive-dep CVEs.

### Changed
- CI: enforce 85% coverage floor on the `test` job. The job now parses `go tool cover -func=coverage.txt` and fails when total coverage falls below the threshold. Codecov upload is unaffected.
- CI: validate every PR title against Conventional Commits. Allowed types: `feat`, `fix`, `chore`, `ci`, `docs`, `refactor`, `test`, `perf`, `build`, `revert`, `style`. Squash-merge turns the PR title into the commit subject, so this is the gate that matters.
- CI: auto-merge Dependabot PRs for the safe class — all `github-actions` ecosystem bumps and `gomod` `direct:development` patch/minor bumps. Production direct deps and major bumps still require human review.

### Dependencies
- Bumped `actions/checkout` from 5.0.0 to 6.0.2 (Dependabot #28).

## [0.3.0] - 2025-10-06

### Added
- **NEW**: `Unmarshal` function for type-safe regex extraction into structs
  - Supports struct tags (`regex:"groupname"`) for field mapping
  - Automatic type conversion for int, uint, float, bool types
  - Field mapping priority: struct tag > exact name > case-insensitive match
  - Comprehensive error handling for validation
- **NEW**: `UnmarshalAll` function for extracting multiple matches into slice of structs
  - Processes all pattern matches in target string
  - Returns slice of populated structs
  - Clears slice when no matches found
- CONTRIBUTING.md guide for contributors and AI agents
- Dependabot configuration for automated dependency updates
- Automatic release tagging workflow (tags releases when release PRs are merged)
  - Validates semver format
  - Prevents duplicate tags
  - Automatically cleans up release branches

### Changed
- Updated golangci-lint configuration to v2 format
- Applied Go 1.24 best practices (range over integers, SetLen for slice clearing)
- Modernized API to use `any` instead of `interface{}`
- Test matrix now only tests on Go 1.24 (matching go.mod requirement)

### Security
- Hardened GitHub Actions workflows with SHA-pinned actions
- Added explicit permissions blocks to workflows
- Removed version comments from action SHAs (Dependabot compatibility)
- Replaced deprecated actions/create-release with GitHub CLI
- Reduced third-party dependencies in CI/CD

### Dependencies
- Bumped actions/checkout from 4.3.0 to 5.0.0
- Bumped actions/setup-go from 5.5.0 to 6.0.0
- Bumped codecov/codecov-action from 4.6.0 to 5.5.1
- Bumped golangci/golangci-lint-action from 4.0.1 to 8.0.0

## [0.2.1] - 2025-10-04

### Added
- `AllNamedGroups` function to extract all values for each named capture group as `map[string][]string`
- Handles patterns with duplicate group names, collecting all matches for each group name
- Comprehensive test coverage for `AllNamedGroups` including duplicate group name scenarios
- Example test for `AllNamedGroups` for pkg.go.dev documentation

## [0.2.0] - 2025-10-04

### Changed
- **BREAKING**: Simplified to function-only API (removed wrapper struct)
- **BREAKING**: Renamed functions for better idiomatic Go style:
  - `SubexpValue` → `FindNamed`
  - `SubexpMap` → `NamedGroups`
- All functions now accept `*regexp.Regexp` as first parameter
- Updated documentation with simpler usage examples
- Updated to Go 1.24

### Removed
- **BREAKING**: Removed `Regexp` wrapper type
- **BREAKING**: Removed `Compile()` and `MustCompile()` constructors
- Removed deprecated `SubexpValue` and `SubexpMap` functions

### Added
- Comprehensive package documentation
- 100% test coverage
- Example tests for pkg.go.dev
- GitHub Actions CI/CD workflows
- golangci-lint configuration
- Badges in README (Go Reference, Go Report Card, Tests, License)
- Automated release workflow

## [0.1.0] - Initial Release

### Added
- Basic named capture group extraction functionality
- `Regextra` wrapper type
- `SubexpValue` and `SubexpMap` methods
