#!/usr/bin/env sh
# Cross-compile the Mission Control agent for all supported platforms into
# ./bin, ready to be served by the server at /download/<name>.
#
# The agent is CGO-free, so this cross-compiles cleanly with no toolchain setup.
# Point the server at the output with MC_AGENT_BIN_DIR=./bin.
set -eu

OUT="${1:-bin}"
mkdir -p "$OUT"

targets="
linux amd64
linux arm64
darwin amd64
darwin arm64
windows amd64
"

echo "$targets" | while read -r os arch; do
  [ -z "$os" ] && continue
  name="mission-control-agent-${os}-${arch}"
  [ "$os" = "windows" ] && name="${name}.exe"
  echo "building $name"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -ldflags="-s -w" -o "$OUT/$name" ./apps/agent
done

echo "done → $OUT"
ls -la "$OUT"
