package engine_test

import (
	"testing"

	"github.com/dylhunn/dragontoothmg"

	"github.com/cjunius/goChess/internal/engine"
)

// Reference node counts from the Chess Programming Wiki. These are exact and
// act as a regression net for the whole move-generation path.
func TestPerftStartpos(t *testing.T) {
	cases := []struct {
		depth int
		want  int64
		long  bool
	}{
		{1, 20, false},
		{2, 400, false},
		{3, 8902, false},
		{4, 197281, false},
		{5, 4865609, true},
		{6, 119060324, true},
	}
	for _, c := range cases {
		if c.long && testing.Short() {
			continue
		}
		b := dragontoothmg.ParseFen(dragontoothmg.Startpos)
		if got := engine.Perft(&b, c.depth); got != c.want {
			t.Errorf("perft(startpos, %d) = %d, want %d", c.depth, got, c.want)
		}
	}
}

// "Kiwipete" – a dense middlegame position that exercises castling, promotion,
// en passant and pins all at once.
func TestPerftKiwipete(t *testing.T) {
	const fen = "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1"
	cases := []struct {
		depth int
		want  int64
		long  bool
	}{
		{1, 48, false},
		{2, 2039, false},
		{3, 97862, false},
		{4, 4085603, true},
	}
	for _, c := range cases {
		if c.long && testing.Short() {
			continue
		}
		b := dragontoothmg.ParseFen(fen)
		if got := engine.Perft(&b, c.depth); got != c.want {
			t.Errorf("perft(kiwipete, %d) = %d, want %d", c.depth, got, c.want)
		}
	}
}

func BenchmarkPerftStartpos(b *testing.B) {
	for i := 0; i < b.N; i++ {
		board := dragontoothmg.ParseFen(dragontoothmg.Startpos)
		engine.Perft(&board, 4)
	}
}
