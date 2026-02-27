package blobbus

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"testing"
	"time"
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

func TestUploadGateSerializesConcurrentUploads(t *testing.T) {
	store, err := NewStore(Config{CapBytes: 1024, DataDir: t.TempDir(), ListenAddr: ":0"})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	store.freeBytes = func(string) (int64, error) { return 1 << 30, nil }

	pipeR, pipeW := io.Pipe()
	firstDone := make(chan error, 1)
	go func() {
		_, uploadErr := store.Upload(context.Background(), pipeR, 6, "application/octet-stream")
		firstDone <- uploadErr
	}()

	if _, err := pipeW.Write([]byte("abc")); err != nil {
		t.Fatalf("pipe write: %v", err)
	}

	secondDone := make(chan error, 1)
	go func() {
		_, uploadErr := store.Upload(context.Background(), bytes.NewReader([]byte("z")), 1, "application/octet-stream")
		secondDone <- uploadErr
	}()

	select {
	case err := <-secondDone:
		t.Fatalf("second upload finished early with err=%v", err)
	case <-time.After(120 * time.Millisecond):
		// Expected: blocked by upload gate.
	}

	if _, err := pipeW.Write([]byte("def")); err != nil {
		t.Fatalf("pipe write: %v", err)
	}
	_ = pipeW.Close()

	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first upload error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first upload timed out")
	}

	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("second upload error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("second upload remained blocked")
	}
}

func TestReaderWithOpenFDSurvivesEviction(t *testing.T) {
	store, err := NewStore(Config{CapBytes: 8, DataDir: t.TempDir(), ListenAddr: ":0"})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	store.freeBytes = func(string) (int64, error) { return 1 << 30, nil }

	a, err := store.Upload(context.Background(), bytes.NewReader([]byte("aaaaa")), 5, "application/octet-stream")
	if err != nil {
		t.Fatalf("upload A: %v", err)
	}

	_, file, err := store.Open(a.ID)
	if err != nil {
		t.Fatalf("open A: %v", err)
	}
	defer file.Close()

	_, err = store.Upload(context.Background(), bytes.NewReader([]byte("bbbbb")), 5, "application/octet-stream")
	if err != nil {
		t.Fatalf("upload B: %v", err)
	}

	if _, err := store.Stat(a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("A should be evicted, got err=%v", err)
	}

	got, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("read open fd after eviction: %v", err)
	}
	if string(got) != "aaaaa" {
		t.Fatalf("open fd data mismatch: got %q", string(got))
	}
}

func TestUploadRejectsWhenPhysicalFreeSpaceCannotFit(t *testing.T) {
	store, err := NewStore(Config{CapBytes: 100, DataDir: t.TempDir(), ListenAddr: ":0"})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	store.freeBytes = func(string) (int64, error) { return 0, nil }

	_, err = store.Upload(context.Background(), bytes.NewReader([]byte("a")), 1, "application/octet-stream")
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Upload() err = %v, want ErrTooLarge", err)
	}
}

func TestStartupCleansTmpPartFilesBestEffort(t *testing.T) {
	dataDir := t.TempDir()
	tmpDir := dataDir + string(os.PathSeparator) + ".tmp"
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		t.Fatalf("mkdir tmp dir: %v", err)
	}
	partPath := tmpDir + string(os.PathSeparator) + "stale.part"
	if err := os.WriteFile(partPath, []byte("partial"), 0o600); err != nil {
		t.Fatalf("write stale part: %v", err)
	}
	if err := os.WriteFile(tmpDir+string(os.PathSeparator)+"keep.txt", []byte("x"), 0o600); err != nil {
		t.Fatalf("write keep file: %v", err)
	}

	if _, err := NewStore(Config{CapBytes: 10, DataDir: dataDir, ListenAddr: ":0"}); err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	if _, err := os.Stat(partPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale part should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(tmpDir + string(os.PathSeparator) + "keep.txt"); err != nil {
		t.Fatalf("non-part file should remain, err=%v", err)
	}
}
