package uci_test

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dylhunn/dragontoothmg"

	"github.com/cjunius/goChess/internal/engine"
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

func TestUciAdvertisesPonderOption(t *testing.T) {
	out := run(t, "uci\nquit\n")
	if !strings.Contains(out, "option name Ponder type check default false") {
		t.Errorf("missing Ponder option:\n%s", out)
	}
}

func TestInfiniteSearchStopsOnStop(t *testing.T) {
	start := time.Now()
	out := run(t, "position startpos\ngo infinite\nstop\nquit\n")
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("infinite search took %s to stop", elapsed)
	}
	if n := strings.Count(out, "bestmove"); n != 1 {
		t.Fatalf("want exactly one bestmove, got %d:\n%s", n, out)
	}
}

func TestPonderHitReleasesBestMove(t *testing.T) {
	// White has mate in one (a1a8). Pondering, then ponderhit lets the search
	// spend its budget, find the mate and report it.
	out := run(t, strings.Join([]string{
		"position fen 6k1/5ppp/8/8/8/8/8/R5K1 w - - 0 1",
		"go ponder movetime 2000",
		"ponderhit",
		"quit",
		"",
	}, "\n"))
	if n := strings.Count(out, "bestmove"); n != 1 {
		t.Fatalf("want exactly one bestmove, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "bestmove a1a8") {
		t.Errorf("want 'bestmove a1a8' after ponderhit:\n%s", out)
	}
}

func TestPonderStopReleasesBestMoveWithoutHit(t *testing.T) {
	start := time.Now()
	out := run(t, strings.Join([]string{
		"position startpos moves e2e4",
		"go ponder wtime 60000 btime 60000",
		"stop",
		"quit",
		"",
	}, "\n"))
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("pondering search took %s to stop after 'stop'", elapsed)
	}
	if n := strings.Count(out, "bestmove"); n != 1 {
		t.Fatalf("want exactly one bestmove, got %d:\n%s", n, out)
	}
}

func TestOpeningBookShortCircuitsSearch(t *testing.T) {
	start := dragontoothmg.ParseFen(dragontoothmg.Startpos)
	key := engine.PolyglotKey(&start)

	// One entry: start position -> g1f3 (from file 6 row 0, to file 5 row 2).
	move := uint16(5 | 2<<3 | 6<<6 | 0<<9)
	var rec [16]byte
	binary.BigEndian.PutUint64(rec[0:], key)
	binary.BigEndian.PutUint16(rec[8:], move)
	binary.BigEndian.PutUint16(rec[10:], 1)

	path := filepath.Join(t.TempDir(), "book.bin")
	if err := os.WriteFile(path, rec[:], 0o600); err != nil {
		t.Fatal(err)
	}

	start2 := time.Now()
	out := run(t, strings.Join([]string{
		"setoption name BookFile value " + path,
		"setoption name OwnBook value true",
		"position startpos",
		"go depth 20",
		"quit",
		"",
	}, "\n"))
	if elapsed := time.Since(start2); elapsed > time.Second {
		t.Fatalf("book move took %s — it should skip the search", elapsed)
	}
	if !strings.Contains(out, "book loaded: 1 entries") {
		t.Errorf("book was not loaded:\n%s", out)
	}
	if !strings.Contains(out, "bestmove g1f3") {
		t.Errorf("want the book move g1f3:\n%s", out)
	}
}

func TestBestMoveCarriesPonderMove(t *testing.T) {
	out := run(t, "position startpos\ngo depth 6\nquit\n")
	if !strings.Contains(out, "bestmove ") {
		t.Fatalf("no bestmove:\n%s", out)
	}
	line := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "bestmove ") {
			line = l
		}
	}
	if !strings.Contains(line, " ponder ") {
		t.Errorf("bestmove line has no ponder move: %q", line)
	}
}
