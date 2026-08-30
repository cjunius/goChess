// Command gochess is the entry point for the chess engine. With no arguments it
// speaks UCI on stdin/stdout; subcommands expose perft and a fixed benchmark.
//
//	gochess                       # UCI mode (for a GUI or another engine)
//	gochess uci                   # UCI mode, explicit
//	gochess perft <depth> [fen]   # timed perft series from a position
//	gochess bench                 # fixed search benchmark (nodes/sec)
//	gochess version               # build metadata
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dylhunn/dragontoothmg"

	"github.com/cjunius/goChess/internal/engine"
	"github.com/cjunius/goChess/internal/uci"
)

// Overridden at build time via -ldflags (see .goreleaser.yaml / Makefile).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		mustUCI()
		return
	}
	switch args[0] {
	case "uci":
		mustUCI()
	case "perft":
		runPerft(args[1:])
	case "bench":
		runBench()
	case "version", "-v", "--version":
		fmt.Printf("gochess %s (commit %s, built %s)\n", version, commit, date)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "gochess: unknown command %q\n\n%s", args[0], usage)
		os.Exit(2)
	}
}

const usage = `usage: gochess [command]

  (no command)          speak UCI on stdin/stdout
  uci                   speak UCI on stdin/stdout
  perft <depth> [fen]   print a timed perft series (default: start position)
  bench                 run the fixed search benchmark
  version               print build metadata
`

func mustUCI() {
	if err := uci.Run(os.Stdin, os.Stdout, version); err != nil {
		fmt.Fprintln(os.Stderr, "gochess:", err)
		os.Exit(1)
	}
}

func runPerft(args []string) {
	depth := 5
	fen := dragontoothmg.Startpos
	if len(args) > 0 {
		d, err := strconv.Atoi(args[0])
		if err != nil || d < 1 {
			fmt.Fprintf(os.Stderr, "gochess: invalid depth %q\n", args[0])
			os.Exit(2)
		}
		depth = d
	}
	if len(args) > 1 {
		fen = strings.Join(args[1:], " ")
	}

	board := dragontoothmg.ParseFen(fen)
	for _, row := range engine.PerftSeries(&board, depth) {
		nps := float64(row.Nodes) / row.Elapsed.Seconds()
		fmt.Printf("depth %2d  nodes %13d  %10s  %12.0f nps\n",
			row.Depth, row.Nodes, row.Elapsed.Round(time.Millisecond), nps)
	}
}

func runBench() {
	positions := []string{
		dragontoothmg.Startpos,
		"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1",
		"8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1",
		"r4rk1/1pp1qppp/p1np1n2/2b1p1B1/2B1P1b1/P1NP1N2/1PP1QPPP/R4RK1 w - - 0 10",
	}
	var totalNodes int64
	start := time.Now()
	for _, fen := range positions {
		board := dragontoothmg.ParseFen(fen)
		res := engine.Search(&board, engine.SearchParams{MaxDepth: 6})
		totalNodes += res.Nodes
		fmt.Printf("%-74s  bestmove %-6s  score %6d  depth %d\n",
			fen, res.BestMove.String(), res.Score, res.Depth)
	}
	elapsed := time.Since(start)
	fmt.Printf("\nbench: %d nodes in %s (%.0f nps)\n",
		totalNodes, elapsed.Round(time.Millisecond), float64(totalNodes)/elapsed.Seconds())
}
