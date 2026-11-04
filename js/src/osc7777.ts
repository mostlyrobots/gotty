export interface DownloadEvent {
    filename: string;
    size: number;
    data: Uint8Array;
}

export const MAX_PENDING_TRANSFERS = 8;
export const MAX_TOTAL_DOWNLOAD_BYTES = 100 * 1024 * 1024; // 100MB max reassembled file size
export const TRANSFER_TTL_MS = 60_000; // 60 seconds TTL for incomplete multi-chunk transfers

interface ChunkState {
    filename: string;
    total: number;
    size: number;
    updatedAt: number;
    accumulatedBytes: number;
    chunks: Map<number, Uint8Array>;
}

function decodeBase64Bytes(b64: string): Uint8Array {
    const bin = atob(b64);
    const bytes = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) {
        bytes[i] = bin.charCodeAt(i);
    }
    return bytes;
}

function decodeBase64UTF8(b64: string): string {
    return new TextDecoder().decode(decodeBase64Bytes(b64));
}

const DANGEROUS_DOWNLOAD_EXTENSIONS = /\.(exe|msi|bat|cmd|ps1|sh|vbs|scr|app|command|desktop|html|htm|svg)$/i;

export function sanitizeDownloadFilename(raw: string): string {
    // Strip path traversal segments, control characters, and Unicode bidi overrides
    const base = raw.replace(/\\/g, "/").split("/").pop() || "";
    let cleaned = base.replace(/[\x00-\x1f\x7f\u202a-\u202e\u2066-\u2069]/g, "").trim();
    if (!cleaned || cleaned === "." || cleaned === "..") {
        return "download";
    }
    if (cleaned.startsWith(".")) {
        cleaned = "_" + cleaned.slice(1);
    }
    if (DANGEROUS_DOWNLOAD_EXTENSIONS.test(cleaned)) {
        cleaned += ".download";
    }
    return cleaned;
}

export function triggerBrowserDownload(filename: string, data: Uint8Array): void {
    const safeName = sanitizeDownloadFilename(filename);
    const blob = new Blob([data as unknown as BlobPart], { type: "application/octet-stream" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = safeName;
    a.style.display = "none";
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    setTimeout(() => URL.revokeObjectURL(url), 5000);
}

export class Osc7777Receiver {
    private pending = new Map<string, ChunkState>();
    private onComplete: (dl: DownloadEvent) => void;

    constructor(onComplete: (dl: DownloadEvent) => void) {
        this.onComplete = onComplete;
    }

    private evictExpired(now: number): void {
        for (const [id, state] of this.pending.entries()) {
            if (now - state.updatedAt > TRANSFER_TTL_MS) {
                this.pending.delete(id);
            }
        }
    }

    /**
     * Handles the payload inside \x1b]7777;<payload>\x07
     * Returns true so xterm.js suppresses rendering the sequence.
     */
    handle(rawPayload: string): boolean {
        const parts = rawPayload.split(";");
        if (parts.length < 2 || parts[0] !== "download") {
            return true;
        }

        const kv: Record<string, string> = {};
        for (let i = 1; i < parts.length; i++) {
            const eq = parts[i].indexOf("=");
            if (eq > 0) {
                kv[parts[i].slice(0, eq)] = parts[i].slice(eq + 1);
            }
        }

        if (!kv.filename || kv.data === undefined) {
            return true;
        }

        try {
            const now = Date.now();
            this.evictExpired(now);

            const filename = sanitizeDownloadFilename(decodeBase64UTF8(kv.filename));
            const chunkBytes = decodeBase64Bytes(kv.data);
            const totalSize = kv.size ? parseInt(kv.size, 10) : chunkBytes.byteLength;

            if (
                Number.isNaN(totalSize) ||
                totalSize < 0 ||
                totalSize > MAX_TOTAL_DOWNLOAD_BYTES ||
                chunkBytes.byteLength > MAX_TOTAL_DOWNLOAD_BYTES
            ) {
                return true;
            }

            // Single-frame download
            if (!kv.id || kv.total === undefined || kv.chunk === undefined) {
                this.onComplete({
                    filename,
                    size: totalSize,
                    data: chunkBytes,
                });
                return true;
            }

            // Multi-chunk download
            const id = kv.id;
            const seq = parseInt(kv.chunk, 10);
            const total = parseInt(kv.total, 10);

            if (Number.isNaN(seq) || Number.isNaN(total) || seq < 0 || total <= 0 || seq >= total || total > 4096) {
                return true;
            }

            let state = this.pending.get(id);
            if (!state) {
                if (this.pending.size >= MAX_PENDING_TRANSFERS) {
                    const oldestKey = this.pending.keys().next().value;
                    if (oldestKey !== undefined) {
                        this.pending.delete(oldestKey);
                    }
                }
                state = {
                    filename,
                    total,
                    size: totalSize,
                    updatedAt: now,
                    accumulatedBytes: 0,
                    chunks: new Map<number, Uint8Array>(),
                };
                this.pending.set(id, state);
            }

            const existingChunk = state.chunks.get(seq);
            const nextBytes =
                state.accumulatedBytes - (existingChunk ? existingChunk.byteLength : 0) + chunkBytes.byteLength;
            if (nextBytes > MAX_TOTAL_DOWNLOAD_BYTES) {
                this.pending.delete(id);
                return true;
            }

            state.updatedAt = now;
            state.accumulatedBytes = nextBytes;
            state.chunks.set(seq, chunkBytes);

            if (state.chunks.size === state.total) {
                let combinedLength = 0;
                for (let i = 0; i < state.total; i++) {
                    const c = state.chunks.get(i);
                    if (!c) return true;
                    combinedLength += c.byteLength;
                }

                const merged = new Uint8Array(combinedLength);
                let offset = 0;
                for (let i = 0; i < state.total; i++) {
                    const c = state.chunks.get(i)!;
                    merged.set(c, offset);
                    offset += c.byteLength;
                }

                this.pending.delete(id);
                this.onComplete({
                    filename: state.filename,
                    size: combinedLength,
                    data: merged,
                });
            }
        } catch (err) {
            console.error("Failed to process OSC 7777 download frame:", err);
        }

        return true;
    }
}
