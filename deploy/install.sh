#!/usr/bin/env sh
# Mission Control agent installer for Linux and macOS.
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/avi-pathak/mission-control.ai/main/deploy/install.sh | sh
#
# Env:
#   MC_SERVER_URL   server ws(s) endpoint (default ws://localhost:8080)
#   MC_API_KEY      API key (required in production)
#   MC_VERSION      release tag (default: latest)
set -eu

REPO="avi-pathak/mission-control.ai"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
CONFIG_DIR="${HOME}/.mission-control"
BIN="mission-control-agent"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) echo "unsupported arch: $arch" >&2; exit 1 ;;
esac

version="${MC_VERSION:-latest}"
echo "Installing ${BIN} (${os}/${arch}, ${version})..."

# Download binary from GitHub releases.
if [ "$version" = "latest" ]; then
  url="https://github.com/${REPO}/releases/latest/download/${BIN}-${os}-${arch}"
else
  url="https://github.com/${REPO}/releases/download/${version}/${BIN}-${os}-${arch}"
fi

tmp="$(mktemp)"
curl -fsSL "$url" -o "$tmp" || { echo "download failed: $url" >&2; exit 1; }
chmod +x "$tmp"

if [ -w "$INSTALL_DIR" ]; then
  mv "$tmp" "${INSTALL_DIR}/${BIN}"
else
  sudo mv "$tmp" "${INSTALL_DIR}/${BIN}"
fi

# Write a starter config if none exists.
mkdir -p "$CONFIG_DIR"
if [ ! -f "${CONFIG_DIR}/agent.yaml" ]; then
  cat > "${CONFIG_DIR}/agent.yaml" <<EOF
serverUrl: "${MC_SERVER_URL:-ws://localhost:8080}"
enrollToken: "${MC_ENROLL_TOKEN:-}"
apiKey: "${MC_API_KEY:-}"
providers:
  - "claude-code"
discoverEverySeconds: 5
metricsEverySeconds: 3
heartbeatEverySeconds: 10
logLevel: "info"
EOF
  echo "Wrote ${CONFIG_DIR}/agent.yaml"
fi

# If an enrollment token was provided, start immediately so the agent
# self-enrolls and appears in the dashboard.
if [ -n "${MC_ENROLL_TOKEN:-}" ]; then
  echo "Starting agent to enroll..."
  exec "${INSTALL_DIR}/${BIN}" --config "${CONFIG_DIR}/agent.yaml"
fi

echo "Done. Start with:"
echo "  ${BIN} --config ${CONFIG_DIR}/agent.yaml"
