# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Move generation built on `github.com/dylhunn/dragontoothmg`.
- `internal/engine`: perft (with start-position and Kiwipete regression tests),
  static evaluation (material + piece-square tables + bishop pair), and an
  iterative-deepening negamax alpha-beta search with quiescence search,
  a shared lock-free transposition table, Lazy SMP, and a hard time budget.
- `internal/engine`: search heuristics — killer moves + a `[side][from][to]`
  history table in move ordering, null-move pruning, and late move reductions.
- `internal/engine`: principal variation search with a triangular PV table,
  aspiration windows from depth 5, and a fail-soft alpha-beta.
- `internal/engine`: richer Lazy SMP — a 6-bit transposition-table generation
  that ages out the previous search's entries, plus per-helper root-move and
  aspiration-window skew.
- `internal/engine`: tapered PeSTO evaluation (middlegame/endgame material and
  piece-square tables interpolated by game phase) with passed-pawn, mobility,
  king-safety and tempo terms, guarded by a colour-mirror symmetry test.
- `internal/engine`: hand-rolled Polyglot opening book — `PolyglotKey`,
  `OpenBook`, `Book.Probe`.
- `internal/uci`: a UCI protocol loop (`uci`, `isready`, `ucinewgame`,
  `position`, `go`, `stop`, `ponderhit`, `quit`) supporting `go depth`,
  `go movetime`, `go wtime/btime/winc/binc`, `go infinite`, and `go ponder`.
  Search runs on a goroutine: `stop` and pondering work, `info … pv …` is
  streamed per iteration, and `bestmove` carries a `ponder` move.
- `internal/uci`: `Ponder`, `OwnBook` and `BookFile` options.
- `cmd/gochess`: CLI with `uci`, `perft`, `bench`, and `version` subcommands.
- Repository scaffolding: CI, lint, CodeQL and release workflows; issue and PR
  templates; `CODEOWNERS`; Dependabot; `Makefile`; `Dockerfile`; GoReleaser.
- `docs/adr/0003`: record deferring Syzygy tablebases (no pure-Go prober; the
  build is strictly `CGO_ENABLED=0`).

### Changed

- Replaced the `notnil/chess` prototype (random mover + perft) with the
  `dragontoothmg`-based engine.
- Evaluation is now tapered (PeSTO) rather than a single Michniewski
  piece-square table set; move choices and reported scores shift accordingly.

### Removed

- Retired the Go Report Card badge from the README.

[Unreleased]: https://github.com/cjunius/goChess/commits/main
