package blobbus

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestNewUniqueIDRegeneratesOnCollision(t *testing.T) {
	store, err := NewStore(Config{CapBytes: 1024, DataDir: t.TempDir(), ListenAddr: ":0"})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	store.freeBytes = func(string) (int64, error) { return 1 << 30, nil }

	firstRaw := bytes.Repeat([]byte{0x11}, 16)
	secondRaw := bytes.Repeat([]byte{0x22}, 16)
	store.rand = bytes.NewReader(append(firstRaw, secondRaw...))

	firstID := base64.RawURLEncoding.EncodeToString(firstRaw)
	store.metadata[firstID] = BlobMeta{ID: firstID}

	id, err := store.newUniqueID()
	if err != nil {
		t.Fatalf("newUniqueID() error = %v", err)
	}
	if id == firstID {
		t.Fatalf("id collision not retried")
	}
}
