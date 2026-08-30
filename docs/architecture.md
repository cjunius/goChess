# Architecture

## Overview

```
                 ┌────────────────────┐
 stdin/stdout ─► │  internal/uci      │  UCI protocol loop
                 │  (session state)   │
                 └─────────┬──────────┘
                           │ dragontoothmg.Board
                 ┌─────────▼──────────┐
                 │  internal/engine   │  perft · Evaluate · Search
                 └─────────┬──────────┘
                           │ GenerateLegalMoves / Apply / OurKingInCheck
                 ┌─────────▼──────────┐
                 │  dragontoothmg     │  magic-bitboard legal move generator
                 │  (external, GPLv3) │
                 └────────────────────┘
```

`cmd/gochess` wires these together and adds the `perft` / `bench` CLI
subcommands.

## Components

### `dragontoothmg` (dependency)

Provides the board representation (`Board`, bitboards per piece per color),
legal move generation (`GenerateLegalMoves`), make/unmake (`Apply` returns an
unapply closure), FEN parsing, an incrementally-updated Zobrist hash
(`Board.Hash()`), and a reference `Perft`. We do not reimplement any of this.

### `internal/engine`

- **perft.go** — thin wrappers over `dragontoothmg.Perft` plus timed series and
  divide helpers. Backed by regression tests against published node counts.
- **eval.go** — `Evaluate(*Board) int`. Material values + Michniewski
  piece-square tables + a bishop-pair bonus. Returns a score relative to the
  side to move (negamax convention). Piece-square lookups use little-endian
  rank-file indexing: white reads `pst[sq]`, black reads `pst[sq^56]`.
- **search.go** — `Search(*Board, SearchParams) SearchResult`. Iterative
  deepening around a negamax alpha-beta core, with:
  - quiescence search at the horizon (captures and promotions only),
  - MVV-LVA move ordering,
  - mate-distance-aware scoring (`mateScore - ply`),
  - a hard wall-clock budget checked every 2048 nodes; the last fully completed
    depth is returned.

### `internal/uci`

A line-oriented reader for `uci`, `isready`, `ucinewgame`, `position`
(`startpos` / `fen`, with `moves`), `go` (`depth`, `movetime`, `wtime`/`btime`),
`stop`, `d`, and `quit`. Search is synchronous, so `stop` is a no-op and
`bestmove` is emitted as soon as `go` returns.

## Deliberately not here yet

Transposition table, killer/history heuristics, null-move pruning, LMR, aspiration
windows, opening book, tablebases, pondering, `SearchMoves`/`MultiPV`. The Zobrist
hash needed for a TT is already exposed by `dragontoothmg`.

## Key invariants

1. **Move generation is never reimplemented.** Any bug there is a `dragontoothmg`
   bug or a misuse of its API; fix it with a perft test.
2. **`Search` leaves the board unmodified** — every `Apply` is paired with its
   unapply, including on early returns.
3. **Evaluation sign convention**: positive = good for the side to move.
4. **Untrusted input**: FEN and UCI strings come from GUIs and tournament
   managers. Parsing must not panic (see SECURITY.md).
