<!-- GoTTY - Web Preview Button & Menu -->
<!-- Authors: Google Gemini 4 Argon & the original GoTTY project -->
<script lang="ts">
  import { onMount } from "svelte";
  import { ExternalLink, Globe } from "lucide-svelte";
  import Tooltip from "./Tooltip.svelte";
  import {
    DEFAULT_PREVIEW_ENDPOINTS,
    configurePreviewTarget,
    fetchPreviewState,
    type PreviewEndpoint,
  } from "../api";

  let menuOpen = $state(false);
  let endpoints = $state<PreviewEndpoint[]>(DEFAULT_PREVIEW_ENDPOINTS);
  let selectedHost = $state("localhost");
  let customHost = $state("");
  let useCustomHost = $state(false);
  let targetPort = $state(8080);
  let scheme = $state<"auto" | "http" | "https">("auto");
  let loading = $state(false);
  let errorMsg = $state("");

  const COMMON_PORTS = [8080, 3000, 5173, 8000, 80, 443];

  let effectiveHost = $derived(
    useCustomHost ? customHost.trim() || "localhost" : selectedHost
  );

  async function refreshPreviewEndpoints() {
    try {
      const state = await fetchPreviewState();
      if (state.endpoints && state.endpoints.length > 0) {
        endpoints = state.endpoints;
      }
      if (state.active) {
        targetPort = state.active.target_port || 8080;
        scheme = (state.active.scheme as "auto" | "http" | "https") || "auto";
        const matchesPreset = endpoints.some(
          (ep) => ep.host === state.active.target_host
        );
        if (matchesPreset) {
          selectedHost = state.active.target_host;
          useCustomHost = false;
        } else if (state.active.target_host) {
          customHost = state.active.target_host;
          useCustomHost = true;
        }
      }
    } catch {
      // Keep defaults
    }
  }

  onMount(() => {
    refreshPreviewEndpoints();
  });

  async function toggleMenu() {
    menuOpen = !menuOpen;
    errorMsg = "";
    if (menuOpen) {
      await refreshPreviewEndpoints();
    }
  }

  function buildPreviewPath(host: string, port: number, proto: string): string {
    const cleanHost = host.trim() || "localhost";
    const cleanPort = Math.max(1, Math.min(65535, Number(port) || 8080));
    if (proto === "http" || proto === "https") {
      return `./preview/${proto}/${cleanHost}:${cleanPort}/`;
    }
    return `./preview/${cleanHost}:${cleanPort}/`;
  }

  async function launchPreview() {
    const host = effectiveHost;
    const port = Math.max(1, Math.min(65535, Number(targetPort) || 8080));
    errorMsg = "";
    loading = true;

    // Open the window synchronously on user click to prevent popup blockers
    const previewPath = buildPreviewPath(host, port, scheme);
    const previewWin = window.open(previewPath, "_blank", "noopener");

    try {
      await configurePreviewTarget(host, port, scheme);
      menuOpen = false;
    } catch (err: any) {
      errorMsg = err?.message || "Failed to configure preview target";
      if (previewWin && !previewWin.closed) {
        previewWin.close();
      }
    } finally {
      loading = false;
    }
  }

  function handleWindowClick(e: MouseEvent) {
    if (!menuOpen) return;
    const target = e.target as HTMLElement | null;
    if (target && !target.closest(".gotty-preview-menu")) {
      menuOpen = false;
    }
  }
</script>

<svelte:window onclick={handleWindowClick} />

