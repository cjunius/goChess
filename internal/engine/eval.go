package engine

import (
	"math/bits"

	"github.com/dylhunn/dragontoothmg"
)

// Centipawn material values for move ordering only (MVV-LVA in orderMoves). The
// returned evaluation uses the tapered mgValue / egValue pairs below instead.
// The king value only matters for ordering, since both sides always have one.
var pieceValue = [7]int{
	dragontoothmg.Pawn:   100,
	dragontoothmg.Knight: 320,
	dragontoothmg.Bishop: 330,
	dragontoothmg.Rook:   500,
	dragontoothmg.Queen:  900,
	dragontoothmg.King:   20000,
}

// Tapered evaluation (PeSTO). Every positional term is accumulated as a
// (middlegame, endgame) pair and interpolated by game phase: a full board is
// pure middlegame, a bare-piece endgame is pure endgame.
//
// The piece-square tables below are the PeSTO tables written in the customary
// a8..h1 order (rank 8 first, as a board looks from White's side). dragontoothmg
// squares are little-endian (a1 = 0), so a White piece on square sq reads
// table[sq^56] and a Black piece reads table[sq].
const (
	maxPhase     = 24
	tempo        = 8
	bishopPairMG = 22
	bishopPairEG = 40
)

// gamePhaseInc is added to the running phase for each piece of that type on the
// board (both sides); the sum is clamped to maxPhase.
var gamePhaseInc = [7]int{
	dragontoothmg.Knight: 1,
	dragontoothmg.Bishop: 1,
	dragontoothmg.Rook:   2,
	dragontoothmg.Queen:  4,
}

var (
	mgValue = [7]int{
		dragontoothmg.Pawn: 82, dragontoothmg.Knight: 337, dragontoothmg.Bishop: 365,
		dragontoothmg.Rook: 477, dragontoothmg.Queen: 1025, dragontoothmg.King: 0,
	}
	egValue = [7]int{
		dragontoothmg.Pawn: 94, dragontoothmg.Knight: 281, dragontoothmg.Bishop: 297,
		dragontoothmg.Rook: 512, dragontoothmg.Queen: 936, dragontoothmg.King: 0,
	}

	mgPST = [7][64]int{
		dragontoothmg.Pawn:   mgPawn,
		dragontoothmg.Knight: mgKnight,
		dragontoothmg.Bishop: mgBishop,
		dragontoothmg.Rook:   mgRook,
		dragontoothmg.Queen:  mgQueen,
		dragontoothmg.King:   mgKing,
	}
	egPST = [7][64]int{
		dragontoothmg.Pawn:   egPawn,
		dragontoothmg.Knight: egKnight,
		dragontoothmg.Bishop: egBishop,
		dragontoothmg.Rook:   egRook,
		dragontoothmg.Queen:  egQueen,
		dragontoothmg.King:   egKing,
	}
)

// Evaluate returns a static score for the position in centipawns from the point
// of view of the side to move (positive = better for that side) — the sign
// convention negamax expects.
func Evaluate(b *dragontoothmg.Board) int {
	wmg, weg := pstScore(&b.White, true)
	bmg, beg := pstScore(&b.Black, false)
	mg, eg := wmg-bmg, weg-beg

	tmg, teg := evalTerms(b)
	mg += tmg
	eg += teg

	phase := gamePhase(b)
	score := (mg*phase + eg*(maxPhase-phase)) / maxPhase
	if !b.Wtomove {
		score = -score
	}
	return score + tempo
}

// gamePhase sums gamePhaseInc over every non-pawn piece on the board, clamped to
// maxPhase (early promotions can otherwise overshoot).
func gamePhase(b *dragontoothmg.Board) int {
	p := bits.OnesCount64(b.White.Knights|b.Black.Knights)*gamePhaseInc[dragontoothmg.Knight] +
		bits.OnesCount64(b.White.Bishops|b.Black.Bishops)*gamePhaseInc[dragontoothmg.Bishop] +
		bits.OnesCount64(b.White.Rooks|b.Black.Rooks)*gamePhaseInc[dragontoothmg.Rook] +
		bits.OnesCount64(b.White.Queens|b.Black.Queens)*gamePhaseInc[dragontoothmg.Queen]
	if p > maxPhase {
		p = maxPhase
	}
	return p
}

