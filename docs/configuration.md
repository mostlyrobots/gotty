# Configuration & Customization

GoTTY uses two configuration files:
1. **Per-User Preferences (`~/.gotty/config.yaml`)**: Updated from the in-app **Settings** drawer (`PUT /api/user/config`) and stored in each user's home directory.
2. **Server Configuration (`~/.gotty`)**: Optional HCL file loaded by the `gotty` server process at startup (override path with `--config`).

---

## 1. Per-User Configuration (`~/.gotty/config.yaml`)

When a user clicks **Save Preferences** in the Settings drawer, GoTTY writes `/home/<username>/.gotty/config.yaml`.

### Example `~/.gotty/config.yaml`
```yaml
version: "1.0"
terminal:
  theme: "catppuccin-mocha"
  font_size: 14
  font_family: "'JetBrains Mono', monospace"
  cursor_blink: true
  scrollback_lines: 10000
session:
  reap_timeout: "never"
  default_mode: "sandbox"
keybindings:
  copy_on_select: true
  clipboard_shortcut: "ctrl_shift"
  close_tab_shortcut: "Alt+W"
  new_tab_shortcut: "Alt+T"
```

### Fields

#### `terminal`
- `theme`: Terminal color palette. Built-in options in the UI are:
  - `catppuccin-mocha` (default)
  - `tokyo-night`
  - `nord`
  - `monokai`
  - `solarized-dark`
- `font_size`: Font size in pixels (`8`–`32`, default `14`).
- `font_family`: CSS font-family string passed to xterm.js. The Settings drawer lists preloaded web fonts (`JetBrains Mono`, `Fira Code`, `Source Code Pro`, `IBM Plex Mono`, `Ubuntu Mono`, `Inconsolata`, `Space Mono`, `Roboto Mono`, `Anonymous Pro`, `Cousine`, `Share Tech Mono`), locally detected system monospace fonts, or a custom font string.
- `cursor_blink`: Whether the terminal cursor blinks (`true` or `false`).
- `scrollback_lines`: Number of lines retained in the client-side xterm.js scrollback buffer (default `10000`).

#### `session`
- `reap_timeout`: How long a detached session (with zero connected browser tabs) remains running on the server before being terminated:
  - `never` (default — runs until explicitly closed or server restart)
  - `1h`, `2h`, `8h`, `24h` (or any valid Go duration string such as `30m`)
- `default_mode`: Default execution mode when clicking the `+` new-tab button (`sandbox` or `host`). Note that `host` only takes effect if the user belongs to a permitted host group (`wheel`, `sudo`, `gotty-host`).

#### `keybindings`
- `copy_on_select`: When `true`, selecting text in the terminal copies it to the clipboard and shows a `Copied` indicator.
- `clipboard_shortcut`:
  - `ctrl_shift`: Handles `Ctrl+Shift+C` (copy) and `Ctrl+Shift+V` (paste) inside the terminal.
  - `browser_default`: Leaves keyboard copy/paste shortcuts to the browser/OS.
- `new_tab_shortcut`: Shortcut to open a new session tab (default `Alt+T`).
- `close_tab_shortcut`: Shortcut to close and terminate the active tab (default `Alt+W`).

---

## 2. Renaming Terminal Tabs

New tabs default to the title `Terminal` (or `Session N`). Tabs can be renamed in three ways:
1. **Double-click** the tab title in the top bar, type a new name, and press `Enter`.
2. Click the **pencil icon** on the active tab.
3. Emit a standard OSC 2 window title escape sequence from inside the shell:
   ```bash
   printf '\033]2;Build Server\007'
   ```

---

## 3. Server Configuration File (`~/.gotty`)

Server-wide flags can be placed in `~/.gotty` (HCL format) instead of passing them on the command line:

```hcl
address = "127.0.0.1"
port = "8080"
permit_write = true
enable_proxy_auth = true
proxy_auth_header = "X-Forwarded-User"
host_groups = "wheel,sudo,gotty-host"
container_runtime = "podman"
container_image = "gotty-sandbox:latest"
default_mode = "sandbox"
confirm_close = true
mobile_keys = true
```
