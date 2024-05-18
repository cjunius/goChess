package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/notnil/chess"
)

func main() {
    game := chess.NewGame()
	for i := range 6 {
		start := time.Now()
    	nodes := perft(i, game.Position())
		end := time.Now()
		diff := end.Sub(start)
		fmt.Println("Depth " + strconv.Itoa(i) + " nodes " + strconv.Itoa(nodes) + " time " + diff.String())
	}
}

func perft(depth int, position *chess.Position) (int) {
	if depth == 1 {
		return len(position.ValidMoves())
	} else if depth > 1{
		nodes := 0
		for _, move := range position.ValidMoves() {
			newPos := position.Update(move)
            nodes += perft(depth - 1, newPos)
        }
		return nodes
	} else {
		return 1
	}
}