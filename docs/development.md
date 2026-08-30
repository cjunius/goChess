# Development

## Prerequisites

- Go (see `go.mod` for the minimum version)
- `make`
- `golangci-lint`, `gofumpt`, `goimports`, `govulncheck` — `make dev-tools`

## First-time setup

```bash
git clone https://github.com/cjunius/goChess
cd goChess
go mod download
make dev-tools
make check      # tidy + lint + vuln + test
```

## Common tasks

| Command          | What it does                                        |
| ---------------- | --------------------------------------------------- |
| `make build`     | compile `./bin/gochess`                             |
| `make run`       | build and start UCI mode                            |
| `make test`      | `go test -race -short ./...`                        |
| `make test-long` | full suite incl. perft 6 and Kiwipete depth 4      |
| `make bench`     | Go micro-benchmarks                                 |
| `make cover`     | write `coverage.html`                               |
| `make lint`      | `golangci-lint run`                                 |
| `make vuln`      | `govulncheck ./...`                                 |
| `make fmt`       | `gofumpt` + `goimports`                             |

## Manual UCI session

```
$ ./bin/gochess
uci
id name goChess dev
id author Christopher Junius
uciok
position startpos moves e2e4
go movetime 500
info depth 6 score cp 24 nodes 41233 time measured pv e7e5
bestmove e7e5
quit
```

## Perft

Perft counts leaf nodes of the move tree and is the definitive move-generation
test.

```bash
./bin/gochess perft 6
./bin/gochess perft 4 "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1"
```

If a count is wrong, use `engine.PerftDivide` to find which move's subtree
diverges from a reference (e.g. Stockfish `go perft N`), then recurse.

## Strength testing

Search and evaluation changes must be validated with a match, not intuition.
Use `cutechess-cli` against the previous build:

```bash
cutechess-cli \
  -engine cmd=./bin/gochess-new name=new \
  -engine cmd=./bin/gochess-old name=old \
  -each proto=uci tc=10+0.1 -games 2 -rounds 1000 -repeat \
  -openings file=book.epd format=epd order=random \
  -sprt elo0=0 elo1=5 alpha=0.05 beta=0.05 \
  -concurrency 8
```

## Releasing

See [../MAINTAINERS.md](../MAINTAINERS.md#release-process).
