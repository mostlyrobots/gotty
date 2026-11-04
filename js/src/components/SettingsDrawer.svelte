<script lang="ts">
  import { fly, fade } from "svelte/transition";
  import { X, Save, Sparkles } from "lucide-svelte";
  import { DEFAULT_USER_CONFIG, TERMINAL_THEMES, type UserConfig } from "../api";
  import {
    GOOGLE_WEB_FONTS,
    POPULAR_SYSTEM_FONTS,
    SYSTEM_DEFAULTS,
    detectInstalledPopularFonts,
    introspectSystemFonts,
  } from "../fonts";

  interface Props {
    open: boolean;
    config: UserConfig;
    onClose: () => void;
    onSave: (cfg: UserConfig) => Promise<void>;
  }

  let { open, config, onClose, onSave }: Props = $props();
  let draft = $state<UserConfig>(structuredClone(DEFAULT_USER_CONFIG));
  let saving = $state(false);

  let detectedFonts = $state<string[]>([]);
  let systemScanned = $state(false);
  let scanning = $state(false);
  let customMode = $state(false);
  let hasLocalFontApi = $state(false);

  function normalizeFont(f: string): string {
    return f.replace(/['"]/g, "").trim().toLowerCase();
  }

  function getSelectedFontValue(): string {
    if (customMode) return "__custom__";
    const current = (draft.terminal.font_family || "").trim();
    for (const d of SYSTEM_DEFAULTS) {
      if (d.value === current) return d.value;
    }
    const currentNorm = normalizeFont(current.split(",")[0]);
    for (const f of GOOGLE_WEB_FONTS) {
      if (normalizeFont(f) === currentNorm) return `'${f}', monospace`;
    }
    for (const f of detectedFonts) {
      if (normalizeFont(f) === currentNorm) return `'${f}', monospace`;
    }
    for (const f of POPULAR_SYSTEM_FONTS) {
      if (normalizeFont(f) === currentNorm) return `'${f}', monospace`;
    }
    return "__custom__";
  }

  function handleFontSelect(e: Event) {
    const val = (e.target as HTMLSelectElement).value;
    if (val === "__custom__") {
      customMode = true;
    } else {
      customMode = false;
      draft.terminal.font_family = val;
    }
  }

  async function scanSystemFonts() {
    if (scanning) return;
    scanning = true;
    try {
      const systemMonoFonts = await introspectSystemFonts();
      if (systemMonoFonts.length > 0) {
        const set = new Set([...detectedFonts, ...systemMonoFonts]);
        detectedFonts = Array.from(set).sort((a, b) => a.localeCompare(b));
        systemScanned = true;
      }
    } finally {
      scanning = false;
    }
  }

  $effect(() => {
    if (open) {
      draft = structuredClone(config);
      hasLocalFontApi = typeof window !== "undefined" && "queryLocalFonts" in window;
      const detected = detectInstalledPopularFonts();
      detectedFonts = detected;
      const val = getSelectedFontValue();
      customMode = val === "__custom__";
    }
  });

  async function handleSubmit(e: Event) {
    e.preventDefault();
    saving = true;
    try {
      await onSave(draft);
      onClose();
    } finally {
      saving = false;
    }
  }

  const themeOptions = Object.keys(TERMINAL_THEMES);
  const reapOptions: Array<{ value: UserConfig["session"]["reap_timeout"]; label: string }> = [
    { value: "never", label: "Never (Keep Running)" },
    { value: "1h", label: "1 Hour" },
    { value: "2h", label: "2 Hours" },
    { value: "8h", label: "8 Hours" },
    { value: "24h", label: "24 Hours (1 Day)" },
  ];
</script>

{#if open}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    transition:fade={{ duration: 150 }}
    class="fixed inset-0 z-40 bg-black/50 backdrop-blur-xs"
    onclick={onClose}
  ></div>

  <aside
    transition:fly={{ x: 340, duration: 220 }}
    class="fixed top-0 right-0 bottom-0 z-50 w-80 max-w-full bg-[#181825] border-l border-[#313244] text-[#cdd6f4] shadow-2xl flex flex-col"
    data-testid="settings-drawer"
  >
    <div class="flex items-center justify-between px-4 py-3 border-b border-[#313244]">
      <h2 class="text-sm font-semibold tracking-wide">User Preferences (~/.gotty/config.yaml)</h2>
      <button
        type="button"
        onclick={onClose}
        class="p-1 rounded hover:bg-[#313244] text-[#a6adc8] hover:text-[#cdd6f4]"
      >
        <X class="w-4 h-4" />
      </button>
    </div>

    <form onsubmit={handleSubmit} class="flex-1 overflow-y-auto p-4 space-y-5 text-xs">
      <div class="space-y-3">
        <h3 class="text-[11px] font-bold uppercase tracking-wider text-[#89b4fa]">
          Appearance & Theme
        </h3>

        <label class="block space-y-1">
          <span class="text-[#bac2de]">Color Themes</span>
          <select
            bind:value={draft.terminal.theme}
            class="w-full bg-[#1e1e2e] border border-[#313244] rounded-md px-2.5 py-1.5 text-[#cdd6f4] outline-none focus:border-[#89b4fa]"
          >
            {#each themeOptions as t}
              <option value={t}>{t}</option>
            {/each}
          </select>
        </label>

        <div class="grid grid-cols-2 gap-3">
          <label class="block space-y-1">
            <span class="text-[#bac2de]">Text Size</span>
            <input
              type="number"
              min="8"
              max="32"
              bind:value={draft.terminal.font_size}
              class="w-full bg-[#1e1e2e] border border-[#313244] rounded-md px-2.5 py-1.5 text-[#cdd6f4] outline-none focus:border-[#89b4fa]"
            />
          </label>

          <label class="block space-y-1">
            <span class="text-[#bac2de]">Scrollback Lines</span>
            <input
              type="number"
              min="500"
              max="100000"
              step="500"
              bind:value={draft.terminal.scrollback_lines}
              class="w-full bg-[#1e1e2e] border border-[#313244] rounded-md px-2.5 py-1.5 text-[#cdd6f4] outline-none focus:border-[#89b4fa]"
            />
          </label>
        </div>

        <div class="space-y-1.5">
          <div class="flex items-center justify-between">
            <span class="text-[#bac2de]">Font</span>
            {#if hasLocalFontApi}
              <button
                type="button"
                onclick={scanSystemFonts}
                disabled={scanning}
                title="Detect all monospace fonts installed on your OS using the browser Local Font Access API"
                class="inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded bg-[#313244]/60 hover:bg-[#89b4fa]/20 hover:text-[#89b4fa] text-[#a6adc8] transition-colors cursor-pointer"
              >
                <Sparkles class="w-2.5 h-2.5 {scanning ? 'animate-spin' : 'text-[#89b4fa]'}" />
                <span>{scanning ? "Scanning..." : (systemScanned ? "Re-scan System" : "Scan System Fonts")}</span>
              </button>
            {/if}
          </div>

          <select
            value={getSelectedFontValue()}
            onchange={handleFontSelect}
            class="w-full bg-[#1e1e2e] border border-[#313244] rounded-md px-2.5 py-1.5 text-[#cdd6f4] outline-none focus:border-[#89b4fa]"
          >
            <optgroup label="System Defaults">
              {#each SYSTEM_DEFAULTS as def}
                <option value={def.value}>{def.label}</option>
              {/each}
            </optgroup>

            <optgroup label="Web Fonts (Preloaded & Distinct)">
              {#each GOOGLE_WEB_FONTS as f}
                <option value={`'${f}', monospace`}>{f}</option>
              {/each}
            </optgroup>

            {#if detectedFonts.length > 0}
              <optgroup label="Detected on Your System">
                {#each detectedFonts as f}
                  <option value={`'${f}', monospace`}>{f}</option>
                {/each}
              </optgroup>
            {/if}

            <optgroup label="Standard System Monospaced">
              {#each POPULAR_SYSTEM_FONTS as f}
                {#if !detectedFonts.includes(f)}
                  <option value={`'${f}', monospace`}>{f}</option>
                {/if}
              {/each}
            </optgroup>

            <option value="__custom__">Custom / Other Font...</option>
          </select>

          {#if customMode || getSelectedFontValue() === "__custom__"}
            <input
              type="text"
              bind:value={draft.terminal.font_family}
              placeholder="e.g. 'Fira Code', 'JetBrains Mono', monospace"
              class="w-full bg-[#1e1e2e] border border-[#313244] rounded-md px-2.5 py-1.5 text-xs text-[#cdd6f4] outline-none focus:border-[#89b4fa]"
            />
          {/if}

          <!-- Live Font Preview Card -->
          <div class="mt-2 p-2.5 rounded-lg bg-[#11111b] border border-[#313244] text-xs">
            <div class="text-[10px] text-[#6c7086] mb-1.5 flex items-center justify-between">
              <span class="font-semibold uppercase tracking-wider text-[#89b4fa]">Live Font Preview</span>
              <span class="truncate max-w-[150px] text-[9px] text-[#bac2de]">{draft.terminal.font_family}</span>
            </div>
            <div
              class="whitespace-pre overflow-x-auto text-[#a6e3a1] py-2 px-2.5 rounded bg-[#181825] border border-[#313244]/60 leading-relaxed"
              style="font-family: {draft.terminal.font_family}; font-size: {Math.min(14, Math.max(11, draft.terminal.font_size))}px; font-feature-settings: 'liga' 1, 'calt' 1; font-variant-ligatures: normal;"
            >
0123456789  != &lt;= &gt;= -&gt; =&gt;
let status = "ready"; // 0 O l 1 I
            </div>
          </div>
        </div>

        <label class="flex items-center justify-between pt-1 cursor-pointer">
          <span class="text-[#bac2de]">Cursor Blink</span>
          <input
            type="checkbox"
            bind:checked={draft.terminal.cursor_blink}
            class="rounded accent-[#89b4fa]"
          />
        </label>
      </div>

      <div class="space-y-3 pt-2 border-t border-[#313244]">
        <h3 class="text-[11px] font-bold uppercase tracking-wider text-[#89b4fa]">
          Session Lifecycle
        </h3>

        <label class="block space-y-1">
          <span class="text-[#bac2de]">Detached Session Reap Policy</span>
          <select
            bind:value={draft.session.reap_timeout}
            class="w-full bg-[#1e1e2e] border border-[#313244] rounded-md px-2.5 py-1.5 text-[#cdd6f4] outline-none focus:border-[#89b4fa]"
          >
            {#each reapOptions as opt}
              <option value={opt.value}>{opt.label}</option>
            {/each}
          </select>
        </label>

        <label class="block space-y-1">
          <span class="text-[#bac2de]">Default Shell Environment</span>
          <select
            bind:value={draft.session.default_mode}
            class="w-full bg-[#1e1e2e] border border-[#313244] rounded-md px-2.5 py-1.5 text-[#cdd6f4] outline-none focus:border-[#89b4fa]"
          >
            <option value="sandbox">Unprivileged Container (Isolated Sandbox)</option>
            <option value="host">Host Direct Shell (Requires Sudo)</option>
          </select>
        </label>
      </div>

      <div class="space-y-3 pt-2 border-t border-[#313244]">
        <h3 class="text-[11px] font-bold uppercase tracking-wider text-[#89b4fa]">
          Copy Settings
        </h3>

        <label class="flex items-center justify-between cursor-pointer">
          <div>
            <span class="text-[#bac2de]">Copy on Select</span>
            <p class="text-[10px] text-[#6c7086]">Automatically copy selected text to clipboard</p>
          </div>
          <input
            type="checkbox"
            bind:checked={draft.keybindings.copy_on_select}
            class="rounded accent-[#89b4fa]"
          />
        </label>

        <label class="block space-y-1">
          <span class="text-[#bac2de]">Shortcut Mode</span>
          <select
            bind:value={draft.keybindings.clipboard_shortcut}
            class="w-full bg-[#1e1e2e] border border-[#313244] rounded-md px-2.5 py-1.5 text-[#cdd6f4] outline-none focus:border-[#89b4fa]"
          >
            <option value="ctrl_shift">Ctrl+Shift+C / Ctrl+Shift+V</option>
            <option value="browser_default">Browser Default (OS level)</option>
          </select>
        </label>
      </div>

      <div class="space-y-3 pt-2 border-t border-[#313244]">
        <h3 class="text-[11px] font-bold uppercase tracking-wider text-[#89b4fa]">
          Keybindings
        </h3>

        <div class="grid grid-cols-2 gap-3">
          <label class="block space-y-1">
            <span class="text-[#bac2de]">New Tab Shortcut</span>
            <input
              type="text"
              bind:value={draft.keybindings.new_tab_shortcut}
              class="w-full bg-[#1e1e2e] border border-[#313244] rounded-md px-2.5 py-1.5 text-[#cdd6f4] outline-none focus:border-[#89b4fa]"
            />
          </label>

          <label class="block space-y-1">
            <span class="text-[#bac2de]">Close Tab Shortcut</span>
            <input
              type="text"
              bind:value={draft.keybindings.close_tab_shortcut}
              class="w-full bg-[#1e1e2e] border border-[#313244] rounded-md px-2.5 py-1.5 text-[#cdd6f4] outline-none focus:border-[#89b4fa]"
            />
          </label>
        </div>
      </div>

      <div class="pt-4">
        <button
          type="submit"
          disabled={saving}
          class="w-full flex items-center justify-center gap-2 py-2 px-4 rounded-md bg-[#89b4fa] text-[#11111b] font-semibold hover:bg-[#b4befe] transition-colors disabled:opacity-50"
        >
          <Save class="w-4 h-4" />
          <span>{saving ? "Saving..." : "Save Preferences"}</span>
        </button>
      </div>
    </form>
  </aside>
{/if}
