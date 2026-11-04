import { describe, it, expect, vi, beforeEach } from "vitest";
import {
    DEFAULT_PREVIEW_ENDPOINTS,
    DEFAULT_USER_CONFIG,
    TERMINAL_THEMES,
    createSession,
    deleteSession,
    fetchPreviewState,
    configurePreviewTarget,
    fetchSessions,
    fetchUserConfig,
    saveUserConfig,
    uploadFileToSession,
} from "../api";

describe("Session & User Config REST API client", () => {
    beforeEach(() => {
        vi.restoreAllMocks();
    });

    it("provides all required terminal themes from blueprint", () => {
        expect(Object.keys(TERMINAL_THEMES)).toEqual(
            expect.arrayContaining([
                "catppuccin-mocha",
                "nord",
                "tokyo-night",
                "monokai",
                "solarized-dark",
            ])
        );
    });

    it("fetches active sessions from GET ./api/sessions", async () => {
        const mockSessions = [
            { id: "uuid-1", name: "Session 1", username: "devuser", is_host_access: true },
        ];
        globalThis.fetch = vi.fn().mockResolvedValue({
            ok: true,
            json: async () => mockSessions,
        } as any);

        const sessions = await fetchSessions();
        expect(sessions).toEqual(mockSessions);
        expect(globalThis.fetch).toHaveBeenCalledWith("./api/sessions", expect.any(Object));
    });

    it("creates a new session via POST ./api/sessions", async () => {
        globalThis.fetch = vi.fn().mockResolvedValue({
            ok: true,
            json: async () => ({
                session_id: "uuid-2",
                name: "Router CLI",
                username: "devuser",
                is_host_access: true,
            }),
        } as any);

        const tab = await createSession("Router CLI");
        expect(tab.id).toBe("uuid-2");
        expect(tab.name).toBe("Router CLI");
        expect(tab.is_host_access).toBe(true);
    });

    it("deletes a session via DELETE ./api/sessions/:id", async () => {
        globalThis.fetch = vi.fn().mockResolvedValue({ ok: true } as any);
        await deleteSession("uuid-2");
        expect(globalThis.fetch).toHaveBeenCalledWith(
            "./api/sessions/uuid-2",
            expect.objectContaining({ method: "DELETE" })
        );
    });

    it("uploads a file via POST ./api/sessions/:id/upload", async () => {
        globalThis.fetch = vi.fn().mockResolvedValue({
            ok: true,
            json: async () => ({ status: "ok", path: "/home/devuser/work/test.txt" }),
        } as any);

        const file = new File(["hello"], "test.txt", { type: "text/plain" });
        const res = await uploadFileToSession("uuid-1", file);
        expect(res.path).toBe("/home/devuser/work/test.txt");
    });

    it("loads and saves UserConfig via /api/user/config", async () => {
        const customCfg = structuredClone(DEFAULT_USER_CONFIG);
        customCfg.terminal.theme = "nord";
        customCfg.session.reap_timeout = "8h";

        globalThis.fetch = vi.fn().mockResolvedValue({
            ok: true,
            json: async () => customCfg,
        } as any);

        const saved = await saveUserConfig(customCfg);
        expect(saved.terminal.theme).toBe("nord");
        expect(saved.session.reap_timeout).toBe("8h");

        const loaded = await fetchUserConfig();
        expect(loaded.terminal.theme).toBe("nord");
    });

    it("fetches and configures Web Preview endpoints via /api/preview without hardcoded LAN IPs", async () => {
        expect(DEFAULT_PREVIEW_ENDPOINTS).toEqual([
            { host: "localhost", label: "localhost (127.0.0.1)", iface: "lo" },
        ]);

        globalThis.fetch = vi.fn().mockResolvedValueOnce({
            ok: true,
            json: async () => ({
                active: {
                    target_host: "localhost",
                    target_port: 8080,
                    scheme: "auto",
                    forwarder_port: 8080,
                    forwarder_active: true,
                    preview_url: "./preview/localhost:8080/",
                },
                endpoints: [
                    { host: "localhost", label: "localhost (127.0.0.1)", iface: "lo" },
                    { host: "192.168.1.50", label: "192.168.1.50 (eth0)", iface: "eth0" },
                ],
            }),
        } as any);

        const state = await fetchPreviewState();
        expect(state.active.target_port).toBe(8080);
        expect(state.endpoints.map((e) => e.host)).toEqual(["localhost", "192.168.1.50"]);

        globalThis.fetch = vi.fn().mockResolvedValueOnce({
            ok: true,
            json: async () => ({
                status: "ok",
                active: {
                    target_host: "192.168.1.50",
                    target_port: 3000,
                    scheme: "http",
                    forwarder_port: 8080,
                    forwarder_active: true,
                    preview_url: "./preview/192.168.1.50:3000/",
                },
            }),
        } as any);

        const updated = await configurePreviewTarget("192.168.1.50", 3000, "http");
        expect(updated.target_host).toBe("192.168.1.50");
        expect(updated.target_port).toBe(3000);
    });
});
