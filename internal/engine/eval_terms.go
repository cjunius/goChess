package engine

import (
	"math/bits"

	"github.com/dylhunn/dragontoothmg"
)

// Positional evaluation terms layered on top of material + piece-square tables:
// passed pawns, piece mobility, and king safety. Each is returned as a
// (middlegame, endgame) delta from White's point of view and folded into the
// taper by Evaluate.

// Mobility coefficients: score = k * (attacked-squares - pivot), where the pivot
// is roughly the average count so a typical piece scores near zero.
const (
	knightMobK   = 4
	bishopMobKMG = 3
	bishopMobKEG = 3
	rookMobKMG   = 2
	rookMobKEG   = 4
	queenMobKMG  = 1
	queenMobKEG  = 2

	pawnShieldPen = 14
)

// passed bonuses indexed by the pawn's rank from its own side (1..6).
var (
	passedMG = [8]int{0, 5, 10, 15, 25, 45, 70, 0}
	passedEG = [8]int{0, 15, 20, 35, 60, 100, 160, 0}

	// kingDangerTable maps summed attacker "units" near the king to an mg penalty.
	kingDangerTable = [21]int{
		0, 0, 3, 6, 10, 15, 21, 28, 36, 45, 55,
		66, 78, 91, 105, 120, 136, 153, 171, 190, 210,
	}
)

var (
	fileMask        [8]uint64
	knightAttacks   [64]uint64
	kingRing        [64]uint64
	whitePassedMask [64]uint64
	blackPassedMask [64]uint64
)

func init() {
	for f := 0; f < 8; f++ {
		fileMask[f] = 0x0101010101010101 << uint(f)
	}

	knightDeltas := [8][2]int{{1, 2}, {2, 1}, {2, -1}, {1, -2}, {-1, -2}, {-2, -1}, {-2, 1}, {-1, 2}}
	for sq := 0; sq < 64; sq++ {
		f, r := sq%8, sq/8

		for _, d := range knightDeltas {
			nf, nr := f+d[0], r+d[1]
			if nf >= 0 && nf < 8 && nr >= 0 && nr < 8 {
				knightAttacks[sq] |= 1 << uint(nr*8+nf)
			}
		}
		for df := -1; df <= 1; df++ {
			for dr := -1; dr <= 1; dr++ {
				if df == 0 && dr == 0 {
					continue
				}
				nf, nr := f+df, r+dr
				if nf >= 0 && nf < 8 && nr >= 0 && nr < 8 {
					kingRing[sq] |= 1 << uint(nr*8+nf)
				}
			}
		}

		files := fileMask[f]
		if f > 0 {
			files |= fileMask[f-1]
		}
		if f < 7 {
			files |= fileMask[f+1]
		}
		var wFwd, bFwd uint64
		for rr := r + 1; rr < 8; rr++ {
			wFwd |= 0xff << uint(rr*8)
		}
		for rr := 0; rr < r; rr++ {
			bFwd |= 0xff << uint(rr*8)
		}
		whitePassedMask[sq] = files & wFwd
		blackPassedMask[sq] = files & bFwd
	}
}

// evalTerms returns the summed (mg, eg) positional deltas from White's view.
func evalTerms(b *dragontoothmg.Board) (mg, eg int) {
	pmg, peg := passedPawns(b)
	mmg, meg := mobility(b)
	return pmg + mmg + kingSafety(b), peg + meg
}

func passedPawns(b *dragontoothmg.Board) (mg, eg int) {
	wp, bp := b.White.Pawns, b.Black.Pawns
	for x := wp; x != 0; x &= x - 1 {
		sq := bits.TrailingZeros64(x)
		if bp&whitePassedMask[sq] == 0 {
			r := sq / 8
			mg += passedMG[r]
			eg += passedEG[r]
		}
	}
	for x := bp; x != 0; x &= x - 1 {
		sq := bits.TrailingZeros64(x)
		if wp&blackPassedMask[sq] == 0 {
			r := 7 - sq/8
			mg -= passedMG[r]
			eg -= passedEG[r]
		}
	}
	return mg, eg
}

func mobility(b *dragontoothmg.Board) (mg, eg int) {
	all := b.White.All | b.Black.All
	wmg, weg := sideMobility(&b.White, b.White.All, all)
	bmg, beg := sideMobility(&b.Black, b.Black.All, all)
	return wmg - bmg, weg - beg
}

