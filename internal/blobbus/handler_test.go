package blobbus

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz(t *testing.T) {
	cfg := Config{CapBytes: 1, DataDir: t.TempDir(), ListenAddr: ":0"}
	store, err := NewStore(cfg)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	h := NewHandler(store)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}
