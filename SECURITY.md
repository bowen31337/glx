# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.x.x   | :white_check_mark: |

## Reporting a Vulnerability

If you discover a security vulnerability in GLX, please report it responsibly:

1. **Do not** open a public GitHub issue for security vulnerabilities
2. Report via [GitHub Security Advisories](https://github.com/genealogix/glx/security/advisories/new)
3. Include a description of the vulnerability, steps to reproduce, and potential impact

## What to expect

- **Acknowledgment** within 48 hours of your report
- **Assessment** within 1 week — we'll confirm the vulnerability and its severity
- **Fix timeline** depends on severity:
  - Critical: patch release within 72 hours
  - High: patch release within 1 week
  - Medium/Low: included in next scheduled release

## Severity Classification

We do not use CVSS scores to classify severity. Like the Go security team and the curl project, we find that CVSS is often a poor fit for the actual risk profile of a given vulnerability. Instead, we assess severity based on:

- **Impact** — What can an attacker do? (data loss, code execution, information disclosure, denial of service)
- **Exploitability** — How difficult is it to exploit, and what access is required?
- **Affected surface** — Does it affect all users, or only specific configurations?
- **Real-world risk** — Given that GLX processes local YAML files with no network-facing surface in normal usage, most vulnerabilities are inherently constrained in scope

Severity levels are assigned at our discretion, with the intent to be transparent and consistent.

## Bug Bounty

This project does not offer a paid bug bounty program. We are an open source project maintained by volunteers. Security researchers are welcome to report vulnerabilities via GitHub Security Advisories — we appreciate responsible disclosure and will credit reporters in release notes where appropriate.

## Security Measures

- **govulncheck** runs in CI on pushes to main, pull requests, and weekly to detect known vulnerabilities in dependencies
- **gosec** performs static security analysis on pushes to main, pull requests, and weekly
- Weekly scheduled scans catch newly disclosed vulnerabilities in existing dependencies

## Scope

This policy covers the GLX CLI tool and the go-glx library. GLX archives are YAML files processed locally — there is no network-facing attack surface in normal usage.
