// Package diceware provides utilities for generating memorable-but-random
// strings.
package diceware

import (
	"math/rand/v2"
	"strings"
)

// GenWord generates a short string.
func GenWord(r *rand.Rand) string {
	var sb strings.Builder
	for i := range 3 {
		if i > 0 {
			sb.WriteRune('-')
		}
		sb.WriteString(corpus[r.IntN(len(corpus))])
	}
	return sb.String()
}
