<script lang="ts">
  import { Upload, User } from "lucide-svelte";
  import Tooltip from "./Tooltip.svelte";
  import TabList from "./TabList.svelte";
  import NewTabButton from "./NewTabButton.svelte";
  import PreviewButton from "./PreviewButton.svelte";
  import SettingsButton from "./SettingsButton.svelte";
  import type { SessionTab } from "../api";

  interface Props {
    tabs: SessionTab[];
    activeTabId: string;
    onSelect: (id: string) => void;
    onClose: (id: string) => void;
    onRename: (id: string, newName: string) => void;
    onNewTab: (mode?: string) => void;
    onToggleSettings: () => void;
    onUploadClick: () => void;
    defaultMode?: string;
  }

  let {
    tabs,
    activeTabId,
    onSelect,
    onClose,
    onRename,
    onNewTab,
    onToggleSettings,
    onUploadClick,
    defaultMode = "sandbox",
  }: Props = $props();

  let activeTab = $derived(tabs.find((t) => t.id === activeTabId));
  let isHostAccess = $derived(tabs.some((t) => t.is_host_access));
</script>

<header class="flex items-center justify-between px-3 h-10 bg-[#11111b] border-b border-[#313244] select-none shrink-0 relative z-30">
  <div class="flex items-center gap-1.5 min-w-0 flex-1">
    <div class="min-w-0 flex-1 overflow-x-auto no-scrollbar">
      <TabList {tabs} {activeTabId} {onSelect} {onClose} {onRename} {onNewTab} />
    </div>
    <div class="shrink-0 relative z-50">
      <NewTabButton {onNewTab} {isHostAccess} {defaultMode} />
    </div>
  </div>

  <div class="flex items-center gap-2 shrink-0 pl-2">
    {#if activeTab?.username}
      <div
        class="hidden md:flex items-center gap-1.5 px-2 py-0.5 rounded bg-[#181825] border border-[#313244] text-[11px] text-[#bac2de]"
      >
        <User class="w-3 h-3 text-[#89b4fa]" />
        <span>{activeTab.username}</span>
      </div>
    {/if}

    <PreviewButton />

    <Tooltip text="Upload File" position="bottom">
      <button
        type="button"
        onclick={onUploadClick}
        class="flex items-center justify-center p-1.5 rounded-md text-[#a6adc8] hover:text-[#cdd6f4] hover:bg-[#313244] transition-colors"
      >
        <Upload class="w-4 h-4" />
      </button>
    </Tooltip>

    <SettingsButton {onToggleSettings} />
  </div>
</header>