// pstScore returns the (mg, eg) material + piece-square total for one side.
func pstScore(bb *dragontoothmg.Bitboards, white bool) (mg, eg int) {
	for p := dragontoothmg.Pawn; p <= dragontoothmg.King; p++ {
		board := pieceBoard(bb, p)
		for board != 0 {
			sq := bits.TrailingZeros64(board)
			board &= board - 1
			idx := sq
			if white {
				idx ^= 56
			}
			mg += mgValue[p] + mgPST[p][idx]
			eg += egValue[p] + egPST[p][idx]
		}
	}
	if bits.OnesCount64(bb.Bishops) >= 2 {
		mg += bishopPairMG
		eg += bishopPairEG
	}
	return mg, eg
}

// pieceBoard returns the bitboard for piece type p within bb.
func pieceBoard(bb *dragontoothmg.Bitboards, p int) uint64 {
	switch p {
	case dragontoothmg.Pawn:
		return bb.Pawns
	case dragontoothmg.Knight:
		return bb.Knights
	case dragontoothmg.Bishop:
		return bb.Bishops
	case dragontoothmg.Rook:
		return bb.Rooks
	case dragontoothmg.Queen:
		return bb.Queens
	default:
		return bb.Kings
	}
}

var (
	mgPawn = [64]int{
		0, 0, 0, 0, 0, 0, 0, 0,
		98, 134, 61, 95, 68, 126, 34, -11,
		-6, 7, 26, 31, 65, 56, 25, -20,
		-14, 13, 6, 21, 23, 12, 17, -23,
		-27, -2, -5, 12, 17, 6, 10, -25,
		-26, -4, -4, -10, 3, 3, 33, -12,
		-35, -1, -20, -23, -15, 24, 38, -22,
		0, 0, 0, 0, 0, 0, 0, 0,
	}
	egPawn = [64]int{
		0, 0, 0, 0, 0, 0, 0, 0,
		178, 173, 158, 134, 147, 132, 165, 187,
		94, 100, 85, 67, 56, 53, 82, 84,
		32, 24, 13, 5, -2, 4, 17, 17,
		13, 9, -3, -7, -7, -8, 3, -1,
		4, 7, -6, 1, 0, -5, -1, -8,
		13, 8, 8, 10, 13, 0, 2, -7,
		0, 0, 0, 0, 0, 0, 0, 0,
	}
	mgKnight = [64]int{
		-167, -89, -34, -49, 61, -97, -15, -107,
		-73, -41, 72, 36, 23, 62, 7, -17,
		-47, 60, 37, 65, 84, 129, 73, 44,
		-9, 17, 19, 53, 37, 69, 18, 22,
		-13, 4, 16, 13, 28, 19, 21, -8,
		-23, -9, 12, 10, 19, 17, 25, -16,
		-29, -53, -12, -3, -1, 18, -14, -19,
		-105, -21, -58, -33, -17, -28, -19, -23,
	}
	egKnight = [64]int{
		-58, -38, -13, -28, -31, -27, -63, -99,
		-25, -8, -25, -2, -9, -25, -24, -52,
		-24, -20, 10, 9, -1, -9, -19, -41,
		-17, 3, 22, 22, 22, 11, 8, -18,
		-18, -6, 16, 25, 16, 17, 4, -18,
		-23, -3, -1, 15, 10, -3, -20, -22,
		-42, -20, -10, -5, -2, -20, -23, -44,
		-29, -51, -23, -15, -22, -18, -50, -64,
	}
	mgBishop = [64]int{
		-29, 4, -82, -37, -25, -42, 7, -8,
		-26, 16, -18, -13, 30, 59, 18, -47,
		-16, 37, 43, 40, 35, 50, 37, -2,
		-4, 5, 19, 50, 37, 37, 7, -2,
		-6, 13, 13, 26, 34, 12, 10, 4,
		0, 15, 15, 15, 14, 27, 18, 10,
		4, 15, 16, 0, 7, 21, 33, 1,
		-33, -3, -14, -21, -13, -12, -39, -21,
	}
	egBishop = [64]int{
		-14, -21, -11, -8, -7, -9, -17, -24,
		-8, -4, 7, -12, -3, -13, -4, -14,
		2, -8, 0, -1, -2, 6, 0, 4,
		-3, 9, 12, 9, 14, 10, 3, 2,
		-6, 3, 13, 19, 7, 10, -3, -9,
		-12, -3, 8, 10, 13, 3, -7, -15,
		-14, -18, -7, -1, 4, -9, -15, -27,
		-23, -9, -23, -5, -9, -16, -5, -17,
	}
	mgRook = [64]int{
		32, 42, 32, 51, 63, 9, 31, 43,
		27, 32, 58, 62, 80, 67, 26, 44,
		-5, 19, 26, 36, 17, 45, 61, 16,
		-24, -11, 7, 26, 24, 35, -8, -20,
		-36, -26, -12, -1, 9, -7, 6, -23,
		-45, -25, -16, -17, 3, 0, -5, -33,
		-44, -16, -20, -9, -1, 11, -6, -71,
		-19, -13, 1, 17, 16, 7, -37, -26,
	}
	egRook = [64]int{
		13, 10, 18, 15, 12, 12, 8, 5,
		11, 13, 13, 11, -3, 3, 8, 3,
		7, 7, 7, 5, 4, -3, -5, -3,
		4, 3, 13, 1, 2, 1, -1, 2,
		3, 5, 8, 4, -5, -6, -8, -11,
		-4, 0, -5, -1, -7, -12, -8, -16,
		-6, -6, 0, 2, -9, -9, -11, -3,
		-9, 2, 3, -1, -5, -13, 4, -20,
	}
	mgQueen = [64]int{
		-28, 0, 29, 12, 59, 44, 43, 45,
		-24, -39, -5, 1, -16, 57, 28, 54,
		-13, -17, 7, 8, 29, 56, 47, 57,
		-27, -27, -16, -16, -1, 17, -2, 1,
		-9, -26, -9, -10, -2, -4, 3, -3,
		-14, 2, -11, -2, -5, 2, 14, 5,
		-35, -8, 11, 2, 8, 15, -3, 1,
		-1, -18, -9, 10, -15, -25, -31, -50,
	}
	egQueen = [64]int{
		-9, 22, 22, 27, 27, 19, 10, 20,
		-17, 20, 32, 41, 58, 25, 30, 0,
		-20, 6, 9, 49, 47, 35, 19, 9,
		3, 22, 24, 45, 57, 40, 57, 36,
		-18, 28, 19, 47, 31, 34, 39, 23,
		-16, -27, 15, 6, 9, 17, 10, 5,
		-22, -23, -30, -16, -16, -23, -36, -32,
		-33, -28, -22, -43, -5, -32, -20, -41,
	}
	mgKing = [64]int{
		-65, 23, 16, -15, -56, -34, 2, 13,
		29, -1, -20, -7, -8, -4, -38, -29,
		-9, 24, 2, -16, -20, 6, 22, -22,
		-17, -20, -12, -27, -30, -25, -14, -36,
		-49, -1, -27, -39, -46, -44, -33, -51,
		-14, -14, -22, -46, -44, -30, -15, -27,
		1, 7, -8, -64, -43, -16, 9, 8,
		-15, 36, 12, -54, 8, -28, 24, 14,
	}
	egKing = [64]int{
		-74, -35, -18, -18, -11, 15, 4, -17,
		-12, 17, 14, 17, 17, 38, 23, 11,
		10, 17, 23, 15, 20, 45, 44, 13,
		-8, 22, 24, 27, 26, 33, 26, 3,
		-18, -4, 21, 24, 27, 23, 9, -11,
		-19, -3, 11, 21, 23, 16, 7, -9,
		-27, -11, 4, 13, 14, 4, -5, -17,
		-53, -34, -21, -11, -28, -14, -24, -43,
	}
)
