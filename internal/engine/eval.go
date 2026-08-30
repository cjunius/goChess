package engine

import (
	"math/bits"

	"github.com/dylhunn/dragontoothmg"
)

// Centipawn material values, indexed by the dragontoothmg piece constants
// (Pawn=1 .. King=6). The king value only matters for move ordering, never
// for the returned evaluation, since both sides always have exactly one.
var pieceValue = [7]int{
	dragontoothmg.Pawn:   100,
	dragontoothmg.Knight: 320,
	dragontoothmg.Bishop: 330,
	dragontoothmg.Rook:   500,
	dragontoothmg.Queen:  900,
	dragontoothmg.King:   20000,
}

const bishopPairBonus = 30

// Piece-square tables (Michniewski's "simplified evaluation function"),
// written in a1..h8 order (rank 1 first). dragontoothmg uses little-endian
// rank-file square numbering, so a white piece on square sq reads pst[sq]
// directly and a black piece reads the vertically mirrored pst[sq^56].
var (
	pawnPST = [64]int{
		0, 0, 0, 0, 0, 0, 0, 0,
		5, 10, 10, -20, -20, 10, 10, 5,
		5, -5, -10, 0, 0, -10, -5, 5,
		0, 0, 0, 20, 20, 0, 0, 0,
		5, 5, 10, 25, 25, 10, 5, 5,
		10, 10, 20, 30, 30, 20, 10, 10,
		50, 50, 50, 50, 50, 50, 50, 50,
		0, 0, 0, 0, 0, 0, 0, 0,
	}
	knightPST = [64]int{
		-50, -40, -30, -30, -30, -30, -40, -50,
		-40, -20, 0, 5, 5, 0, -20, -40,
		-30, 5, 10, 15, 15, 10, 5, -30,
		-30, 0, 15, 20, 20, 15, 0, -30,
		-30, 5, 15, 20, 20, 15, 5, -30,
		-30, 0, 10, 15, 15, 10, 0, -30,
		-40, -20, 0, 0, 0, 0, -20, -40,
		-50, -40, -30, -30, -30, -30, -40, -50,
	}
	bishopPST = [64]int{
		-20, -10, -10, -10, -10, -10, -10, -20,
		-10, 5, 0, 0, 0, 0, 5, -10,
		-10, 10, 10, 10, 10, 10, 10, -10,
		-10, 0, 10, 10, 10, 10, 0, -10,
		-10, 5, 5, 10, 10, 5, 5, -10,
		-10, 0, 5, 10, 10, 5, 0, -10,
		-10, 0, 0, 0, 0, 0, 0, -10,
		-20, -10, -10, -10, -10, -10, -10, -20,
	}
	rookPST = [64]int{
		0, 0, 0, 5, 5, 0, 0, 0,
		-5, 0, 0, 0, 0, 0, 0, -5,
		-5, 0, 0, 0, 0, 0, 0, -5,
		-5, 0, 0, 0, 0, 0, 0, -5,
		-5, 0, 0, 0, 0, 0, 0, -5,
		-5, 0, 0, 0, 0, 0, 0, -5,
		5, 10, 10, 10, 10, 10, 10, 5,
		0, 0, 0, 0, 0, 0, 0, 0,
	}
	queenPST = [64]int{
		-20, -10, -10, -5, -5, -10, -10, -20,
		-10, 0, 0, 0, 0, 0, 0, -10,
		-10, 0, 5, 5, 5, 5, 0, -10,
		-5, 0, 5, 5, 5, 5, 0, -5,
		0, 0, 5, 5, 5, 5, 0, -5,
		-10, 5, 5, 5, 5, 5, 0, -10,
		-10, 0, 5, 0, 0, 0, 0, -10,
		-20, -10, -10, -5, -5, -10, -10, -20,
	}
	kingPST = [64]int{
		20, 30, 10, 0, 0, 10, 30, 20,
		20, 20, 0, 0, 0, 0, 20, 20,
		-10, -20, -20, -20, -20, -20, -20, -10,
		-20, -30, -30, -40, -40, -30, -30, -20,
		-30, -40, -40, -50, -50, -40, -40, -30,
		-30, -40, -40, -50, -50, -40, -40, -30,
		-30, -40, -40, -50, -50, -40, -40, -30,
		-30, -40, -40, -50, -50, -40, -40, -30,
	}
)

// Evaluate returns a static score for the position in centipawns, from the
// point of view of the side to move (positive = better for that side). This is
// the sign convention negamax expects.
func Evaluate(b *dragontoothmg.Board) int {
	score := evalSide(&b.White, true) - evalSide(&b.Black, false)
	if b.Wtomove {
		return score
	}
	return -score
}

func evalSide(bb *dragontoothmg.Bitboards, white bool) int {
	s := evalPiece(bb.Pawns, &pawnPST, pieceValue[dragontoothmg.Pawn], white) +
		evalPiece(bb.Knights, &knightPST, pieceValue[dragontoothmg.Knight], white) +
		evalPiece(bb.Bishops, &bishopPST, pieceValue[dragontoothmg.Bishop], white) +
		evalPiece(bb.Rooks, &rookPST, pieceValue[dragontoothmg.Rook], white) +
		evalPiece(bb.Queens, &queenPST, pieceValue[dragontoothmg.Queen], white) +
		evalPiece(bb.Kings, &kingPST, pieceValue[dragontoothmg.King], white)
	if bits.OnesCount64(bb.Bishops) >= 2 {
		s += bishopPairBonus
	}
	return s
}

func evalPiece(board uint64, pst *[64]int, value int, white bool) int {
	s := 0
	for board != 0 {
		sq := bits.TrailingZeros64(board)
		board &= board - 1
		s += value
		if white {
			s += pst[sq]
		} else {
			s += pst[sq^56]
		}
	}
	return s
}
