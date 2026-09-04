package random

import (
	"crypto/rand"
	"github.com/google/uuid"
	"math/big"
	"strings"
)

func GetUUID() string {
	code := uuid.New().String()
	code = strings.Replace(code, "-", "", -1)
	return code
}

const keyChars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const keyNumbers = "0123456789"

// randString returns a cryptographically secure random string of length n
// drawn from chars. It fails closed by returning an empty string on error.
func randString(n int, chars string) string {
	result := make([]byte, n)
	max := big.NewInt(int64(len(chars)))
	for i := range result {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return ""
		}
		result[i] = chars[idx.Int64()]
	}
	return string(result)
}

func GenerateKey() string {
	prefix := randString(16, keyChars)
	uuid_ := GetUUID()
	key := make([]byte, 48)
	copy(key[:16], prefix)
	for i := 0; i < 32; i++ {
		c := uuid_[i]
		if i%2 == 0 && c >= 'a' && c <= 'z' {
			c = c - 'a' + 'A'
		}
		key[i+16] = c
	}
	return string(key)
}

func GetRandomString(length int) string {
	return randString(length, keyChars)
}

func GetRandomNumberString(length int) string {
	return randString(length, keyNumbers)
}

// RandRange returns a random number between min and max (max is not included)
func RandRange(min, max int) int {
	if max <= min {
		return min
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min)))
	if err != nil {
		return min
	}
	return min + int(n.Int64())
}
