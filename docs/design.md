# Design

How the engine is put together, what each technique buys, and the backlog of
ideas not yet implemented. For the component diagram and invariants see
[architecture.md](architecture.md); for build, perft, and strength-testing
workflow see [development.md](development.md); for the reasons behind the big
choices see the [ADRs](adr/).

## Architecture

`engine.Search` is the search core. It is a plain function around a `searcher`
value (node counter, deadline, per-ply killer moves, `[side][from][to]` history
table, triangular PV table) with a few free collaborators rather than an
injected object graph — small enough that dependency injection would cost more
than it buys.

| collaborator | responsibility |
|---|---|
| `dragontoothmg.Board` | board state, legal move generation, `Apply` / unapply, Zobrist hash, FEN — never reimplemented (see [ADR 0002](adr/0002-use-dragontoothmg-for-move-generation.md)) |
| `engine.Evaluate` | static evaluation from the side-to-move's view (positive = better for that side) |
| `searcher.orderMoves` | orders moves to maximise alpha-beta cut-offs: TT/hash move, promotions, MVV-LVA captures, the two killer moves for the ply, then quiets by history score |
| `engine.TT` | shared, lock-free transposition table keyed by `Board.Hash`; probed/stored inside `negamax` and seeded into move ordering |
| `searcher` | carries the node count, TT handle, shared stop flag, wall-clock deadline, and the per-searcher killer/history tables; answers `timeUp` and flips `stopped` |

`internal/uci` is the protocol layer and the only place that prints `info` /
`bestmove`. It owns the persistent `engine.TT` (`Hash` option), the `Threads` /
`Ponder` settings, and the optional Polyglot book (`OwnBook` / `BookFile`). It
runs `Search` on a goroutine and serialises stdout with a mutex, so `stop`,
`ponderhit` and streamed `info` all work while the search runs. `cmd/gochess`
wires these together and adds the `perft` and `bench` subcommands.

## Implemented

### [Search](https://www.chessprogramming.org/Search)

