// GoTTY - Font Family Introspection Utilities
// Discovers installed monospace fonts via Local Font Access API, Google Web Fonts, and Canvas measurement
// Authors: Google Gemini 4 Argon & the original GoTTY project

export const GOOGLE_WEB_FONTS = [
  "JetBrains Mono",
  "Fira Code",
  "Source Code Pro",
  "Roboto Mono",
  "Inconsolata",
  "Space Mono",
  "Ubuntu Mono",
  "IBM Plex Mono",
  "Anonymous Pro",
  "Courier Prime",
] as const;

export const POPULAR_SYSTEM_FONTS = [
  "Cascadia Code",
  "Cascadia Mono",
  "Consolas",
  "Menlo",
  "Monaco",
  "SF Mono",
  "DejaVu Sans Mono",
  "Liberation Mono",
  "Hack",
  "Courier New",
] as const;

export const POPULAR_MONO_FONTS = [
  ...GOOGLE_WEB_FONTS,
  ...POPULAR_SYSTEM_FONTS,
] as const;

export const SYSTEM_DEFAULTS = [
  { label: "GoTTY Default (JetBrains Mono / Menlo)", value: "JetBrains Mono, Menlo, monospace" },
  { label: "System Default Monospace", value: "ui-monospace, monospace" },
] as const;

/**
 * Checks whether characters in the given font family have identical advance width (monospace).
 */
export function isMonospaceFont(fontFamily: string): boolean {
  if (typeof document === "undefined") return true;

  const canvas = document.createElement("canvas");
  const ctx = canvas.getContext("2d");
  if (!ctx) return true;

  ctx.font = `72px "${fontFamily}", monospace`;
  const wI = ctx.measureText("i").width;
  const wW = ctx.measureText("W").width;
  const wM = ctx.measureText("m").width;
  const wDot = ctx.measureText(".").width;

  const tolerance = 0.05;
  return (
    Math.abs(wI - wW) < tolerance &&
    Math.abs(wW - wM) < tolerance &&
    Math.abs(wM - wDot) < tolerance
  );
}

/**
 * Probes whether a font is installed by comparing canvas text measurements against generic fallback fonts.
 */
export function isFontAvailable(fontFamily: string): boolean {
  if (typeof document === "undefined") return false;

  const canvas = document.createElement("canvas");
  const ctx = canvas.getContext("2d");
  if (!ctx) return false;

  const testString = "mmmmmmmmmmlli!@#$%^&*()1234567890";
  const size = "72px";

  // Measure baselines
  ctx.font = `${size} monospace`;
  const baseMono = ctx.measureText(testString).width;

  ctx.font = `${size} sans-serif`;
  const baseSans = ctx.measureText(testString).width;

  ctx.font = `${size} serif`;
  const baseSerif = ctx.measureText(testString).width;

  // Measure font with fallbacks
  ctx.font = `${size} "${fontFamily}", monospace`;
  const testMono = ctx.measureText(testString).width;

  ctx.font = `${size} "${fontFamily}", sans-serif`;
  const testSans = ctx.measureText(testString).width;

  ctx.font = `${size} "${fontFamily}", serif`;
  const testSerif = ctx.measureText(testString).width;

  return testMono !== baseMono || testSans !== baseSans || testSerif !== baseSerif;
}

/**
 * Checks the curated list of popular monospace fonts and returns those installed on the system.
 */
export function detectInstalledPopularFonts(): string[] {
  const detected: string[] = [];
  for (const font of POPULAR_SYSTEM_FONTS) {
    if (isFontAvailable(font)) {
      detected.push(font);
    }
  }
  return detected;
}

/**
 * Uses the modern Local Font Access API (window.queryLocalFonts) if supported by the browser,
 * filters for monospace fonts using character-width inspection, and returns font family names.
 */
export async function introspectSystemFonts(): Promise<string[]> {
  if (typeof window === "undefined") return [];

  const queryLocalFonts = (window as unknown as { queryLocalFonts?: () => Promise<Array<{ family: string }>> }).queryLocalFonts;
  if (typeof queryLocalFonts === "function") {
    try {
      const fonts = await queryLocalFonts();
      const uniqueFamilies = new Set<string>();
      for (const f of fonts) {
        if (f.family && !uniqueFamilies.has(f.family)) {
          uniqueFamilies.add(f.family);
        }
      }

      const monoFonts: string[] = [];
      for (const family of uniqueFamilies) {
        if (isMonospaceFont(family)) {
          monoFonts.push(family);
        }
      }
      return monoFonts.sort((a, b) => a.localeCompare(b));
    } catch {
      // Permission denied or API aborted
    }
  }

  return [];
}
