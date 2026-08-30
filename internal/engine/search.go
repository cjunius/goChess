package engine

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dylhunn/dragontoothmg"
)

const (
	// mateScore is the score for being checkmated at ply 0. Scores within
	// maxPly of it encode "mate in N", closer to mateScore meaning sooner.
	mateScore = 1_000_000
	drawScore = 0
	infinity  = 1 << 30
	maxPly    = 64
)

// SearchParams controls a single Search call.
type SearchParams struct {
	// MaxDepth caps iterative deepening. Zero or negative means "use maxPly".
	MaxDepth int
	// MoveTime is a hard wall-clock budget. Zero means "no time limit; obey
	// MaxDepth only". When set, the last fully completed depth is returned.
	MoveTime time.Duration
	// Threads is the number of Lazy-SMP workers. Zero or one searches on a
	// single goroutine; higher values run that many workers over one shared TT.
	Threads int
	// TT is the transposition table to search against. When nil a private
	// default-sized table is allocated for this call only, so results are not
	// carried between moves — callers that want that (the UCI layer) pass a
	// table they own.
	TT *TT
}

// SearchResult is the outcome of a Search call.
type SearchResult struct {
	BestMove dragontoothmg.Move
	Score    int
	Depth    int
	Nodes    int64
	Elapsed  time.Duration
}

type searcher struct {
	tt       *TT
	stop     *atomic.Bool // shared across Lazy-SMP workers; set once time is up
	id       int
	nodes    int64
	deadline time.Time
	stopped  bool
}

func (s *searcher) timeUp() bool {
	if s.stop != nil && s.stop.Load() {
		return true
	}
	if s.deadline.IsZero() {
		return false
	}
	return time.Now().After(s.deadline)
}

// Search runs iterative-deepening alpha-beta and returns the best move it found.
// The board is left unmodified (every applied move is unapplied).
func Search(b *dragontoothmg.Board, p SearchParams) SearchResult {
	if p.MaxDepth <= 0 || p.MaxDepth > maxPly {
		p.MaxDepth = maxPly
	}
	tt := p.TT
	if tt == nil {
		tt = NewTT(defaultHashMB)
	}
	threads := p.Threads
	if threads < 1 {
		threads = 1
	}

	start := time.Now()
	var res SearchResult
	if b.White.Kings == 0 || b.Black.Kings == 0 {
		return res // illegal position, nothing sensible to search
	}
	if len(b.GenerateLegalMoves()) == 0 {
		return res
	}

	var deadline time.Time
	if p.MoveTime > 0 {
		deadline = start.Add(p.MoveTime)
	}
	stop := new(atomic.Bool)

	if threads > 1 {
		res = searchLazySMP(b, p.MaxDepth, deadline, stop, tt, threads)
	} else {
		s := &searcher{tt: tt, stop: stop, deadline: deadline}
		res = s.runIterativeDeepening(b, p.MaxDepth, 1)
	}
	res.Elapsed = time.Since(start)
	return res
}

// searchLazySMP runs `threads` workers over one shared transposition table. Each
// worker deepens independently on its own copy of the board; workers seeded with
// a different start depth diverge into different subtrees, and the shared TT
// lets every worker profit from what the others have already searched. The
// deepest completed result wins. See https://www.chessprogramming.org/Lazy_SMP.
func searchLazySMP(b *dragontoothmg.Board, maxDepth int, deadline time.Time, stop *atomic.Bool, tt *TT, threads int) SearchResult {
	results := make([]SearchResult, threads)
	var wg sync.WaitGroup
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// dragontoothmg.Board is all value types (no pointers, slices, or
			// maps), so a plain copy gives each worker an independent board.
			board := *b
			startDepth := 1 + id%2
			if startDepth > maxDepth {
				startDepth = maxDepth
			}
			s := &searcher{tt: tt, stop: stop, deadline: deadline, id: id}
			results[id] = s.runIterativeDeepening(&board, maxDepth, startDepth)
		}(i)
	}
	wg.Wait()

	best := results[0]
	var nodes int64
	for _, r := range results {
		nodes += r.Nodes
		if r.BestMove == 0 {
			continue
		}
		if r.Depth > best.Depth || (r.Depth == best.Depth && r.Score > best.Score) {
			best = r
		}
	}
	best.Nodes = nodes
	return best
}

// runIterativeDeepening deepens from startDepth to maxDepth on b, keeping the
// last fully completed depth. startDepth is above 1 only for Lazy-SMP workers.
func (s *searcher) runIterativeDeepening(b *dragontoothmg.Board, maxDepth, startDepth int) SearchResult {
	var res SearchResult
	root := b.GenerateLegalMoves()
	if len(root) == 0 {
		return res
	}
	res.BestMove = root[0]

	for depth := startDepth; depth <= maxDepth; depth++ {
		score, move, ok := s.searchRoot(b, depth)
		if !ok {
			break // out of time: keep the previous completed depth
		}
		res.BestMove, res.Score, res.Depth = move, score, depth
		if score >= mateScore-maxPly || score <= -mateScore+maxPly {
			if s.stop != nil {
				s.stop.Store(true) // forced mate: let the other workers stop too
			}
			break
		}
		if s.timeUp() {
			break
		}
	}
	res.Nodes = s.nodes
	return res
}

