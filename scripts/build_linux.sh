#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/build/linux"

mkdir -p "$OUT"

echo "[SIDE] Building Linux binary..."
cd "$ROOT"
go build -trimpath -ldflags="-s -w" -o "$OUT/side" ./cmd/side

echo "[SIDE] Build completed."
echo "[SIDE] Output: $OUT/side"
