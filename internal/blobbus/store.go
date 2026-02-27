package blobbus

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Store owns filesystem paths and in-memory state.
type Store struct {
	cfg     Config
	dataDir string
	tmpDir  string
}

func NewStore(cfg Config) (*Store, error) {
	s := &Store{
		cfg:     cfg,
		dataDir: cfg.DataDir,
		tmpDir:  filepath.Join(cfg.DataDir, ".tmp"),
	}

	if err := s.prepareDirs(); err != nil {
		return nil, err
	}

	if err := s.cleanTmpPartFiles(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Store) prepareDirs() error {
	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(s.tmpDir, 0o755); err != nil {
		return fmt.Errorf("create tmp dir: %w", err)
	}
	return nil
}

func (s *Store) cleanTmpPartFiles() error {
	entries, err := os.ReadDir(s.tmpDir)
	if err != nil {
		return fmt.Errorf("read tmp dir: %w", err)
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
