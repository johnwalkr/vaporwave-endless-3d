#!/bin/bash
# Builds the Quest/web version into web/dist (static files: host anywhere with HTTPS).
set -euo pipefail
cd "$(dirname "$0")"
rm -rf dist && mkdir -p dist
(cd .. && GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o web/dist/ve3d.wasm ./cmd/webxr)
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" dist/ 2>/dev/null || cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" dist/
cp index.html main.js dist/
touch dist/.nojekyll
echo "built web/dist ($(du -sh dist | cut -f1))"
