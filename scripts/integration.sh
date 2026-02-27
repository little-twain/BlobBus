#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
PORT=$((20000 + RANDOM % 10000))
ADDR="127.0.0.1:${PORT}"
BASE_URL="http://${ADDR}"

cleanup() {
  if [[ -n "${SERVER_PID:-}" ]]; then
    kill "${SERVER_PID}" >/dev/null 2>&1 || true
    wait "${SERVER_PID}" >/dev/null 2>&1 || true
  fi
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

assert_eq() {
  local got="$1"
  local want="$2"
  local msg="$3"
  if [[ "$got" != "$want" ]]; then
    echo "assert failed: ${msg}. got=${got} want=${want}" >&2
    exit 1
  fi
}

extract_json_field() {
  local key="$1"
  local file="$2"
  sed -n "s/.*\"${key}\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" "$file" | head -n1
}

cd "${ROOT_DIR}"

CAP_BYTES=8 \
DATA_DIR="${TMP_DIR}/data" \
LISTEN_ADDR="${ADDR}" \
GOCACHE="${TMP_DIR}/gocache" \
GOMODCACHE="${TMP_DIR}/gomodcache" \
go run ./cmd/blobbus >"${TMP_DIR}/server.log" 2>&1 &
SERVER_PID=$!

for _ in {1..100}; do
  if curl -sSf "${BASE_URL}/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done
curl -sSf "${BASE_URL}/healthz" >/dev/null

# Upload A
status=$(curl -sS -o "${TMP_DIR}/a.json" -w "%{http_code}" -X POST \
  -H "Content-Type: application/octet-stream" \
  --data-binary "aaaaa" "${BASE_URL}/v1/blobs")
assert_eq "$status" "201" "upload A"
a_id=$(extract_json_field "id" "${TMP_DIR}/a.json")

# GET A
status=$(curl -sS -o "${TMP_DIR}/a.bin" -w "%{http_code}" "${BASE_URL}/v1/blobs/${a_id}")
assert_eq "$status" "200" "get A"
assert_eq "$(cat "${TMP_DIR}/a.bin")" "aaaaa" "A payload"

# HEAD A
status=$(curl -sS -I -o "${TMP_DIR}/a.head" -w "%{http_code}" "${BASE_URL}/v1/blobs/${a_id}")
assert_eq "$status" "200" "head A"
grep -qi '^Content-Length: 5' "${TMP_DIR}/a.head"

# 411 (chunked / no length)
status=$(curl -sS -o /dev/null -w "%{http_code}" -X POST \
  -H "Transfer-Encoding: chunked" \
  --data-binary "abc" "${BASE_URL}/v1/blobs")
assert_eq "$status" "411" "chunked upload should be rejected"

# 413 (> cap)
status=$(curl -sS -o /dev/null -w "%{http_code}" -X POST \
  --data-binary "123456789" "${BASE_URL}/v1/blobs")
assert_eq "$status" "413" "oversized upload"

# Upload B (forces FIFO eviction of A)
status=$(curl -sS -o "${TMP_DIR}/b.json" -w "%{http_code}" -X POST \
  --data-binary "bbbbb" "${BASE_URL}/v1/blobs")
assert_eq "$status" "201" "upload B"
b_id=$(extract_json_field "id" "${TMP_DIR}/b.json")

status=$(curl -sS -o /dev/null -w "%{http_code}" "${BASE_URL}/v1/blobs/${a_id}")
assert_eq "$status" "404" "A should be evicted"

status=$(curl -sS -o "${TMP_DIR}/b.bin" -w "%{http_code}" "${BASE_URL}/v1/blobs/${b_id}")
assert_eq "$status" "200" "B should exist"
assert_eq "$(cat "${TMP_DIR}/b.bin")" "bbbbb" "B payload"

echo "integration checks passed"
