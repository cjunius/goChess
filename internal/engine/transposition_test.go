package engine

import (
	"sync"
	"testing"

	"github.com/dylhunn/dragontoothmg"
)

func TestTTStoreThenProbeExact(t *testing.T) {
	tt := NewTT(1)
	key := uint64(0xdeadbeefcafef00d)
	move, _ := dragontoothmg.ParseMove("e2e4")

	tt.store(key, 5, 42, boundExact, move, 0)

	score, got, cutoff := tt.probe(key, 5, -100, 100, 0)
	if !cutoff {
		t.Fatalf("expected an exact-bound cut-off")
	}
	if score != 42 {
		t.Errorf("score = %d, want 42", score)
	}
	if got != move {
		t.Errorf("move = %s, want %s", got.String(), move.String())
	}
}

func TestTTShallowEntryDoesNotCutButKeepsMove(t *testing.T) {
	tt := NewTT(1)
	key := uint64(1)
	move, _ := dragontoothmg.ParseMove("d2d4")
	tt.store(key, 3, 10, boundExact, move, 0)

	score, got, cutoff := tt.probe(key, 6, -100, 100, 0)
	if cutoff {
		t.Errorf("a depth-3 entry must not satisfy a depth-6 probe")
	}
	if score != 0 {
		t.Errorf("score = %d, want 0 when no cut-off", score)
	}
	if got != move {
		t.Errorf("move = %s, want the stored %s for ordering", got.String(), move.String())
	}
}

func TestTTBoundsRespectWindow(t *testing.T) {
	tt := NewTT(1)
	key := uint64(7)

	tt.store(key, 4, 50, boundLower, 0, 0)
	if _, _, cutoff := tt.probe(key, 4, 0, 40, 0); !cutoff {
		t.Errorf("lower bound 50 should cut when beta=40")
	}
	if _, _, cutoff := tt.probe(key, 4, 0, 60, 0); cutoff {
		t.Errorf("lower bound 50 should not cut when beta=60")
	}

	tt.store(key, 4, 50, boundUpper, 0, 0)
	if _, _, cutoff := tt.probe(key, 4, 60, 100, 0); !cutoff {
		t.Errorf("upper bound 50 should cut when alpha=60")
	}
	if _, _, cutoff := tt.probe(key, 4, 40, 100, 0); cutoff {
		t.Errorf("upper bound 50 should not cut when alpha=40")
	}
}

func TestTTMateScoreIsRebasedByPly(t *testing.T) {
	tt := NewTT(1)
	key := uint64(99)
	// "mate in 1 from the root" is mateScore-1. Store it as seen from ply 4.
	rootScore := mateScore - 1
	tt.store(key, 10, rootScore, boundExact, 0, 4)

	score, _, cutoff := tt.probe(key, 10, -infinity, infinity, 4)
	if !cutoff {
		t.Fatal("expected a cut-off")
	}
	if score != rootScore {
		t.Errorf("probed mate score = %d, want %d (round-trip through ply rebasing)", score, rootScore)
	}
	if score < mateScore-maxPly {
		t.Errorf("score %d no longer reads as a mate", score)
	}
}

func TestTTMissOnKeyMismatch(t *testing.T) {
	tt := NewTT(1)
	tt.store(1, 5, 10, boundExact, 0, 0)
	if _, _, cutoff := tt.probe(2, 1, -100, 100, 0); cutoff {
		t.Errorf("probe of an unstored key must miss")
	}
}

// TestTTConcurrentAccessIsRaceFree exercises the lockless entries from many
// goroutines at once; run with -race.
func TestTTConcurrentAccessIsRaceFree(t *testing.T) {
	tt := NewTT(1)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 20000; i++ {
				key := uint64((i % 512) + 1)
				tt.store(key, i%64, (i%200)-100, boundExact, dragontoothmg.Move(i&0xffff), 0)
				tt.probe(key, i%64, -100, 100, 0)
			}
		}(g)
	}
	wg.Wait()
}
