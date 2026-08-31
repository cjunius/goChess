# goChess

[![CI](https://github.com/cjunius/goChess/actions/workflows/ci.yml/badge.svg)](https://github.com/cjunius/goChess/actions/workflows/ci.yml)
[![Lint](https://github.com/cjunius/goChess/actions/workflows/lint.yml/badge.svg)](https://github.com/cjunius/goChess/actions/workflows/lint.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/cjunius/goChess.svg)](https://pkg.go.dev/github.com/cjunius/goChess)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](LICENSE)

A UCI chess engine written in Go, built on the
[`dragontoothmg`](https://github.com/dylhunn/dragontoothmg) magic-bitboard legal
move generator.

## Status

Early. The engine plays legal chess via a UCI loop with:

- **Move generation** — delegated to `dragontoothmg` (verified with perft).
- **Evaluation** — tapered PeSTO material + piece-square tables, bishop pair,
  passed pawns, mobility, king safety, tempo.
- **Search** — iterative deepening with aspiration windows, principal variation
  search, quiescence search, a shared aged transposition table with hash-move
  ordering, MVV-LVA + killer-move + history move ordering, null-move pruning,
  late move reductions, Lazy SMP (multi-threaded search), asynchronous search
  with working `stop` and pondering, hard time limits.
- **Opening book** — optional Polyglot `.bin` book.

Not yet implemented: Syzygy endgame tablebases, static exchange evaluation.
See the [roadmap](#roadmap).

## Install

```bash
go install github.com/cjunius/goChess/cmd/gochess@latest
```

Or build from source:

```bash
git clone https://github.com/cjunius/goChess
cd goChess
make build            # produces ./bin/gochess
```

## Usage

```bash
gochess                      # UCI mode — point a GUI (CuteChess, Arena) at this
gochess perft 6              # timed perft series from the start position
gochess perft 4 "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1"
gochess bench                # fixed search benchmark (nodes/sec)
gochess version
```

### Playing a game

Add the binary as an engine in any UCI GUI, or pipe commands directly:

```
setoption name Hash value 128
setoption name Threads value 8
position startpos moves e2e4 e7e5
go movetime 1000
```

### UCI options

| Option | Default | Range | Meaning |
|---|---|---|---|
| `Hash` | 64 | 1–4096 | Transposition-table size in MiB. |
| `Threads` | 1 | 1–256 | Lazy-SMP worker count (capped at the machine's core count at search time). |
| `Ponder` | false | check | Let a GUI drive `go ponder` / `ponderhit` so the engine thinks on the opponent's clock. |
| `OwnBook` | false | check | Play from the Polyglot book when the position is in it. |
| `BookFile` | — | string | Path to a Polyglot `.bin` book; loaded when set. |

## Development

```bash
make test          # go test -race ./...
make lint          # golangci-lint
make vuln          # govulncheck
make cover         # coverage report
```

See [CONTRIBUTING.md](CONTRIBUTING.md) and [docs/development.md](docs/development.md).

## Project layout

```
cmd/gochess/       main entry point + CLI subcommands
internal/engine/   perft, evaluation, search
internal/uci/      UCI protocol loop
docs/              architecture notes and ADRs
```

## Roadmap

- [x] Move generation on `dragontoothmg` + perft regression tests
- [x] `Evaluate(pos)` — material + piece-square tables
- [x] Negamax + alpha-beta + iterative deepening + time budget
- [x] UCI loop
- [x] Quiescence search + MVV-LVA move ordering
- [x] Transposition table (Zobrist hash from `dragontoothmg`) + hash-move ordering
- [x] Lazy SMP — multi-threaded search over the shared TT (`Threads` UCI option)
- [x] Killer moves + history heuristic
- [x] Null-move pruning + late move reductions
- [x] Aspiration windows + principal variation search
- [x] Tapered PeSTO evaluation + pawn/mobility/king-safety terms
- [x] Asynchronous search — working `stop` and pondering
- [x] Opening book (Polyglot)
- [ ] Syzygy tablebase probing ([ADR 0003](docs/adr/0003-defer-syzygy-tablebases.md))
- [ ] Strength testing harness (SPRT via cutechess-cli)

## License

[GPL-3.0](LICENSE). `dragontoothmg` is also GPL-3.0.
