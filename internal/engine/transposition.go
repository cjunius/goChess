package engine

import (
	"sync/atomic"

	"github.com/dylhunn/dragontoothmg"
)

// Transposition-table bound flags. They record how a stored score relates to the
// true value of the position: EXACT is the value itself, LOWER is a fail-high
// (the value is at least this), UPPER is a fail-low (the value is at most this).
const (
	boundExact = 1
	boundLower = 2
	boundUpper = 3
)

const (
	defaultHashMB = 64
	// One ttEntry is two uint64 words.
	ttEntryBytes = 16
)

// ttEntry is a single table slot, accessed with Hyatt's lockless XOR trick so
// Lazy-SMP workers can share the table without a mutex. word0 holds key ^ data
// and word1 holds data; a reader recovers key' = word0 ^ word1 and rejects the
// entry unless key' matches the probe key. A write torn across two goroutines
// therefore fails that check and simply reads as a miss.
type ttEntry struct {
	lock atomic.Uint64 // key ^ data
	data atomic.Uint64 // packed move | score | depth | bound
}

// data layout (64 bits):
//
//	bits  0..15  best move (dragontoothmg.Move, uint16)
//	bits 16..47  score, int32 two's-complement (mate scores need > 16 bits)
//	bits 48..55  depth, uint8
//	bits 56..57  bound flag
//	bits 58..63  generation (6-bit search counter, for ageing)
func packTT(move dragontoothmg.Move, score, depth, bound, gen int) uint64 {
	switch {
	case depth < 0:
		depth = 0
	case depth > 255:
		depth = 255
	}
	// score fits int32 (mate scores are ~1e6); the mask keeps only its low 32
	// bits and unpackTT sign-extends them back.
	return uint64(uint16(move)) |
		(uint64(uint32(score)) << 16) | //nolint:gosec // deliberate low-32-bit pack; unpackTT restores the sign
		(uint64(depth&0xff) << 48) |
		(uint64(bound&3) << 56) |
		(uint64(gen&0x3f) << 58)
}

func unpackTT(data uint64) (move dragontoothmg.Move, score, depth, bound, gen int) {
	move = dragontoothmg.Move(data & 0xffff)
	score = int(int32(data >> 16)) //nolint:gosec // sign-extend the packed int32 score
	depth = int((data >> 48) & 0xff)
	bound = int((data >> 56) & 3)
	gen = int((data >> 58) & 0x3f)
	return
}

// TT is a fixed-size, power-of-two transposition table keyed by the Zobrist hash
// that dragontoothmg maintains incrementally (dragontoothmg.Board.Hash). It is
// safe for concurrent use by the Lazy-SMP workers.
type TT struct {
	entries []ttEntry
	mask    uint64
	// gen is bumped once per search (NewSearch). store treats entries from an
	// older generation as free space, so a new search reclaims the previous
	// search's table instead of being blocked by its deeper entries.
	gen atomic.Uint32
}

// NewTT returns a table that occupies about mb megabytes, rounded down to the
// nearest power-of-two number of entries. mb <= 0 selects the default size.
func NewTT(mb int) *TT {
	if mb <= 0 {
		mb = defaultHashMB
	}
	want := (mb * 1024 * 1024) / ttEntryBytes
	size := uint64(1)
	for size<<1 <= uint64(want) {
		size <<= 1
	}
	return &TT{entries: make([]ttEntry, size), mask: size - 1}
}

// NewSearch advances the table's generation. Call it once at the start of every
// search so entries left by the previous search become preferentially
// replaceable (see store).
func (t *TT) NewSearch() {
	t.gen.Add(1)
}

// Clear empties every slot. Call it between games (UCI "ucinewgame"); stale
// entries from an unrelated position are otherwise indistinguishable from live
// ones once their key happens to collide.
func (t *TT) Clear() {
	for i := range t.entries {
		t.entries[i].lock.Store(0)
		t.entries[i].data.Store(0)
	}
}

// probe looks up key. cutoff is true when the stored score is usable directly
// for the (depth, alpha, beta) window; move is the stored best move and is
// returned for move ordering even when cutoff is false. ply rebases mate scores
// from the stored node back to the search root.
func (t *TT) probe(key uint64, depth, alpha, beta, ply int) (score int, move dragontoothmg.Move, cutoff bool) {
	e := &t.entries[key&t.mask]
	lock := e.lock.Load()
	data := e.data.Load()
	if data == 0 || lock^data != key {
		return 0, 0, false
	}
	m, eScore, eDepth, bound, _ := unpackTT(data)
	if eDepth < depth {
		return 0, m, false
	}
	s := ttProbeScore(eScore, ply)
	switch bound {
	case boundExact:
		return s, m, true
	case boundLower:
		if s >= beta {
			return s, m, true
		}
	case boundUpper:
		if s <= alpha {
			return s, m, true
		}
	}
	return 0, m, false
}

// store records a result for key. An existing entry is kept only when it is for
// the same key, from the current generation, and searched deeper — so within one
// search the table is depth-preferred, but a new search (NewSearch) overwrites
// the previous one's entries regardless of their depth.
func (t *TT) store(key uint64, depth, score, bound int, move dragontoothmg.Move, ply int) {
	gen := int(t.gen.Load() & 0x3f)
	e := &t.entries[key&t.mask]
	if old := e.data.Load(); old != 0 && e.lock.Load()^old == key {
		if _, _, oldDepth, _, oldGen := unpackTT(old); oldGen == gen && oldDepth > depth {
			return
		}
	}
	data := packTT(move, ttStoreScore(score, ply), depth, bound, gen)
	e.lock.Store(key ^ data)
	e.data.Store(data)
}

// ttStoreScore rebases a root-relative score to the node that is storing it, so
// "mate in N from the root" becomes "mate in N from here". Only mate scores
// move; ordinary centipawn scores are stored unchanged.
func ttStoreScore(score, ply int) int {
	switch {
	case score >= mateScore-maxPly:
		return score + ply
	case score <= -mateScore+maxPly:
		return score - ply
	}
	return score
}

// ttProbeScore is the inverse of ttStoreScore: it rebases a stored score back to
// the search root.
func ttProbeScore(score, ply int) int {
	switch {
	case score >= mateScore-maxPly:
		return score - ply
	case score <= -mateScore+maxPly:
		return score + ply
	}
	return score
}
