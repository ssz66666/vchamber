package server

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

func assertCryptoPRNG() {
	buf := make([]byte, 1)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		panic(fmt.Sprintf("crypto/rand failed with %v, aborting...", err))
	}
}

// GenerateKey returns a cryptographically secure URL safe random string of length n.
func GenerateKey(n int) (string, error) {
	buf := make([]byte, n)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(buf), nil

}
