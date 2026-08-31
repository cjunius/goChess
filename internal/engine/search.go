package engine

import (
	"sort"
	"strings"
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
	// mateThreshold is the smallest score still considered "a mate": any score
	// at least this big encodes a forced mate for the side to move.
	mateThreshold = mateScore - maxPly

	// nmpMinDepth is the shallowest depth at which null-move pruning is tried.
	nmpMinDepth = 3
	// lmrMinDepth / lmrMinMove gate late move reductions: only in subtrees at
	// least this deep, and only for quiet moves this far down the ordered list.
	lmrMinDepth = 3
	lmrMinMove  = 3
	// historyMax caps a history counter so repeated cut-offs cannot dwarf the
	// capture scores in move ordering.
	historyMax = 1 << 22

	// Aspiration windows: from aspMinDepth, each iteration first searches a
	// window aspBaseDelta wide centred on the previous score, doubling the delta
	// on the side that fails until the score lands inside or the window has
	// grown past aspMaxDelta (at which point it opens fully).
	aspMinDepth  = 5
	aspBaseDelta = 25
	aspMaxDelta  = 400
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
	// PV is the principal variation for the deepest completed iteration, best
	// move first. PV[0] equals BestMove whenever the search completed a depth.
	PV []dragontoothmg.Move
}

type searcher struct {
	tt       *TT
	stop     *atomic.Bool // shared across Lazy-SMP workers; set once time is up
	id       int
	nodes    int64
	deadline time.Time
	stopped  bool

	// killers holds, per ply, up to two quiet moves that most recently caused a
	// beta cut-off at that ply; history accumulates depth^2 for every quiet move
	// that caused a cut-off, indexed by [side][from][to]. Both are per-searcher,
	// so Lazy-SMP workers keep independent tables and need no synchronisation.
	killers [maxPly + 1][2]dragontoothmg.Move
	history [2][64][64]int

	// pv is a triangular principal-variation table: pv[ply][:pvLen[ply]] is the
	// best line found from that ply down, maintained only at nodes that raise
	// alpha. The root line is pv[0][:pvLen[0]].
	pv    [maxPly + 1][maxPly + 1]dragontoothmg.Move
	pvLen [maxPly + 1]int
}

