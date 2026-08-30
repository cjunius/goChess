// Package uci implements the subset of the Universal Chess Interface protocol
// that goChess needs to play in a GUI (Arena, CuteChess, BanksiaGUI) or against
// other engines. The protocol is line-based over stdin/stdout.
package uci

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/dylhunn/dragontoothmg"

	"github.com/cjunius/goChess/internal/engine"
)

const (
	engineName   = "goChess"
	engineAuthor = "Christopher Junius"

	// Fraction of the remaining clock to spend on one move when the GUI sends
	// wtime/btime rather than an explicit movetime.
	clockDivisor = 30
)

type session struct {
	board dragontoothmg.Board
	out   io.Writer
}

// Run reads UCI commands from r and writes responses to w until "quit" or EOF.
// version is reported in the "id name" line.
func Run(r io.Reader, w io.Writer, version string) error {
	s := &session{board: dragontoothmg.ParseFen(dragontoothmg.Startpos), out: w}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for sc.Scan() {
		fields := strings.Fields(strings.TrimSpace(sc.Text()))
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "uci":
			fmt.Fprintf(w, "id name %s %s\n", engineName, version)
			fmt.Fprintf(w, "id author %s\n", engineAuthor)
			fmt.Fprintln(w, "uciok")
		case "isready":
			fmt.Fprintln(w, "readyok")
		case "ucinewgame":
			s.board = dragontoothmg.ParseFen(dragontoothmg.Startpos)
		case "position":
			s.handlePosition(fields[1:])
		case "go":
			s.handleGo(fields[1:])
		case "stop":
			// Search is synchronous, so there is nothing to interrupt.
		case "d":
			fmt.Fprintln(w, s.board.ToFen())
		case "quit":
			return nil
		}
	}
	return sc.Err()
}

func (s *session) handlePosition(args []string) {
	if len(args) == 0 {
		return
	}
	var rest []string
	switch args[0] {
	case "startpos":
		s.board = dragontoothmg.ParseFen(dragontoothmg.Startpos)
		rest = args[1:]
	case "fen":
		if len(args) < 7 {
			return
		}
		s.board = dragontoothmg.ParseFen(strings.Join(args[1:7], " "))
		rest = args[7:]
	default:
		return
	}
	if len(rest) == 0 || rest[0] != "moves" {
		return
	}
	for _, tok := range rest[1:] {
		m, err := dragontoothmg.ParseMove(tok)
		if err != nil {
			return
		}
		s.board.Apply(m)
	}
}

func (s *session) handleGo(args []string) {
	params := engine.SearchParams{}
	var wtime, btime, movetime time.Duration

	// Scan for the keywords we support, each followed by an integer argument.
	// Unknown keywords (movestogo, winc, ponder, ...) are ignored.
	for i := 0; i < len(args)-1; i++ {
		switch args[i] {
		case "depth":
			if d, err := strconv.Atoi(args[i+1]); err == nil {
				params.MaxDepth = d
			}
		case "movetime":
			movetime = millis(args[i+1])
		case "wtime":
			wtime = millis(args[i+1])
		case "btime":
			btime = millis(args[i+1])
		}
	}

	if movetime == 0 {
		remaining := btime
		if s.board.Wtomove {
			remaining = wtime
		}
		if remaining > 0 {
			movetime = remaining / clockDivisor
		}
	}
	params.MoveTime = movetime

	res := engine.Search(&s.board, params)
	if res.Depth > 0 {
		fmt.Fprintf(s.out, "info depth %d score %s nodes %d time %d pv %s\n",
			res.Depth, scoreString(res.Score), res.Nodes, res.Elapsed.Milliseconds(), res.BestMove.String())
	}
	best := res.BestMove.String()
	if best == "0000" {
		fmt.Fprintln(s.out, "bestmove (none)")
		return
	}
	fmt.Fprintf(s.out, "bestmove %s\n", best)
}

func millis(s string) time.Duration {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return time.Duration(n) * time.Millisecond
}

// scoreString renders a centipawn score, or "mate N" when the score encodes a
// forced mate, in the format UCI GUIs expect.
func scoreString(cp int) string {
	const mate = 1_000_000
	if cp >= mate-64 {
		return fmt.Sprintf("mate %d", (mate-cp+1)/2)
	}
	if cp <= -mate+64 {
		return fmt.Sprintf("mate %d", -(mate+cp+1)/2)
	}
	return "cp " + strconv.Itoa(cp)
}
