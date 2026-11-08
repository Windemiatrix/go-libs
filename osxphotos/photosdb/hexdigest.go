package photosdb

import (
	"encoding/hex"

	"golang.org/x/crypto/blake2b"
)

// hexdigest is utils.hexdigest (utils.py:536-540): blake2b with a 20-byte digest.
func hexdigest(s string) string {
	h, _ := blake2b.New(20, nil) // the error is only for a bad size or key
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
