# Engine Strength

**Estimated playing strength: ~1900–2150 Elo (likely ~2000) on the CCRL blitz
scale.**

This is a reasoned estimate from the feature set and search speed, **not a
measured result**. goChess has not yet played a rated match or an SPRT gauntlet
against calibrated opponents. Treat the number as an order-of-magnitude guide
until [the strength-testing harness](#measuring-it-properly) produces real data.

## How the estimate is derived

### Positive contributors

- **Move generation** — delegated to `dragontoothmg`, a magic-bitboard legal
  move generator, and verified exact with perft (start position to depth 6,
  Kiwipete to depth 4). No legality bugs, no pseudo-legal filtering cost.
- **Search** — iterative-deepening negamax with alpha-beta pruning, a fail-hard
  window, and mate-distance-aware scoring (`mateScore - ply`, so the engine
  prefers the shortest mate and the longest defence).
- **Quiescence search** at the horizon (captures and promotions only), so the
  static evaluation is never taken in the middle of a capture sequence — this
  alone removes most one-move blunders.
- **Move ordering** — hash move, then promotions, then captures by MVV-LVA (most
  valuable victim, least valuable attacker), then the two killer moves for the
  ply, then quiet moves by history score. Good ordering is what makes alpha-beta
  actually prune, and it is also what makes null-move pruning and LMR safe.
- **Evaluation** — tapered PeSTO material and piece-square tables plus
  bishop-pair, passed-pawn, mobility, king-safety and tempo terms. Untuned, but
  it captures development, central control, king safety, the two-bishop
  advantage, and the middlegame/endgame shift.
- **Compiled and fast** — ~2.5M nodes/sec in a single thread on an Apple M4
  (`gochess bench`), and Lazy SMP puts the other cores to work. That is ~50–60×
  the node rate of a typical Python engine, so effective search depth is
  respectable — and with reductions and null-move pruning narrowing the tree,
  the single-threaded search now reaches depth 12 from the opening in ~1s.
- **Robust I/O** — FEN and UCI parsing is fuzz-safe and non-panicking; the
  engine will not forfeit on a malformed GUI command.

### Recently added

- **Transposition table + hash-move ordering.** A shared, lock-free table keyed
  by the `dragontoothmg` Zobrist hash now caches EXACT / LOWER / UPPER bounds
  and the best move (mate scores rebased by ply). It roughly cuts the node count
  of a depth-7 search from the opening by ~7× and seeds move ordering with the
  hash move.
- **Lazy SMP.** `Threads` workers deepen in parallel on private board copies
  over the one shared table. With 4–8 cores this reaches a given depth several
  times faster than the single-threaded search did.
- **Killer moves + history heuristic.** Quiet moves that cause a beta cut-off
  are tried first at sibling nodes (two killers per ply) and accumulate a
  `depth²` bonus in a `[side][from][to]` history table that orders the rest.
  Both tables are per-searcher, so Lazy-SMP workers stay independent.
- **Null-move pruning** (`R = 2`, `3` from depth 6) with the standard guards
  (not in check, depth ≥ 3, non-mate beta, non-pawn material, no consecutive
  nulls).
- **Late move reductions.** Late quiet moves are searched 1–2 ply shallower and
  re-searched at full depth only when the reduced search beats alpha.
- **Principal variation search + aspiration windows.** Only the first move at a
  node gets a full window; the rest are null-window scouts. From depth 5 each
  iteration starts inside a ±25 cp window around the previous score.
- **Tapered PeSTO evaluation** with passed-pawn, mobility and king-safety terms,
  replacing the single Michniewski PST set.
- **Asynchronous search** — `stop` aborts immediately and pondering
  (`go ponder` / `ponderhit`) lets the engine think on the opponent's clock.
- **TT ageing + richer Lazy SMP** — a generation counter reclaims the previous
  search's entries, and helper workers skew their root move order and aspiration
  window.
- **Polyglot opening book** (`OwnBook` / `BookFile`).

Together these cut a depth-8 search from the opening from ~5.9M nodes to ~167k
and let the single-threaded search reach depth 12 in roughly the time depth 8
used to cost. See [performance.md](performance.md).

### Limiting factors

- **No search extensions** (check extensions, singular extensions). Tactical
  lines that need one extra ply past the horizon are missed.
- **Hand-set evaluation weights, never tuned.** The tapered PeSTO tables and the
  passed-pawn / mobility / king-safety coefficients are literature defaults, not
  Texel- or gradient-tuned for this engine. No isolated/doubled/backward pawn,
  rook-on-open-file or outpost terms yet.
- **Thin endgame play.** No tablebase probing (see
  [ADR 0003](adr/0003-defer-syzygy-tablebases.md)), no KPK / KBNK knowledge, no
  contempt. The 50-move rule is honoured but threefold repetition is not
  detected inside the search.
- **Lazy SMP is still "lazy".** Helpers skew their root order and aspiration
  window and the TT is aged, but there is no explicit root-move splitting or
  shared-PV coordination, so scaling past a handful of threads is modest.
- **No SEE.** Captures are ordered by MVV-LVA only; losing captures are not
  pruned in quiescence.

## Calibration against known engines

| Reference engine | ~CCRL blitz | Relevant comparison |
| ---------------- | ----------- | ------------------- |
| TSCP 1.81 | ~1700 | Similar search shape but no null-move / LMR. goChess should now be clearly stronger. |
| Sungorus 1.4 | ~2000 | TT + null-move + PVS + killers — a close match to goChess's search feature set. goChess should be around here or a little above. |
| CT800 / Claudia class | ~2100+ | Full modern pruning set plus a tuned eval. Reachable once the evaluation weights are tuned and a few structural terms are added. |

The feature set now resembles a "complete first-generation pruning engine" with
a tapered evaluation — tactically sharp for its node rate; the remaining
positional gaps (untuned weights, no pawn-structure or outpost terms) are the
main thing left holding the rating down.

## Time-control sensitivity

Deeper search still helps goChess more than most engines: the evaluation is the
ceiling, so every extra ply that sharpens the tactics is high-value.

| Time control | Estimated Elo | Notes |
| ------------ | ------------- | ----- |
| Bullet (1+0) | ~1750–1950 | Depth 8–11; the crude eval costs the most here. |
| Blitz (3+2 / 5+0) | ~1900–2150 | Depth 11–14; the headline estimate. |
| Rapid (15+10) | ~2000–2250 | Depth 14–18; tactics rarely miss, eval ceiling bites. |
| Classical (40/40) | ~2050–2300 | Eval-limited more than depth-limited. |

## Where the number would move

Rough, independent Elo deltas from the [roadmap](../README.md#roadmap),
assuming each is implemented competently and validated by SPRT:

| Change | Estimated Elo | Status |
| ------ | ------------- | ------ |
| Transposition table + hash-move ordering | +150 to +250 | done, pending SPRT |
| Lazy SMP (8 threads) | +100 to +150 | done (basic), pending SPRT |
| Killer moves + history heuristic | +50 to +100 | done, pending SPRT |
| Null-move pruning | +50 to +80 | done, pending SPRT |
| Late move reductions | +50 to +100 | done, pending SPRT |
| Aspiration windows / PVS | +20 to +50 | done, pending SPRT |
| Game-phase eval interpolation (tapered eval) | +30 to +60 | done, pending SPRT |
| Passed pawns / king safety / mobility terms | +40 to +80 | done, pending SPRT |
| Texel-tuned evaluation weights | +40 to +80 | |
| Opening book (Polyglot) | +20 to +40 at short TC | done, pending SPRT |
| Syzygy tablebase probing | +10 to +20 | deferred ([ADR 0003](adr/0003-defer-syzygy-tablebases.md)) |

With the TT, Lazy SMP, killers/history, null-move pruning, LMR, PVS/aspiration
and a tapered evaluation all in, goChess plausibly sits in the ~2000–2250 range;
tuning the evaluation weights and adding a few structural terms is what targets
2300+.

## Measuring it properly

The estimate above is replaced by data as soon as there is a gauntlet result.
The intended process (see [docs/development.md](development.md#strength-testing)):

1. Build the candidate and the previous release as two binaries.
2. Run `cutechess-cli` self-play with an SPRT stopping rule
   (`elo0=0 elo1=5`, `alpha=beta=0.05`) at `tc=10+0.1`, from a varied EPD
   opening book, `-repeat`, `-concurrency 8`.
3. Only merge a search/eval change if it passes SPRT (or is Elo-neutral and
   justified on other grounds).
4. Periodically run a gauntlet against a ladder of rated reference engines
   (TSCP, Sungorus, Fruit 2.1, …) to anchor an absolute CCRL-comparable number.

Until step 4 has run, cite this document as an estimate, not a rating.
