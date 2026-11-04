import { describe, it, expect, vi } from "vitest";
import { Osc7777Receiver, sanitizeDownloadFilename, type DownloadEvent } from "../osc7777";

function toB64(s: string): string {
    return btoa(s);
}

describe("Osc7777Receiver (godl protocol)", () => {
    it("parses single-frame OSC 7777 download sequence", () => {
        const events: DownloadEvent[] = [];
        const receiver = new Osc7777Receiver((dl) => events.push(dl));

        const filename = toB64("network.conf");
        const content = "interface eth0\naddress 192.168.1.1\n";
        const data = toB64(content);

        const handled = receiver.handle(`download;filename=${filename};size=${content.length};data=${data}`);
        expect(handled).toBe(true);
        expect(events).toHaveLength(1);
        expect(events[0].filename).toBe("network.conf");
        expect(events[0].size).toBe(content.length);
        expect(new TextDecoder().decode(events[0].data)).toBe(content);
    });

    it("reassembles multi-chunk OSC 7777 download sequence in order", () => {
        const events: DownloadEvent[] = [];
        const receiver = new Osc7777Receiver((dl) => events.push(dl));

        const filename = toB64("archive.bin");
        const part0 = "FIRST_CHUNK_";
        const part1 = "SECOND_CHUNK";

        receiver.handle(
            `download;id=tx123;chunk=1;total=2;filename=${filename};size=24;data=${toB64(part1)}`
        );
        expect(events).toHaveLength(0);

        receiver.handle(
            `download;id=tx123;chunk=0;total=2;filename=${filename};size=24;data=${toB64(part0)}`
        );
        expect(events).toHaveLength(1);
        expect(events[0].filename).toBe("archive.bin");
        expect(new TextDecoder().decode(events[0].data)).toBe("FIRST_CHUNK_SECOND_CHUNK");
    });

    it("ignores malformed OSC 7777 payloads gracefully", () => {
        const cb = vi.fn();
        const receiver = new Osc7777Receiver(cb);

        expect(receiver.handle("unknown;foo=bar")).toBe(true);
        expect(receiver.handle("download;filename=abc")).toBe(true);
        expect(cb).not.toHaveBeenCalled();
    });

    it("enforces bounded pending transfer queue and rejects invalid chunk bounds", () => {
        const cb = vi.fn();
        const receiver = new Osc7777Receiver(cb);
        const filename = toB64("test.bin");
        const data = toB64("chunk");

        // Negative or out-of-range chunk indices must be ignored
        receiver.handle(`download;id=bad1;chunk=-1;total=2;filename=${filename};size=10;data=${data}`);
        receiver.handle(`download;id=bad2;chunk=2;total=2;filename=${filename};size=10;data=${data}`);

        // Flooding with 12 incomplete transfers evicts oldest beyond MAX_PENDING_TRANSFERS (8)
        for (let i = 0; i < 12; i++) {
            receiver.handle(`download;id=flood-${i};chunk=0;total=2;filename=${filename};size=10;data=${data}`);
        }
        // Completing flood-0 now should not fire because it was evicted
        receiver.handle(`download;id=flood-0;chunk=1;total=2;filename=${filename};size=10;data=${data}`);
        expect(cb).not.toHaveBeenCalled();

        // Completing flood-11 (still in queue) succeeds
        receiver.handle(`download;id=flood-11;chunk=1;total=2;filename=${filename};size=10;data=${data}`);
        expect(cb).toHaveBeenCalledTimes(1);
    });

    it("sanitizes dangerous and traversal filenames in OSC 7777 downloads", () => {
        expect(sanitizeDownloadFilename("../../.bashrc")).toBe("_bashrc");
        expect(sanitizeDownloadFilename("C:\\Users\\admin\\payload.exe")).toBe("payload.exe.download");
        expect(sanitizeDownloadFilename("evil.sh")).toBe("evil.sh.download");
        expect(sanitizeDownloadFilename("report.tar.gz")).toBe("report.tar.gz");
    });
});
