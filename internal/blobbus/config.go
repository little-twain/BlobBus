package blobbus

import (
	"fmt"
	"os"
	"strconv"
)

const (
	defaultDataDir    = "/var/lib/blobbus"
	defaultListenAddr = ":8080"
)

// Config defines process configuration loaded from environment variables.
type Config struct {
	CapBytes   int64
	DataDir    string
	ListenAddr string
}

func LoadConfigFromEnv() (Config, error) {
	capRaw := os.Getenv("CAP_BYTES")
	if capRaw == "" {
		return Config{}, fmt.Errorf("CAP_BYTES is required")
	}

	capBytes, err := strconv.ParseInt(capRaw, 10, 64)
	if err != nil || capBytes <= 0 {
		return Config{}, fmt.Errorf("invalid CAP_BYTES %q", capRaw)
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = defaultDataDir
	}

	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = defaultListenAddr
	}

	return Config{
		CapBytes:   capBytes,
		DataDir:    dataDir,
		ListenAddr: listenAddr,
	}, nil
}
