# CLI Reference (`gotty` & `godl`)

## `gotty` Command Line Usage

```text
Usage: gotty [options] <command> [<arguments...>]
```

> [!NOTE]
> In normal operation behind a reverse proxy such as [Nginx Proxy Manager](https://nginxproxymanager.com/), bind `--address` to `127.0.0.1` and enable `--proxy-auth` and `--permit-write`.

### Session, Auth & Container Options

| Flag | Env Var | HCL Key (`~/.gotty`) | Default | Description |
| :--- | :--- | :--- | :--- | :--- |
| `--proxy-auth` | `$GOTTY_PROXY_AUTH` | `enable_proxy_auth` | `false` | Require a reverse-proxy identity header mapped in `/etc/passwd` |
| `--proxy-auth-header value` | `$GOTTY_PROXY_AUTH_HEADER` | `proxy_auth_header` | `""` | Specific HTTP header to read proxy identity from (empty checks standard proxy headers) |
| `--default-user value` | `$GOTTY_DEFAULT_USER` | `default_user` | `""` | Fallback Linux username when no proxy identity header is present (only for local/trusted testing) |
| `--passwd-file value` | `$GOTTY_PASSWD_FILE` | `passwd_file` | `"/etc/passwd"` | Path to `passwd` file used for username and GECOS email lookup |
| `--group-file value` | `$GOTTY_GROUP_FILE` | `group_file` | `"/etc/group"` | Path to `group` file used for privilege resolution |
| `--host-groups value` | `$GOTTY_HOST_GROUPS` | `host_groups` | `"wheel,sudo,gotty-host"` | Comma-separated Linux groups permitted to open `host` shell sessions |
| `--container-runtime value` | `$GOTTY_CONTAINER_RUNTIME` | `container_runtime` | `""` (auto) | Container runtime for `sandbox` sessions (`docker` or `podman`) |
| `--container-image value` | `$GOTTY_CONTAINER_IMAGE` | `container_image` | `"gotty-sandbox:latest"` | Container image used for `sandbox` sessions |
| `--default-mode value` | `$GOTTY_DEFAULT_MODE` | `default_mode` | `"sandbox"` | Default session mode (`sandbox` or `host`) |

### Network, TLS & Access Options

| Flag | Env Var | HCL Key (`~/.gotty`) | Default | Description |
| :--- | :--- | :--- | :--- | :--- |
| `--address value`, `-a` | `$GOTTY_ADDRESS` | `address` | `"0.0.0.0"` | IP address(es) to listen on (comma-separated for multiple) |
| `--port value`, `-p` | `$GOTTY_PORT` | `port` | `"8080"` | Port number to listen on (`0` for random port) |
| `--path value`, `-m` | `$GOTTY_PATH` | `path` | `"/"` | Base URL path prefix |
| `--permit-write`, `-w` | `$GOTTY_PERMIT_WRITE` | `permit_write` | `false` | Permit clients to write input to the PTY |
| `--credential value`, `-c` | `$GOTTY_CREDENTIAL` | `credential` | `""` | Credential for standalone Basic Auth (`user:pass`) |
| `--random-url`, `-r` | `$GOTTY_RANDOM_URL` | `enable_random_url` | `false` | Append a random string to the server URL |
| `--random-url-length value` | `$GOTTY_RANDOM_URL_LENGTH` | `random_url_length` | `8` | Random URL token length |
| `--tls`, `-t` | `$GOTTY_TLS` | `enable_tls` | `false` | Enable standalone TLS/SSL |
| `--tls-crt value` | `$GOTTY_TLS_CRT` | `tls_crt_file` | `"~/.gotty.crt"` | TLS/SSL certificate file path |
| `--tls-key value` | `$GOTTY_TLS_KEY` | `tls_key_file` | `"~/.gotty.key"` | TLS/SSL private key file path |
| `--tls-ca-crt value` | `$GOTTY_TLS_CA_CRT` | `tls_ca_crt_file` | `"~/.gotty.ca.crt"` | CA certificate file for mutual TLS client authentication |
| `--ws-origin value` | `$GOTTY_WS_ORIGIN` | `ws_origin` | `""` | Regular expression matching allowed WebSocket `Origin` URLs |
| `--ping-interval value` | `$GOTTY_PING_INTERVAL` | `ping_interval` | `30` | WebSocket server ping interval in seconds (`0` to disable) |
| `--max-connection value` | `$GOTTY_MAX_CONNECTION` | `max_connection` | `0` | Maximum concurrent connections (`0` for unlimited) |

### Terminal & UI Options

| Flag | Env Var | HCL Key (`~/.gotty`) | Default | Description |
| :--- | :--- | :--- | :--- | :--- |
| `--title-format value` | `$GOTTY_TITLE_FORMAT` | `title_format` | `"Terminal"` | Browser window title template |
| `--enable-webgl` | `$GOTTY_ENABLE_WEBGL` | `enable_webgl` | `true` | Enable WebGL renderer in xterm.js |
| `--confirm-close` | `$GOTTY_CONFIRM_CLOSE` | `confirm_close` | `true` | Ask the browser to confirm before closing/reloading an active tab |
| `--mobile-keys` | `$GOTTY_MOBILE_KEYS` | `mobile_keys` | `true` | Show special keys bar (`Esc`, `Ctrl`, `Alt`, `Tab`, arrows) on touch devices |
| `--favicon value` | `$GOTTY_FAVICON` | `favicon` | `""` | Custom favicon (local file path, HTTP URL, or data URI) |
| `--config value` | `$GOTTY_CONFIG` | — | `"~/.gotty"` | Server HCL configuration file path |
| `--quiet` | `$GOTTY_QUIET` | `quiet` | `false` | Disable log output |
| `--version`, `-v` | — | — | — | Print version and exit |

---

## `godl` File Download Helper

`godl` is installed alongside `gotty` in `/usr/local/bin/godl`. Run it inside a GoTTY terminal session to send one or more files to your browser over OSC 7777 escape sequences:

```text
Usage: godl <filepath> [filepath2 ...]
```

### Example
```bash
godl ./logs.tar.gz
```
