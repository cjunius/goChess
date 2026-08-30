# Engine Strength

**Estimated playing strength: ~1500–1750 Elo (likely ~1600) on the CCRL blitz
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
- **Move ordering** — promotions first, then captures by MVV-LVA (most valuable
  victim, least valuable attacker), then quiet moves. Good ordering is what
  makes alpha-beta actually prune.
- **Evaluation** — material values + Michniewski piece-square tables + a
  bishop-pair bonus. Crude, but it captures development, central control, king
  placement and the two-bishop advantage.
- **Compiled and fast** — ~2.7M nodes/sec in a single thread on an Apple M4
  (`gochess bench`). That is ~50–60× the node rate of a typical Python engine,
  so effective search depth is respectable even without a transposition table.
- **Robust I/O** — FEN and UCI parsing is fuzz-safe and non-panicking; the
  engine will not forfeit on a malformed GUI command.

### Limiting factors

- **No transposition table.** The Zobrist hash is already exposed by
  `dragontoothmg`, but nothing caches search results yet. Transpositions are
  re-searched from scratch, and there is no hash move to seed move ordering.
  This is the single largest missing feature — worth an estimated 150–250 Elo.
- **No killer or history heuristic.** Quiet-move ordering is essentially
  arbitrary, so cut-offs deep in the tree are later than they should be.
- **No null-move pruning, no late move reductions, no futility or delta
  pruning, no aspiration windows.** The tree is close to full-width
  alpha-beta + quiescence. Effective middlegame depth is roughly 6–9 ply at
  blitz time controls, where a pruning-heavy engine of the same speed would
  reach 12–16.
- **No search extensions** (check extensions, singular extensions). Tactical
  lines that need one extra ply past the horizon are missed.
- **Hand-set evaluation weights, never tuned.** No Texel tuning, no
  game-phase interpolation (a single PST set is used from opening to endgame),
  no explicit terms for passed pawns, pawn structure, mobility, rook-on-open-
  file, or king safety beyond the king PST.
- **Thin endgame play.** No tablebase probing, no KPK / KBNK knowledge, no
  contempt. The 50-move rule is honoured but threefold repetition is not
  detected inside the search.
- **Single-threaded.** No Lazy SMP or any other parallel search; the other
  nine cores of the test machine sit idle.
- **Synchronous search.** `stop` is a no-op and there is no pondering, so the
  engine cannot think on the opponent's clock or bail out of a bad time
  allocation.

## Calibration against known engines

| Reference engine | ~CCRL blitz | Relevant comparison |
| ---------------- | ----------- | ------------------- |
| TSCP 1.81 | ~1700 | Similar search shape (alpha-beta, iterative deepening, quiescence, MVV-LVA) **plus** a transposition table. goChess should land at or just below TSCP until the TT lands. |
| Sungorus 1.4 | ~2000 | TT + null-move + PVS + killers. Clearly stronger than goChess today. |
| CT800 / Claudia class | ~2100+ | Full modern pruning set. Out of reach without the roadmap features. |

The feature set most closely resembles a "first working alpha-beta engine" —
tactically sound at shallow depth, positionally simplistic, and losing rating to
every missing pruning technique. The fast node rate keeps it from dropping into
true beginner territory.

## Time-control sensitivity

Deeper search helps goChess more than most engines, because without pruning its
depth is unusually shallow for its speed — each extra ply is high-value.

| Time control | Estimated Elo | Notes |
| ------------ | ------------- | ----- |
| Bullet (1+0) | ~1350–1550 | Depth 4–6; positional weaknesses dominate. |
| Blitz (3+2 / 5+0) | ~1500–1750 | Depth 6–9; the headline estimate. |
| Rapid (15+10) | ~1650–1900 | Depth 9–12; tactics get sharper, eval ceiling starts to bite. |
| Classical (40/40) | ~1750–2000 | Depth-limited by the missing TT more than by the clock. |

## Where the number would move

Rough, independent Elo deltas from the [roadmap](../README.md#roadmap),
assuming each is implemented competently and validated by SPRT:

| Change | Estimated Elo |
| ------ | ------------- |
| Transposition table + hash-move ordering | +150 to +250 |
| Killer moves + history heuristic | +50 to +100 |
| Null-move pruning | +50 to +80 |
| Late move reductions | +50 to +100 |
| Aspiration windows | +10 to +30 |
| Game-phase eval interpolation (tapered eval) | +30 to +60 |
| Passed pawns / king safety / mobility terms | +40 to +80 |
| Texel-tuned evaluation weights | +40 to +80 |
| Opening book (Polyglot) | +20 to +40 at short TC |
| Syzygy tablebase probing | +10 to +20 |
| Lazy SMP (8 threads) | +100 to +150 |

Landing the transposition table, killers/history, null-move and LMR together
would plausibly put goChess in the 1950–2150 range; adding tapered/tuned eval
and Lazy SMP on top targets 2300+.

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
