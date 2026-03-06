#!/usr/bin/env bash
set -euo pipefail

IMAGE_NAME=${IMAGE_NAME:-codex-auth-refresher:smoke}
PORT=${PORT:-18080}
AUTH_DIR=${AUTH_DIR:-"$(pwd)/auth"}
CID=""

cleanup() {
  if [[ -n "$CID" ]]; then
    docker rm -f "$CID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

mkdir -p "$AUTH_DIR"
chmod 0777 "$AUTH_DIR"

docker build -t "$IMAGE_NAME" .
CID=$(docker run -d \
  -p "$PORT":8080 \
  -v "$AUTH_DIR:/data/auth" \
  -e CODEX_AUTH_DIR=/data/auth \
  -e CODEX_WEB_ENABLE=true \
  "$IMAGE_NAME")

for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:${PORT}/healthz" >/dev/null \
    && curl -fsS "http://127.0.0.1:${PORT}/readyz" >/dev/null \
    && curl -fsS "http://127.0.0.1:${PORT}/" >/dev/null \
    && curl -fsS "http://127.0.0.1:${PORT}/v1/dashboard" >/dev/null; then
    echo "docker smoke test passed"
    exit 0
  fi
  sleep 1
done

echo "docker smoke test failed" >&2
docker logs "$CID" || true
exit 1
