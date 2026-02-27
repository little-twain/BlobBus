package blobbus

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func NewHandler(store *Store) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/blobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		handleUpload(w, r, store)
	})

	mux.HandleFunc("/v1/blobs/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/blobs/")
		if !isValidID(id) {
			http.NotFound(w, r)
			return
		}

		switch r.Method {
		case http.MethodGet:
			handleGet(w, r, store, id)
		case http.MethodHead:
			handleHead(w, r, store, id)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return mux
}

type uploadResponse struct {
	ID        string `json:"id"`
	Size      int64  `json:"size"`
	ETag      string `json:"etag"`
	CreatedAt string `json:"created_at"`
}

func handleUpload(w http.ResponseWriter, r *http.Request, store *Store) {
	if hasChunkedEncoding(r) || r.ContentLength < 0 {
		http.Error(w, "length required", http.StatusLengthRequired)
		return
	}

	meta, err := store.Upload(r.Context(), r.Body, r.ContentLength, r.Header.Get("Content-Type"))
	if err != nil {
		switch {
		case errors.Is(err, ErrLengthRequired):
			http.Error(w, "length required", http.StatusLengthRequired)
		case errors.Is(err, ErrTooLarge):
			http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		default:
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	writeJSON(w, http.StatusCreated, uploadResponse{
		ID:        meta.ID,
		Size:      meta.Size,
		ETag:      meta.ETag,
		CreatedAt: meta.CreatedAt.Format(time.RFC3339),
	})
}

func handleGet(w http.ResponseWriter, r *http.Request, store *Store, id string) {
	meta, file, err := store.Open(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	writeBlobHeaders(w, meta)
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, file)
}

func handleHead(w http.ResponseWriter, r *http.Request, store *Store, id string) {
	meta, err := store.Stat(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	writeBlobHeaders(w, meta)
	w.WriteHeader(http.StatusOK)
}

func writeBlobHeaders(w http.ResponseWriter, meta BlobMeta) {
	w.Header().Set("Content-Length", strconv.FormatInt(meta.Size, 10))
	if meta.ContentType == "" {
		w.Header().Set("Content-Type", "application/octet-stream")
	} else {
		w.Header().Set("Content-Type", meta.ContentType)
	}
	w.Header().Set("ETag", meta.ETag)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func hasChunkedEncoding(r *http.Request) bool {
	for _, enc := range r.TransferEncoding {
		if strings.EqualFold(enc, "chunked") {
			return true
		}
	}
	return false
}
