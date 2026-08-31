// Package uci implements the subset of the Universal Chess Interface protocol
// that goChess needs to play in a GUI (Arena, CuteChess, BanksiaGUI) or against
// other engines. The protocol is line-based over stdin/stdout.
package uci

import (
	"bufio"
	"fmt"
	"io"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

	defaultHashMB  = 64
	minHashMB      = 1
	maxHashMB      = 4096
	maxThreads     = 256
	defaultThreads = 1
)

type session struct {
	board   dragontoothmg.Board
	out     io.Writer
	mu      sync.Mutex // guards writes to out and the search field
	tt      *engine.TT
	hashMB  int
	threads int
	ponder  bool // the "Ponder" option; a GUI only sends "go ponder" when set
	ownBook bool
	book    *engine.Book
	search  *activeSearch
}

// activeSearch tracks the goroutine running the current search so "stop",
// "ponderhit" and "quit" can reach it.
type activeSearch struct {
	stop      *atomic.Bool
	ponderHit *atomic.Bool
	bounded   bool          // has a depth or movetime limit and is not pondering
	hasBudget bool          // a movetime / clock budget was given
	release   chan struct{} // closed on ponderhit or stop; gates the bestmove
	relOnce   sync.Once
	done      chan struct{} // closed once bestmove has been emitted
}

func (as *activeSearch) signalRelease() { as.relOnce.Do(func() { close(as.release) }) }

// Run reads UCI commands from r and writes responses to w until "quit" or EOF.
// version is reported in the "id name" line.
func Run(r io.Reader, w io.Writer, version string) error {
	s := &session{
		board:   dragontoothmg.ParseFen(dragontoothmg.Startpos),
		out:     w,
		hashMB:  defaultHashMB,
		threads: defaultThreads,
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for sc.Scan() {
		fields := strings.Fields(strings.TrimSpace(sc.Text()))
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "uci":
			s.emit("id name %s %s\n", engineName, version)
			s.emit("id author %s\n", engineAuthor)
			s.emit("option name Hash type spin default %d min %d max %d\n", defaultHashMB, minHashMB, maxHashMB)
			s.emit("option name Threads type spin default %d min 1 max %d\n", defaultThreads, maxThreads)
			s.emit("option name Ponder type check default false\n")
			s.emit("option name OwnBook type check default false\n")
			s.emit("option name BookFile type string default\n")
			s.emit("uciok\n")
		case "isready":
			s.ensureTT()
			s.emit("readyok\n")
		case "setoption":
			s.handleSetOption(fields[1:])
		case "ucinewgame":
			s.stopSearch()
			s.board = dragontoothmg.ParseFen(dragontoothmg.Startpos)
			if s.tt != nil {
				s.tt.Clear()
			}
		case "position":
			s.stopSearch()
			s.handlePosition(fields[1:])
		case "go":
			s.handleGo(fields[1:])
		case "stop":
			s.stopSearch()
		case "ponderhit":
			s.ponderHit()
		case "d":
			s.emit("%s\n", s.board.ToFen())
		case "quit":
			s.endSearch(true)
			return nil
		}
	}
	s.endSearch(true)
	return sc.Err()
}

// emit writes one formatted line to out under the mutex, so the search goroutine
// and the command loop never interleave output.
func (s *session) emit(format string, args ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintf(s.out, format, args...)
}

// ensureTT lazily allocates the transposition table at the configured size.
func (s *session) ensureTT() {
	if s.tt == nil {
		s.tt = engine.NewTT(s.hashMB)
	}
}

// stopSearch aborts the running search (if any) and blocks until its bestmove
// has been emitted.
func (s *session) stopSearch() { s.endSearch(false) }

// endSearch blocks until the running search (if any) has emitted its bestmove.
// It aborts the search unless graceful is set and the search is guaranteed to
// finish on its own soon (a fixed depth / movetime search a GUI is waiting on,
// or a pondering search that already got its ponderhit and has a budget). A
// "quit" or EOF must never hang on an infinite or still-pondering search.
func (s *session) endSearch(graceful bool) {
	s.mu.Lock()
	as := s.search
	s.mu.Unlock()
	if as == nil {
		return
	}
	willFinish := as.bounded || (as.hasBudget && as.ponderHit.Load())
	if !graceful || !willFinish {
		as.stop.Store(true)
	}
	as.signalRelease()
	<-as.done
}

// ponderhit tells a pondering search that the opponent played the expected move,
// so it should start spending its time budget and report as normal.
func (s *session) ponderHit() {
	s.mu.Lock()
	as := s.search
	s.mu.Unlock()
	if as == nil {
		return
	}
	as.ponderHit.Store(true)
	as.signalRelease()
}

