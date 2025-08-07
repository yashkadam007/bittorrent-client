package pieces

import (
	"crypto/sha1"
)

// VerifyPieceHash verifies that the given data matches the expected hash
func VerifyPieceHash(data []byte, expectedHash [20]byte) bool {
	actualHash := sha1.Sum(data)
	return actualHash == expectedHash
}
