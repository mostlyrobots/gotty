import { describe, it, expect, vi } from "vitest";
import {
  POPULAR_MONO_FONTS,
  SYSTEM_DEFAULTS,
  isMonospaceFont,
  isFontAvailable,
  detectInstalledPopularFonts,
  introspectSystemFonts,
} from "../fonts";

describe("Font Introspection Utilities", () => {
  it("provides popular mono fonts and system defaults", () => {
    expect(POPULAR_MONO_FONTS.length).toBeGreaterThan(15);
    expect(POPULAR_MONO_FONTS).toContain("JetBrains Mono");
    expect(POPULAR_MONO_FONTS).toContain("Fira Code");
    expect(SYSTEM_DEFAULTS.length).toBe(2);
  });

  it("evaluates monospace character width consistency", () => {
    // Mock canvas context
    const origCreateElement = document.createElement.bind(document);
    vi.spyOn(document, "createElement").mockImplementation((tagName: string) => {
      if (tagName === "canvas") {
        return {
          getContext: () => ({
            font: "",
            measureText: (text: string) => {
              // Simulated monospace: every char is 10px
              return { width: text.length * 10 };
            },
          }),
        } as unknown as HTMLCanvasElement;
      }
      return origCreateElement(tagName);
    });

    expect(isMonospaceFont("JetBrains Mono")).toBe(true);
    vi.restoreAllMocks();
  });

  it("detects proportional fonts and rejects them from monospace check", () => {
    vi.spyOn(document, "createElement").mockImplementation((tagName: string) => {
      if (tagName === "canvas") {
        return {
          getContext: () => ({
            font: "",
            measureText: (text: string) => {
              // Simulated proportional: "i" is narrow, "W" is wide
              if (text === "i") return { width: 5 };
              if (text === "W") return { width: 25 };
              return { width: 12 };
            },
          }),
        } as unknown as HTMLCanvasElement;
      }
      return document.createElement(tagName);
    });

    expect(isMonospaceFont("Arial")).toBe(false);
    vi.restoreAllMocks();
  });

  it("introspects fonts via queryLocalFonts API when available", async () => {
    const mockFonts = [
      { family: "JetBrains Mono" },
      { family: "Arial" },
      { family: "Fira Code" },
    ];

    (window as any).queryLocalFonts = vi.fn().mockResolvedValue(mockFonts);

    // Mock monospace check
    vi.spyOn(document, "createElement").mockImplementation((tagName: string) => {
      if (tagName === "canvas") {
        return {
          getContext: () => ({
            font: "",
            measureText: (text: string) => {
              return { width: text.length * 10 };
            },
          }),
        } as unknown as HTMLCanvasElement;
      }
      return document.createElement(tagName);
    });

    const result = await introspectSystemFonts();
    expect(result).toContain("JetBrains Mono");
    expect(result).toContain("Fira Code");
    expect((window as any).queryLocalFonts).toHaveBeenCalled();

    delete (window as any).queryLocalFonts;
    vi.restoreAllMocks();
  });
});
