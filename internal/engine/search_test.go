package engine_test

import (
	"testing"
	"time"

	"github.com/dylhunn/dragontoothmg"

	"github.com/cjunius/goChess/internal/engine"
)

func TestSearchFindsMateInOne(t *testing.T) {
	// Black king boxed on g8 by its own pawns; Ra1-a8 is mate.
	b := dragontoothmg.ParseFen("6k1/5ppp/8/8/8/8/8/R5K1 w - - 0 1")
	res := engine.Search(&b, engine.SearchParams{MaxDepth: 3})
	if got := res.BestMove.String(); got != "a1a8" {
		t.Fatalf("best move = %s (score %d), want a1a8", got, res.Score)
	}
	if res.Score < mateThreshold {
		t.Errorf("score = %d, want a mate score (>= %d)", res.Score, mateThreshold)
	}
}

func TestSearchWinsFreeMaterial(t *testing.T) {
	// White rook on d1 can take the undefended queen on d8.
	b := dragontoothmg.ParseFen("3q2k1/8/8/8/8/8/6K1/3R4 w - - 0 1")
	res := engine.Search(&b, engine.SearchParams{MaxDepth: 4})
	if got := res.BestMove.String(); got != "d1d8" {
		t.Fatalf("best move = %s, want d1d8", got)
	}
}

func TestSearchDoesNotHangQueen(t *testing.T) {
	// White queen on d2 can grab the rook on d8, but Kxd8 then trades the queen
	// for a rook and leaves K vs K. The engine must keep the queen instead.
	b := dragontoothmg.ParseFen("3rk3/8/8/8/8/8/3Q4/4K3 w - - 0 1")
	res := engine.Search(&b, engine.SearchParams{MaxDepth: 5})
	if got := res.BestMove.String(); got == "d2d8" {
		t.Fatalf("engine hung its queen with %s", got)
	}
}

func TestSearchRespectsMoveTime(t *testing.T) {
	b := dragontoothmg.ParseFen(dragontoothmg.Startpos)
	start := time.Now()
	res := engine.Search(&b, engine.SearchParams{MaxDepth: maxDepthUnbounded, MoveTime: 200 * time.Millisecond})
	elapsed := time.Since(start)
	if elapsed > time.Second {
		t.Fatalf("search ran %s, expected it to stop near 200ms", elapsed)
	}
	if res.BestMove.String() == "0000" {
		t.Fatal("search returned no move")
	}
}

func TestSearchWithTranspositionTableFindsSameMove(t *testing.T) {
	// A shared TT must not change the result of a fixed-depth search, only its
	// speed. Compare a run with a private table against one with an explicit
	// table passed in.
	fen := "r4rk1/1pp1qppp/p1np1n2/2b1p1B1/2B1P1b1/P1NP1N2/1PP1QPPP/R4RK1 w - - 0 10"
	b1 := dragontoothmg.ParseFen(fen)
	plain := engine.Search(&b1, engine.SearchParams{MaxDepth: 6})

	b2 := dragontoothmg.ParseFen(fen)
	withTT := engine.Search(&b2, engine.SearchParams{MaxDepth: 6, TT: engine.NewTT(16)})

	if plain.BestMove != withTT.BestMove {
		t.Errorf("best move changed with a TT: %s vs %s", plain.BestMove.String(), withTT.BestMove.String())
	}
}

func TestSearchTranspositionTableCutsNodeCount(t *testing.T) {
	// Re-searching the same position with a warm TT should visit far fewer
	// nodes than the cold search did.
	fen := "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1"
	tt := engine.NewTT(16)

	b1 := dragontoothmg.ParseFen(fen)
	cold := engine.Search(&b1, engine.SearchParams{MaxDepth: 6, TT: tt})

	b2 := dragontoothmg.ParseFen(fen)
	warm := engine.Search(&b2, engine.SearchParams{MaxDepth: 6, TT: tt})

	if warm.Nodes >= cold.Nodes {
		t.Errorf("warm search visited %d nodes, cold visited %d; expected the warm run to be cheaper", warm.Nodes, cold.Nodes)
	}
}