func (s *searcher) searchRoot(b *dragontoothmg.Board, depth int) (score int, best dragontoothmg.Move, ok bool) {
	key := b.Hash()
	_, ttMove, _ := s.tt.probe(key, depth, -infinity, infinity, 0)

	moves := orderMoves(b, b.GenerateLegalMoves(), ttMove)
	alpha, beta := -infinity, infinity
	bestScore := -infinity
	for _, m := range moves {
		unapply := b.Apply(m)
		v := -s.negamax(b, depth-1, -beta, -alpha, 1)
		unapply()
		if s.stopped {
			return 0, dragontoothmg.Move(0), false
		}
		if v > bestScore {
			bestScore, best = v, m
		}
		if v > alpha {
			alpha = v
		}
	}
	s.tt.store(key, depth, bestScore, boundExact, best, 0)
	return bestScore, best, true
}

func (s *searcher) negamax(b *dragontoothmg.Board, depth, alpha, beta, ply int) int {
	s.nodes++
	if s.nodes&2047 == 0 && s.timeUp() {
		s.stopped = true
		if s.stop != nil {
			s.stop.Store(true)
		}
		return 0
	}
	if b.White.Kings == 0 || b.Black.Kings == 0 {
		return kingCaptureScore(b, ply)
	}
	if b.Halfmoveclock >= 100 {
		return drawScore
	}
	if depth <= 0 {
		return s.quiesce(b, alpha, beta, ply)
	}

	alphaOrig := alpha
	key := b.Hash()
	ttScore, ttMove, cutoff := s.tt.probe(key, depth, alpha, beta, ply)
	if cutoff {
		return ttScore
	}

	moves := b.GenerateLegalMoves()
	if len(moves) == 0 {
		if b.OurKingInCheck() {
			return -mateScore + ply // checkmate; prefer the shortest mate
		}
		return drawScore // stalemate
	}

	best := -infinity
	var bestMove dragontoothmg.Move
	for _, m := range orderMoves(b, moves, ttMove) {
		unapply := b.Apply(m)
		v := -s.negamax(b, depth-1, -beta, -alpha, ply+1)
		unapply()
		if s.stopped {
			return 0
		}
		if v > best {
			best, bestMove = v, m
		}
		if v > alpha {
			alpha = v
		}
		if alpha >= beta {
			break // fail-high: opponent won't enter this line
		}
	}

	bound := boundExact
	switch {
	case best <= alphaOrig:
		bound = boundUpper
	case best >= beta:
		bound = boundLower
	}
	s.tt.store(key, depth, best, bound, bestMove, ply)
	return best
}

// quiesce searches only "loud" moves (captures and promotions) past the horizon
// so the static eval is never taken in the middle of a capture sequence.
func (s *searcher) quiesce(b *dragontoothmg.Board, alpha, beta, ply int) int {
	s.nodes++
	if b.White.Kings == 0 || b.Black.Kings == 0 {
		return kingCaptureScore(b, ply)
	}
	stand := Evaluate(b)
	if stand >= beta {
		return beta
	}
	if stand > alpha {
		alpha = stand
	}
	if ply >= maxPly {
		return stand
	}

	for _, m := range orderMoves(b, b.GenerateLegalMoves(), 0) {
		if !dragontoothmg.IsCapture(m, b) && m.Promote() == dragontoothmg.Nothing {
			continue
		}
		unapply := b.Apply(m)
		v := -s.quiesce(b, -beta, -alpha, ply+1)
		unapply()
		if s.stopped {
			return 0
		}
		if v >= beta {
			return beta
		}
		if v > alpha {
			alpha = v
		}
	}
	return alpha
}

// kingCaptureScore scores the illegal position left when a previous ply captured
// a king. dragontoothmg can generate such a move when the position it is given
// has the side *not* to move in check; without this guard the next
// GenerateLegalMoves call panics on an empty king bitboard. These positions
// never arise from legal play.
func kingCaptureScore(b *dragontoothmg.Board, ply int) int {
	ourKings := b.White.Kings
	if !b.Wtomove {
		ourKings = b.Black.Kings
	}
	if ourKings == 0 {
		return -mateScore + ply // our king was just captured
	}
	return mateScore - ply // the opponent's king is gone
}

// orderMoves sorts moves best-first so alpha-beta prunes as early as possible:
// the transposition-table move first, then promotions, then captures by MVV-LVA
// (most valuable victim, least valuable attacker), then quiet moves.
func orderMoves(b *dragontoothmg.Board, moves []dragontoothmg.Move, ttMove dragontoothmg.Move) []dragontoothmg.Move {
	type scored struct {
		move  dragontoothmg.Move
		score int
	}
	list := make([]scored, len(moves))
	for i := range moves {
		m := moves[i]
		sc := 0
		if ttMove != 0 && m == ttMove {
			sc = 1 << 20
		} else {
			if p := m.Promote(); p != dragontoothmg.Nothing {
				sc += 90000 + pieceValue[p]
			}
			if dragontoothmg.IsCapture(m, b) {
				victim, _ := dragontoothmg.GetPieceType(m.To(), b)
				attacker, _ := dragontoothmg.GetPieceType(m.From(), b)
				sc += 10000 + pieceValue[victim]*8 - pieceValue[attacker]
			}
		}
		list[i] = scored{m, sc}
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].score > list[j].score })
	out := make([]dragontoothmg.Move, len(moves))
	for i := range list {
		out[i] = list[i].move
	}
	return out
}