// setPV records m as the best move at ply and splices the child line at ply+1
// behind it. Called whenever a move raises alpha.
func (s *searcher) setPV(ply int, m dragontoothmg.Move) {
	s.pv[ply][0] = m
	if ply+1 >= len(s.pv) {
		s.pvLen[ply] = 1
		return
	}
	n := copy(s.pv[ply][1:], s.pv[ply+1][:s.pvLen[ply+1]])
	s.pvLen[ply] = n + 1
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

	prevScore := 0
	for depth := startDepth; depth <= maxDepth; depth++ {
		score, move, ok := s.searchDepth(b, depth, prevScore)
		if !ok {
			break // out of time: keep the previous completed depth
		}
		res.BestMove, res.Score, res.Depth = move, score, depth
		res.PV = append(res.PV[:0], s.pv[0][:s.pvLen[0]]...)
		prevScore = score
		if score >= mateThreshold || score <= -mateThreshold {
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

// searchDepth runs one iterative-deepening iteration for depth. From aspMinDepth
// it wraps searchRoot in an aspiration window centred on the previous score,
// widening the failing side until the score lands inside; shallow depths and
// positions with a known mate score search the full window directly.
func (s *searcher) searchDepth(b *dragontoothmg.Board, depth, prevScore int) (int, dragontoothmg.Move, bool) {
	if depth < aspMinDepth || prevScore >= mateThreshold || prevScore <= -mateThreshold {
		return s.searchRoot(b, depth, -infinity, infinity)
	}

	delta := aspBaseDelta
	alpha := max(prevScore-delta, -infinity)
	beta := min(prevScore+delta, infinity)
	for {
		score, move, ok := s.searchRoot(b, depth, alpha, beta)
		if !ok {
			return 0, dragontoothmg.Move(0), false
		}
		switch {
		case score <= alpha:
			alpha = max(alpha-delta, -infinity)
			delta *= 2
		case score >= beta:
			beta = min(beta+delta, infinity)
			delta *= 2
		default:
			return score, move, true
		}
		if delta > aspMaxDelta {
			alpha, beta = -infinity, infinity
		}
	}
}

// searchRoot searches every legal move at the root inside the window [alpha,
// beta] and returns the (fail-soft) score of the best one. The first move gets
// the full window; the rest are scouted with a null window and re-searched only
// when the scout lands inside. A score at or above beta means a move failed high
// and the remaining moves were skipped — the caller must widen and retry.
func (s *searcher) searchRoot(b *dragontoothmg.Board, depth, alpha, beta int) (score int, best dragontoothmg.Move, ok bool) {
	alphaOrig := alpha
	key := b.Hash()
	_, ttMove, _ := s.tt.probe(key, depth, -infinity, infinity, 0)

	moves := s.orderMoves(b, b.GenerateLegalMoves(), ttMove, 0)
	s.pvLen[0] = 0
	bestScore := -infinity
	for i, m := range moves {
		unapply := b.Apply(m)
		var v int
		if i == 0 {
			v = -s.negamax(b, depth-1, -beta, -alpha, 1, true)
		} else {
			v = -s.negamax(b, depth-1, -alpha-1, -alpha, 1, true)
			if v > alpha && v < beta {
				v = -s.negamax(b, depth-1, -beta, -alpha, 1, true)
			}
		}
		unapply()
		if s.stopped {
			return 0, dragontoothmg.Move(0), false
		}
		if v > bestScore {
			bestScore, best = v, m
		}
		if v > alpha {
			alpha = v
			s.setPV(0, m)
		}
		if alpha >= beta {
			break // fail-high: caller widens beta and re-searches
		}
	}

	bound := boundExact
	switch {
	case bestScore <= alphaOrig:
		bound = boundUpper
	case bestScore >= beta:
		bound = boundLower
	}
	s.tt.store(key, depth, bestScore, bound, best, 0)
	return bestScore, best, true
}

// outOfTime bumps the node counter and, every 2048 nodes, checks the clock. It
// latches s.stopped (and the shared stop flag) so every frame unwinds fast.
func (s *searcher) outOfTime() bool {
	s.nodes++
	if s.nodes&2047 != 0 || !s.timeUp() {
		return false
	}
	s.stopped = true
	if s.stop != nil {
		s.stop.Store(true)
	}
	return true
}

// terminalScore handles the non-search node types: a captured king, the
// fifty-move rule, and the quiescence hand-off at the horizon.
func (s *searcher) terminalScore(b *dragontoothmg.Board, depth, alpha, beta, ply int) (int, bool) {
	switch {
	case b.White.Kings == 0 || b.Black.Kings == 0:
		return kingCaptureScore(b, ply), true
	case b.Halfmoveclock >= 100:
		return drawScore, true
	case depth <= 0:
		return s.quiesce(b, alpha, beta, ply), true
	}
	return 0, false
}

func (s *searcher) negamax(b *dragontoothmg.Board, depth, alpha, beta, ply int, canNull bool) int {
	if s.outOfTime() {
		return 0
	}
	if v, done := s.terminalScore(b, depth, alpha, beta, ply); done {
		return v
	}
	if ply <= maxPly {
		s.pvLen[ply] = 0
	}

	alphaOrig := alpha
	key := b.Hash()
	ttScore, ttMove, cutoff := s.tt.probe(key, depth, alpha, beta, ply)
	if cutoff {
		return ttScore
	}

	inCheck := b.OurKingInCheck()
	if v, ok := s.tryNullMove(b, depth, beta, ply, canNull, inCheck); ok {
		return v
	}

	moves := b.GenerateLegalMoves()
	if len(moves) == 0 {
		if inCheck {
			return -mateScore + ply // checkmate; prefer the shortest mate
		}
		return drawScore // stalemate
	}

	best := -infinity
	var bestMove dragontoothmg.Move
	for i, m := range s.orderMoves(b, moves, ttMove, ply) {
		v := s.searchMove(b, m, i, depth, alpha, beta, ply, inCheck)
		if s.stopped {
			return 0
		}
		if v > best {
			best, bestMove = v, m
		}
		if v > alpha {
			alpha = v
			if ply < maxPly {
				s.setPV(ply, m)
			}
		}
		if alpha >= beta {
			if isQuiet(b, m) {
				s.recordCutoff(b, m, depth, ply)
			}
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

// searchMove applies m, searches the resulting position, and returns its score
// from the current side's point of view. This is principal variation search: the
// first move gets a full window at full depth; every later move is first probed
// with a null window (and, for late quiets, a reduced depth — LMR), and only
// re-searched at full depth / full window when that probe beats alpha.
func (s *searcher) searchMove(b *dragontoothmg.Board, m dragontoothmg.Move, moveIdx, depth, alpha, beta, ply int, inCheck bool) int {
	quiet := isQuiet(b, m)
	unapply := b.Apply(m)
	defer unapply()
	givesCheck := b.OurKingInCheck()

	newDepth := depth - 1
	if moveIdx == 0 {
		return -s.negamax(b, newDepth, -beta, -alpha, ply+1, true)
	}

	red := 0
	if depth >= lmrMinDepth && moveIdx >= lmrMinMove && quiet && !inCheck && !givesCheck {
		red = 1
		if moveIdx >= 6 && depth >= 5 {
			red = 2
		}
	}

	v := -s.negamax(b, newDepth-red, -alpha-1, -alpha, ply+1, true)
	if v > alpha && red > 0 {
		// The reduction was too aggressive: retry at full depth, still scouting.
		v = -s.negamax(b, newDepth, -alpha-1, -alpha, ply+1, true)
	}
	if v > alpha && v < beta {
		// Scout landed inside the window: this move may be part of the PV, so
		// resolve its true score with the full window.
		v = -s.negamax(b, newDepth, -beta, -alpha, ply+1, true)
	}
	return v
}

// tryNullMove implements null-move pruning: if handing the opponent a free move
// still leaves the static evaluation at or above beta, the position is good
// enough that a full search is very unlikely to drop below beta, so return a
// cut-off. Skipped in check, in shallow subtrees, when the side to move has only
// pawns (the classic zugzwang trap), when beta is already a mate score, or
// immediately after another null move. Returns (score, true) when it cuts.
func (s *searcher) tryNullMove(b *dragontoothmg.Board, depth, beta, ply int, canNull, inCheck bool) (int, bool) {
	if !canNull || inCheck || depth < nmpMinDepth || beta >= mateThreshold {
		return 0, false
	}
	if !hasNonPawnMaterial(b) || Evaluate(b) < beta {
		return 0, false
	}

	r := 2
	if depth >= 6 {
		r = 3
	}
	nb := nullMoveBoard(b)
	score := -s.negamax(&nb, depth-1-r, -beta, -beta+1, ply+1, false)
	if s.stopped {
		return 0, true // value ignored; the caller re-checks s.stopped
	}
	if score >= beta {
		if score >= mateThreshold {
			score = beta // a mate found only past a null move is not proven
		}
		return score, true
	}
	return 0, false
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

	for _, m := range s.orderMoves(b, b.GenerateLegalMoves(), 0, ply) {
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

// recordCutoff credits a quiet move that produced a beta cut-off: it becomes a
// killer for its ply and its history counter grows by depth^2.
func (s *searcher) recordCutoff(b *dragontoothmg.Board, m dragontoothmg.Move, depth, ply int) {
	if ply >= 0 && ply <= maxPly {
		k := &s.killers[ply]
		if k[0] != m {
			k[1] = k[0]
			k[0] = m
		}
	}
	h := &s.history[sideIndex(b)][m.From()][m.To()]
	if *h += depth * depth; *h > historyMax {
		*h = historyMax
	}
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

// sideIndex is 0 for White to move, 1 for Black — the first index of history.
func sideIndex(b *dragontoothmg.Board) int {
	if b.Wtomove {
		return 0
	}
	return 1
}

// isQuiet reports whether m is neither a capture nor a promotion.
func isQuiet(b *dragontoothmg.Board, m dragontoothmg.Move) bool {
	return m.Promote() == dragontoothmg.Nothing && !dragontoothmg.IsCapture(m, b)
}

// hasNonPawnMaterial reports whether the side to move has a piece other than
// pawns and the king — the precondition that makes null-move pruning safe.
func hasNonPawnMaterial(b *dragontoothmg.Board) bool {
	side := &b.White
	if !b.Wtomove {
		side = &b.Black
	}
	return side.Knights|side.Bishops|side.Rooks|side.Queens != 0
}

// nullMoveBoard returns b with the side to move flipped and any en-passant right
// dropped — the position reached by "passing". It goes through FEN because
// dragontoothmg keeps the Zobrist hash and en-passant square in unexported
// fields, and a stale hash would corrupt the shared transposition table.
func nullMoveBoard(b *dragontoothmg.Board) dragontoothmg.Board {
	f := strings.Fields(b.ToFen())
	if f[1] == "w" {
		f[1] = "b"
	} else {
		f[1] = "w"
	}
	f[3] = "-"
	return dragontoothmg.ParseFen(strings.Join(f, " "))
}

// orderMoves sorts moves best-first so alpha-beta prunes as early as possible:
// the transposition-table move, then promotions, then captures by MVV-LVA (most
// valuable victim, least valuable attacker), then the two killer moves for this
// ply, then the remaining quiet moves by history score.
func (s *searcher) orderMoves(b *dragontoothmg.Board, moves []dragontoothmg.Move, ttMove dragontoothmg.Move, ply int) []dragontoothmg.Move {
	var k0, k1 dragontoothmg.Move
	if ply >= 0 && ply <= maxPly {
		k0, k1 = s.killers[ply][0], s.killers[ply][1]
	}
	side := sideIndex(b)

	type scored struct {
		move  dragontoothmg.Move
		score int
	}
	list := make([]scored, len(moves))
	for i := range moves {
		m := moves[i]
		var sc int
		switch {
		case ttMove != 0 && m == ttMove:
			sc = 1 << 24
		case m.Promote() != dragontoothmg.Nothing:
			sc = 1<<20 + pieceValue[m.Promote()]
			if dragontoothmg.IsCapture(m, b) {
				victim, _ := dragontoothmg.GetPieceType(m.To(), b)
				sc += pieceValue[victim]
			}
		case dragontoothmg.IsCapture(m, b):
			victim, _ := dragontoothmg.GetPieceType(m.To(), b)
			attacker, _ := dragontoothmg.GetPieceType(m.From(), b)
			sc = 1<<19 + pieceValue[victim]*8 - pieceValue[attacker]
		case m == k0:
			sc = 1<<18 + 1
		case m == k1:
			sc = 1 << 18
		default:
			sc = s.history[side][m.From()][m.To()]
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
