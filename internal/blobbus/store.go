package blobbus

import (
	"context"
	"crypto/md5" // #nosec G501 -- BlobBus spec requires MD5 ETag for interoperability.
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const fsSafetyMargin int64 = 4 * 1024

// BlobMeta contains immutable metadata for a stored blob.
type BlobMeta struct {
	ID          string
	Size        int64
	ContentType string
	ETag        string
	CreatedAt   time.Time
}

// Store owns filesystem paths and in-memory state.
type Store struct {
	cfg       Config
	dataDir   string
	tmpDir    string
	rand      io.Reader
	now       func() time.Time
	freeBytes func(string) (int64, error)

	uploadGate sync.Mutex

	mu        sync.RWMutex
	fifo      []string
	metadata  map[string]BlobMeta
	usedBytes int64
}

func NewStore(cfg Config) (*Store, error) {
	s := &Store{
		cfg:       cfg,
		dataDir:   cfg.DataDir,
		tmpDir:    filepath.Join(cfg.DataDir, ".tmp"),
		rand:      crand.Reader,
		now:       time.Now,
		freeBytes: freeBytesOnFS,
		fifo:      make([]string, 0),
		metadata:  make(map[string]BlobMeta),
	}

	if err := s.prepareDirs(); err != nil {
		return nil, err
	}

	// Startup cleanup is best-effort.
	_ = s.cleanTmpPartFiles()

	return s, nil
}

func (s *Store) prepareDirs() error {
	if err := os.MkdirAll(s.dataDir, 0o700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(s.tmpDir, 0o700); err != nil {
		return fmt.Errorf("create tmp dir: %w", err)
	}
	return nil
}

func (s *Store) cleanTmpPartFiles() error {
	entries, err := os.ReadDir(s.tmpDir)
	if err != nil {
		return nil
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".part") {
			continue
		}
		_ = os.Remove(filepath.Join(s.tmpDir, e.Name()))
	}
	return nil
}

func (s *Store) blobPath(id string) string {
	return filepath.Join(s.dataDir, id)
}

func (s *Store) Upload(ctx context.Context, body io.Reader, size int64, contentType string) (BlobMeta, error) {
	if size < 0 {
		return BlobMeta{}, ErrLengthRequired
	}
	if size > s.cfg.CapBytes {
		return BlobMeta{}, ErrTooLarge
	}

	s.uploadGate.Lock()
	defer s.uploadGate.Unlock()

	id, err := s.newUniqueID()
	if err != nil {
		return BlobMeta{}, fmt.Errorf("generate id: %w", err)
	}

	if err := s.ensureCapacity(size); err != nil {
		return BlobMeta{}, err
	}

	suffix, err := randomHex(s.rand, 4)
	if err != nil {
		return BlobMeta{}, fmt.Errorf("generate temp suffix: %w", err)
	}

	tmpPath := filepath.Join(s.tmpDir, fmt.Sprintf("%s.%s.part", id, suffix))
	finalPath := s.blobPath(id)

	etag, err := s.writeTempBlob(ctx, tmpPath, body, size)
	if err != nil {
		_ = os.Remove(tmpPath)
		return BlobMeta{}, err
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return BlobMeta{}, fmt.Errorf("commit blob: %w", err)
	}

	meta := BlobMeta{
		ID:          id,
		Size:        size,
		ContentType: contentType,
		ETag:        etag,
		CreatedAt:   s.now().UTC(),
	}

	s.mu.Lock()
	s.metadata[id] = meta
	s.fifo = append(s.fifo, id)
	s.usedBytes += size
	s.mu.Unlock()

	return meta, nil
}

func (s *Store) Open(id string) (BlobMeta, *os.File, error) {
	s.mu.RLock()
	meta, ok := s.metadata[id]
	s.mu.RUnlock()
	if !ok {
		return BlobMeta{}, nil, ErrNotFound
	}

	f, err := os.Open(s.blobPath(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return BlobMeta{}, nil, ErrNotFound
		}
		return BlobMeta{}, nil, fmt.Errorf("open blob: %w", err)
	}

	return meta, f, nil
}

func (s *Store) Stat(id string) (BlobMeta, error) {
	s.mu.RLock()
	meta, ok := s.metadata[id]
	s.mu.RUnlock()
	if !ok {
		return BlobMeta{}, ErrNotFound
	}

	if _, err := os.Stat(s.blobPath(id)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return BlobMeta{}, ErrNotFound
		}
		return BlobMeta{}, fmt.Errorf("stat blob: %w", err)
	}
	return meta, nil
}

func (s *Store) ensureCapacity(incomingSize int64) error {
	if incomingSize > s.cfg.CapBytes {
		return ErrTooLarge
	}

	for {
		free, err := s.freeBytes(s.dataDir)
		if err != nil {
			return fmt.Errorf("check free space: %w", err)
		}

		s.mu.Lock()
		logicalEnough := s.usedBytes+incomingSize <= s.cfg.CapBytes
		physicalEnough := free >= incomingSize+fsSafetyMargin
		if logicalEnough && physicalEnough {
			s.mu.Unlock()
			return nil
		}

		if len(s.fifo) == 0 {
			s.mu.Unlock()
			return ErrTooLarge
		}

		oldestID := s.fifo[0]
		s.fifo = s.fifo[1:]
		oldestMeta := s.metadata[oldestID]
		delete(s.metadata, oldestID)
		s.usedBytes -= oldestMeta.Size
		if s.usedBytes < 0 {
			s.usedBytes = 0
		}
		s.mu.Unlock()

		err = os.Remove(s.blobPath(oldestID))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("evict %s: %w", oldestID, err)
		}
	}
}

func (s *Store) writeTempBlob(ctx context.Context, path string, body io.Reader, size int64) (string, error) {
	// #nosec G304 -- path is generated internally under s.tmpDir, never from user input.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("open temp file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	// #nosec G401 -- BlobBus API requires MD5 digest as ETag.
	sum := md5.New()
	buf := make([]byte, 64*1024)
	remaining := size

	for remaining > 0 {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		toRead := int64(len(buf))
		if remaining < toRead {
			toRead = remaining
		}

		n, readErr := io.ReadFull(body, buf[:toRead])
		if readErr != nil {
			return "", fmt.Errorf("read body: %w", readErr)
		}

		chunk := buf[:n]
		for len(chunk) > 0 {
			written, writeErr := file.Write(chunk)
			if written > 0 {
				_, _ = sum.Write(chunk[:written])
				remaining -= int64(written)
				chunk = chunk[written:]
			}

			if writeErr != nil {
				if errors.Is(writeErr, syscall.ENOSPC) {
					if err := s.ensureCapacity(remaining); err != nil {
						return "", err
					}
					continue
				}
				return "", fmt.Errorf("write temp file: %w", writeErr)
			}
		}
	}

	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func (s *Store) newUniqueID() (string, error) {
	for attempts := 0; attempts < 128; attempts++ {
		id, err := generateID(s.rand)
		if err != nil {
			return "", err
		}

		s.mu.RLock()
		_, exists := s.metadata[id]
		s.mu.RUnlock()
		if exists {
			continue
		}

		_, err = os.Stat(s.blobPath(id))
		if err == nil {
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		return id, nil
	}
	return "", errors.New("too many id collisions")
}

func freeBytesOnFS(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}

	blocks := uint64(stat.Bavail)
	blockSize := uint64(stat.Bsize)
	if blocks == 0 || blockSize == 0 {
		return 0, nil
	}
	if blocks > math.MaxInt64/blockSize {
		return math.MaxInt64, nil
	}

	return int64(blocks * blockSize), nil
}