func TestSearchLazySMPFindsMate(t *testing.T) {
	b := dragontoothmg.ParseFen("6k1/5ppp/8/8/8/8/8/R5K1 w - - 0 1")
	res := engine.Search(&b, engine.SearchParams{MaxDepth: 4, Threads: 4})
	if got := res.BestMove.String(); got != "a1a8" {
		t.Fatalf("Lazy-SMP best move = %s (score %d), want a1a8", got, res.Score)
	}
	if res.Score < mateThreshold {
		t.Errorf("score = %d, want a mate score (>= %d)", res.Score, mateThreshold)
	}
}

func TestSearchLazySMPRespectsMoveTime(t *testing.T) {
	b := dragontoothmg.ParseFen(dragontoothmg.Startpos)
	start := time.Now()
	res := engine.Search(&b, engine.SearchParams{
		MaxDepth: maxDepthUnbounded,
		MoveTime: 200 * time.Millisecond,
		Threads:  4,
	})
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Lazy-SMP search ran %s, expected it to stop near 200ms", elapsed)
	}
	if res.BestMove.String() == "0000" {
		t.Fatal("Lazy-SMP search returned no move")
	}
	if res.Depth == 0 {
		t.Errorf("Lazy-SMP search completed no full depth")
	}
}

func TestSearchLazySMPWinsFreeMaterial(t *testing.T) {
	b := dragontoothmg.ParseFen("3q2k1/8/8/8/8/8/6K1/3R4 w - - 0 1")
	res := engine.Search(&b, engine.SearchParams{MaxDepth: 5, Threads: 4})
	if got := res.BestMove.String(); got != "d1d8" {
		t.Fatalf("Lazy-SMP best move = %s, want d1d8", got)
	}
}

func TestSearchFindsMateWithPruningActive(t *testing.T) {
	// A forced mate in 3 that the search only sees several plies deep, so the
	// answer must survive killers/history ordering, LMR reductions, and the
	// null-move guard that switches off near mate scores.
	b := dragontoothmg.ParseFen("5rk1/pp4pp/8/8/8/8/PP1Q2PP/5RK1 w - - 0 1")
	res := engine.Search(&b, engine.SearchParams{MaxDepth: 8})
	if got := res.BestMove.String(); got != "d2d5" {
		t.Fatalf("best move = %s (score %d), want d2d5", got, res.Score)
	}
	if res.Score < mateThreshold {
		t.Errorf("score = %d, want a mate score (>= %d)", res.Score, mateThreshold)
	}
}

func TestSearchReducedTreeStillReachesDeepDepth(t *testing.T) {
	// With killers/history + NMP + LMR the start position should complete a
	// depth well past what full-width alpha-beta manages in the same node
	// budget. This also exercises the LMR re-search path heavily.
	b := dragontoothmg.ParseFen(dragontoothmg.Startpos)
	res := engine.Search(&b, engine.SearchParams{MaxDepth: 9})
	if res.Depth != 9 {
		t.Fatalf("completed depth %d, want 9", res.Depth)
	}
	if res.BestMove.String() == "0000" {
		t.Fatal("no move returned")
	}
}

func TestSearchPrunedResultMatchesPlainOnTactics(t *testing.T) {
	// The pruning heuristics change the node count, never the verdict on a
	// forced tactic: the mate and the free rook must still be found.
	mate := dragontoothmg.ParseFen("6k1/5ppp/8/8/8/8/8/R5K1 w - - 0 1")
	mateBest := engine.Search(&mate, engine.SearchParams{MaxDepth: 5}).BestMove
	if got := mateBest.String(); got != "a1a8" {
		t.Errorf("mate: best move = %s, want a1a8", got)
	}
	rook := dragontoothmg.ParseFen("3q2k1/8/8/8/8/8/6K1/3R4 w - - 0 1")
	rookBest := engine.Search(&rook, engine.SearchParams{MaxDepth: 6}).BestMove
	if got := rookBest.String(); got != "d1d8" {
		t.Errorf("free material: best move = %s, want d1d8", got)
	}
}

const (
	mateThreshold     = 1_000_000 - 64
	maxDepthUnbounded = 64
)
