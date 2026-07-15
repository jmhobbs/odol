#!/usr/bin/env bash
# Builds the WASM bundle consumed by web/index.html. Output goes to
# web/public/, which is gitignored (toolchain-version-specific build
# artifacts) - index.html/main.js/worker.js reference it by relative path
# and are committed directly.
set -euo pipefail

cd "$(dirname "$0")/.."

mkdir -p web/public
GOOS=js GOARCH=wasm go build -o web/public/odol.wasm ./cmd/wasm
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/public/wasm_exec.js

echo "Built web/public/odol.wasm and web/public/wasm_exec.js"
echo "Serve locally with, e.g.:"
echo "  python3 -m http.server --directory web 8080"
echo "then open http://localhost:8080/index.html"
