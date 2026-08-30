package engine

import (
	"testing"
	"time"

	"github.com/dylhunn/dragontoothmg"
)

func TestNullMoveBoardFlipsSideAndDropsEnPassant(t *testing.T) {
	// After 1.e4 the en-passant square is e3 and White is to move having just
	// pushed; take Black to move here so there is a real ep target.
	b := dragontoothmg.ParseFen("rnbqkbnr/pppp1ppp/8/4p3/4P3/8/PPPP1PPP/RNBQKBNR w KQkq e6 0 3")
	nb := nullMoveBoard(&b)

	if nb.Wtomove == b.Wtomove {
		t.Errorf("side to move not flipped: still %v", nb.Wtomove)
	}
	if nb.Hash() == b.Hash() {
		t.Errorf("hash unchanged after null move: %#x", nb.Hash())
	}
	if got := field(nb.ToFen(), 3); got != "-" {
		t.Errorf("en-passant square = %q, want %q", got, "-")
	}
	// Piece placement must be identical.
	if field(nb.ToFen(), 0) != field(b.ToFen(), 0) {
		t.Errorf("piece placement changed: %q vs %q", field(nb.ToFen(), 0), field(b.ToFen(), 0))
	}
}

func TestNullMoveBoardFromBlack(t *testing.T) {
	b := dragontoothmg.ParseFen("rnbqkbnr/pppp1ppp/8/4p3/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 2")
	nb := nullMoveBoard(&b)
	if !nb.Wtomove {
		t.Errorf("null move from Black should leave White to move")
	}
}

