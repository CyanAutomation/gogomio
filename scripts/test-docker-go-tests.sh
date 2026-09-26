#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
runner="$repo_root/scripts/run-docker-go-tests.sh"
temporary_dir="$(mktemp -d)"
trap 'rm -rf "$temporary_dir"' EXIT

cat > "$temporary_dir/fake-go" <<'FAKE_GO'
#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_GO_CALLS"
if [ "$1" = list ] && [ "$2" = ./... ]; then
  printf '%s\n' 'example.com/project/one' 'example.com/project/two'
fi
FAKE_GO
chmod +x "$temporary_dir/fake-go"

assert_calls() {
  local arch="$1"
  local expected="$2"
  local calls_file="$temporary_dir/$arch.calls"

  TARGETARCH="$arch" \
    GO_BIN="$temporary_dir/fake-go" \
    TEST_LOG_DIR="$temporary_dir/$arch-logs" \
    FAKE_GO_CALLS="$calls_file" \
    sh "$runner"

  if [[ "$(cat "$calls_file")" != "$expected" ]]; then
    printf 'Unexpected go invocations for %s:\n%s\nExpected:\n%s\n' \
      "$arch" "$(cat "$calls_file")" "$expected" >&2
    exit 1
  fi
}

assert_calls arm64 $'list ./...\ntest example.com/project/one\ntest example.com/project/two'
assert_calls amd64 $'list ./...\ntest -race example.com/project/one\ntest -race example.com/project/two'

echo 'Docker Go test architecture selection tests passed.'
