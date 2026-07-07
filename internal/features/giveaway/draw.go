package giveaway

import (
	"math/rand/v2"

	"github.com/dia-bot/dia/internal/store"
)

// drawWinners picks up to winnerCount distinct winners from the entries,
// weighted by each entrant's ticket count, excluding any ids in exclude
// (previous winners on a reroll, and the host when ExcludeHost is set). It draws
// without replacement, so a member wins at most once, and returns fewer than
// winnerCount when the eligible pool is smaller (the "not enough entrants" case).
func drawWinners(entries []store.GiveawayEntry, winnerCount int, exclude map[int64]bool) []int64 {
	type cand struct {
		id     int64
		weight int
	}
	cands := make([]cand, 0, len(entries))
	for _, e := range entries {
		if exclude[e.UserID] {
			continue
		}
		w := e.Entries
		if w < 1 {
			w = 1
		}
		cands = append(cands, cand{id: e.UserID, weight: w})
	}

	// Cap the initial capacity at the pool size: a winner count can never exceed
	// the number of candidates, and a wildly large requested count must not drive
	// a huge allocation.
	winners := make([]int64, 0, min(winnerCount, len(cands)))
	for len(winners) < winnerCount && len(cands) > 0 {
		total := 0
		for _, c := range cands {
			total += c.weight
		}
		r := rand.IntN(total) // [0, total)
		idx := len(cands) - 1
		for i, c := range cands {
			if r < c.weight {
				idx = i
				break
			}
			r -= c.weight
		}
		winners = append(winners, cands[idx].id)
		// Remove the chosen candidate (order-preserving not required).
		cands[idx] = cands[len(cands)-1]
		cands = cands[:len(cands)-1]
	}
	return winners
}
