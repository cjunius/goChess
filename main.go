package main

import (
	"fmt"
	"math/rand"

	"github.com/notnil/chess"
)

func main() {
    game := chess.NewGame()
    for game.Outcome() == chess.NoOutcome {
        moves := game.ValidMoves()
        move := moves[rand.Intn(len(moves))]
        game.Move(move)
    }

    fmt.Println(game.Position().Board().Draw())
    fmt.Printf("Game completed. %s by %s.\n", game.Outcome(), game.Method())
    fmt.Println(game.String())
}