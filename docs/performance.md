# Performance

Benchmark of the current engine (iterative deepening, negamax alpha-beta,
bounded quiescence, MVV-LVA move ordering, a shared transposition table, Lazy
SMP, hard time limit; **no** killers/history, null-move pruning, or LMR yet) on
one machine: Apple M4, 10 cores, Go 1.27, `darwin/arm64`. Search is from the
starting position unless noted. "Nodes" is the engine's node counter (`negamax`
+ `quiesce` calls). Numbers are single-run and rounded; with a shared TT and
multiple workers the search is no longer bit-for-bit deterministic, so node
counts wobble a few percent between runs.

## What `go` / `go depth N` runs today: iterative deepening + TT (+ optional Lazy SMP)

`go` deepens from depth 1 until it runs out of `movetime` (or hits `depth N`)
and returns the last fully completed depth. Every `negamax` node probes and
stores the shared transposition table, and the stored move seeds move ordering.
With `Threads > 1` that same search runs on N goroutines over the one table
(Lazy SMP).

Single-threaded, `Hash 128`, generous budget so the run isn't deadline-capped.
"Before TT" is the pre-transposition-table baseline from earlier revisions of
this document:

| depth | time (before TT) | nodes (before TT) | time (with TT) | nodes (with TT) |
|------:|-----------------:|------------------:|---------------:|----------------:|
| 6 | 0.45s |   3,097,162 | 0.09s | ~0.6M |
| 7 | 3.4s  |  23,615,017 | 0.52s | ~3.1M |
| 8 | 30s   | 160,689,328 | 1.3s  | ~5.9M |

The transposition table is the single biggest lever in the engine's history —
it recovers most of the redundant re-search between iterations and across
transposing lines, and cuts depth-8-from-the-opening from ~30s to ~1.3s.

## Lazy SMP scaling

Basic Lazy SMP: workers share the TT and start at staggered depths, but there is
no root-move splitting or aspiration-window skew yet, so on TT-friendly
positions the workers largely re-explore the same tree. Fixed-depth wall-clock
is roughly flat; the value shows up as extra breadth (each worker's TT-perturbed
ordering occasionally finds a better line) and robustness on tactical positions,
plus headroom once root splitting lands. Approximate, `movetime 2000` from the
start position:

| threads | depth reached | nodes |
|--------:|--------------:|------:|
| 1 | 8 |  ~9.1M |
| 2 | 8 | ~17.8M |
| 4 | 8 | ~32.1M |
| 8 | 8 | ~42.6M |

Nodes scale with worker count (overlapping work); turning that into deeper
fixed-time search is what per-worker root-move splitting on the
[roadmap](../README.md#roadmap) is for.

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
clock never armed. It isolates search-tree efficiency, so this is the table to
watch when judging whether a search change helped. Not what the engine runs in a
game.

> **Stale since the TT landed.** The table below predates the transposition
> table. `Search` now always uses a TT (a private one when the caller passes
> none), so a single fixed-depth search from a cold table already benefits from
> intra-search transpositions — the real node counts are lower than shown and
> vary slightly run to run. Regenerate this table with a fresh benchmark.

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
