# Security Policy

## Reporting a vulnerability

If you've found a security issue in `regextra`, **please do not open a public GitHub issue.** Use one of these channels instead:

- **Preferred:** [Open a private security advisory](https://github.com/Jecoms/regextra/security/advisories/new) on this repository. GitHub keeps the report private until we coordinate disclosure.
- **Alternative:** Email the maintainer (see the GitHub profile of the repository owner).

Please include enough detail to reproduce the issue: pattern, input string, Go version, and the version of `regextra` you're using.

## Response expectations

This is a one-maintainer project. Best-effort response targets:

- **Acknowledgement:** within 7 days of report.
- **Triage and severity assessment:** within 14 days.
- **Patch release for confirmed high-severity issues:** as quickly as practical, typically within 14 days of triage.

If a report is rejected (not a vulnerability, out of scope), you'll get an explanation. If you disagree with the assessment, escalate by responding on the same advisory thread.

## Versions supported with security fixes

The latest minor of the current major plus the immediately preceding minor receive security fixes. Older minors do not.

| Version | Supported |
|---------|-----------|
| Latest minor of the current major | ✓ |
| Previous minor of the current major | ✓ |
| Anything older | Upgrade |

When the next major ships, the same rule applies on the new major line; the latest minor of the prior major receives critical fixes for a 6-month transition window, then drops out of support.

## Scope

### Scope of protection

All matching in `regextra` goes through Go's standard library [`regexp`](https://pkg.go.dev/regexp) package, which implements [RE2 syntax](https://github.com/google/re2/wiki/Syntax) and is guaranteed to run in time linear in the size of the input — there is no backtracking engine.

- **Hostile target input: safe.** Matching untrusted *target* text cannot trigger catastrophic backtracking (ReDoS); for any fixed pattern, match time stays linear in the size of the input.
- **Hostile patterns: out of scope — even when the cost is DoS-grade.** Pattern authors control compile and match cost — pattern size still drives memory and CPU, so "linear in the input" is not "immune to expensive patterns". Resource exhaustion driven by a hostile *pattern* is the caller's trust decision, not a `regextra` vulnerability; compiling patterns from untrusted input is the same trust decision as with stdlib `regexp` itself (see "Out of scope" below).

### In scope
- Bugs in `regextra` source code that lead to incorrect behavior in security-relevant contexts (e.g. regex/input handling that allows untrusted input to escape its expected shape).
- Build-time supply-chain issues with the package's own CI / release workflows.

### Out of scope

- Vulnerabilities in caller code that uses `regextra` insecurely — in particular, compiling untrusted regex *patterns* from user input (see "Scope of protection" above).
- Performance issues that aren't a denial-of-service risk — and hostile-pattern cost even when it is (see "Scope of protection" above).
- Issues in Go's standard library `regexp` package — report those upstream to https://github.com/golang/go.
