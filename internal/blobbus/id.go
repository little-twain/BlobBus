package blobbus

import (
	"encoding/base64"
	"encoding/hex"
	"io"
	"regexp"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)

func generateID(randReader io.Reader) (string, error) {
	raw := make([]byte, 16)
	if _, err := io.ReadFull(randReader, raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func randomHex(randReader io.Reader, n int) (string, error) {
	raw := make([]byte, n)
	if _, err := io.ReadFull(randReader, raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func isValidID(id string) bool {
	return idPattern.MatchString(id)
}
