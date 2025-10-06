# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
