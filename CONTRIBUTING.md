# Contributing to goChess

Thanks for your interest in contributing.

## Getting started

1. Install Go (see `go.mod` for the minimum version) and `golangci-lint`.
2. Fork and clone the repository.
3. Run `make dev-tools` to install pinned development tooling.
4. Verify your setup: `make check` (build + lint + test).

## Development workflow

```bash
make build     # compile ./bin/gochess
make test      # go test -race ./...
make test-long # includes deep perft (perft 6, kiwipete depth 4)
make lint      # golangci-lint run
make vuln      # govulncheck ./...
make fmt       # gofumpt + goimports
make check     # everything CI runs
```

- Branch from `main`. Name branches `feature/…`, `fix/…`, or `chore/…`.
- Keep changes focused. One logical change per pull request.
- All code must pass `make check` before review.

## Commit messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(search): add transposition table
fix(uci): handle "go infinite" without a clock
docs: expand perft section in README
```

The changelog and release notes are generated from commit history.

## Pull requests

- Fill out the pull request template.
- Add or update tests. Engine correctness changes must include a perft or
  search regression test.
- Update `CHANGELOG.md` under the `## [Unreleased]` heading.
- Chess strength changes should include before/after results from a match
  (e.g. `cutechess-cli`, ≥1000 games or an SPRT result).

## Testing philosophy

- **Move generation**: perft counts against published reference values are
  authoritative. Never change move generation without a passing perft test.
- **Search**: assert on outcomes (finds mate, wins hanging material, does not
  blunder) rather than exact node counts, which are implementation-sensitive.
- Run `go test -race ./...` locally; the race detector is required in CI.

## Reporting bugs

Use the issue templates. For a wrong move or illegal move, include the FEN, the
move list, and the `go` command (depth or movetime) that reproduces it.

## Security

Do not open public issues for security problems. See [SECURITY.md](SECURITY.md).

## License

By contributing you agree that your contributions are licensed under the
[GPL-3.0](LICENSE).