// handleSetOption parses "setoption name <Name> value <Value>" for the options
// advertised in the "uci" reply. Unknown options are ignored, as the protocol
// requires.
func (s *session) handleSetOption(args []string) {
	var name, value string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "name":
			j := i + 1
			for j < len(args) && args[j] != "value" {
				j++
			}
			name = strings.Join(args[i+1:j], " ")
			i = j - 1
		case "value":
			value = strings.Join(args[i+1:], " ")
			i = len(args)
		}
	}

	switch strings.ToLower(name) {
	case "hash":
		if n, err := strconv.Atoi(value); err == nil {
			s.stopSearch()
			s.hashMB = clamp(n, minHashMB, maxHashMB)
			s.tt = engine.NewTT(s.hashMB) // resize now, before the next search
		}
	case "threads":
		if n, err := strconv.Atoi(value); err == nil {
			s.threads = clamp(n, 1, maxThreads)
		}
	case "ponder":
		s.ponder = strings.EqualFold(value, "true")
	case "ownbook":
		s.ownBook = strings.EqualFold(value, "true")
	case "bookfile":
		s.loadBook(strings.TrimSpace(value))
	}
}

// loadBook (re)loads the Polyglot book. An empty path clears it.
func (s *session) loadBook(path string) {
	if path == "" {
		s.book = nil
		return
	}
	bk, err := engine.OpenBook(path)
	if err != nil {
		s.book = nil
		s.emit("info string book load failed: %v\n", err)
		return
	}
	s.book = bk
	s.emit("info string book loaded: %d entries\n", bk.Len())
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
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
	s.stopSearch()

	var params engine.SearchParams
	var wtime, btime, winc, binc, movetime time.Duration
	ponder := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "ponder":
			ponder = true
		case "infinite":
			params.MaxDepth = 0
		}
		if i+1 >= len(args) {
			break
		}
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
		case "winc":
			winc = millis(args[i+1])
		case "binc":
			binc = millis(args[i+1])
		}
	}

	if movetime == 0 {
		remaining, inc := btime, binc
		if s.board.Wtomove {
			remaining, inc = wtime, winc
		}
		if remaining > 0 {
			movetime = remaining/clockDivisor + inc*3/4
		}
	}
	params.MoveTime = movetime

	// An in-book move short-circuits the search entirely (but never while
	// pondering — there is nothing to ponder on a book move).
	if s.ownBook && s.book != nil && !ponder {
		if mv, ok := s.book.Probe(&s.board); ok {
			s.emit("info depth 0 score cp 0 pv %s\n", mv.String())
			s.emit("bestmove %s\n", mv.String())
			return
		}
	}

	s.ensureTT()
	params.TT = s.tt
	params.Threads = s.threads
	if n := runtime.NumCPU(); params.Threads > n {
		params.Threads = n // never spawn more workers than the machine has cores
	}

	as := &activeSearch{
		stop:      new(atomic.Bool),
		ponderHit: new(atomic.Bool),
		bounded:   !ponder && (params.MaxDepth > 0 || params.MoveTime > 0),
		hasBudget: params.MoveTime > 0,
		release:   make(chan struct{}),
		done:      make(chan struct{}),
	}
	if !ponder {
		as.signalRelease() // no ponder handshake: emit the bestmove as soon as it's ready
	}
	params.Stop = as.stop
	params.Ponder = ponder
	params.PonderHit = as.ponderHit
	params.Info = s.infoPrinter()

	s.mu.Lock()
	s.search = as
	s.mu.Unlock()

	board := s.board
	go func() {
		res := engine.Search(&board, params)
		<-as.release // wait for ponderhit / stop before moving
		s.emitBestMove(res)
		s.mu.Lock()
		s.search = nil
		s.mu.Unlock()
		close(as.done)
	}()
}

// infoPrinter returns the per-iteration callback that streams "info" lines.
func (s *session) infoPrinter() func(engine.SearchInfo) {
	return func(in engine.SearchInfo) {
		var pv strings.Builder
		for i, m := range in.PV {
			if i > 0 {
				pv.WriteByte(' ')
			}
			pv.WriteString(m.String())
		}
		var nps int64
		if in.Elapsed > 0 {
			nps = in.Nodes * int64(time.Second) / int64(in.Elapsed)
		}
		s.emit("info depth %d score %s nodes %d nps %d time %d pv %s\n",
			in.Depth, scoreString(in.Score), in.Nodes, nps, in.Elapsed.Milliseconds(), pv.String())
	}
}

func (s *session) emitBestMove(res engine.SearchResult) {
	if res.BestMove == 0 || res.BestMove.String() == "0000" {
		s.emit("bestmove (none)\n")
		return
	}
	if len(res.PV) >= 2 {
		s.emit("bestmove %s ponder %s\n", res.BestMove.String(), res.PV[1].String())
		return
	}
	s.emit("bestmove %s\n", res.BestMove.String())
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