- [Negamax](https://www.chessprogramming.org/Negamax) with [alpha-beta pruning](https://www.chessprogramming.org/Alpha-Beta), fail-soft, and [Principal Variation Search](https://www.chessprogramming.org/Principal_Variation_Search) — the first move at each node gets a full window, later moves a null-window scout re-searched only when it beats alpha (LMR is folded in as an extra reduction on that scout)
- [Iterative Deepening](https://www.chessprogramming.org/Iterative_Deepening) with [Aspiration Windows](https://www.chessprogramming.org/Aspiration_Windows) — from depth 5 each iteration first searches a ±25 cp window around the previous score, doubling the failing side until the score lands inside. The last fully completed depth is the one returned
- A real [principal variation](https://www.chessprogramming.org/Principal_Variation) — a triangular PV table produces the full line, streamed in the `info … pv …` output each iteration
- [Transposition Table](https://www.chessprogramming.org/Transposition_Table) — power-of-two table keyed by `dragontoothmg.Board.Hash()`; stores EXACT / LOWER / UPPER bounds with the best move, mate scores rebased by ply on store and probe. Lock-free (Hyatt XOR) so Lazy-SMP workers share one table. The stored move seeds move ordering even when the entry is too shallow to cut. A 6-bit generation ages entries: a new search (`NewSearch`) reclaims the previous search's slots regardless of their depth
- [Lazy SMP](https://www.chessprogramming.org/Lazy_SMP) — `Threads` workers run iterative deepening in parallel on private board copies over the shared TT; workers start at staggered depths, and helpers additionally skew their root move order and use a wider, asymmetric aspiration window so they diverge. The deepest completed result wins
- Asynchronous search — the UCI layer runs `Search` on a goroutine, so `stop` aborts immediately and pondering (`go ponder` / `ponderhit`) works; a pondering search ignores the clock until the ponder move is confirmed
- [Quiescence Search](https://www.chessprogramming.org/Quiescence_Search) at the horizon — captures and promotions only, depth-bounded by `maxPly`
- [Null Move Pruning](https://www.chessprogramming.org/Null_Move_Pruning) — `R = 2`, or `3` from depth 6; tried only when not in check, at depth ≥ 3, with a non-mate beta, with non-pawn material for the side to move, and not immediately after another null move. The pass position is built through FEN because `dragontoothmg` keeps the Zobrist hash and en-passant square in unexported fields
- [Late Move Reductions](https://www.chessprogramming.org/Late_Move_Reductions) — from depth 3, quiet moves past the third in the ordered list are searched 1 ply shallower (2 from the seventh move at depth ≥ 5); a reduced search that beats alpha is repeated at full depth. Moves that give or evade check are never reduced
- [Move Ordering](https://www.chessprogramming.org/Move_Ordering) — TT/hash move, then promotions, then [MVV-LVA](https://www.chessprogramming.org/MVV-LVA) captures, then the two [killer moves](https://www.chessprogramming.org/Killer_Heuristic) for the ply, then quiet moves by [history](https://www.chessprogramming.org/History_Heuristic) score (`depth²` per beta cut-off, per `[side][from][to]`, clamped). Killer and history tables are per-searcher, so Lazy-SMP workers keep independent copies
- [Mate-distance scoring](https://www.chessprogramming.org/Mate_Distance_Pruning) — `mateScore - ply`, so the shortest mate is preferred; a proven mate ends iterative deepening early
- Draw detection — the fifty-move rule (`Halfmoveclock >= 100`) is scored `0` inside the tree
- Time management — a hard wall-clock budget checked every 2048 nodes; the UCI layer spends `1/30` of the remaining clock plus `3/4` of the increment when the GUI sends `wtime` / `btime` (`winc` / `binc`) instead of `movetime`
- [Opening book](https://www.chessprogramming.org/Opening_Book) — optional [Polyglot](http://hgm.nubati.net/book_format.html) `.bin` book (`OwnBook` / `BookFile`); a book hit is played instantly with no search
- King-capture guard — scores the illegal position `dragontoothmg` can hand back when the side not to move was already in check, instead of panicking on an empty king bitboard

### [Evaluation](https://www.chessprogramming.org/Evaluation)

- [Tapered eval](https://www.chessprogramming.org/Tapered_Eval) — [PeSTO](https://www.chessprogramming.org/PeSTO%27s_Evaluation_Function) middlegame/endgame material values and piece-square tables interpolated by game phase (`N/B = 1`, `R = 2`, `Q = 4`, clamped to 24); black reads the vertically mirrored square (`sq^56`)
- [Bishop pair](https://www.chessprogramming.org/Bishop_Pair), [passed pawns](https://www.chessprogramming.org/Passed_Pawn) (bonus by rank, endgame-weighted), [mobility](https://www.chessprogramming.org/Mobility) for N/B/R/Q, [king safety](https://www.chessprogramming.org/King_Safety) (attacker weight in the king ring + missing-pawn-shield penalty, middlegame only), and a [tempo](https://www.chessprogramming.org/Tempo) bonus — all accumulated as `(mg, eg)` pairs and folded into the taper
- A colour-mirror symmetry test guards the whole evaluation

Evaluation is recomputed from scratch on every call; there is no incremental
update and no pawn or evaluation hash.

## Backlog

### Search

- Tuning the aspiration / null-move / LMR formulas (adaptive `R`, reduction from the history score) against SPRT
- [Check extensions](https://www.chessprogramming.org/Check_Extensions) and other search extensions
- [Static Exchange Evaluation](https://www.chessprogramming.org/Static_Exchange_Evaluation) for capture ordering and bad-capture pruning in quiescence
- Threefold-[repetition](https://www.chessprogramming.org/Repetitions) detection (needs a position history the current `Search` does not keep)
- [Syzygy endgame tablebases](https://www.chessprogramming.org/Endgame_Tablebases) — deferred; see [ADR 0003](adr/0003-defer-syzygy-tablebases.md)

### Evaluation

- Pawn structure — [isolated](https://www.chessprogramming.org/Isolated_Pawn) / [doubled](https://www.chessprogramming.org/Doubled_Pawn) / [backward](https://www.chessprogramming.org/Backward_Pawn) pawns
- [Rook on open file](https://www.chessprogramming.org/Rook_on_Open_File), [knight outposts](https://www.chessprogramming.org/Outpost)
- Evaluation-weight tuning (Texel / gradient) against a labelled position set
- [Incremental updates](https://www.chessprogramming.org/Incremental_Updates) of material + PST on make/unmake
- [Pawn](https://www.chessprogramming.org/Pawn_Hash_Table) and [evaluation](https://www.chessprogramming.org/Evaluation_Hash_Table) hash tables

### Alternative search algorithms to evaluate

- [MTD(f)](https://www.chessprogramming.org/MTD\(f\))
- [NegaC*](https://www.chessprogramming.org/NegaC*)

## Infrastructure the design assumes

- **Move generation is never reimplemented.** Any discrepancy is a `dragontoothmg`
  bug or an API misuse, found and fixed with a perft test (`engine.PerftDivide`
  bisects against a reference).
- **`Search` leaves the board unmodified** — every `Apply` is paired with its
  unapply, including on early returns.
- **Evaluation sign convention**: positive = good for the side to move.
- **Untrusted input**: FEN and UCI strings come from GUIs and tournament
  managers; parsing must not panic (see [SECURITY.md](../SECURITY.md)).
- **Strength changes are proven by a match**, not intuition — SPRT via
  `cutechess-cli` against the previous build (see
  [development.md](development.md#strength-testing)).
