#!/bin/sh
set -eu
set -f

: "${TARGETARCH:?TARGETARCH must be set by Docker BuildKit}"
GO_BIN="${GO_BIN:-/usr/local/go/bin/go}"
TEST_LOG_DIR="${TEST_LOG_DIR:-/tmp/test-logs}"

case "$TARGETARCH" in
  amd64|arm64) ;;
  *)
    printf 'Unsupported Docker test architecture: %s\n' "$TARGETARCH" >&2
    exit 2
    ;;
esac

mkdir -p "$TEST_LOG_DIR"
status=0
packages=$("$GO_BIN" list ./...)

run_package_tests() {
  if [ "$TARGETARCH" = arm64 ]; then
    CGO_ENABLED=1 "$GO_BIN" test "$1"
  else
    CGO_ENABLED=1 "$GO_BIN" test -race "$1"
  fi
}

for pkg in $packages; do
  safe_pkg=$(printf '%s' "$pkg" | sed 's/[^a-zA-Z0-9_-]/_/g')
  log_file="$TEST_LOG_DIR/${safe_pkg}.log"
  if ! run_package_tests "$pkg" > "$log_file" 2>&1; then
    status=1
    cat "$log_file"
  fi
done

test "$status" -eq 0
