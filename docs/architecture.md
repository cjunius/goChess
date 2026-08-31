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
- **eval.go / eval_terms.go** — `Evaluate(*Board) int`. Tapered PeSTO material +
  piece-square tables interpolated by game phase, plus bishop-pair, passed-pawn,
  mobility, king-safety and tempo terms (each an `(mg, eg)` pair). Returns a
  score relative to the side to move (negamax convention). Piece-square tables
  are stored a8-first: white reads `pst[sq^56]`, black reads `pst[sq]`.
- **search.go** — `Search(*Board, SearchParams) SearchResult`. Iterative
  deepening around a fail-soft negamax alpha-beta core, with:
  - principal variation search (full window for move 0, null-window scout +
    re-search for the rest) and a triangular PV table,
  - aspiration windows from depth 5, widening the failing side,
  - quiescence search at the horizon (captures and promotions only),
  - transposition-table probes/stores with hash-move ordering,
  - move ordering: TT move, promotions, MVV-LVA captures, two killer moves
    per ply, then quiet moves by a `[side][from][to]` history score,
  - null-move pruning and late move reductions,
  - mate-distance-aware scoring (`mateScore - ply`), ply-rebased through the TT,
  - Lazy SMP: `SearchParams.Threads` workers deepen independently on their own
    board copy over one shared TT, with per-helper root-move and aspiration
    skew; the deepest completed result wins,
  - `SearchParams.Stop` / `Ponder` / `PonderHit` / `Info` for asynchronous
    driving: a hard wall-clock budget checked every 2048 nodes (ignored while
    pondering), an external abort flag, and a per-iteration callback.
- **transposition.go** — `TT`, a fixed-size power-of-two table keyed by
  `Board.Hash`. Each 16-byte slot is a pair of `atomic.Uint64` words accessed
  with Hyatt's lockless XOR trick (`word0 = key ^ data`), so the Lazy-SMP
  workers share it without a mutex; a write torn across goroutines reads as a
  miss. `data` packs move, int32 score, depth, bound flag and a 6-bit
  generation; `NewSearch` bumps the generation so `store` treats the previous
  search's entries as replaceable.
- **polyglot.go** — `PolyglotKey`, `OpenBook`, `Book.Probe`: the Polyglot book
  Zobrist key (distinct from `Board.Hash`) and a reader for `.bin` book files.

### `internal/uci`

A line-oriented reader for `uci`, `isready`, `setoption`, `ucinewgame`,
`position` (`startpos` / `fen`, with `moves`), `go` (`depth`, `movetime`,
`wtime`/`btime`/`winc`/`binc`, `infinite`, `ponder`), `stop`, `ponderhit`, `d`,
and `quit`. It owns the persistent `engine.TT` (sized by the `Hash` option,
cleared on `ucinewgame`) and the `Threads` / `Ponder` settings.

Search runs on its own goroutine: `go` returns immediately, `engine.Search`
streams `info` lines through a callback and the goroutine emits `bestmove`
(with a `ponder` move from the PV) once the search ends. `stop` aborts it;
`ponderhit` converts a pondering search onto its time budget; a mutex serialises
all writes to stdout. `quit` / EOF wait for a bounded search a GUI is blocking on
and abort anything infinite.

## Deliberately not here yet

Static exchange evaluation, search extensions, opening book, tablebases,
`SearchMoves`/`MultiPV`.

## Key invariants

1. **Move generation is never reimplemented.** Any bug there is a `dragontoothmg`
   bug or a misuse of its API; fix it with a perft test.
2. **`Search` leaves the board unmodified** — every `Apply` is paired with its
   unapply, including on early returns.
3. **Evaluation sign convention**: positive = good for the side to move.
4. **Untrusted input**: FEN and UCI strings come from GUIs and tournament
   managers. Parsing must not panic (see SECURITY.md).
5. **The TT is the only shared mutable state between Lazy-SMP workers.** Every
   worker has its own `dragontoothmg.Board` copy and its own `searcher`; the
   table is safe for concurrent use, so no other synchronisation is needed.
