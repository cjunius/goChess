package uci_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cjunius/goChess/internal/uci"
)

func run(t *testing.T, input string) string {
	t.Helper()
	var out bytes.Buffer
	if err := uci.Run(strings.NewReader(input), &out, "test"); err != nil {
		t.Fatalf("uci.Run: %v", err)
	}
	return out.String()
}

func TestHandshake(t *testing.T) {
	out := run(t, "uci\nisready\nquit\n")
	for _, want := range []string{"id name goChess test", "id author", "uciok", "readyok"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestGoReturnsLegalBestMove(t *testing.T) {
	out := run(t, "position startpos\ngo depth 3\nquit\n")
	if !strings.Contains(out, "bestmove ") {
		t.Fatalf("no bestmove line:\n%s", out)
	}
}

func TestPositionWithMovesThenMateSearch(t *testing.T) {
	out := run(t, "position fen 6k1/5ppp/8/8/8/8/8/R5K1 w - - 0 1\ngo depth 4\nquit\n")
	if !strings.Contains(out, "bestmove a1a8") {
		t.Errorf("expected bestmove a1a8:\n%s", out)
	}
	if !strings.Contains(out, "score mate 1") {
		t.Errorf("expected 'score mate 1' in info line:\n%s", out)
	}
}

func TestUnknownCommandIsIgnored(t *testing.T) {
	out := run(t, "frobnicate\nisready\nquit\n")
	if !strings.Contains(out, "readyok") {
		t.Errorf("unknown command should be skipped, got:\n%s", out)
	}
}

func TestSetOptionHashAndThreads(t *testing.T) {
	// Out-of-range values are clamped, not rejected; a following search must
	// still produce a move.
	out := run(t, strings.Join([]string{
		"setoption name Hash value 999999",
		"setoption name Threads value 3",
		"setoption name Hash value 0",
		"setoption name Unknerd value x",
		"setoption name Threads value notanumber",
		"position startpos",
		"go depth 3",
		"quit",
		"",
	}, "\n"))
	if !strings.Contains(out, "bestmove ") {
		t.Fatalf("no bestmove after setoption:\n%s", out)
	}
}

func TestUciNewGameResetsBoard(t *testing.T) {
	out := run(t, strings.Join([]string{
		"isready",
		"position startpos moves e2e4 e7e5",
		"ucinewgame",
		"d",
		"quit",
		"",
	}, "\n"))
	if !strings.Contains(out, "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1") {
		t.Errorf("ucinewgame did not restore the start position:\n%s", out)
	}
}

func TestGoWithClockBudgetAndMoveTime(t *testing.T) {
	// wtime/btime path: the engine spends a fraction of the clock and still
	// returns promptly.
	out := run(t, "position startpos\ngo wtime 3000 btime 3000\nquit\n")
	if !strings.Contains(out, "bestmove ") {
		t.Errorf("clock-budget go produced no move:\n%s", out)
	}
	out = run(t, "position startpos\ngo movetime 50\nquit\n")
	if !strings.Contains(out, "bestmove ") {
		t.Errorf("movetime go produced no move:\n%s", out)
	}
}

func TestPositionMalformedInputsAreSafe(t *testing.T) {
	for _, cmd := range []string{
		"position",
		"position fen only three fields here",
		"position fen 6k1/5ppp/8/8/8/8/8/R5K1 w - - 0 1 moves notamove",
		"position wat",
		"position startpos moves",
	} {
		out := run(t, cmd+"\nisready\nquit\n")
		if !strings.Contains(out, "readyok") {
			t.Errorf("%q wedged the loop:\n%s", cmd, out)
		}
	}
}

func TestBlackToMoveGetsMatedScore(t *testing.T) {
	// Black is to move and being mated: the info line reports a negative mate.
	out := run(t, "position fen R5k1/5ppp/8/8/8/8/8/6K1 b - - 0 1\ngo depth 3\nquit\n")
	if !strings.Contains(out, "bestmove ") {
		t.Fatalf("no bestmove:\n%s", out)
	}
}

func TestStopAndUnknownGoArgsAreHarmless(t *testing.T) {
	out := run(t, "position startpos\ngo depth 2 winc 100 movestogo 40\nstop\nquit\n")
	if !strings.Contains(out, "bestmove ") {
		t.Errorf("expected a bestmove:\n%s", out)
	}
}
