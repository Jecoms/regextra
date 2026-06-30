# Contributing to regextra

Thank you for your interest in contributing! This guide provides essential information for both human developers and AI agents working on this project.

## Project Overview

`regextra` extends Go's standard library `regexp` package with convenient utilities for working with named capture groups. The package focuses on:

- **Simplicity**: Reduce boilerplate for common regex operations
- **Type Safety**: Enable struct unmarshaling from regex matches (like `json.Unmarshal`)
- **Performance**: Use efficient stdlib implementations where possible
- **Idiomatic Go**: Follow Go best practices and mirror stdlib patterns

## Development Setup

### Requirements
- **Go 1.22 or later** (currently targeting Go 1.24)
- golangci-lint for linting
- Git for version control

### Getting Started

```bash
# Clone the repository
git clone https://github.com/Jecoms/regextra.git
cd regextra

# Install dependencies
go mod download

# Run tests
go test -v -race ./...

# Run tests with coverage
go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...

# Run linter
golangci-lint run

# Run go vet
go vet ./...

# Check formatting
gofmt -s -l .
```

## Code Standards

### Testing
- **All new features must include tests**
- Use table-driven tests for multiple test cases
- Include both success and error cases
- Test edge cases (nil pointers, empty strings, no matches)
- Add example tests for public APIs (they appear in godoc)

#### Test the public contract, not internals
- **Assert observable behavior through the exported API** (`Unmarshal`,
  `UnmarshalAll`, `Compile`/`Decoder`, etc.), never the internals of an
  unexported function. If a behavior matters, it is reachable from the public
  surface — drive it from there so tests survive refactors of the internals.
  Behavior that looks invisible usually isn't: a forward-compat rule like "an
  unknown tag option is ignored" is observable as a no-op — pair it with a
  visible effect such as `default=` to prove the parser accepted it.
- **Do not pin implementation detail** — private field shapes, whether a map
  is nil vs empty, or intermediate parser state. Pinning internals makes tests
  brittle and turns harmless refactors into false regressions.
- The rule is about what you *assert*, not the package clause. A test in
  `package regextra` (rather than `package regextra_test`) is fine when it
  still asserts observable behavior — several existing files do this only to
  drop the `rx.` qualifier. It is not a license to read unexported state.
- If a unit ever genuinely needs isolation the public API can't reach,
  introduce a seam — an interface or injected dependency, with a fake/stub in
  the test — instead of reaching inside. (regextra is a pure library today, so
  this is rare.) Benchmarks that must measure unexported cost, like
  `bench_internal_test.go`, are the one accepted reason to live inside the
  package and touch internals.

### Documentation
- **All exported functions must have godoc comments**
- Start with the function name: `// FunctionName does...`
- Include usage examples in the godoc
- Update README.md when adding new public APIs
- Add example tests that demonstrate usage

### Code Style
- Follow the conventions in `Effective Go` and `Go Code Review Comments`
- Keep functions focused and single-purpose
- Prefer simplicity over cleverness

## API Design Principles

### Mirror stdlib Patterns
When designing new features, follow established stdlib patterns if using same names:
- `Unmarshal(re, target, v any)` mirrors `json.Unmarshal`
- Return errors for validation issues, not panics

### Performance Considerations
- Prefer stdlib implementations over custom code
- Profile before optimizing

## Project Structure

```
regextra/
├── main.go           # Core implementation; May be broken out into separate files in the future
├── main_test.go      # Core implementation tests
├── README.md         # Public API documentation
├── CONTRIBUTING.md   # This file
├── CHANGELOG.md      # Version history
├── go.mod            # Module definition
├── .golangci.yml     # Linter configuration
└── .github/
    └── workflows/
        ├── test.yml     # CI: tests, coverage, linting
        └── release.yml  # CD: automated releases
```

## Testing Guidelines

### Running Tests
```bash
# All tests with race detection
go test -v -race ./...

# With coverage report
go test -v -race -coverprofile=coverage.txt ./...

# Single test
go test -v -run TestUnmarshal

# Benchmarks (if added)
go test -bench=. -benchmem
```

### Test Coverage Expectations
- Aim for coverage: >92% (enforced by the CI coverage gate in `.github/workflows/test.yml`)
- All public APIs must be tested
- Critical error paths must be covered
- Edge cases: nil pointers, empty inputs, no matches

## Pull Request Process

### Before Submitting
1. **Run tests locally**: `go test -v -race ./...`
2. **Run linter**: `golangci-lint run`
3. **Check formatting**: `gofmt -s -l .`
4. **Update documentation**: Add examples and update README if needed

### PR Guidelines
- **One feature per PR**: Keep changes focused
- **Write clear descriptions**: Explain what and why
- **Reference issues**: Link related issues or discussions
- **Update tests**: Include test coverage for changes
- **Breaking changes**: Clearly document in PR

### Commit Message Format

This project uses [Conventional Commits](https://www.conventionalcommits.org/) for
commit subjects. Because `main` is squash-merged, **the PR title becomes the
squashed commit subject**, so PR titles must follow the same format. A GitHub
Action (`.github/workflows/commitlint.yml`) validates the PR title on every PR.

**Required shape:**
```
<type>(<optional-scope>)!: <subject>
```
- `<type>` — one of the allowed types in the table below
- `(<scope>)` — optional; lowercase `[a-z0-9_./-]`, e.g. `(unmarshal)`, `(deps)`, `(ci)`
- `!` — optional; marks a breaking change
- `<subject>` — short, imperative description (no trailing period)

**Allowed types:**

| Type       | Use for                                                  |
|------------|----------------------------------------------------------|
| `feat`     | New user-facing feature or public API addition           |
| `fix`      | Bug fix in existing behavior                             |
| `perf`     | Performance improvement (no behavior change)             |
| `refactor` | Code restructuring without behavior or API change        |
| `style`    | Formatting, whitespace, comments — no code change        |
| `test`     | Adding or updating tests                                 |
| `docs`     | Documentation only (README, godoc, CONTRIBUTING, etc.)   |
| `build`    | Build system, `go.mod`, dependencies that ship to users  |
| `ci`       | CI configuration and scripts (`.github/workflows`, etc.) |
| `chore`    | Routine maintenance that doesn't fit elsewhere           |
| `revert`   | Reverts a previous commit                                |

**Examples:**
```
feat: add UnmarshalAll function for multiple matches
feat(decoder): add Decoder[T].Iter for range-over-func streaming
fix(unmarshal): handle nil destination pointer
fix!: rename Decode to Unmarshal (breaking)
perf: avoid allocation in hot path of FindNamed
refactor: split main.go into regextra.go + unmarshal.go
test: add fuzz tests for Unmarshal
docs: expand package doc for pkg.go.dev landing
build(deps): bump golang.org/x/tools to v0.30
ci: pin golangci-lint version
chore(deps): bump actions/checkout from 5.0.0 to 6.0.2
revert: revert "feat: add experimental decoder cache"
```

**Breaking changes:** Add `!` after the type/scope (e.g. `feat!:` or
`feat(api)!:`) and explain the break in the PR body.

## Getting Help

- **Issues**: Open a GitHub issue for bugs or feature requests
- **Discussions**: Use GitHub Discussions for questions

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Code of Conduct

Be respectful, collaborative, and constructive.

---

**For AI Agents**: This project values clean, well-tested code that follows Go best practices. When adding features, prioritize simplicity and maintainability over cleverness. Always include tests and documentation for new functionality.
