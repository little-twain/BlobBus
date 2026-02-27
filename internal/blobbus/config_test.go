package blobbus

import (
	"os"
	"testing"
)

func TestLoadConfigFromEnv(t *testing.T) {
	t.Setenv("CAP_BYTES", "123")
	t.Setenv("DATA_DIR", "/tmp/data")
	t.Setenv("LISTEN_ADDR", ":9999")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv() error = %v", err)
	}

	if cfg.CapBytes != 123 {
		t.Fatalf("CapBytes = %d, want 123", cfg.CapBytes)
	}
	if cfg.DataDir != "/tmp/data" {
		t.Fatalf("DataDir = %q, want /tmp/data", cfg.DataDir)
	}
	if cfg.ListenAddr != ":9999" {
		t.Fatalf("ListenAddr = %q, want :9999", cfg.ListenAddr)
	}
}

func TestLoadConfigFromEnvMissingCap(t *testing.T) {
	_ = os.Unsetenv("CAP_BYTES")
	if _, err := LoadConfigFromEnv(); err == nil {
		t.Fatal("expected error for missing CAP_BYTES")
	}
}
