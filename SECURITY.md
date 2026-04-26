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

The latest minor of the current major plus the previous minor receive security fixes. Older versions do not.

| Version | Supported |
|---------|-----------|
| Latest minor (e.g. v0.5.x) | ✓ |
| Previous minor (e.g. v0.4.x) | ✓ |
| Anything older | Upgrade |

After v1.0.0 ships, the same rule applies on the v1 line; the latest v0.x will receive critical fixes for a 6-month window after v1.0 then drop out of support.

## Scope

In scope:
- Bugs in `regextra` source code that lead to incorrect behavior in security-relevant contexts (e.g. regex/input handling that allows untrusted input to escape its expected shape).
- Build-time supply-chain issues with the package's own CI / release workflows.

Out of scope:
- Vulnerabilities in caller code that uses `regextra` insecurely (passing untrusted regex patterns from user input is generally a bad idea regardless of which package you use).
- Performance issues that aren't a denial-of-service risk.
- Issues in Go's standard library `regexp` package — report those upstream to https://github.com/golang/go.
