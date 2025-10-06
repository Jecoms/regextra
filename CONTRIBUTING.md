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
- Aim for coverage: >85%
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
Use conventional commits:
```
feat: add new feature
fix: resolve bug
chore: update dependencies
refactor: improve code structure
docs: update documentation
test: add missing tests
```

Examples:
```
feat: add UnmarshalAll function for multiple matches
fix: handle nil pointer in Unmarshal validation
chore: update golangci-lint config to v2
refactor: apply Go 1.24 best practices
```

## Getting Help

- **Issues**: Open a GitHub issue for bugs or feature requests
- **Discussions**: Use GitHub Discussions for questions

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Code of Conduct

Be respectful, collaborative, and constructive.

---

**For AI Agents**: This project values clean, well-tested code that follows Go best practices. When adding features, prioritize simplicity and maintainability over cleverness. Always include tests and documentation for new functionality.