func sideMobility(bb *dragontoothmg.Bitboards, own, all uint64) (mg, eg int) {
	for x := bb.Knights; x != 0; x &= x - 1 {
		c := bits.OnesCount64(knightAttacks[bits.TrailingZeros64(x)] &^ own)
		mg += knightMobK * (c - 4)
		eg += knightMobK * (c - 4)
	}
	for x := bb.Bishops; x != 0; x &= x - 1 {
		sq := uint8(bits.TrailingZeros64(x)) //nolint:gosec // square index, 0..63
		c := bits.OnesCount64(dragontoothmg.CalculateBishopMoveBitboard(sq, all) &^ own)
		mg += bishopMobKMG * (c - 6)
		eg += bishopMobKEG * (c - 6)
	}
	for x := bb.Rooks; x != 0; x &= x - 1 {
		sq := uint8(bits.TrailingZeros64(x)) //nolint:gosec // square index, 0..63
		c := bits.OnesCount64(dragontoothmg.CalculateRookMoveBitboard(sq, all) &^ own)
		mg += rookMobKMG * (c - 7)
		eg += rookMobKEG * (c - 7)
	}
	for x := bb.Queens; x != 0; x &= x - 1 {
		sq := uint8(bits.TrailingZeros64(x)) //nolint:gosec // square index, 0..63
		att := dragontoothmg.CalculateBishopMoveBitboard(sq, all) | dragontoothmg.CalculateRookMoveBitboard(sq, all)
		c := bits.OnesCount64(att &^ own)
		mg += queenMobKMG * (c - 14)
		eg += queenMobKEG * (c - 14)
	}
	return mg, eg
}

// kingSafety returns the mg-only delta from White's view: each side's own king
// danger (attackers near the king + a missing-pawn-shield penalty) counts
// against it.
func kingSafety(b *dragontoothmg.Board) int {
	all := b.White.All | b.Black.All
	white := kingDanger(b.White.Kings, &b.Black, all) + pawnShield(b.White.Kings, b.White.Pawns, true)
	black := kingDanger(b.Black.Kings, &b.White, all) + pawnShield(b.Black.Kings, b.Black.Pawns, false)
	return black - white
}

func kingDanger(kingBB uint64, attacker *dragontoothmg.Bitboards, all uint64) int {
	if kingBB == 0 {
		return 0
	}
	ring := kingRing[bits.TrailingZeros64(kingBB)]
	units := 0
	for x := attacker.Knights; x != 0; x &= x - 1 {
		units += 2 * bits.OnesCount64(knightAttacks[bits.TrailingZeros64(x)]&ring)
	}
	for x := attacker.Bishops; x != 0; x &= x - 1 {
		sq := uint8(bits.TrailingZeros64(x)) //nolint:gosec // square index, 0..63
		units += 2 * bits.OnesCount64(dragontoothmg.CalculateBishopMoveBitboard(sq, all)&ring)
	}
	for x := attacker.Rooks; x != 0; x &= x - 1 {
		sq := uint8(bits.TrailingZeros64(x)) //nolint:gosec // square index, 0..63
		units += 3 * bits.OnesCount64(dragontoothmg.CalculateRookMoveBitboard(sq, all)&ring)
	}
	for x := attacker.Queens; x != 0; x &= x - 1 {
		sq := uint8(bits.TrailingZeros64(x)) //nolint:gosec // square index, 0..63
		att := dragontoothmg.CalculateBishopMoveBitboard(sq, all) | dragontoothmg.CalculateRookMoveBitboard(sq, all)
		units += 5 * bits.OnesCount64(att&ring)
	}
	if units >= len(kingDangerTable) {
		units = len(kingDangerTable) - 1
	}
	return kingDangerTable[units]
}

// pawnShield penalises files in front of the king (its own file and the two
// neighbours) that have no friendly pawn on the next two ranks.
func pawnShield(kingBB, pawns uint64, white bool) int {
	if kingBB == 0 {
		return 0
	}
	ksq := bits.TrailingZeros64(kingBB)
	kf, kr := ksq%8, ksq/8
	pen := 0
	for f := kf - 1; f <= kf+1; f++ {
		if f < 0 || f > 7 {
			continue
		}
		var mask uint64
		if white {
			for rr := kr + 1; rr <= kr+2 && rr < 8; rr++ {
				mask |= 1 << uint(rr*8+f)
			}
		} else {
			for rr := kr - 1; rr >= kr-2 && rr >= 0; rr-- {
				mask |= 1 << uint(rr*8+f)
			}
		}
		if mask != 0 && pawns&mask == 0 {
			pen += pawnShieldPen
		}
	}
	return pen
}
