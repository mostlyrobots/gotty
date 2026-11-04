<script lang="ts">
  import XtermInstance from "./XtermInstance.svelte";
  import type { SessionTab, UserConfig } from "../api";
  import type { DownloadEvent } from "../osc7777";

  interface Props {
    tabs: SessionTab[];
    activeTabId: string;
    userConfig: UserConfig;
    onOsc7777: (dl: DownloadEvent) => void;
    onToast: (text: string, type?: "success" | "error") => void;
    onStatusChange: (id: string, connected: boolean) => void;
  }

  let {
    tabs,
    activeTabId,
    userConfig,
    onOsc7777,
    onToast,
    onStatusChange,
  }: Props = $props();
</script>

<main class="relative flex-1 min-h-0 w-full bg-[#1e1e2e] overflow-hidden">
  {#each tabs as tab (tab.id)}
    <!-- Tab containers remain mounted to preserve xterm WebGL canvas state -->
    <div class="h-full w-full" class:hidden={tab.id !== activeTabId}>
      <XtermInstance
        session={tab}
        active={tab.id === activeTabId}
        {userConfig}
        {onOsc7777}
        {onToast}
        {onStatusChange}
      />
    </div>
  {/each}
</main>
