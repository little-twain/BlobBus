package blobbus

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type testUploadResponse struct {
	ID        string `json:"id"`
	Size      int64  `json:"size"`
	ETag      string `json:"etag"`
	CreatedAt string `json:"created_at"`
}

func newTestStoreAndHandler(t *testing.T, capBytes int64) (*Store, http.Handler) {
	t.Helper()

	store, err := NewStore(Config{CapBytes: capBytes, DataDir: t.TempDir(), ListenAddr: ":0"})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	store.freeBytes = func(string) (int64, error) { return 1 << 30, nil }

	return store, NewHandler(store)
}

func uploadBlob(t *testing.T, h http.Handler, body []byte, contentType string) testUploadResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/v1/blobs", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, body = %s", rr.Code, rr.Body.String())
	}

	var out testUploadResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal upload response: %v", err)
	}
	return out
}

func TestHealthz(t *testing.T) {
	_, h := newTestStoreAndHandler(t, 1024)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestUploadGetHeadRoundTrip(t *testing.T) {
	_, h := newTestStoreAndHandler(t, 1024)
	payload := []byte("hello-blobbus")

	uploaded := uploadBlob(t, h, payload, "text/plain")
	if len(uploaded.ID) != 22 {
		t.Fatalf("id length = %d, want 22", len(uploaded.ID))
	}
	if uploaded.Size != int64(len(payload)) {
		t.Fatalf("size = %d, want %d", uploaded.Size, len(payload))
	}
	if uploaded.ETag == "" {
		t.Fatal("etag must not be empty")
	}

	getRR := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/v1/blobs/"+uploaded.ID, nil)
	h.ServeHTTP(getRR, getReq)

	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status = %d", getRR.Code)
	}
	if !bytes.Equal(getRR.Body.Bytes(), payload) {
		t.Fatalf("GET body mismatch: got %q want %q", getRR.Body.Bytes(), payload)
	}
	if got := getRR.Header().Get("Content-Length"); got != strconv.Itoa(len(payload)) {
		t.Fatalf("GET Content-Length = %q", got)
	}
	if got := getRR.Header().Get("ETag"); got != uploaded.ETag {
		t.Fatalf("GET ETag = %q, want %q", got, uploaded.ETag)
	}

	headRR := httptest.NewRecorder()
	headReq := httptest.NewRequest(http.MethodHead, "/v1/blobs/"+uploaded.ID, nil)
	h.ServeHTTP(headRR, headReq)
	if headRR.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d", headRR.Code)
	}
	if got := headRR.Header().Get("Content-Length"); got != strconv.Itoa(len(payload)) {
		t.Fatalf("HEAD Content-Length = %q", got)
	}
	if got := headRR.Header().Get("ETag"); got != uploaded.ETag {
		t.Fatalf("HEAD ETag = %q, want %q", got, uploaded.ETag)
	}
}

func TestUploadMissingLengthReturns411(t *testing.T) {
	_, h := newTestStoreAndHandler(t, 1024)

	req := httptest.NewRequest(http.MethodPost, "/v1/blobs", strings.NewReader("abc"))
	req.ContentLength = -1
	req.TransferEncoding = nil
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusLengthRequired {
		t.Fatalf("status = %d, want 411", rr.Code)
	}
}

func TestUploadChunkedReturns411(t *testing.T) {
	_, h := newTestStoreAndHandler(t, 1024)

	req := httptest.NewRequest(http.MethodPost, "/v1/blobs", strings.NewReader("abc"))
	req.ContentLength = -1
	req.TransferEncoding = []string{"chunked"}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusLengthRequired {
		t.Fatalf("status = %d, want 411", rr.Code)
	}
}

func TestUploadLargerThanCapReturns413(t *testing.T) {
	_, h := newTestStoreAndHandler(t, 3)

	req := httptest.NewRequest(http.MethodPost, "/v1/blobs", strings.NewReader("abcd"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rr.Code)
	}
}

func TestGetUnknownIDReturns404(t *testing.T) {
	_, h := newTestStoreAndHandler(t, 1024)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/blobs/AAAAAAAAAAAAAAAAAAAAAA", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestInvalidIDReturns404(t *testing.T) {
	_, h := newTestStoreAndHandler(t, 1024)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/blobs/not-valid", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestFIFOEvictionByCapacity(t *testing.T) {
	_, h := newTestStoreAndHandler(t, 8)

	a := uploadBlob(t, h, []byte("aaaaa"), "")
	b := uploadBlob(t, h, []byte("bbbbb"), "")

	getA := httptest.NewRecorder()
	h.ServeHTTP(getA, httptest.NewRequest(http.MethodGet, "/v1/blobs/"+a.ID, nil))
	if getA.Code != http.StatusNotFound {
		t.Fatalf("A status = %d, want 404", getA.Code)
	}

	getB := httptest.NewRecorder()
	h.ServeHTTP(getB, httptest.NewRequest(http.MethodGet, "/v1/blobs/"+b.ID, nil))
	if getB.Code != http.StatusOK {
		t.Fatalf("B status = %d, want 200", getB.Code)
	}
}

func TestConcurrentGetsSameBlob(t *testing.T) {
	_, h := newTestStoreAndHandler(t, 2048)
	payload := []byte("concurrent-body")
	uploaded := uploadBlob(t, h, payload, "")

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/v1/blobs/"+uploaded.ID, nil)
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rr.Code)
				return
			}
			body, err := io.ReadAll(rr.Result().Body)
			if err != nil {
				t.Errorf("read body: %v", err)
				return
			}
			if !bytes.Equal(body, payload) {
				t.Errorf("payload mismatch")
			}
		}()
	}
	wg.Wait()
}
