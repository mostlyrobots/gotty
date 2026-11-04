<script lang="ts">
  import { Settings } from "lucide-svelte";
  import Tooltip from "./Tooltip.svelte";

  interface Props {
    onToggleSettings: () => void;
  }

  let { onToggleSettings }: Props = $props();
</script>

<Tooltip text="Settings" position="bottom">
  <button
    type="button"
    onclick={onToggleSettings}
    data-testid="settings-button"
    class="flex items-center justify-center p-1.5 rounded-md text-[#a6adc8] hover:text-[#cdd6f4] hover:bg-[#313244] transition-colors"
  >
    <Settings class="w-4 h-4" />
  </button>
</Tooltip>
