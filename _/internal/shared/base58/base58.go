package base58

import (
	"math/big"

	"github.com/jackc/pgx/v5/pgtype"
)

// Base58 alphabet (Bitcoin-style, no 0, O, I, l to avoid confusion)
const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

var base = big.NewInt(58)

// Encode encodes a byte slice to base58 string
func Encode(input []byte) string {
	if len(input) == 0 {
		return ""
	}

	// Convert bytes to big integer
	num := new(big.Int).SetBytes(input)

	// Handle zero
	if num.Cmp(big.NewInt(0)) == 0 {
		return string(alphabet[0])
	}

	// Convert to base58
	var encoded []byte
	zero := big.NewInt(0)
	mod := new(big.Int)

	for num.Cmp(zero) > 0 {
		num.DivMod(num, base, mod)
		encoded = append(encoded, alphabet[mod.Int64()])
	}

	// Add leading zeros (represented as '1' in base58)
	for _, b := range input {
		if b != 0 {
			break
		}
		encoded = append(encoded, alphabet[0])
	}

	// Reverse the result
	for i, j := 0, len(encoded)-1; i < j; i, j = i+1, j-1 {
		encoded[i], encoded[j] = encoded[j], encoded[i]
	}

	return string(encoded)
}

// EncodeUUID encodes a pgtype.UUID to base58 string
func EncodeUUID(uuid pgtype.UUID) string {
	if !uuid.Valid {
		return ""
	}
	return Encode(uuid.Bytes[:])
}
