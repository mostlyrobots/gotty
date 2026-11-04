<script lang="ts">
  import { Plus, ChevronDown, Shield, Box } from "lucide-svelte";
  import Tooltip from "./Tooltip.svelte";

  interface Props {
    onNewTab: (mode?: string) => void;
    isHostAccess?: boolean;
    defaultMode?: string;
  }

  let { onNewTab, isHostAccess = true, defaultMode = "sandbox" }: Props = $props();
  let menuOpen = $state(false);
  let containerEl = $state<HTMLDivElement | null>(null);
  let menuPos = $state({ top: 40, left: 100 });

  function handleSelect(mode?: string) {
    menuOpen = false;
    onNewTab(mode);
  }

  function toggleMenu(e: MouseEvent) {
    e.stopPropagation();
    if (!menuOpen && containerEl) {
      const rect = containerEl.getBoundingClientRect();
      menuPos = { top: rect.bottom + 4, left: Math.max(8, rect.left - 80) };
    }
    menuOpen = !menuOpen;
  }
</script>

<div bind:this={containerEl} class="relative inline-flex items-center">
  <Tooltip text={`New Tab (${defaultMode}) (Alt+T)`} position="bottom">
    <button
      type="button"
      onclick={() => handleSelect(undefined)}
      data-testid="new-tab-button"
      class="flex items-center justify-center p-1.5 rounded-l-md text-[#a6adc8] hover:text-[#cdd6f4] hover:bg-[#313244] transition-colors"
    >
      <Plus class="w-4 h-4" />
    </button>
  </Tooltip>

  <Tooltip text="Session Type" position="bottom">
    <button
      type="button"
      onclick={toggleMenu}
      data-testid="new-tab-dropdown-button"
      class="flex items-center justify-center p-1 rounded-r-md text-[#a6adc8] hover:text-[#cdd6f4] hover:bg-[#313244] transition-colors border-l border-[#313244]/60"
    >
      <ChevronDown class="w-3.5 h-3.5" />
    </button>
  </Tooltip>

  {#if menuOpen}
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="fixed inset-0 z-40 bg-transparent"
      onclick={() => (menuOpen = false)}
    ></div>

    <div
      class="fixed z-50 w-56 py-1 bg-[#181825] border border-[#313244] rounded-md shadow-2xl text-xs text-[#cdd6f4]"
      style="top: {menuPos.top}px; left: {menuPos.left}px;"
    >
      <div
        class="px-3 py-1 text-[10px] font-semibold text-[#89b4fa] uppercase tracking-wider border-b border-[#313244]/60"
        title="Select session environment"
      >
        Session Type
      </div>

      <button
        type="button"
        onclick={() => handleSelect("sandbox")}
        title="Launch new Sandboxed Container (Unprivileged Ubuntu 24.04)"
        class="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-[#313244] transition-colors"
      >
        <Box class="w-3.5 h-3.5 text-emerald-400 shrink-0" />
        <div>
          <div class="font-medium text-[#cdd6f4]">Sandboxed Container</div>
          <div class="text-[10px] text-[#a6adc8]">Unprivileged isolated sandbox</div>
        </div>
      </button>

      {#if isHostAccess}
        <button
          type="button"
          onclick={() => handleSelect("host")}
          title="Launch new Host Native Shell (Direct access with sudo)"
          class="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-[#313244] transition-colors"
        >
          <Shield class="w-3.5 h-3.5 text-amber-400 shrink-0" />
          <div>
            <div class="font-medium text-[#cdd6f4]">Host Native Shell</div>
            <div class="text-[10px] text-[#a6adc8]">Host direct access (requires sudo)</div>
          </div>
        </button>
      {/if}
    </div>
  {/if}
</div>
