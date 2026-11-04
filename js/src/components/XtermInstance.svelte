<script lang="ts">
  import { onMount } from "svelte";
  import { UploadCloud, Copy } from "lucide-svelte";
  import { fade } from "svelte/transition";
  import { GoTTYXterm } from "../xterm";
  import { WebTTY, protocols } from "../webtty";
  import { ConnectionFactory } from "../websocket";
  import { initMobileKeys } from "../mobile-keys";
  import {
    TERMINAL_THEMES,
    uploadFileToSession,
    type SessionTab,
    type UserConfig,
  } from "../api";
  import type { DownloadEvent } from "../osc7777";

  declare var gotty_auth_token: string | undefined;
  declare var gotty_ws_query_args: string | undefined;
  declare var gotty_preferences: Record<string, unknown> | undefined;
  declare var gotty_mobile_keys: boolean | undefined;

  interface Props {
    session: SessionTab;
    active: boolean;
    userConfig: UserConfig;
    onOsc7777: (dl: DownloadEvent) => void;
    onToast: (text: string, type?: "success" | "error") => void;
    onStatusChange: (id: string, connected: boolean) => void;
  }

  let {
    session,
    active,
    userConfig,
    onOsc7777,
    onToast,
    onStatusChange,
  }: Props = $props();

  let containerEl: HTMLDivElement | undefined = $state();
  let isDragging = $state(false);
  let copyToastVisible = $state(false);
  let copyToastTimer: ReturnType<typeof setTimeout> | null = null;
  let xtermInstance: GoTTYXterm | null = null;
  let closeWebTTY: (() => void) | null = null;

  function triggerCopyToast() {
    copyToastVisible = true;
    if (copyToastTimer) clearTimeout(copyToastTimer);
    copyToastTimer = setTimeout(() => {
      copyToastVisible = false;
    }, 2000);
  }

  function applyConfigToTerm(term: GoTTYXterm, cfg: UserConfig) {
    const themeObj =
      TERMINAL_THEMES[cfg.terminal.theme] ||
      TERMINAL_THEMES["catppuccin-mocha"];

    term.setPreferences({
      "font-size": cfg.terminal.font_size,
      "font-family": cfg.terminal.font_family,
      "cursor-blink": cfg.terminal.cursor_blink,
      "scrollback-lines": cfg.terminal.scrollback_lines,
      theme: themeObj,
    });
    term.refit();
  }

  onMount(() => {
    if (!containerEl) return;

    const initialPrefs =
      typeof gotty_preferences !== "undefined" ? gotty_preferences : {};
    const term = new GoTTYXterm(containerEl, initialPrefs);
    xtermInstance = term;

    applyConfigToTerm(term, userConfig);

    if (typeof gotty_mobile_keys !== "undefined" && gotty_mobile_keys) {
      initMobileKeys(term, true);
    }

    term.onOsc7777 = (dl) => {
      onOsc7777(dl);
    };

    let lastCopiedText = "";
    // Copy-on-select support
    const selectionDisposable = term.term.onSelectionChange(() => {
      if (userConfig.keybindings.copy_on_select && term.term.hasSelection()) {
        const text = term.term.getSelection();
        if (text && text.trim().length > 0 && text !== lastCopiedText) {
          lastCopiedText = text;
          if (navigator.clipboard?.writeText) {
            navigator.clipboard.writeText(text).then(() => {
              triggerCopyToast();
            }).catch(() => {});
          }
        }
      } else if (!term.term.hasSelection()) {
        lastCopiedText = "";
      }
    });

    // Custom key event handler for Ctrl+Shift+C / Ctrl+Shift+V
    term.term.attachCustomKeyEventHandler((event: KeyboardEvent) => {
      const mode = userConfig.keybindings.clipboard_shortcut || "ctrl_shift";
      if (mode === "ctrl_shift") {
        if (event.ctrlKey && event.shiftKey && (event.key === "C" || event.key === "c")) {
          if (event.type === "keydown") {
            const text = term.term.getSelection();
            if (text && text.length > 0) {
              if (navigator.clipboard?.writeText) {
                navigator.clipboard.writeText(text).then(() => {
                  triggerCopyToast();
                }).catch(() => {});
              }
            }
          }
          return false;
        }
        if (event.ctrlKey && event.shiftKey && (event.key === "V" || event.key === "v")) {
          if (event.type === "keydown") {
            if (navigator.clipboard?.readText) {
              navigator.clipboard.readText().then((clipText) => {
                if (clipText) {
                  term.term.paste(clipText);
                }
              }).catch(() => {});
            }
          }
          return false;
        }
      }
      return true;
    });

    const httpsEnabled = window.location.protocol === "https:";
    const baseQuery =
      typeof gotty_ws_query_args !== "undefined" && gotty_ws_query_args !== ""
        ? gotty_ws_query_args
        : "";
    const sessionParam =
      session.id && session.id !== "default"
        ? `session_id=${encodeURIComponent(session.id)}`
        : "";
    const combinedQuery = [baseQuery, sessionParam].filter(Boolean).join("&");
    const wsUrl =
      (httpsEnabled ? "wss://" : "ws://") +
      window.location.host +
      window.location.pathname +
      "ws" +
      (combinedQuery ? "?" + combinedQuery : "");

    const authToken =
      typeof gotty_auth_token !== "undefined" ? gotty_auth_token : "";
    const factory = new ConnectionFactory(wsUrl, protocols);
    const wt = new WebTTY(term, factory, combinedQuery, authToken);

    wt.onConnect = () => {
      onStatusChange(session.id, true);
    };
    wt.onDisconnect = () => {
      onStatusChange(session.id, false);
    };

    closeWebTTY = wt.open();

    return () => {
      if (copyToastTimer) clearTimeout(copyToastTimer);
      selectionDisposable.dispose();
      if (closeWebTTY) closeWebTTY();
      term.close();
    };
  });

  // Reactively apply userConfig updates without recreating the xterm canvas
  $effect(() => {
    if (xtermInstance && userConfig) {
      applyConfigToTerm(xtermInstance, userConfig);
    }
  });

  // Refit and focus when switching back to this tab
  $effect(() => {
    if (active && xtermInstance) {
      requestAnimationFrame(() => {
        xtermInstance?.fit();
        xtermInstance?.focus();
      });
    }
  });

  async function handleFiles(files: FileList | null) {
    if (!files || files.length === 0) return;
    for (let i = 0; i < files.length; i++) {
      const file = files[i];
      try {
        const res = await uploadFileToSession(session.id, file);
        onToast(`File written to ${res.path}`, "success");
      } catch (err: any) {
        onToast(
          `Upload failed for ${file.name}: ${err?.message || "Unknown error"}`,
          "error"
        );
      }
    }
  }

  function handleDragOver(e: DragEvent) {
    e.preventDefault();
    isDragging = true;
  }

  function handleDragLeave(e: DragEvent) {
    e.preventDefault();
    isDragging = false;
  }

  async function handleDrop(e: DragEvent) {
    e.preventDefault();
    isDragging = false;
    await handleFiles(e.dataTransfer?.files || null);
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="relative h-full w-full overflow-hidden"
  ondragover={handleDragOver}
  ondragleave={handleDragLeave}
  ondrop={handleDrop}
>
  <div
    bind:this={containerEl}
    id={active ? "terminal" : undefined}
    class="h-full w-full"
  ></div>

  {#if isDragging}
    <div
      class="absolute inset-0 z-40 flex flex-col items-center justify-center bg-[#11111b]/85 border-2 border-dashed border-[#89b4fa] rounded-lg backdrop-blur-sm pointer-events-none"
    >
      <UploadCloud class="w-12 h-12 text-[#89b4fa] mb-2 animate-bounce" />
      <p class="text-sm font-semibold text-[#cdd6f4]">
        Drop file to upload into active directory ($PWD)
      </p>
      <p class="text-xs text-[#a6adc8] mt-1">
        Falls back to $HOME if $PWD is unavailable
      </p>
    </div>
  {/if}

  {#if copyToastVisible}
    <div
      transition:fade={{ duration: 200 }}
      class="pointer-events-none fixed top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 z-50 flex items-center gap-2 px-4 py-2.5 rounded-lg bg-[#313244]/90 backdrop-blur-sm border border-[#45475a] text-[#cdd6f4] shadow-2xl text-sm font-medium select-none"
    >
      <Copy class="w-4 h-4 text-[#89b4fa]" />
      <span>Copied</span>
    </div>
  {/if}
</div>
