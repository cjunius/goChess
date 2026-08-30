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

const (
	mateThreshold     = 1_000_000 - 64
	maxDepthUnbounded = 64
)
