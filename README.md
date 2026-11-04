# GoTTY

[![MIT License](http://img.shields.io/badge/license-MIT-blue.svg?style=flat-square)](LICENSE)

GoTTY shares a terminal over the web. This repository builds on the original [yudai/gotty](https://github.com/yudai/gotty) and [sorenisanerd/gotty](https://github.com/sorenisanerd/gotty) project by adding a multi-tab frontend, server-side session persistence, reverse-proxy user mapping, container-isolated sessions, file upload/download utilities, and a local HTTP/WebSocket preview proxy.

*Original work by the [GoTTY project](https://github.com/yudai/gotty) ([Iwasaki Yudai](https://github.com/yudai/gotty) and [Soren L. Hansen](https://github.com/sorenisanerd/gotty)).*  
*Multi-session and container support authored by **Google Gemini 4 Argon**.*

---

## Deployment Model: Requires a Frontend Reverse Proxy

> [!IMPORTANT]
> **This application is not intended to be exposed directly to the network on its own.**  
> It is designed to run on `127.0.0.1` (or an internal container bridge) behind an authenticating frontend proxy such as [Nginx Proxy Manager (NPM)](https://nginxproxymanager.com/), Authelia, Authentik, or OAuth2 Proxy. The frontend proxy is responsible for handling SSL/TLS termination, authenticating users, and forwarding the authenticated username or email address to GoTTY via HTTP headers (such as `X-Forwarded-User` or `Remote-User`).

---

## What Was Added to Upstream GoTTY

Upstream GoTTY runs a single command per WebSocket connection and terminates the process when the browser tab closes. This fork adds:

1. **Multi-Tab Interface**: A Svelte 5 + xterm.js top bar that manages multiple terminal sessions in one browser window without requiring `tmux` or `screen`.
2. **Server-Side Session Persistence (`SessionManager`)**: Terminal processes stay running on the server when a browser disconnects or reloads. Each session keeps a 1MB circular ring buffer in memory so reconnecting clients receive recent terminal output before live output resumes.
3. **Reverse-Proxy User Mapping (`/etc/passwd` & `/etc/group`)**: Reads the identity header forwarded by the reverse proxy and resolves it against `/etc/passwd` (matching either the Linux username or an email address in the 5th GECOS field). Group membership in `/etc/group` determines whether the user is limited to a sandboxed container or can open a host shell.
4. **Two Session Modes (`sandbox` and `host`)**:
   - **Sandbox (`sandbox`)**: Starts an Ubuntu 24.04 container (`gotty-sandbox:latest`) running under the user's UID/GID with only `/home/<username>` mounted. Standard users are restricted to this mode.
   - **Host Shell (`host`)**: Allocates a native host PTY under the user's UID, GID, and supplementary groups. Available only to users in `wheel`, `sudo`, or `gotty-host` (configurable via `--host-groups`).
5. **File Upload & Download (`godl`)**:
   - **Upload**: Dragging files onto the terminal (or clicking the upload button in the top bar) sends them via `POST /api/sessions/:id/upload` into the shell's current working directory (`/proc/<pid>/cwd`), falling back to `/home/<username>`.
   - **Download**: Running `godl <filepath>` inside a session encodes the file into OSC 7777 terminal escape sequences that the browser intercepts and saves through the standard browser download dialog.
6. **Local Web Preview Proxy**: A top-bar menu that proxies HTTP, HTTPS, and WebSocket traffic to local services running on the server (`/preview/<host>:<port>/`). On startup, GoTTY introspects active network interfaces so host-authorized users can select `localhost` or local private interface IPs; sandboxed users are restricted to `localhost`.
7. **Per-User Configuration (`~/.gotty/config.yaml`)**: A settings drawer that saves theme, font family, font size, scrollback length, clipboard behavior, keyboard shortcuts, and detached session cleanup timers to the user's home directory.

---

## Setup & Installation

### 1. Run the Installer
[`./install.sh`](install.sh) checks for Node/NPM and Go, builds the `gotty-sandbox:latest` container image from [`Containerfile`](Containerfile) if Docker or Podman is installed, compiles the frontend and the `gotty` and `godl` binaries to `/usr/local/bin`, creates the `gotty-host` group, and writes `/etc/systemd/system/gotty.service`.

```bash
chmod +x ./install.sh
./install.sh
```

### 2. Map Users in `/etc/passwd` and `/etc/group`
When `--proxy-auth` is enabled, GoTTY maps the header sent by your proxy to a local Linux user account:

```bash
# Map an email address to an existing Linux user via the GECOS field in /etc/passwd:
sudo usermod -c "Dev User,devuser@example.com" devuser

# (Optional) Allow the user to open Host Shell sessions by adding them to gotty-host:
sudo groupadd -f gotty-host
sudo usermod -aG gotty-host devuser

# Or create a user restricted to Sandbox container sessions only:
sudo useradd -m -s /bin/bash -c "Guest User,guest@example.com" guest
```

### 3. Configure the Frontend Proxy
Set up your reverse proxy (for example, [Nginx Proxy Manager](https://nginxproxymanager.com/)) to handle HTTPS and authentication in front of GoTTY:

1. Point the proxy host to `http://127.0.0.1:8080`.
2. Enable **Websockets Support**.
3. Configure an **Access List** (Basic Auth or an external auth provider such as Authelia/OAuth2 Proxy).
4. In the **Advanced** Nginx configuration tab, forward the authenticated user and host headers:
   ```nginx
   proxy_set_header Host $host;
   proxy_set_header X-Forwarded-Host $host;
   proxy_set_header X-Forwarded-User $remote_user;
   client_max_body_size 512M;
   ```

### 4. Start the Service
```bash
sudo systemctl daemon-reload
sudo systemctl enable --now gotty
```

---

## How to Use the System

### Managing Terminal Tabs & Session Modes
- **New Tab**: Click the `+` button in the top bar (or press `Alt+T`) to open a new session using your default mode.
- **Selecting a Mode**: Click the chevron next to the `+` button to choose between:
  - **Sandbox Container**: Runs inside an isolated Ubuntu 24.04 container with `/home/<username>` mounted.
  - **Host Shell**: Runs directly on the host OS (only works if your Linux user belongs to `gotty-host`, `wheel`, or `sudo`; otherwise the server places the session in a sandbox container).
- **Renaming a Tab**: Double-click the tab title, click the pencil icon on the active tab, or emit a standard xterm title sequence from the shell (`printf '\033]2;Custom Name\007'`).
- **Closing vs. Detaching**:
  - Closing the browser window leaves the session running on the server; reopening the page reconnects to your active tabs and replays the 1MB output buffer.
  - Clicking the `×` button on a tab (or pressing `Alt+W`) sends `DELETE /api/sessions/:id` and terminates that underlying shell/container.

### Uploading & Downloading Files
- **Upload**: Drag and drop one or more files onto the terminal area, or click the **Upload** icon in the top bar. The server checks `/proc/<pid>/cwd` of the active shell process and writes the file into that directory (or into `/home/<username>` if the working directory cannot be resolved).
- **Download (`godl`)**: From inside the terminal, run:
  ```bash
  godl /path/to/file.tar.gz
  ```
  `godl` reads the file and writes base64-encoded OSC 7777 chunks to stdout. The browser intercepts those sequences and saves the file locally.

### Using Web Preview
1. Start an HTTP/HTTPS server inside your session (for example, `python3 -m http.server 8000` or a dev server on port `3000` or `8080`).
2. Click the **Preview** icon (browser window with eye) in the top-right bar.
3. Choose the target interface IP (populated at server startup by introspecting local network interfaces), set the port and protocol (`Auto`, `HTTP`, or `HTTPS`), and click **Preview**.
4. A new browser tab opens at `./preview/<host>:<port>/`, proxying requests and WebSockets through GoTTY to that local port.

### Adjusting User Preferences
Click the **Settings** (gear) icon in the top-right bar to configure:
- **Color Theme**: `catppuccin-mocha`, `tokyo-night`, `nord`, `monokai`, or `solarized-dark`.
- **Text Size & Scrollback**: Font size (`8px`–`32px`) and scrollback buffer lines.
- **Font Family**: Select from preloaded web fonts, system monospace fonts, or scan local system fonts via the browser's Local Font Access API.
- **Detached Session Reap Policy**: How long a session with no connected browser tabs stays alive before automatic termination (`never`, `1h`, `2h`, `8h`, `24h`).
- **Copy & Keybindings**: Toggle `Copy on Select`, switch between `Ctrl+Shift+C/V` and browser default copy/paste, and customize new/close tab shortcuts.

Clicking **Save Preferences** writes these settings to `~/.gotty/config.yaml`.

---

## Documentation

- **[End-to-End Deployment & Container Guide](docs/deployment.md)** — Full deployment walkthrough covering the `gotty-sandbox:latest` container build (`Containerfile`), host user/group setup, systemd configuration, and [Nginx Proxy Manager](https://nginxproxymanager.com/) setup.
- **[Architecture & Internals](docs/architecture.md)** — How `SessionManager`, the 1MB ring buffer, container vs. host PTY spawning, file transfers, and the preview proxy work.
- **[Authentication & Security](docs/authentication-and-security.md)** — Reverse-proxy requirement, [Nginx Proxy Manager](https://nginxproxymanager.com/) configuration, `/etc/passwd` and `/etc/group` resolution, and request validation.
- **[Configuration & Customization](docs/configuration.md)** — Per-user `~/.gotty/config.yaml` schema, UI settings, and server `~/.gotty` HCL configuration.
- **[CLI Reference](docs/cli-reference.md)** — Command-line flags and environment variables for `gotty` and `godl`.

---

## Development & Testing

```bash
# Build frontend assets
cd js && npm install && npm run build && cd ..

# Run Go & Frontend tests
go test ./...
cd js && npm test && cd ..

# Compile binaries
CGO_ENABLED=0 go build -o gotty .
CGO_ENABLED=0 go build -o godl ./cmd/godl
```

---

## License

Released under the [MIT License](LICENSE).
