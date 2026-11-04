#!/usr/bin/env bash
# GoTTY - Automated Installer
# Authors: Google Gemini 4 Argon & the original GoTTY project
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$PROJECT_ROOT"

# 1. Discover Node/NPM (checks PATH, then NVM paths)
if ! command -v npm >/dev/null 2>&1; then
    NVM_NODE_BIN="$(find "$HOME/.nvm/versions/node" -maxdepth 2 -type d -name "bin" 2>/dev/null | sort -V | tail -n 1 || true)"
    if [ -n "$NVM_NODE_BIN" ] && [ -d "$NVM_NODE_BIN" ]; then
        export PATH="$NVM_NODE_BIN:$PATH"
    fi
fi

# 2. Discover or install Go toolchain
if ! command -v go >/dev/null 2>&1; then
    if [ -x /usr/local/go/bin/go ]; then
        export PATH="/usr/local/go/bin:$PATH"
    elif [ -x "$HOME/go/bin/go" ]; then
        export PATH="$HOME/go/bin:$PATH"
    else
        echo "==> Go compiler not found. Downloading official Go binary from go.dev..."
        GO_TARBALL="go1.27.3.linux-amd64.tar.gz"
        if [ "$(uname -m)" = "aarch64" ] || [ "$(uname -m)" = "arm64" ]; then
            GO_TARBALL="go1.27.3.linux-arm64.tar.gz"
        fi
        SUDO_CMD=""
        [ "$(id -u)" -ne 0 ] && SUDO_CMD="sudo"
        TMP_GO_DIR="$(mktemp -d)"
        trap 'rm -rf "$TMP_GO_DIR"' EXIT
        curl -fsSL "https://go.dev/dl/${GO_TARBALL}" -o "$TMP_GO_DIR/${GO_TARBALL}"
        GO_SHA256="$(curl -fsSL "https://go.dev/dl/${GO_TARBALL}.sha256" | tr -d '[:space:]')"
        echo "${GO_SHA256}  $TMP_GO_DIR/${GO_TARBALL}" | sha256sum -c -
        $SUDO_CMD tar -C /usr/local -xzf "$TMP_GO_DIR/${GO_TARBALL}"
        rm -rf "$TMP_GO_DIR"
        $SUDO_CMD ln -sf /usr/local/go/bin/go /usr/local/bin/go
        $SUDO_CMD ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
        export PATH="/usr/local/go/bin:/usr/local/bin:$PATH"
    fi
fi

# 3. Build frontend bundle (Svelte 5 + Vite)
echo "==> [1/4] Building frontend assets..."
if command -v npm >/dev/null 2>&1; then
    (cd "$PROJECT_ROOT/js" && npm install && npm run build)
else
    echo "    Using pre-compiled assets in bindata/static/"
fi

# 4. Build and install Go binaries
echo "==> [2/4] Compiling and installing gotty & godl..."
CGO_ENABLED=0 go build -o "$PROJECT_ROOT/gotty" .
CGO_ENABLED=0 go build -o "$PROJECT_ROOT/godl" ./cmd/godl

SUDO_CMD=""
[ "$(id -u)" -ne 0 ] && SUDO_CMD="sudo"

$SUDO_CMD install -m 0755 "$PROJECT_ROOT/gotty" /usr/local/bin/gotty
$SUDO_CMD install -m 0755 "$PROJECT_ROOT/godl" /usr/local/bin/godl

if command -v groupadd >/dev/null 2>&1; then
    $SUDO_CMD groupadd -f gotty-host || true
fi

# 5. Detect container engine (Docker or Podman) and build sandbox image
echo "==> [3/4] Detecting container engine..."
CONTAINER_RUNTIME=""
if command -v docker >/dev/null 2>&1; then
    CONTAINER_RUNTIME="docker"
    echo "    Found Docker: $(command -v docker)"
elif command -v podman >/dev/null 2>&1; then
    CONTAINER_RUNTIME="podman"
    echo "    Found Podman: $(command -v podman)"
else
    echo "    Neither Docker nor Podman found. Defaulting to host shell execution."
fi

if [ -n "$CONTAINER_RUNTIME" ] && [ -f "$PROJECT_ROOT/Containerfile" ]; then
    echo "==> Building gotty-sandbox:latest container image..."
    $SUDO_CMD $CONTAINER_RUNTIME build -t gotty-sandbox:latest -f "$PROJECT_ROOT/Containerfile" "$PROJECT_ROOT" || true
fi

# 6. Configure systemd service
echo "==> [4/4] Configuring systemd service..."
RUNTIME_FLAG=""
AFTER_UNIT="network.target"
if [ "$CONTAINER_RUNTIME" = "docker" ]; then
    RUNTIME_FLAG="--container-runtime docker --container-image gotty-sandbox:latest"
    AFTER_UNIT="network.target docker.service"
elif [ "$CONTAINER_RUNTIME" = "podman" ]; then
    RUNTIME_FLAG="--container-runtime podman --container-image gotty-sandbox:latest"
    AFTER_UNIT="network.target podman.service"
fi

SERVICE_CONTENT="[Unit]
Description=GoTTY Multi-Session Web Terminal
After=${AFTER_UNIT}

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/gotty --address 127.0.0.1 --port 8080 --permit-write --proxy-auth ${RUNTIME_FLAG} /bin/bash
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
"

if [ -d /etc/systemd/system ]; then
    printf "%s" "$SERVICE_CONTENT" | $SUDO_CMD tee /etc/systemd/system/gotty.service >/dev/null
    echo "    Created /etc/systemd/system/gotty.service"
fi

cat <<EOF

================================================================================
GoTTY & godl successfully installed to /usr/local/bin/!

QUICK START CHECKLIST:
  1. Map user email in /etc/passwd (GECOS field):
       sudo usermod -c "Your Name,you@example.com" \$USER

  2. (Optional) For Router / Host CLI access, add user to gotty-host group:
       sudo usermod -aG gotty-host \$USER

  3. Reverse Proxy (NPM):
       Forward to http://127.0.0.1:8080 with Websockets enabled.
       Add header: proxy_set_header X-Forwarded-User \$remote_user;

  4. Start service:
       sudo systemctl daemon-reload
       sudo systemctl enable --now gotty
================================================================================
EOF
