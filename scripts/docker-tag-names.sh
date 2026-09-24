#!/usr/bin/env bash
set -euo pipefail

die() {
  printf 'Invalid Docker image tag list: %s\n' "$1" >&2
  exit 1
}

prefix='cyanautomation/gogomio:'
tag_names_csv=''

while IFS= read -r image || [[ -n "$image" ]]; do
  [[ -n "$image" ]] || continue

  case "$image" in
    "$prefix"*) tag="${image#"$prefix"}" ;;
    *) die 'image name does not match the gogomio repository' ;;
  esac

  if [[ ! "$tag" =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]*$ ]]; then
    die 'tag contains unsupported characters'
  fi
  if (( ${#tag} > 128 )); then
    die 'tag exceeds 128 characters'
  fi

  if [[ -n "$tag_names_csv" ]]; then
    tag_names_csv+=','
  fi
  tag_names_csv+="$tag"
done

[[ -n "$tag_names_csv" ]] || die 'at least one image tag is required'
printf '%s\n' "$tag_names_csv"
