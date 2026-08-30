package engine

import (
	"time"

	"github.com/dylhunn/dragontoothmg"
)

// Perft counts the number of leaf nodes in the move tree to the given depth.
// It is the canonical correctness test for a move generator: the counts for
// well-known positions are published and must match exactly.
func Perft(b *dragontoothmg.Board, depth int) int64 {
	return dragontoothmg.Perft(b, depth)
}

// PerftRow is a single timed perft result, used by the CLI.
type PerftRow struct {
	Depth   int
	Nodes   int64
	Elapsed time.Duration
}

// PerftSeries runs perft for every depth in [1, depth] and returns timed rows.
func PerftSeries(b *dragontoothmg.Board, depth int) []PerftRow {
	rows := make([]PerftRow, 0, depth)
	for d := 1; d <= depth; d++ {
		start := time.Now()
		nodes := dragontoothmg.Perft(b, d)
		rows = append(rows, PerftRow{Depth: d, Nodes: nodes, Elapsed: time.Since(start)})
	}
	return rows
}

// PerftDivide returns the per-move node counts one ply below the root. It is the
// standard tool for bisecting a move-generation discrepancy against a reference.
func PerftDivide(b *dragontoothmg.Board, depth int) map[string]int64 {
	out := make(map[string]int64)
	for _, m := range b.GenerateLegalMoves() {
		unapply := b.Apply(m)
		out[m.String()] = dragontoothmg.Perft(b, depth-1)
		unapply()
	}
	return out
}
