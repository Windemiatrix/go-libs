package template

import (
	"fmt"
	"math/big"
	"strings"
)

// The alphabet and the length of the shortuuid package of Python, which
// uuid_to_shortuuid calls (utils.py:543-545); 22 is ceil(log(2**128, 57)).
const (
	shortUUIDAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	shortUUIDLen      = 22
)

// The ValueError of UUID, which upstream does not catch.
var errNotUUID = fmt.Errorf("%w: shortuuid of a uuid that is not a UUID", ErrRender)

// shortUUID is shortuuid.encode(UUID(uuid)) (utils.py:545).
func shortUUID(uuid string) (string, error) {
	hex := strings.ReplaceAll(strings.ReplaceAll(uuid, "urn:", ""), "uuid:", "")
	hex = strings.ReplaceAll(strings.Trim(hex, "{}"), "-", "")
	if len(hex) != 32 { // before the parse, as UUID: the uuid of a Photo has no bound, and math/big reads it in more than linear time
		return "", errNotUUID
	}
	n, ok := new(big.Int).SetString(hex, 16)
	if !ok {
		return "", errNotUUID
	}
	base, digit := big.NewInt(int64(len(shortUUIDAlphabet))), new(big.Int)
	out := []byte(strings.Repeat(shortUUIDAlphabet[:1], shortUUIDLen))
	for i := len(out) - 1; n.Sign() > 0; i-- {
		n.DivMod(n, base, digit)
		out[i] = shortUUIDAlphabet[digit.Int64()]
	}
	return string(out), nil
}
