#!/usr/bin/env bash
set -euo pipefail

die() {
  printf 'Invalid Docker tag input: %s\n' "$1" >&2
  exit 1
}

if [[ $# -ne 1 ]]; then
  die 'expected one comma-separated tag argument'
fi

input="$1"
if [[ -z "$input" ]]; then
  die 'at least one tag is required'
fi
if [[ "$input" == *$'\n'* || "$input" == *$'\r'* ]]; then
  die 'tags must be on one line'
fi
if [[ "$input" == ,* || "$input" == *, || "$input" == *,,* ]]; then
  die 'empty tags are not allowed'
fi

IFS=',' read -r -a raw_tags <<<"$input"
if [[ ${#raw_tags[@]} -eq 0 ]]; then
  die 'at least one tag is required'
fi

for raw_tag in "${raw_tags[@]}"; do
  tag="${raw_tag#"${raw_tag%%[![:space:]]*}"}"
  tag="${tag%"${tag##*[![:space:]]}"}"

  if [[ -z "$tag" ]]; then
    die 'empty tags are not allowed'
  fi
  if [[ ! "$tag" =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]*$ ]]; then
    die "unsupported Docker tag: $tag"
  fi
  if (( ${#tag} > 128 )); then
    die 'Docker tags cannot exceed 128 characters'
  fi
  if (( ${#tag} + 6 > 128 )); then
    die 'Docker tags must leave room for the -arm64 suffix'
  fi

  printf 'cyanautomation/gogomio:%s\n' "$tag"
  printf 'cyanautomation/gogomio:%s-arm64\n' "$tag"
done
