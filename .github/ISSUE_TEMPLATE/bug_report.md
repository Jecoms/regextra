---
name: Bug report
about: Report incorrect behavior in regextra
title: ''
labels: kind:bug
assignees: ''
---

## Summary
<!-- One-sentence description of what's wrong. -->

## Reproduction

```go
re := regexp.MustCompile(`...`)
// minimal code that triggers the bug
```

**Input string:** ``...``
**Expected:** <!-- what you expected to happen -->
**Actual:** <!-- what actually happened, including any error -->

## Environment

- regextra version: <!-- output of `go list -m github.com/jecoms/regextra` -->
- Go version: <!-- output of `go version` -->
- OS / arch: <!-- e.g. macOS 14.5 / arm64 -->

## Additional context
<!-- Any other relevant detail: surrounding pattern, related stdlib regexp behavior, etc. -->
