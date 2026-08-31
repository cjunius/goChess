package engine

import (
	"sort"
	"strings"
	"testing"

	"github.com/dylhunn/dragontoothmg"
)

// mirrorFEN returns the colour-swapped vertical mirror of a position: White and
// Black exchange roles and every rank is reflected. A correct evaluation must
// score the mirror exactly opposite (and equal once the side-to-move sign is
// applied), so Evaluate(pos) must equal Evaluate(mirror(pos)).
func mirrorFEN(fen string) string {
	f := strings.Fields(fen)

	ranks := strings.Split(f[0], "/")
	for i, j := 0, len(ranks)-1; i < j; i, j = i+1, j-1 {
		ranks[i], ranks[j] = ranks[j], ranks[i]
	}
	for i := range ranks {
		ranks[i] = swapCase(ranks[i])
	}
	f[0] = strings.Join(ranks, "/")

	if f[1] == "w" {
		f[1] = "b"
	} else {
		f[1] = "w"
	}

	if f[2] != "-" {
		cr := []byte(swapCase(f[2]))
		sort.Slice(cr, func(i, j int) bool { return castleOrder(cr[i]) < castleOrder(cr[j]) })
		f[2] = string(cr)
	}

	if f[3] != "-" {
		switch f[3][1] {
		case '3':
			f[3] = string(f[3][0]) + "6"
		case '6':
			f[3] = string(f[3][0]) + "3"
		}
	}
	return strings.Join(f, " ")
}

func swapCase(s string) string {
	b := []byte(s)
	for i, c := range b {
		switch {
		case c >= 'a' && c <= 'z':
			b[i] = c - 32
		case c >= 'A' && c <= 'Z':
			b[i] = c + 32
		}
	}
	return string(b)
}

func castleOrder(c byte) int {
	return strings.IndexByte("KQkq", c)
}

func TestEvaluateIsColourSymmetric(t *testing.T) {
	fens := []string{
		dragontoothmg.Startpos,
		"r1bqkbnr/pppp1ppp/2n5/4p3/4P3/5N2/PPPP1PPP/RNBQKB1R w KQkq - 2 3",
		"r4rk1/1pp1qppp/p1np1n2/2b1p1B1/2B1P1b1/P1NP1N2/1PP1QPPP/R4RK1 w - - 0 10",
		"8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1",
		"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1",
		"8/5k2/8/8/3P4/8/5K2/8 w - - 0 1",
		"6k1/5ppp/8/8/8/8/5PPP/6K1 b - - 0 1",
	}
	for _, fen := range fens {
		orig := dragontoothmg.ParseFen(fen)
		mir := dragontoothmg.ParseFen(mirrorFEN(fen))
		if got, want := Evaluate(&mir), Evaluate(&orig); got != want {
			t.Errorf("asymmetric eval for %s: got %d, mirror %d", fen, want, got)
		}
	}
}

func TestGamePhaseBounds(t *testing.T) {
	start := dragontoothmg.ParseFen(dragontoothmg.Startpos)
	if p := gamePhase(&start); p != maxPhase {
		t.Errorf("start position phase = %d, want %d", p, maxPhase)
	}
	bare := dragontoothmg.ParseFen("4k3/pppppppp/8/8/8/8/PPPPPPPP/4K3 w - - 0 1")
	if p := gamePhase(&bare); p != 0 {
		t.Errorf("king-and-pawns phase = %d, want 0", p)
	}
}

func TestStartPositionIsRoughlyBalanced(t *testing.T) {
	b := dragontoothmg.ParseFen(dragontoothmg.Startpos)
	if got := Evaluate(&b); got < 0 || got > 2*tempo {
		t.Errorf("start position eval = %d, want it within [0, %d]", got, 2*tempo)
	}
}

func TestPassedPawnBeatsBlockadedPawn(t *testing.T) {
	// White pawn on e6, no black pawns able to stop it.
	passed := dragontoothmg.ParseFen("4k3/8/4P3/8/8/8/8/4K3 w - - 0 1")
	// Same pawn, but a black pawn on e7 sits in front of it.
	blockaded := dragontoothmg.ParseFen("4k3/4p3/4P3/8/8/8/8/4K3 w - - 0 1")
	if Evaluate(&passed) <= Evaluate(&blockaded) {
		t.Errorf("passed pawn (%d) did not beat blockaded pawn (%d)",
			Evaluate(&passed), Evaluate(&blockaded))
	}
}
