# Performance

Fresh benchmark of the current engine (iterative deepening, negamax alpha-beta,
bounded quiescence, MVV-LVA move ordering, hard time limit; **no** transposition
table, killers/history, null-move pruning, or LMR yet) on one machine: Apple M4,
10 cores, Go 1.27, `darwin/arm64`. Search is from the starting position; rows
stop once a search passes ~5 seconds. "Nodes" is the engine's node counter
(`negamax` + `quiesce` calls). Numbers are single-run and rounded — the search
is single-threaded and deterministic, so node counts repeat exactly and only the
wall-clock wobbles.

## What `go` / `go depth N` runs today: single-process iterative deepening

This is the real search path. There is no Lazy SMP or parallel search — `go`
runs one `Search` on one goroutine, deepening from depth 1 until it runs out of
`movetime` (or hits `depth N`), and returns the last fully completed depth. The
rows below use a generous budget so the run isn't deadline-capped.

| depth | time | nodes | bestmove |
|------:|-----:|------:|:---------|
| 4 | 0.01s |     55,614 | b1c3 |
| 5 | 0.06s |    457,203 | d2d4 |
| 6 | 0.45s |  3,097,162 | d2d4 |
| 7 | 3.4s  | 23,615,017 | e2e4 |
| 8 | 30s   | 160,689,328 | d2d4 |

(Cumulative nodes over the deepening series 1..N; the shallower iterations add
well under 1%.) Depth 7 from the opening lands in ~3s on this machine, depth 8 in
~30s. Adding a transposition table is the single biggest lever left — it would
recover most of the redundant re-search between iterations and across
transposing lines.

## Move generation: `perft` from the start position

Move generation is delegated to `dragontoothmg` and is not the bottleneck.
`gochess perft N` walks the full legal game tree with no evaluation or pruning:

| depth | nodes | time | nps |
|------:|------:|-----:|----:|
| 4 |        197,281 |   5ms | 42M |
| 5 |      4,865,609 |  47ms | 105M |
| 6 |    119,060,324 | 699ms | 170M |
| 7 |  3,195,901,860 | 19.9s | 160M |

~160–170M nodes/sec at the deeper counts, all of which match the published
[perft results](https://www.chessprogramming.org/Perft_Results).

## Single fixed-depth search from the start position

A controlled baseline: one search to exactly depth `N` (no iterative deepening),
clock never armed. Deterministic, and it isolates search-tree efficiency, so
this is the table to watch when judging whether a search change helped. Not what
the engine runs in a game.

The `perft(depth)` column is the size of the *full* legal game tree at that
depth — the branching the search would face with no alpha-beta at all. `pruned`
is `1 - nodes/perft`.

| depth | time | nodes | perft(depth) | pruned |
|------:|-----:|------:|-------------:|-------:|
| 2 | 0.000s |         452 |            400 |     −13% |
| 3 | 0.002s |       5,318 |          8,902 |      40% |
| 4 | 0.011s |      49,804 |        197,281 |      75% |
| 5 | 0.056s |     401,589 |      4,865,609 |    91.8% |
| 6 | 0.40s  |   2,639,959 |    119,060,324 |    97.8% |
| 7 | 2.9s   |  20,517,855 |  3,195,901,860 |    99.4% |
| 8 | 27s    | 137,074,311 | 84,998,978,956 |   99.84% |

The `pruned` column is an *estimate*: the two counts aren't the same unit.
"nodes" counts every node the search visits (internal nodes and quiescence
included, and quiescence looks past `depth` in forcing lines); `perft` is only
the leaves at exactly `depth`. At depth 2 the search actually expands *more*
nodes than the full tree — quiescence chases every capture sequence to its end
and there is no TT to catch repeats — so `pruned` goes negative. From depth 4 on
the trend is real: MVV-LVA ordering plus alpha-beta take the tree from "search
all of it" to "search one node in ~620" by depth 8.

The effective branching factor across the deeper rows is roughly 7 (≈√35, the
alpha-beta ideal for a ~35-move position would be ~6). Closing that last gap —
and cutting the constant factor — is what the transposition table, killer moves,
and null-move pruning on the [roadmap](../README.md#roadmap) are for.
