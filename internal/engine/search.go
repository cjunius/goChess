package engine

import (
	"sort"
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
	nodes    int64
	deadline time.Time
	stopped  bool
}

func (s *searcher) timeUp() bool {
	return !s.deadline.IsZero() && time.Now().After(s.deadline)
}

// Search runs iterative-deepening alpha-beta and returns the best move it found.
// The board is left unmodified (every applied move is unapplied).
func Search(b *dragontoothmg.Board, p SearchParams) SearchResult {
	if p.MaxDepth <= 0 || p.MaxDepth > maxPly {
		p.MaxDepth = maxPly
	}
	s := &searcher{}
	if p.MoveTime > 0 {
		s.deadline = time.Now().Add(p.MoveTime)
	}
	start := time.Now()

	var res SearchResult
	if b.White.Kings == 0 || b.Black.Kings == 0 {
		return res // illegal position, nothing sensible to search
	}
	root := b.GenerateLegalMoves()
	if len(root) == 0 {
		return res
	}
	res.BestMove = root[0]

	for depth := 1; depth <= p.MaxDepth; depth++ {
		score, move, ok := s.searchRoot(b, depth)
		if !ok {
			break // out of time: keep the previous completed depth
		}
		res.BestMove, res.Score, res.Depth = move, score, depth
		if score >= mateScore-maxPly || score <= -mateScore+maxPly {
			break // forced mate found; deeper search cannot improve on it
		}
		if s.timeUp() {
			break
		}
	}
	res.Nodes = s.nodes
	res.Elapsed = time.Since(start)
	return res
}

func (s *searcher) searchRoot(b *dragontoothmg.Board, depth int) (score int, best dragontoothmg.Move, ok bool) {
	moves := orderMoves(b, b.GenerateLegalMoves())
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
	return bestScore, best, true
}

func (s *searcher) negamax(b *dragontoothmg.Board, depth, alpha, beta, ply int) int {
	s.nodes++
	if s.nodes&2047 == 0 && s.timeUp() {
		s.stopped = true
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

	moves := b.GenerateLegalMoves()
	if len(moves) == 0 {
		if b.OurKingInCheck() {
			return -mateScore + ply // checkmate; prefer the shortest mate
		}
		return drawScore // stalemate
	}

	best := -infinity
	for _, m := range orderMoves(b, moves) {
		unapply := b.Apply(m)
		v := -s.negamax(b, depth-1, -beta, -alpha, ply+1)
		unapply()
		if s.stopped {
			return 0
		}
		if v > best {
			best = v
		}
		if v > alpha {
			alpha = v
		}
		if alpha >= beta {
			break // fail-high: opponent won't enter this line
		}
	}
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

	for _, m := range orderMoves(b, b.GenerateLegalMoves()) {
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
// promotions first, then captures by MVV-LVA (most valuable victim, least
// valuable attacker), then quiet moves.
func orderMoves(b *dragontoothmg.Board, moves []dragontoothmg.Move) []dragontoothmg.Move {
	type scored struct {
		move  dragontoothmg.Move
		score int
	}
	list := make([]scored, len(moves))
	for i := range moves {
		m := moves[i]
		sc := 0
		if p := m.Promote(); p != dragontoothmg.Nothing {
			sc += 90000 + pieceValue[p]
		}
		if dragontoothmg.IsCapture(m, b) {
			victim, _ := dragontoothmg.GetPieceType(m.To(), b)
			attacker, _ := dragontoothmg.GetPieceType(m.From(), b)
			sc += 10000 + pieceValue[victim]*8 - pieceValue[attacker]
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
