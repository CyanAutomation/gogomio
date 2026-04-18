#!/usr/bin/env bash
set -euo pipefail

log_bin_path() {
  local bin_name="$1"
  if command -v "$bin_name" >/dev/null 2>&1; then
    echo "[camera-check] ${bin_name}: $(command -v "$bin_name")"
  else
    echo "[camera-check] ${bin_name}: NOT FOUND"
  fi
}

echo "[camera-check] Verifying camera backend binaries"
log_bin_path rpicam-vid
log_bin_path libcamera-vid
log_bin_path ffmpeg

exec /app/gogomio "$@"
