export interface SessionTab {
    id: string;
    name: string;
    username?: string;
    mode?: "sandbox" | "host" | string;
    is_host_access?: boolean;
    attached_clients?: number;
    created_at?: string;
    connected?: boolean;
}

export interface UserConfig {
    version: string;
    terminal: {
        theme: string;
        font_size: number;
        font_family: string;
        cursor_blink: boolean;
        scrollback_lines: number;
    };
    session: {
        reap_timeout: "never" | "1h" | "2h" | "8h" | "24h";
        default_mode?: "sandbox" | "host";
    };
    keybindings: {
        copy_on_select: boolean;
        clipboard_shortcut?: "ctrl_shift" | "browser_default";
        close_tab_shortcut: string;
        new_tab_shortcut: string;
    };
}

export const DEFAULT_USER_CONFIG: UserConfig = {
    version: "1.0",
    terminal: {
        theme: "catppuccin-mocha",
        font_size: 14,
        font_family: "JetBrains Mono, Menlo, monospace",
        cursor_blink: true,
        scrollback_lines: 10000,
    },
    session: {
        reap_timeout: "never",
        default_mode: "sandbox",
    },
    keybindings: {
        copy_on_select: true,
        clipboard_shortcut: "ctrl_shift",
        close_tab_shortcut: "Alt+W",
        new_tab_shortcut: "Alt+T",
    },
};

export const TERMINAL_THEMES: Record<string, Record<string, string>> = {
    "catppuccin-mocha": {
        background: "#1e1e2e",
        foreground: "#cdd6f4",
        cursor: "#f5e0dc",
        cursorAccent: "#1e1e2e",
        selectionBackground: "#585b7066",
        black: "#45475a",
        red: "#f38ba8",
        green: "#a6e3a1",
        yellow: "#f9e2af",
        blue: "#89b4fa",
        magenta: "#f5c2e7",
        cyan: "#94e2d5",
        white: "#bac2de",
        brightBlack: "#585b70",
        brightRed: "#f38ba8",
        brightGreen: "#a6e3a1",
        brightYellow: "#f9e2af",
        brightBlue: "#89b4fa",
        brightMagenta: "#f5c2e7",
        brightCyan: "#94e2d5",
        brightWhite: "#a6adc8",
    },
    "nord": {
        background: "#2e3440",
        foreground: "#d8dee9",
        cursor: "#d8dee9",
        cursorAccent: "#2e3440",
        selectionBackground: "#434c5ecc",
        black: "#3b4252",
        red: "#bf616a",
        green: "#a3be8c",
        yellow: "#ebcb8b",
        blue: "#81a1c1",
        magenta: "#b48ead",
        cyan: "#88c0d0",
        white: "#e5e9f0",
        brightBlack: "#4c566a",
        brightRed: "#bf616a",
        brightGreen: "#a3be8c",
        brightYellow: "#ebcb8b",
        brightBlue: "#81a1c1",
        brightMagenta: "#b48ead",
        brightCyan: "#8fbcbb",
        brightWhite: "#eceff4",
    },
    "tokyo-night": {
        background: "#1a1b26",
        foreground: "#c0caf5",
        cursor: "#c0caf5",
        cursorAccent: "#1a1b26",
        selectionBackground: "#33467caa",
        black: "#15161e",
        red: "#f7768e",
        green: "#9ece6a",
        yellow: "#e0af68",
        blue: "#7aa2f7",
        magenta: "#bb9af7",
        cyan: "#7dcfff",
        white: "#a9b1d6",
        brightBlack: "#414868",
        brightRed: "#f7768e",
        brightGreen: "#9ece6a",
        brightYellow: "#e0af68",
        brightBlue: "#7aa2f7",
        brightMagenta: "#bb9af7",
        brightCyan: "#7dcfff",
        brightWhite: "#c0caf5",
    },
    "monokai": {
        background: "#272822",
        foreground: "#f8f8f2",
        cursor: "#f8f8f0",
        cursorAccent: "#272822",
        selectionBackground: "#49483eaa",
        black: "#272822",
        red: "#f92672",
        green: "#a6e22e",
        yellow: "#f4bf75",
        blue: "#66d9ef",
        magenta: "#ae81ff",
        cyan: "#a1efe4",
        white: "#f8f8f2",
        brightBlack: "#75715e",
        brightRed: "#f92672",
        brightGreen: "#a6e22e",
        brightYellow: "#f4bf75",
        brightBlue: "#66d9ef",
        brightMagenta: "#ae81ff",
        brightCyan: "#a1efe4",
        brightWhite: "#f9f8f5",
    },
    "solarized-dark": {
        background: "#002b36",
        foreground: "#839496",
        cursor: "#93a1a1",
        cursorAccent: "#002b36",
        selectionBackground: "#073642cc",
        black: "#073642",
        red: "#dc322f",
        green: "#859900",
        yellow: "#b58900",
        blue: "#268bd2",
        magenta: "#d33682",
        cyan: "#2aa198",
        white: "#eee8d5",
        brightBlack: "#002b36",
        brightRed: "#cb4b16",
        brightGreen: "#586e75",
        brightYellow: "#657b83",
        brightBlue: "#839496",
        brightMagenta: "#6c71c4",
        brightCyan: "#93a1a1",
        brightWhite: "#fdf6e3",
    },
};

