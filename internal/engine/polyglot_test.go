package engine

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/dylhunn/dragontoothmg"
)

func TestPolyglotKeyMatchesCanonicalVectors(t *testing.T) {
	// The reference keys published with the Polyglot book format.
	tests := []struct {
		moves string
		want  uint64
	}{
		{"", 0x463b96181691fc9c},
		{"e2e4", 0x823c9b50fd114196},
		{"e2e4 d7d5", 0x0756b94461c50fb0},
		{"e2e4 d7d5 e4e5", 0x662fafb965db29d4},
		{"e2e4 d7d5 e4e5 f7f5", 0x22a48b5a8e47ff78},
		{"e2e4 d7d5 e4e5 f7f5 e1e2", 0x652a607ca3f242c1},
		{"e2e4 d7d5 e4e5 f7f5 e1e2 e8f7", 0x00fdd303c946bdd9},
	}
	for _, tt := range tests {
		b := dragontoothmg.ParseFen(dragontoothmg.Startpos)
		if tt.moves != "" {
			for _, tok := range splitFields(tt.moves) {
				m, err := dragontoothmg.ParseMove(tok)
				if err != nil {
					t.Fatalf("%q: bad move %q", tt.moves, tok)
				}
				b.Apply(m)
			}
		}
		if got := PolyglotKey(&b); got != tt.want {
			t.Errorf("after %q: key = %#016x, want %#016x", tt.moves, got, tt.want)
		}
	}
}

func TestOpenBookAndProbe(t *testing.T) {
	start := dragontoothmg.ParseFen(dragontoothmg.Startpos)
	startKey := PolyglotKey(&start)

	// Polyglot move: to_file | to_row<<3 | from_file<<6 | from_row<<9.
	// e2e4: from (file 4, row 1) to (file 4, row 3).
	e2e4 := uint16(4 | 3<<3 | 4<<6 | 1<<9)
	d2d4 := uint16(3 | 3<<3 | 3<<6 | 1<<9)

	path := filepath.Join(t.TempDir(), "test.bin")
	writeBook(t, path, []struct {
		key          uint64
		move, weight uint16
	}{
		{startKey, d2d4, 10},
		{startKey, e2e4, 40}, // higher weight wins
	})

	bk, err := OpenBook(path)
	if err != nil {
		t.Fatalf("OpenBook: %v", err)
	}
	if bk.Len() != 2 {
		t.Fatalf("Len = %d, want 2", bk.Len())
	}
	mv, ok := bk.Probe(&start)
	if !ok {
		t.Fatal("Probe missed the start position")
	}
	if mv.String() != "e2e4" {
		t.Errorf("Probe returned %s, want e2e4 (the higher weight)", mv.String())
	}

	// A position not in the book misses.
	other := dragontoothmg.ParseFen("rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR b KQkq - 0 1")
	if _, ok := bk.Probe(&other); ok {
		t.Error("Probe hit a position that is not in the book")
	}
}

func TestOpenBookRejectsBadSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.bin")
	if err := os.WriteFile(path, []byte("not a multiple of sixteen!"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenBook(path); err == nil {
		t.Error("OpenBook accepted a malformed file")
	}
}

func writeBook(t *testing.T, path string, entries []struct {
	key          uint64
	move, weight uint16
},
) {
	t.Helper()
	buf := make([]byte, 0, len(entries)*16)
	for _, e := range entries {
		var rec [16]byte
		binary.BigEndian.PutUint64(rec[0:], e.key)
		binary.BigEndian.PutUint16(rec[8:], e.move)
		binary.BigEndian.PutUint16(rec[10:], e.weight)
		buf = append(buf, rec[:]...)
	}
	if err := os.WriteFile(path, buf, 0o600); err != nil {
		t.Fatal(err)
	}
}

func splitFields(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ' ' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
