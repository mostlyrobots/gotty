<script lang="ts">
  import { fade } from "svelte/transition";
  import TabItem from "./TabItem.svelte";
  import type { SessionTab } from "../api";

  interface Props {
    tabs: SessionTab[];
    activeTabId: string;
    onSelect: (id: string) => void;
    onClose: (id: string) => void;
    onRename: (id: string, newName: string) => void;
    onNewTab?: (mode?: string) => void;
  }

  let { tabs, activeTabId, onSelect, onClose, onRename, onNewTab }: Props = $props();
</script>

<div class="flex items-end gap-1 overflow-x-auto no-scrollbar pt-1">
  {#each tabs as tab (tab.id)}
    <div transition:fade={{ duration: 150 }}>
      <TabItem
        {tab}
        active={tab.id === activeTabId}
        {onSelect}
        {onClose}
        {onRename}
        {onNewTab}
      />
    </div>
  {/each}
</div>
