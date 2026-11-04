# Authentication & Security

> [!IMPORTANT]
> **GoTTY is not intended to be exposed directly to untrusted networks on its own.**  
> It is built to run behind a frontend reverse proxy—such as [Nginx Proxy Manager (NPM)](https://nginxproxymanager.com/), Authelia, Authentik, Caddy, or OAuth2 Proxy—that terminates SSL/TLS, authenticates the user, and forwards the authenticated user identity to GoTTY.

---

## 1. Frontend Reverse Proxy Setup ([Nginx Proxy Manager](https://nginxproxymanager.com/))

In a standard deployment, GoTTY listens only on `127.0.0.1:8080` (or an internal container network) with `--proxy-auth` enabled:

```bash
gotty --address 127.0.0.1 --port 8080 --permit-write --proxy-auth \
  --container-runtime podman --container-image gotty-sandbox:latest /bin/bash
```

### Configuring [Nginx Proxy Manager](https://nginxproxymanager.com/)
1. **Details Tab**:
   - **Scheme**: `http`
   - **Forward Hostname / IP**: `127.0.0.1` (or the host's internal container bridge IP)
   - **Forward Port**: `8080`
   - **Websockets Support**: Enabled
2. **SSL Tab**:
   - Select or request a Let's Encrypt TLS certificate and enable **Force SSL**.
3. **Access List Tab**:
   - Assign an NPM Access List (Basic Auth) or external authentication forwarder so unauthenticated requests are rejected before reaching GoTTY.
4. **Advanced Tab**:
   - Pass the authenticated username/email and preserve the `Host` header:
     ```nginx
     proxy_set_header Host $host;
     proxy_set_header X-Forwarded-Host $host;
     proxy_set_header X-Forwarded-User $remote_user;
     client_max_body_size 512M;
     ```

---

## 2. Reverse-Proxy Identity Resolution (`/etc/passwd`)

When `--proxy-auth` is enabled, GoTTY reads the user identity from the request headers ([`server/api_handlers.go`](../server/api_handlers.go)).

If `--proxy-auth-header <Header-Name>` is configured, GoTTY only checks that single header. Otherwise, it checks the following headers in order:

1. `X-Forwarded-User`
2. `X-Forwarded-Email`
3. `Remote-User`
4. `Remote-Email`
5. `X-Email`
6. `X-User`
7. `X-Auth-Request-Email`
8. `X-Auth-Request-User`
9. `X-Forwarded-Preferred-Username`

> [!NOTE]
> If `--proxy-auth` is **not** enabled, GoTTY ignores all proxy identity headers so direct clients cannot spoof `X-Forwarded-User`.

### Matching Against `/etc/passwd`
[`auth.Resolver`](../pkg/auth/passwd.go) resolves the header value against `/etc/passwd`:
- **Username Match**: Exact match against the 1st field of `/etc/passwd` (e.g. `devuser`).
- **GECOS Email Match**: Case-insensitive match against any comma-separated subfield or `<email>` inside the 5th field (GECOS) of `/etc/passwd`.

To associate an email address with a Linux user:
```bash
sudo usermod -c "Dev User,devuser@example.com" devuser
```

If the header is missing (and `--default-user` is not set) or does not match an entry in `/etc/passwd`, GoTTY returns `403 Forbidden`.

---

## 3. Privilege Mapping (`/etc/group`)

After resolving the user in `/etc/passwd`, GoTTY checks `/etc/group` ([`pkg/auth/passwd.go`](../pkg/auth/passwd.go)):

| Group Membership | `is_host_access` | Allowed Session Modes | Web Preview Scope |
| :--- | :---: | :--- | :--- |
| Member of `wheel`, `sudo`, or `gotty-host` (or `UID 0`) | `true` | `sandbox` and `host` | `localhost` and local private network interfaces |
| All other Linux users | `false` | `sandbox` only | `localhost` (`127.0.0.1`) only |

You can customize which groups grant host access using `--host-groups` (or `host_groups` in `~/.gotty`).

To grant a user host shell access under the default configuration:
```bash
sudo groupadd -f gotty-host
sudo usermod -aG gotty-host devuser
```

---

## 4. Security Controls in the Server

- **Proxy Header Isolation**: Proxy identity headers are only read when `--proxy-auth` is enabled.
- **WebSocket Session Ownership**: When `--proxy-auth` is enabled, `/ws` connections must pass a `session_id` belonging to the authenticated user.
- **Same-Origin Check**: WebSocket upgrades (`/ws`) and state-changing REST requests (`POST`, `PUT`, `PATCH`, `DELETE`) validate that any `Origin` header matches `Host` / `X-Forwarded-Host` (or `--ws-origin`).
- **Confined File Uploads**: `POST /api/sessions/:id/upload` validates symlink targets with `filepath.EvalSymlinks` to keep sandboxed uploads inside the user's home directory, writes files through Go's root-scoped `os.OpenRoot` API with `O_NOFOLLOW`, and `chown`s files to the user's UID/GID.
- **Preview SSRF & Privilege Restrictions**: The `/preview/` reverse proxy blocks public internet IPs and link-local cloud metadata (`169.254.169.254`), and restricts sandboxed users (`!is_host_access`) to loopback addresses.
- **Constant-Time Basic Auth**: If standalone Basic Auth (`-c user:pass`) is used, credentials are verified with `subtle.ConstantTimeCompare`.
