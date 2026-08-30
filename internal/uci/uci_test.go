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
