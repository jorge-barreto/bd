package idgen

import (
	"crypto/sha256"
	"fmt"
	"math"
	"math/big"
)

const base36 = "abcdefghijklmnopqrstuvwxyz0123456789"

// bytesNeeded returns how many SHA-256 bytes are needed for a given base36 length.
func bytesNeeded(length int) int {
	switch {
	case length <= 3:
		return 2
	case length <= 4:
		return 3
	case length <= 6:
		return 4
	case length <= 8:
		return 5
	default:
		return 6
	}
}

// GenerateHashID produces a deterministic base36 hash ID from the given inputs.
// The same inputs always produce the same output; varying the nonce produces
// a different hash.
func GenerateHashID(prefix, title, desc, creator, createdAt string, length, nonce int) string {
	input := fmt.Sprintf("%s|%s|%s|%s|%d", title, desc, creator, createdAt, nonce)
	sum := sha256.Sum256([]byte(input))

	n := bytesNeeded(length)
	num := new(big.Int).SetBytes(sum[:n])

	b36 := big.NewInt(36)
	result := make([]byte, length)
	for i := length - 1; i >= 0; i-- {
		mod := new(big.Int)
		num.DivMod(num, b36, mod)
		result[i] = base36[mod.Int64()]
	}

	return prefix + "-" + string(result)
}

// ComputeAdaptiveLength returns the minimum base36 ID length such that the
// birthday-paradox collision probability stays below maxProb, given the
// number of existing IDs.
func ComputeAdaptiveLength(existingCount, minLen, maxLen int, maxProb float64) int {
	n := float64(existingCount)
	for length := minLen; length <= maxLen; length++ {
		space := math.Pow(36, float64(length))
		prob := 1.0 - math.Exp(-(n*n)/(2.0*space))
		if prob < maxProb {
			return length
		}
	}
	return maxLen
}
