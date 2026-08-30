# Design

How the engine is put together, what each technique buys, and the backlog of
ideas not yet implemented. For the component diagram and invariants see
[architecture.md](architecture.md); for build, perft, and strength-testing
workflow see [development.md](development.md); for the reasons behind the big
choices see the [ADRs](adr/).

## Architecture

`engine.Search` is the search core. It is a plain function around a `searcher`
value (node counter + deadline) with a few free collaborators rather than an
injected object graph — small enough that dependency injection would cost more
than it buys.

| collaborator | responsibility |
|---|---|
| `dragontoothmg.Board` | board state, legal move generation, `Apply` / unapply, Zobrist hash, FEN — never reimplemented (see [ADR 0002](adr/0002-use-dragontoothmg-for-move-generation.md)) |
| `engine.Evaluate` | static evaluation from the side-to-move's view (positive = better for that side) |
| `engine.orderMoves` | orders moves to maximise alpha-beta cut-offs; TT/hash move first, then promotions, then MVV-LVA captures, then quiets |
| `engine.TT` | shared, lock-free transposition table keyed by `Board.Hash`; probed/stored inside `negamax` and seeded into move ordering |
| `searcher` | carries the node count, TT handle, shared stop flag and wall-clock deadline; answers `timeUp` and flips `stopped` |

`internal/uci` is the protocol layer and the only place that prints `info` /
`bestmove`. It owns the persistent `engine.TT` (`Hash` option) and the `Threads`
setting. Search is synchronous, so `stop` is a no-op and `bestmove` is emitted
as soon as `go` returns. `cmd/gochess` wires these together and adds the `perft`
and `bench` subcommands.

## Implemented

### [Search](https://www.chessprogramming.org/Search)

- [Negamax](https://www.chessprogramming.org/Negamax) with [alpha-beta pruning](https://www.chessprogramming.org/Alpha-Beta) — fail-hard
- [Iterative Deepening](https://www.chessprogramming.org/Iterative_Deepening) — the last fully completed depth is the one returned
- [Transposition Table](https://www.chessprogramming.org/Transposition_Table) — power-of-two table keyed by `dragontoothmg.Board.Hash()`; stores EXACT / LOWER / UPPER bounds with the best move, mate scores rebased by ply on store and probe. Lock-free (Hyatt XOR) so Lazy-SMP workers share one table. The stored move seeds move ordering even when the entry is too shallow to cut
- [Lazy SMP](https://www.chessprogramming.org/Lazy_SMP) — `Threads` workers run iterative deepening in parallel on private board copies over the shared TT; workers start at staggered depths so they diverge, and the deepest completed result wins
- [Quiescence Search](https://www.chessprogramming.org/Quiescence_Search) at the horizon — captures and promotions only, depth-bounded by `maxPly`
- [Move Ordering](https://www.chessprogramming.org/Move_Ordering) — TT/hash move first, then promotions, then [MVV-LVA](https://www.chessprogramming.org/MVV-LVA) captures, then quiet moves
- [Mate-distance scoring](https://www.chessprogramming.org/Mate_Distance_Pruning) — `mateScore - ply`, so the shortest mate is preferred; a proven mate ends iterative deepening early
- Draw detection — the fifty-move rule (`Halfmoveclock >= 100`) is scored `0` inside the tree
- Time management — a hard wall-clock budget checked every 2048 nodes; the UCI layer spends `1/30` of the remaining clock when the GUI sends `wtime` / `btime` instead of `movetime`
- King-capture guard — scores the illegal position `dragontoothmg` can hand back when the side not to move was already in check, instead of panicking on an empty king bitboard

### [Evaluation](https://www.chessprogramming.org/Evaluation)

- Material — standard centipawn values (`P 100 · N 320 · B 330 · R 500 · Q 900`)
- [Piece-square tables](https://www.chessprogramming.org/Piece-Square_Tables) — Michniewski's "simplified evaluation function", single (non-tapered) set; black reads the vertically mirrored square (`sq^56`)
- [Bishop pair](https://www.chessprogramming.org/Bishop_Pair) — `+30` for holding both bishops

Evaluation is recomputed from scratch on every call; there is no incremental
update and no pawn or evaluation hash.

## Backlog

### Search

- [Principal Variation Search](https://www.chessprogramming.org/Principal_Variation_Search) — null-window scout + re-search
- [Killer](https://www.chessprogramming.org/Killer_Heuristic) and [history](https://www.chessprogramming.org/History_Heuristic) heuristics in `orderMoves`
- [Null Move Pruning](https://www.chessprogramming.org/Null_Move_Pruning) and [Late Move Reductions](https://www.chessprogramming.org/Late_Move_Reductions)
- [Aspiration Windows](https://www.chessprogramming.org/Aspiration_Windows) around the previous iteration's score
- [Static Exchange Evaluation](https://www.chessprogramming.org/Static_Exchange_Evaluation) for capture ordering and bad-capture pruning in quiescence
- Threefold-[repetition](https://www.chessprogramming.org/Repetitions) detection (needs a position history the current `Search` does not keep)
- A real principal variation in the `info` line (only `bestmove` is reported today)
- Richer Lazy SMP — per-worker root-move splitting, aspiration-window skew, TT ageing / bucketed replacement
- Asynchronous search so `stop` and pondering actually work
- [Opening book](https://www.chessprogramming.org/Opening_Book) (Polyglot) and [Syzygy endgame tablebases](https://www.chessprogramming.org/Endgame_Tablebases)

### Evaluation

- [Tapered eval](https://www.chessprogramming.org/Tapered_Eval) — mid/endgame PST pairs interpolated by game phase ([PeSTO](https://www.chessprogramming.org/PeSTO%27s_Evaluation_Function))
- Pawn structure — [passed](https://www.chessprogramming.org/Passed_Pawn) / [isolated](https://www.chessprogramming.org/Isolated_Pawn) / [doubled](https://www.chessprogramming.org/Doubled_Pawn) / [backward](https://www.chessprogramming.org/Backward_Pawn) pawns
- [Mobility](https://www.chessprogramming.org/Mobility), [rook on open file](https://www.chessprogramming.org/Rook_on_Open_File), [knight outposts](https://www.chessprogramming.org/Outpost), [king safety](https://www.chessprogramming.org/King_Safety), [tempo](https://www.chessprogramming.org/Tempo)
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
