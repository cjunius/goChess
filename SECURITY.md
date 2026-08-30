# Security Policy

## Supported versions

Security fixes are applied to the latest minor release and the `main` branch.

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |
| < latest| :x:                |

## Reporting a vulnerability

**Do not open a public issue for security vulnerabilities.**

Report privately through GitHub's
[Security Advisories](https://github.com/cjunius/goChess/security/advisories/new),
or email **christopher.junius@gmail.com** with:

- a description of the issue and its impact,
- steps to reproduce (FEN / input sequence / command line),
- affected version or commit,
- any suggested fix.

### What to expect

| Stage                        | Target                     |
| ---------------------------- | -------------------------- |
| Acknowledgement              | within 3 business days     |
| Initial assessment           | within 10 business days    |
| Fix or mitigation plan       | communicated after triage  |
| Public disclosure            | coordinated after a fix    |

We will credit reporters in the advisory unless you ask otherwise.

## Scope

This is a chess engine that parses untrusted input (FEN strings and UCI
commands, often from a GUI or tournament manager). In scope:

- crashes, panics, or unbounded resource use from malformed FEN or UCI input,
- memory-safety issues in dependencies surfaced through this project,
- supply-chain issues in the build or release pipeline.

Out of scope: the engine losing a game, weak play, or perft discrepancies
(report those as normal bugs).

## Dependencies

Dependencies are monitored by Dependabot and scanned with `govulncheck` in CI.