export async function fetchSessions(): Promise<SessionTab[]> {
    const res = await fetch("./api/sessions", { credentials: "same-origin" });
    if (!res.ok) {
        throw new Error(`HTTP ${res.status}`);
    }
    return res.json();
}

export async function createSession(name?: string, mode?: string): Promise<SessionTab> {
    const res = await fetch("./api/sessions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ name: name || "", mode: mode || "" }),
    });
    if (!res.ok) {
        throw new Error(`HTTP ${res.status}`);
    }
    const data = await res.json();
    return {
        id: data.session_id || data.id,
        name: data.name || "Terminal",
        username: data.username,
        mode: data.mode,
        is_host_access: data.is_host_access,
        created_at: data.created_at,
    };
}

export async function deleteSession(id: string): Promise<void> {
    const res = await fetch(`./api/sessions/${encodeURIComponent(id)}`, {
        method: "DELETE",
        credentials: "same-origin",
    });
    if (!res.ok) {
        throw new Error(`HTTP ${res.status}`);
    }
}

export async function renameSession(id: string, name: string): Promise<void> {
    const res = await fetch(`./api/sessions/${encodeURIComponent(id)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ name }),
    });
    if (!res.ok) {
        throw new Error(`HTTP ${res.status}`);
    }
}

export async function uploadFileToSession(id: string, file: File): Promise<{ status: string; path: string }> {
    const form = new FormData();
    form.append("file", file, file.name);

    const res = await fetch(`./api/sessions/${encodeURIComponent(id)}/upload`, {
        method: "POST",
        credentials: "same-origin",
        body: form,
    });
    if (!res.ok) {
        const msg = await res.text();
        throw new Error(msg || `Upload failed (${res.status})`);
    }
    return res.json();
}

export async function fetchUserConfig(): Promise<UserConfig> {
    const res = await fetch("./api/user/config", { credentials: "same-origin" });
    if (!res.ok) {
        return structuredClone(DEFAULT_USER_CONFIG);
    }
    return res.json();
}

export async function saveUserConfig(cfg: UserConfig): Promise<UserConfig> {
    const res = await fetch("./api/user/config", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify(cfg),
    });
    if (!res.ok) {
        throw new Error(`Failed to save config (${res.status})`);
    }
    return res.json();
}

export interface PreviewEndpoint {
    host: string;
    label: string;
    iface?: string;
}

export interface PreviewTargetConfig {
    target_host: string;
    target_port: number;
    scheme: "auto" | "http" | "https" | string;
    forwarder_port: number;
    forwarder_active: boolean;
    preview_url: string;
}

export interface PreviewState {
    active: PreviewTargetConfig;
    endpoints: PreviewEndpoint[];
}

export const DEFAULT_PREVIEW_ENDPOINTS: PreviewEndpoint[] = [
    { host: "localhost", label: "localhost (127.0.0.1)", iface: "lo" },
];

export async function fetchPreviewState(): Promise<PreviewState> {
    const res = await fetch("./api/preview", { credentials: "same-origin" });
    if (!res.ok) {
        return {
            active: {
                target_host: "localhost",
                target_port: 3000,
                scheme: "auto",
                forwarder_port: 3000,
                forwarder_active: false,
                preview_url: "./preview/localhost:3000/",
            },
            endpoints: DEFAULT_PREVIEW_ENDPOINTS,
        };
    }
    return res.json();
}

export async function configurePreviewTarget(
    target_host: string,
    target_port: number,
    scheme: string = "auto"
): Promise<PreviewTargetConfig> {
    const res = await fetch("./api/preview", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ target_host, target_port, scheme }),
    });
    if (!res.ok) {
        const msg = await res.text();
        throw new Error(msg || `Failed to configure preview (${res.status})`);
    }
    const data = await res.json();
    return data.active;
}
