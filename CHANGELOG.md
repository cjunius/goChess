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
- `internal/uci`: a UCI protocol loop (`uci`, `isready`, `ucinewgame`,
  `position`, `go`, `stop`, `quit`) supporting `go depth`, `go movetime`, and
  `go wtime/btime`.
- `cmd/gochess`: CLI with `uci`, `perft`, `bench`, and `version` subcommands.
- Repository scaffolding: CI, lint, CodeQL and release workflows; issue and PR
  templates; `CODEOWNERS`; Dependabot; `Makefile`; `Dockerfile`; GoReleaser.

### Changed

- Replaced the `notnil/chess` prototype (random mover + perft) with the
  `dragontoothmg`-based engine.

[Unreleased]: https://github.com/cjunius/goChess/commits/main
