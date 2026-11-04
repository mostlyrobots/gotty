<script lang="ts">
  import { X, Shield, Box, Pencil, Copy } from "lucide-svelte";
  import Tooltip from "./Tooltip.svelte";
  import type { SessionTab } from "../api";

  interface Props {
    tab: SessionTab;
    active: boolean;
    onSelect: (id: string) => void;
    onClose: (id: string) => void;
    onRename: (id: string, newName: string) => void;
    onNewTab?: (mode?: string) => void;
  }

  let { tab, active, onSelect, onClose, onRename, onNewTab }: Props = $props();
  let editing = $state(false);
  let draftName = $state("");
  let menuOpen = $state(false);
  let menuPos = $state({ top: 0, left: 0 });

  function startRename() {
    draftName = tab.name;
    editing = true;
  }

  function commitRename() {
    if (!editing) return;
    editing = false;
    const trimmed = draftName.trim();
    if (trimmed && trimmed !== tab.name) {
      onRename(tab.id, trimmed);
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === "Enter") {
      commitRename();
    } else if (e.key === "Escape") {
      editing = false;
    }
  }

  function handleContextMenu(e: MouseEvent) {
    e.preventDefault();
    menuPos = { top: e.clientY + 2, left: e.clientX };
    menuOpen = true;
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="group flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-t-md cursor-pointer select-none border-b-2 transition-colors {active
    ? 'bg-[#1e1e2e] text-[#cdd6f4] border-[#89b4fa]'
    : 'bg-[#11111b] text-[#7f849c] border-transparent hover:bg-[#181825] hover:text-[#bac2de]'}"
  onclick={() => onSelect(tab.id)}
  ondblclick={startRename}
  oncontextmenu={handleContextMenu}
  data-testid="session-tab"
>
  <span
    class="h-2 w-2 rounded-full shrink-0 {tab.connected !== false
      ? 'bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,0.6)]'
      : 'bg-amber-400'}"
    title={tab.connected !== false ? "Connected" : "Reconnecting"}
  ></span>

  {#if tab.mode === 'host' || (tab.is_host_access && !tab.mode)}
    <span title="Host Direct Shell" class="text-amber-400 shrink-0">
      <Shield class="w-3.5 h-3.5" />
    </span>
  {:else}
    <span title="Sandboxed Container" class="text-emerald-400 shrink-0 opacity-75">
      <Box class="w-3.5 h-3.5" />
    </span>
  {/if}

  {#if editing}
    <!-- svelte-ignore a11y_autofocus -->
    <input
      type="text"
      bind:value={draftName}
      onblur={commitRename}
      onkeydown={handleKeydown}
      onclick={(e) => e.stopPropagation()}
      autofocus
      class="bg-[#313244] text-[#cdd6f4] px-1.5 py-0.5 rounded text-xs outline-none border border-[#89b4fa] w-28"
    />
  {:else}
    <span class="truncate max-w-[130px]" title="Double-click or right-click to rename">{tab.name}</span>
    <Tooltip text="Rename Tab" position="bottom">
      <button
        type="button"
        class="p-0.5 rounded opacity-0 group-hover:opacity-75 hover:opacity-100 hover:text-[#89b4fa] transition-opacity"
        onclick={(e) => {
          e.stopPropagation();
          startRename();
        }}
      >
        <Pencil class="w-2.5 h-2.5" />
      </button>
    </Tooltip>
  {/if}

  <Tooltip text="Close Tab" position="bottom">
    <button
      type="button"
      class="ml-0.5 p-0.5 rounded hover:bg-[#313244] hover:text-rose-400 opacity-60 group-hover:opacity-100 transition-opacity"
      onclick={(e) => {
        e.stopPropagation();
        onClose(tab.id);
      }}
    >
      <X class="w-3.5 h-3.5" />
    </button>
  </Tooltip>
</div>

{#if menuOpen}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="fixed inset-0 z-50 bg-transparent"
    onclick={() => (menuOpen = false)}
    oncontextmenu={(e) => { e.preventDefault(); menuOpen = false; }}
  ></div>

  <div
    class="fixed z-50 w-44 py-1 bg-[#181825] border border-[#313244] rounded-md shadow-2xl text-xs text-[#cdd6f4]"
    style="top: {menuPos.top}px; left: {menuPos.left}px;"
  >
    <button
      type="button"
      onclick={() => { menuOpen = false; startRename(); }}
      class="w-full flex items-center gap-2 px-3 py-1.5 text-left hover:bg-[#313244] transition-colors"
    >
      <Pencil class="w-3.5 h-3.5 text-[#89b4fa]" />
      <span>Rename Tab</span>
    </button>

    {#if onNewTab}
      <button
        type="button"
        onclick={() => { menuOpen = false; onNewTab(tab.mode); }}
        class="w-full flex items-center gap-2 px-3 py-1.5 text-left hover:bg-[#313244] transition-colors"
      >
        <Copy class="w-3.5 h-3.5 text-[#a6e3a1]" />
        <span>Duplicate Tab</span>
      </button>
    {/if}

    <div class="my-1 border-t border-[#313244]"></div>

    <button
      type="button"
      onclick={() => { menuOpen = false; onClose(tab.id); }}
      class="w-full flex items-center gap-2 px-3 py-1.5 text-left hover:bg-rose-500/20 text-rose-400 transition-colors"
    >
      <X class="w-3.5 h-3.5" />
      <span>Close Tab</span>
    </button>
  </div>
{/if}