func TestHasNonPawnMaterial(t *testing.T) {
	tests := []struct {
		name string
		fen  string
		want bool
	}{
		{"startpos", dragontoothmg.Startpos, true},
		{"only pawns for side to move", "4k3/8/8/8/8/8/4P3/4K3 w - - 0 1", false},
		{"lone king", "4k3/8/8/8/8/8/8/4K3 w - - 0 1", false},
		{"knight only", "4k3/8/8/8/8/8/4N3/4K3 w - - 0 1", true},
		{"opponent has pieces, we do not", "3qk3/8/8/8/8/8/4P3/4K3 w - - 0 1", false},
	}
	for _, tt := range tests {
		b := dragontoothmg.ParseFen(tt.fen)
		if got := hasNonPawnMaterial(&b); got != tt.want {
			t.Errorf("%s: hasNonPawnMaterial = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsQuiet(t *testing.T) {
	b := dragontoothmg.ParseFen("4k3/8/8/3p4/4P3/8/8/4K3 w - - 0 1")
	var capture, quiet dragontoothmg.Move
	for _, m := range b.GenerateLegalMoves() {
		switch m.String() {
		case "e4d5":
			capture = m
		case "e4e5":
			quiet = m
		}
	}
	if isQuiet(&b, capture) {
		t.Errorf("e4d5 is a capture, isQuiet returned true")
	}
	if !isQuiet(&b, quiet) {
		t.Errorf("e4e5 is a quiet push, isQuiet returned false")
	}
}

func TestRecordCutoffUpdatesKillersAndHistory(t *testing.T) {
	b := dragontoothmg.ParseFen(dragontoothmg.Startpos)
	s := &searcher{tt: NewTT(1)}

	moves := b.GenerateLegalMoves()
	first, second := moves[0], moves[1]

	s.recordCutoff(&b, first, 5, 3)
	if s.killers[3][0] != first {
		t.Fatalf("killer[3][0] = %v, want %v", s.killers[3][0], first)
	}
	if got := s.history[sideIndex(&b)][first.From()][first.To()]; got != 25 {
		t.Errorf("history after depth-5 cutoff = %d, want 25", got)
	}

	s.recordCutoff(&b, second, 4, 3)
	if s.killers[3][0] != second || s.killers[3][1] != first {
		t.Errorf("killers not shifted: got [%v %v]", s.killers[3][0], s.killers[3][1])
	}

	// Re-recording the same killer must not duplicate it into both slots.
	s.recordCutoff(&b, second, 2, 3)
	if s.killers[3][1] == second {
		t.Errorf("killer duplicated into both slots")
	}
}

func TestRecordCutoffHistoryIsCapped(t *testing.T) {
	b := dragontoothmg.ParseFen(dragontoothmg.Startpos)
	s := &searcher{tt: NewTT(1)}
	m := b.GenerateLegalMoves()[0]
	for i := 0; i < 5000; i++ {
		s.recordCutoff(&b, m, 63, 1)
	}
	if got := s.history[sideIndex(&b)][m.From()][m.To()]; got != historyMax {
		t.Errorf("history = %d, want it clamped to %d", got, historyMax)
	}
}

func TestOrderMovesRanksKillersAheadOfOtherQuiets(t *testing.T) {
	b := dragontoothmg.ParseFen("r3k2r/pppppppp/8/8/8/8/PPPPPPPP/R3K2R w KQkq - 0 1")
	s := &searcher{tt: NewTT(1)}
	moves := b.GenerateLegalMoves()

	// Pick a quiet move near the end of natural generation order and make it a
	// killer for this ply.
	killer := moves[len(moves)-1]
	s.killers[2][0] = killer

	ordered := s.orderMoves(&b, moves, 0, 2)

	killerPos, lastQuietPos := -1, -1
	for i, m := range ordered {
		if m == killer {
			killerPos = i
		}
		if isQuiet(&b, m) && m != killer {
			lastQuietPos = i
		}
	}
	if killerPos == -1 {
		t.Fatal("killer move missing from ordered list")
	}
	if killerPos > lastQuietPos {
		t.Errorf("killer at %d ranked behind a plain quiet at %d", killerPos, lastQuietPos)
	}
}

func TestOrderMovesUsesHistoryForQuiets(t *testing.T) {
	b := dragontoothmg.ParseFen("r3k2r/pppppppp/8/8/8/8/PPPPPPPP/R3K2R w KQkq - 0 1")
	s := &searcher{tt: NewTT(1)}
	moves := b.GenerateLegalMoves()
	favoured := moves[len(moves)-1]
	s.history[sideIndex(&b)][favoured.From()][favoured.To()] = historyMax

	ordered := s.orderMoves(&b, moves, 0, 1)
	// With no TT move, no promotions and no captures possible here, the
	// history-boosted quiet must sort first.
	if ordered[0] != favoured {
		t.Errorf("history-boosted move ranked %v first, want %v", ordered[0].String(), favoured.String())
	}
}

func TestTryNullMoveCutsWhenWinning(t *testing.T) {
	// White is a whole queen up with pieces on the board: passing still leaves
	// the eval far above a modest beta, so null-move pruning should cut.
	b := dragontoothmg.ParseFen("4k3/8/8/8/8/5N2/4PPPP/Q3K2R w K - 0 1")
	s := &searcher{tt: NewTT(4)}
	score, ok := s.tryNullMove(&b, 5, 200, 1, true, false)
	if !ok {
		t.Fatalf("expected a null-move cut-off, got none")
	}
	if score < 200 {
		t.Errorf("cut-off score %d is below beta 200", score)
	}
}

func TestTryNullMoveSkippedConditions(t *testing.T) {
	b := dragontoothmg.ParseFen("4k3/8/8/8/8/5N2/4PPPP/Q3K2R w K - 0 1")
	s := &searcher{tt: NewTT(4)}

	if _, ok := s.tryNullMove(&b, 5, 200, 1, false, false); ok {
		t.Errorf("null move ran with canNull=false")
	}
	if _, ok := s.tryNullMove(&b, 5, 200, 1, true, true); ok {
		t.Errorf("null move ran while in check")
	}
	if _, ok := s.tryNullMove(&b, 2, 200, 1, true, false); ok {
		t.Errorf("null move ran below nmpMinDepth")
	}
	if _, ok := s.tryNullMove(&b, 5, mateScore-1, 1, true, false); ok {
		t.Errorf("null move ran with a mate-score beta")
	}

	// Only pawns for the side to move: zugzwang guard must block the cut.
	pawnsOnly := dragontoothmg.ParseFen("4k3/8/8/8/8/8/P7/4K3 w - - 0 1")
	if _, ok := s.tryNullMove(&pawnsOnly, 5, -5000, 1, true, false); ok {
		t.Errorf("null move ran with no non-pawn material")
	}

	// Eval below beta: nothing to prune.
	if _, ok := s.tryNullMove(&b, 5, mateThreshold-1, 1, true, false); ok {
		t.Errorf("null move cut with eval far below beta")
	}
}

func TestKingCaptureScore(t *testing.T) {
	// Black has no king: from White's side the opponent king is gone (win); a
	// position with White's own king missing is a loss.
	won := dragontoothmg.ParseFen("8/8/8/8/8/8/8/4K3 w - - 0 1")
	if got := kingCaptureScore(&won, 3); got <= 0 {
		t.Errorf("missing enemy king scored %d, want a win", got)
	}
	lost := dragontoothmg.ParseFen("4k3/8/8/8/8/8/8/8 w - - 0 1")
	if got := kingCaptureScore(&lost, 3); got >= 0 {
		t.Errorf("missing own king scored %d, want a loss", got)
	}
}

func TestNegamaxLeavesBoardUnmodified(t *testing.T) {
	fen := "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1"
	b := dragontoothmg.ParseFen(fen)
	s := &searcher{tt: NewTT(8), deadline: time.Now().Add(time.Second)}
	s.negamax(&b, 5, -infinity, infinity, 0, true)
	if got := b.ToFen(); got != fen {
		t.Errorf("board mutated by negamax:\n got %q\nwant %q", got, fen)
	}
}

// field returns the i-th space-separated field of a FEN string.
func field(fen string, i int) string {
	start, n := 0, 0
	for j := 0; j <= len(fen); j++ {
		if j == len(fen) || fen[j] == ' ' {
			if n == i {
				return fen[start:j]
			}
			n++
			start = j + 1
		}
	}
	return ""
}
