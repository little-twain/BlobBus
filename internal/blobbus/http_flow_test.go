package blobbus

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

func newHTTPTestServer(t *testing.T, capBytes int64) *httptest.Server {
	t.Helper()

	store, err := NewStore(Config{
		CapBytes:   capBytes,
		DataDir:    t.TempDir(),
		ListenAddr: ":0",
	})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	store.freeBytes = func(string) (int64, error) { return 1 << 30, nil }

	return httptest.NewServer(NewHandler(store))
}

func httpUpload(t *testing.T, client *http.Client, baseURL string, payload []byte, contentType string) testUploadResponse {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/blobs", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = int64(len(payload))

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("upload request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload status = %d body = %s", resp.StatusCode, string(body))
	}

	var out testUploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	return out
}

func httpGetBody(t *testing.T, client *http.Client, url string) (int, []byte, http.Header) {
	t.Helper()

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s failed: %v", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read GET body: %v", err)
	}
	return resp.StatusCode, body, resp.Header
}

func TestHTTPFlowFIFOAndNotLRU(t *testing.T) {
	server := newHTTPTestServer(t, 10)
	defer server.Close()
	client := server.Client()

	a := httpUpload(t, client, server.URL, []byte("AAAA"), "application/octet-stream")
	b := httpUpload(t, client, server.URL, []byte("BBBB"), "application/octet-stream")

	// Touch A via GET; if this were LRU, A would become newest.
	statusAGet, bodyAGet, _ := httpGetBody(t, client, server.URL+"/v1/blobs/"+a.ID)
	if statusAGet != http.StatusOK || string(bodyAGet) != "AAAA" {
		t.Fatalf("pre-eviction GET A unexpected status=%d body=%q", statusAGet, string(bodyAGet))
	}

	c := httpUpload(t, client, server.URL, []byte("CCCC"), "application/octet-stream")

	statusA, _, _ := httpGetBody(t, client, server.URL+"/v1/blobs/"+a.ID)
	statusB, bodyB, headerB := httpGetBody(t, client, server.URL+"/v1/blobs/"+b.ID)
	statusC, bodyC, headerC := httpGetBody(t, client, server.URL+"/v1/blobs/"+c.ID)

	if statusA != http.StatusNotFound {
		t.Fatalf("A should be evicted by FIFO even after GET touch; status=%d", statusA)
	}
	if statusB != http.StatusOK || string(bodyB) != "BBBB" {
		t.Fatalf("B should remain; status=%d body=%q", statusB, string(bodyB))
	}
	if statusC != http.StatusOK || string(bodyC) != "CCCC" {
		t.Fatalf("C should remain; status=%d body=%q", statusC, string(bodyC))
	}
	if got := headerB.Get("Content-Length"); got != strconv.Itoa(len(bodyB)) {
		t.Fatalf("B Content-Length = %q", got)
	}
	if got := headerC.Get("Content-Length"); got != strconv.Itoa(len(bodyC)) {
		t.Fatalf("C Content-Length = %q", got)
	}
}

func TestHTTPConcurrentClientsUploadDownload(t *testing.T) {
	server := newHTTPTestServer(t, 1<<20)
	defer server.Close()
	client := server.Client()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			payload := bytes.Repeat([]byte{byte('a' + (i % 26))}, 256+i)
			uploaded := httpUpload(t, client, server.URL, payload, "application/octet-stream")

			status, body, headers := httpGetBody(t, client, server.URL+"/v1/blobs/"+uploaded.ID)
			if status != http.StatusOK {
				t.Errorf("client %d GET status=%d", i, status)
				return
			}
			if !bytes.Equal(body, payload) {
				t.Errorf("client %d payload mismatch", i)
				return
			}
			if got := headers.Get("ETag"); got == "" {
				t.Errorf("client %d missing ETag", i)
			}
		}(i)
	}
	wg.Wait()
}

func TestHTTPUploadGateBlocksSecondUpload(t *testing.T) {
	server := newHTTPTestServer(t, 1<<20)
	defer server.Close()
	client := server.Client()

	pipeR, pipeW := io.Pipe()
	firstStatusCh := make(chan int, 1)
	firstDoneAt := make(chan time.Time, 1)
	firstErrCh := make(chan error, 1)

	go func() {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/blobs", pipeR)
		if err != nil {
			firstErrCh <- err
			return
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		req.ContentLength = 6

		resp, err := client.Do(req)
		if err != nil {
			firstErrCh <- err
			return
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		firstStatusCh <- resp.StatusCode
		firstDoneAt <- time.Now()
	}()

	if _, err := pipeW.Write([]byte("abc")); err != nil {
		t.Fatalf("pipe write first half: %v", err)
	}
	time.Sleep(80 * time.Millisecond)

	secondStatusCh := make(chan int, 1)
	secondDoneAt := make(chan time.Time, 1)
	secondErrCh := make(chan error, 1)
	go func() {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/blobs", bytes.NewReader([]byte("z")))
		if err != nil {
			secondErrCh <- err
			return
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		req.ContentLength = 1

		resp, err := client.Do(req)
		if err != nil {
			secondErrCh <- err
			return
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		secondStatusCh <- resp.StatusCode
		secondDoneAt <- time.Now()
	}()

	select {
	case err := <-firstErrCh:
		t.Fatalf("first upload error: %v", err)
	case err := <-secondErrCh:
		t.Fatalf("second upload error: %v", err)
	case <-secondStatusCh:
		t.Fatal("second upload completed before first upload finished; upload gate not effective")
	case <-time.After(150 * time.Millisecond):
		// Expected: second upload waits for the gate.
	}

	if _, err := pipeW.Write([]byte("def")); err != nil {
		t.Fatalf("pipe write second half: %v", err)
	}
	_ = pipeW.Close()

	var firstStatus int
	select {
	case err := <-firstErrCh:
		t.Fatalf("first upload error: %v", err)
	case firstStatus = <-firstStatusCh:
	case <-time.After(2 * time.Second):
		t.Fatal("first upload timed out")
	}
	if firstStatus != http.StatusCreated {
		t.Fatalf("first upload status=%d, want 201", firstStatus)
	}

	var secondStatus int
	select {
	case err := <-secondErrCh:
		t.Fatalf("second upload error: %v", err)
	case secondStatus = <-secondStatusCh:
	case <-time.After(2 * time.Second):
		t.Fatal("second upload timed out")
	}
	if secondStatus != http.StatusCreated {
		t.Fatalf("second upload status=%d, want 201", secondStatus)
	}

	firstAt := <-firstDoneAt
	secondAt := <-secondDoneAt
	if secondAt.Before(firstAt) {
		t.Fatalf("second upload completed before first upload (first=%v second=%v)", firstAt, secondAt)
	}
}
