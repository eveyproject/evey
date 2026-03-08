package grpcapi

import (
	crypto_rand "crypto/rand"
	"encoding/hex"
)

func secureToken(prefix string, n int) string {
	b := make([]byte, n)
	crypto_rand.Read(b)
	return prefix + hex.EncodeToString(b)
}
