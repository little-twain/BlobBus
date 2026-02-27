package blobbus

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestGenerateIDLengthAndEncoding(t *testing.T) {
	raw := bytes.Repeat([]byte{0xAB}, 16)
	id, err := generateID(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("generateID() error = %v", err)
	}

	if len(id) != 22 {
		t.Fatalf("id length = %d, want 22", len(id))
	}
	if !isValidID(id) {
		t.Fatalf("id %q is not valid", id)
	}

	decoded, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		t.Fatalf("decode id: %v", err)
	}
	if len(decoded) != 16 {
		t.Fatalf("decoded length = %d, want 16", len(decoded))
	}
}

func TestIsValidID(t *testing.T) {
	if !isValidID("Zl2uV7Jr2XhQXo3b8zZq5A") {
		t.Fatal("expected id to be valid")
	}
	if isValidID("short") {
		t.Fatal("expected short id to be invalid")
	}
	if isValidID("this+contains+plus+chars") {
		t.Fatal("expected invalid charset")
	}
}
