# 1. Record architecture decisions

Date: 2026-08-30

## Status

Accepted

## Context

We need to record the architectural decisions made on this project so that
future contributors understand why the code is the way it is.

## Decision

We will use Architecture Decision Records, as
[described by Michael Nygard](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions).

Each record is a short Markdown file in `docs/adr/`, numbered sequentially and
monotonically (`NNNN-title.md`). Records are immutable once accepted; a later
decision that reverses an earlier one is a new record that updates the status of
the old one.

## Consequences

- Decisions have a durable, reviewable home outside commit messages.
- The `docs/adr/` directory is the first stop for "why is this like this?"
