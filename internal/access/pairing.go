package access

import (
	"crypto/rand"
	"math/big"
)

const pairingCodeLength = 6
const pairingCodeChars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no I/O/0/1 to avoid confusion

// GeneratePairingCode returns a random alphanumeric code for sender pairing.
func GeneratePairingCode() string {
	code := make([]byte, pairingCodeLength)
	for i := range code {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(pairingCodeChars))))
		if err != nil {
			// Fallback — should never happen.
			code[i] = pairingCodeChars[i%len(pairingCodeChars)]
			continue
		}
		code[i] = pairingCodeChars[n.Int64()]
	}
	return string(code)
}