<div class="relative gotty-preview-menu">
  <Tooltip text="Preview" position="bottom">
    <button
      type="button"
      aria-label="Preview"
      onclick={(e) => {
        e.stopPropagation();
        toggleMenu();
      }}
      class="flex items-center justify-center p-1.5 rounded-md transition-colors {menuOpen
        ? 'text-[#89b4fa] bg-[#313244]'
        : 'text-[#a6adc8] hover:text-[#cdd6f4] hover:bg-[#313244]'}"
    >
      <!-- Web Page Box with Eye SVG Icon (Google Cloud Shell Web Preview style) -->
      <svg
        class="w-4 h-4"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="1.8"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <!-- Browser Window Frame -->
        <rect x="2" y="3" width="20" height="18" rx="2.5" />
        <line x1="2" y1="8" x2="22" y2="8" />
        <circle cx="5.5" cy="5.5" r="0.8" fill="currentColor" stroke="none" />
        <circle cx="8.5" cy="5.5" r="0.8" fill="currentColor" stroke="none" />
        <!-- Eye inside Browser Viewport -->
        <path d="M6.5 14.5C7.9 12.1 9.8 11 12 11s4.1 1.1 5.5 3.5c-1.4 2.4-3.3 3.5-5.5 3.5s-4.1-1.1-5.5-3.5Z" />
        <circle cx="12" cy="14.5" r="1.6" fill="currentColor" stroke="none" />
      </svg>
    </button>
  </Tooltip>

  {#if menuOpen}
    <div
      role="dialog"
      aria-label="Web Preview Settings"
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
      tabindex="-1"
      class="absolute right-0 top-full mt-1.5 w-80 rounded-xl bg-[#181825] border border-[#313244] shadow-2xl p-3.5 z-[100] text-xs text-[#cdd6f4]"
    >
      <!-- Header & Quick Preview Button -->
      <div class="flex items-center justify-between mb-2.5">
        <div class="flex items-center gap-1.5 font-semibold text-[#cdd6f4]">
          <Globe class="w-3.5 h-3.5 text-[#89b4fa]" />
          <span>Web Preview</span>
        </div>
        <span class="text-[10px] px-1.5 py-0.5 rounded bg-[#313244]/70 text-[#a6adc8] font-mono">
          :8080 &rarr; {effectiveHost}:{targetPort}
        </span>
      </div>

      <!-- Primary Quick Launch Button -->
      <button
        type="button"
        disabled={loading}
        onclick={launchPreview}
        class="w-full flex items-center justify-between px-3 py-2 mb-3 rounded-lg bg-[#89b4fa] hover:bg-[#b4befe] text-[#11111b] font-semibold transition-colors shadow-sm"
      >
        <div class="flex items-center gap-2 min-w-0">
          <ExternalLink class="w-3.5 h-3.5 shrink-0" />
          <span class="truncate">Preview {effectiveHost}:{targetPort}</span>
        </div>
        <span class="text-[10px] uppercase tracking-wider bg-[#11111b]/15 px-1.5 py-0.5 rounded font-mono">
          {scheme}
        </span>
      </button>

      <div class="border-t border-[#313244] pt-2.5 space-y-2.5">
        <!-- Remote Endpoint Selector -->
        <div>
          <span class="block text-[11px] font-medium text-[#a6adc8] mb-1">
            Remote Endpoint (Local IP)
          </span>
          <div class="space-y-1.5">
            <select
              value={useCustomHost ? "__custom__" : selectedHost}
              onchange={(e) => {
                const val = (e.target as HTMLSelectElement).value;
                if (val === "__custom__") {
                  useCustomHost = true;
                } else {
                  useCustomHost = false;
                  selectedHost = val;
                }
              }}
              class="w-full px-2.5 py-1.5 rounded-lg bg-[#11111b] border border-[#313244] text-[#cdd6f4] text-xs font-mono focus:outline-none focus:border-[#89b4fa]"
            >
              {#each endpoints as ep}
                <option value={ep.host}>{ep.label}</option>
              {/each}
              <option value="__custom__">Custom local IP...</option>
            </select>

            {#if useCustomHost}
              <input
                type="text"
                bind:value={customHost}
                placeholder="e.g. 127.0.0.1 or local interface IP"
                class="w-full px-2.5 py-1.5 rounded-lg bg-[#11111b] border border-[#313244] text-[#cdd6f4] text-xs font-mono focus:outline-none focus:border-[#89b4fa]"
              />
            {/if}
          </div>
        </div>

        <!-- Port & Protocol Row -->
        <div class="grid grid-cols-3 gap-2">
          <div class="col-span-2">
            <span class="block text-[11px] font-medium text-[#a6adc8] mb-1">
              Remote Port
            </span>
            <input
              type="number"
              min="1"
              max="65535"
              bind:value={targetPort}
              onkeydown={(e) => {
                if (e.key === "Enter") launchPreview();
              }}
              class="w-full px-2.5 py-1.5 rounded-lg bg-[#11111b] border border-[#313244] text-[#cdd6f4] text-xs font-mono focus:outline-none focus:border-[#89b4fa]"
            />
          </div>

          <div>
            <span class="block text-[11px] font-medium text-[#a6adc8] mb-1">
              Protocol
            </span>
            <select
              bind:value={scheme}
              class="w-full px-2 py-1.5 rounded-lg bg-[#11111b] border border-[#313244] text-[#cdd6f4] text-xs focus:outline-none focus:border-[#89b4fa]"
            >
              <option value="auto">Auto</option>
              <option value="http">HTTP</option>
              <option value="https">HTTPS</option>
            </select>
          </div>
        </div>

        <!-- Quick Port Pills -->
        <div>
          <span class="block text-[10px] text-[#6c7086] mb-1">Quick Ports</span>
          <div class="flex flex-wrap gap-1">
            {#each COMMON_PORTS as p}
              <button
                type="button"
                onclick={() => (targetPort = p)}
                class="px-2 py-0.5 rounded text-[11px] font-mono transition-colors {Number(
                  targetPort
                ) === p
                  ? 'bg-[#89b4fa]/20 text-[#89b4fa] border border-[#89b4fa]/40'
                  : 'bg-[#11111b] text-[#a6adc8] border border-[#313244] hover:border-[#45475a] hover:text-[#cdd6f4]'}"
              >
                {p}
              </button>
            {/each}
          </div>
        </div>

        {#if errorMsg}
          <div class="p-2 rounded bg-[#f38ba8]/15 border border-[#f38ba8]/30 text-[#f38ba8] text-[11px]">
            {errorMsg}
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>
