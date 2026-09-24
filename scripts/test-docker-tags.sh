#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
parser="$repo_root/scripts/docker-tags.sh"
temporary_dir="$(mktemp -d)"
trap 'rm -rf "$temporary_dir"' EXIT

assert_tags() {
  local input="$1"
  local expected="$2"
  local actual

  actual="$(bash "$parser" "$input")"
  if [[ "$actual" != "$expected" ]]; then
    printf 'For input %q expected:\n%s\nGot:\n%s\n' "$input" "$expected" "$actual" >&2
    exit 1
  fi
}

assert_rejected() {
  local input="$1"

  if bash "$parser" "$input" >/dev/null 2>&1; then
    printf 'Expected input to be rejected: %q\n' "$input" >&2
    exit 1
  fi
}

assert_tags 'latest, 0.2.0-rc1' $'cyanautomation/gogomio:latest\ncyanautomation/gogomio:latest-arm64\ncyanautomation/gogomio:0.2.0-rc1\ncyanautomation/gogomio:0.2.0-rc1-arm64'
assert_tags 'A_1,release.candidate-2' $'cyanautomation/gogomio:A_1\ncyanautomation/gogomio:A_1-arm64\ncyanautomation/gogomio:release.candidate-2\ncyanautomation/gogomio:release.candidate-2-arm64'
max_tag="$(printf '%122s' '' | tr ' ' x)"
assert_tags "$max_tag" "$(printf 'cyanautomation/gogomio:%s\ncyanautomation/gogomio:%s-arm64' "$max_tag" "$max_tag")"

assert_rejected ''
assert_rejected ',latest'
assert_rejected 'latest,'
assert_rejected 'latest,,0.2.0'
assert_rejected 'bad/tag'
assert_rejected "\$(touch \"$temporary_dir/injected\")"
assert_rejected "\"; touch \"$temporary_dir/quoted-injected\"; #"
assert_rejected "$(printf 'x%.0s' {1..123})"
assert_rejected $'latest\n0.2.0'

if [[ -e "$temporary_dir/injected" || -e "$temporary_dir/quoted-injected" ]]; then
  echo 'Tag validation executed command-substitution text.' >&2
  exit 1
fi

echo 'Docker tag validation tests passed.'
