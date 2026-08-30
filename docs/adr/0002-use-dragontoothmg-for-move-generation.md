# 2. Use dragontoothmg for move generation

Date: 2026-08-30

## Status

Accepted

## Context

A chess engine needs fast, correct, legal move generation. The prototype used
`github.com/notnil/chess`, which is convenient but allocation-heavy and slow
(it builds move objects and validates via full position copies), making it
unsuitable for a search that visits millions of nodes per second.

Writing a magic-bitboard generator from scratch is a large, error-prone effort
(sliding-piece attack tables, pin/check evasion, en passant edge cases). It is a
worthwhile project on its own but not the point of *this* project, which is
search and evaluation.

Options considered:

1. Keep `notnil/chess`.
2. Write our own bitboard move generator.
3. Build on an existing Go bitboard generator — `dylhunn/dragontoothmg`.

## Decision

Use `github.com/dylhunn/dragontoothmg`.

- It is a dedicated magic-bitboard **legal** move generator (no
  make/unmake-to-test needed for legality).
- Make/unmake via `Apply` returning an unapply closure — allocation-light and
  exactly what alpha-beta wants.
- It exposes per-piece bitboards (for evaluation), an incrementally updated
  Zobrist hash (for a future transposition table), FEN parsing, and a reference
  `Perft`.
- It is GPL-3.0, the same license as this project.

Trade-offs accepted:

- The dependency is unmaintained (last commit 2022) and has no tagged releases;
  we pin a specific pseudo-version and vendor-verify via `go.sum` + the Go
  checksum database. If it ever needs fixing, forking is straightforward — it is
  a small, single-package library.
- Its API uses exported-but-lightly-documented types; we wrap the pieces we use
  behind `internal/engine` so a future swap (e.g. to our own generator) is
  localized.

## Consequences

- `internal/engine` and `internal/uci` depend directly on `dragontoothmg`
  types (`Board`, `Move`). A replacement would touch both packages.
- Move-generation correctness is asserted only through perft tests; we trust the
  library otherwise.
- We inherit the library's board limits and its behavior on malformed FEN;
  parsing robustness is covered in SECURITY.md scope.
