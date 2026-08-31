package engine

import (
	"encoding/binary"
	"fmt"
	"math/bits"
	"os"
	"sort"
	"strings"

	"github.com/dylhunn/dragontoothmg"
)

// Polyglot opening book support: the Zobrist key defined by the Polyglot ".bin"
// format (which differs from dragontoothmg's own hash) and a reader for the
// 16-byte-per-entry book files those keys index.

// PolyglotKey returns the Polyglot Zobrist key for b. See
// http://hgm.nubati.net/book_format.html.
func PolyglotKey(b *dragontoothmg.Board) uint64 {
	var key uint64

	for pt := dragontoothmg.Pawn; pt <= dragontoothmg.King; pt++ {
		white := pieceBoard(&b.White, pt)
		black := pieceBoard(&b.Black, pt)
		// Polyglot "kind_of_piece": black pawn 0, white pawn 1, black knight 2, …
		wKind := 2*(pt-1) + 1
		bKind := 2 * (pt - 1)
		for x := white; x != 0; x &= x - 1 {
			key ^= polyglotRandom[64*wKind+bits.TrailingZeros64(x)]
		}
		for x := black; x != 0; x &= x - 1 {
			key ^= polyglotRandom[64*bKind+bits.TrailingZeros64(x)]
		}
	}

	f := strings.Fields(b.ToFen())
	for _, c := range f[2] {
		switch c {
		case 'K':
			key ^= polyglotRandom[768]
		case 'Q':
			key ^= polyglotRandom[769]
		case 'k':
			key ^= polyglotRandom[770]
		case 'q':
			key ^= polyglotRandom[771]
		}
	}

	// En passant only counts when a pawn of the side to move can actually make
	// the capture — matching Polyglot, not FEN.
	if f[3] != "-" {
		if ts, err := dragontoothmg.AlgebraicToIndex(f[3]); err == nil {
			file := int(ts) % 8
			var capSq int
			var ourPawns uint64
			if b.Wtomove {
				capSq, ourPawns = int(ts)-8, b.White.Pawns
			} else {
				capSq, ourPawns = int(ts)+8, b.Black.Pawns
			}
			canCapture := (file > 0 && ourPawns&(1<<uint(capSq-1)) != 0) ||
				(file < 7 && ourPawns&(1<<uint(capSq+1)) != 0)
			if canCapture {
				key ^= polyglotRandom[772+file]
			}
		}
	}

	if b.Wtomove {
		key ^= polyglotRandom[780]
	}
	return key
}

type bookEntry struct {
	key    uint64
	move   uint16
	weight uint16
}

// Book is a parsed Polyglot opening book, sorted by key.
type Book struct {
	entries []bookEntry
}

// Len reports the number of entries in the book.
func (bk *Book) Len() int { return len(bk.entries) }

// OpenBook reads and parses a Polyglot ".bin" file.
func OpenBook(path string) (*Book, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data)%16 != 0 {
		return nil, fmt.Errorf("polyglot: %s: size %d is not a positive multiple of 16", path, len(data))
	}
	entries := make([]bookEntry, len(data)/16)
	for i := range entries {
		off := i * 16
		entries[i] = bookEntry{
			key:    binary.BigEndian.Uint64(data[off:]),
			move:   binary.BigEndian.Uint16(data[off+8:]),
			weight: binary.BigEndian.Uint16(data[off+10:]),
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })
	return &Book{entries: entries}, nil
}

// Probe returns the highest-weighted book move for b, or ok=false when the
// position is not in the book (or its stored move is not legal here).
func (bk *Book) Probe(b *dragontoothmg.Board) (dragontoothmg.Move, bool) {
	key := PolyglotKey(b)
	i := sort.Search(len(bk.entries), func(i int) bool { return bk.entries[i].key >= key })

	best := -1
	for ; i < len(bk.entries) && bk.entries[i].key == key; i++ {
		if best < 0 || bk.entries[i].weight > bk.entries[best].weight {
			best = i
		}
	}
	if best < 0 {
		return 0, false
	}
	return decodePolyglotMove(bk.entries[best].move, b)
}

// decodePolyglotMove converts a Polyglot-encoded move to a dragontoothmg move,
// translating the "king onto its own rook" castling encoding, and only returns
// ok when the result is legal in b.
func decodePolyglotMove(pm uint16, b *dragontoothmg.Board) (dragontoothmg.Move, bool) {
	from := int((pm>>9)&7)*8 + int((pm>>6)&7)
	to := int((pm>>3)&7)*8 + int(pm&7)

	var mv dragontoothmg.Move
	mv.Setfrom(dragontoothmg.Square(from)) //nolint:gosec // 0..63
	mv.Setto(dragontoothmg.Square(to))     //nolint:gosec // 0..63
	switch (pm >> 12) & 7 {
	case 1:
		mv.Setpromote(dragontoothmg.Knight)
	case 2:
		mv.Setpromote(dragontoothmg.Bishop)
	case 3:
		mv.Setpromote(dragontoothmg.Rook)
	case 4:
		mv.Setpromote(dragontoothmg.Queen)
	}

	if (b.White.Kings|b.Black.Kings)&(1<<uint(from)) != 0 {
		switch {
		case from == 4 && to == 7:
			mv.Setto(6)
		case from == 4 && to == 0:
			mv.Setto(2)
		case from == 60 && to == 63:
			mv.Setto(62)
		case from == 60 && to == 56:
			mv.Setto(58)
		}
	}

	for _, lm := range b.GenerateLegalMoves() {
		if lm == mv {
			return mv, true
		}
	}
	return 0, false
}
