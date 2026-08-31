# Performance

> Note: the node counts and depths below were measured before principal
> variation search, aspiration windows and the tapered PeSTO evaluation landed.
> The shape of the story (narrow tree, modest SMP overlap) still holds; the
> exact figures will be lower on nodes / higher on depth. Re-benchmark before
> quoting.

Benchmark of the engine (iterative deepening, fail-soft negamax alpha-beta with
principal variation search and aspiration windows, bounded quiescence, a shared
aged transposition table, MVV-LVA + killer + history move ordering, null-move
pruning, late move reductions, Lazy SMP, hard time limit) on one machine: Apple
M4, 10 cores, Go 1.27, `darwin/arm64`. Search is from the starting position
unless noted. "Nodes" is the engine's node counter
(`negamax` + `quiesce` calls). Numbers are single-run and rounded; with a shared
TT and multiple workers the search is no longer bit-for-bit deterministic, so
node counts wobble a few percent between runs.

## What `go` / `go depth N` runs today

`go` deepens from depth 1 until it runs out of `movetime` (or hits `depth N`)
and returns the last fully completed depth. Every `negamax` node probes and
stores the shared transposition table; move ordering is TT move → promotions →
MVV-LVA captures → killer moves → history; interior nodes try a null move first
and reduce late quiet moves. With `Threads > 1` that same search runs on N
goroutines over the one table (Lazy SMP).

Single-threaded, `Hash 128`, generous budget so the run isn't deadline-capped.
The two earlier columns are from previous revisions of this document: "before
TT" is plain alpha-beta + quiescence, "TT only" adds the transposition table but
none of the ordering/pruning heuristics below.

| depth | nodes (before TT) | nodes (TT only) | time (now) | nodes (now) |
|------:|------------------:|----------------:|-----------:|------------:|
|  6 |   3,097,162 | ~0.6M | 0.02s |    33,000 |
|  7 |  23,615,017 | ~3.1M | 0.02s |    40,000 |
|  8 | 160,689,328 | ~5.9M | 0.07s |   167,000 |
|  9 |           — |     — | 0.15s |   364,000 |
| 10 |           — |     — | 0.23s |   548,000 |
| 11 |           — |     — | 0.67s | 1,426,000 |
| 12 |           — |     — | 1.13s | 2,285,000 |

Killers/history + null-move pruning + LMR are the second big lever after the TT:
a depth-8 search from the opening drops from ~5.9M nodes to ~167k (~35×), and the
engine now reaches depth 12 in about the wall-clock time depth 8 used to take.
Node rate is roughly unchanged at ~2.5M nps (`gochess bench`) — each node costs
a little more now (a static eval and a check test per interior node, plus a
FEN round-trip per null move) but there are far fewer of them.

The effective branching factor across the deeper rows is ~1.8 (√ of the
node-count ratio between adjacent depths), versus ~7 for TT-only alpha-beta and
√35 ≈ 5.9 for the theoretical alpha-beta minimum — reductions and null-move
pruning search a tree much narrower than full-width minimax.

## Lazy SMP scaling

Lazy SMP: workers share the TT (aged per search) and start at staggered depths;
helpers also skew their root move order and use a wider, asymmetric aspiration
window. There is still no explicit root-move splitting or shared-PV coordination,
so on TT-friendly positions the workers overlap substantially. The value shows
up as extra breadth and tactical robustness, and as a modest depth gain at fixed
time. Approximate, `movetime 2000` from the start position:

| threads | depth reached | nodes |
|--------:|--------------:|------:|
| 1 | 13 |  ~3.8M |
| 2 | 13 |  ~7.9M |
| 4 | 14 | ~14.4M |
| 8 | 14 | ~20.2M |

Nodes scale with worker count (overlapping work); turning that into consistently
deeper fixed-time search is what per-worker root-move splitting on the
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

## Regenerating these numbers

```bash
gochess bench                     # 4-position fixed-depth-6 node/nps summary
gochess perft 6                   # timed perft series
```

The per-depth tables above come from a short throwaway test that calls
`engine.Search` with `SearchParams{MaxDepth: d, TT: engine.NewTT(128)}` for each
depth and prints `res.Elapsed` / `res.Nodes`. Re-run it after any search change
and update the "nodes (now)" column; a regression there is the first sign a
heuristic is mis-tuned.
