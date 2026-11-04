<script lang="ts">
  import { fly } from "svelte/transition";
  import { CheckCircle2, AlertCircle, X } from "lucide-svelte";

  export interface ToastMessage {
    id: string;
    text: string;
    type?: "success" | "error" | "info";
  }

  interface Props {
    toasts: ToastMessage[];
    onDismiss: (id: string) => void;
  }

  let { toasts, onDismiss }: Props = $props();
</script>

<div
  class="fixed top-12 left-1/2 -translate-x-1/2 z-50 flex flex-col gap-2 pointer-events-none max-w-lg w-full px-4"
  data-testid="toast-container"
>
  {#each toasts as toast (toast.id)}
    <div
      transition:fly={{ y: -16, duration: 200 }}
      class="pointer-events-auto flex items-center justify-between gap-3 px-4 py-2.5 rounded-lg shadow-lg border backdrop-blur-md text-xs font-medium {toast.type ===
      'error'
        ? 'bg-rose-950/90 border-rose-500/50 text-rose-100'
        : 'bg-emerald-950/90 border-emerald-500/50 text-emerald-100'}"
      role="status"
      data-testid="toast-banner"
    >
      <div class="flex items-center gap-2 min-w-0">
        {#if toast.type === "error"}
          <AlertCircle class="w-4 h-4 text-rose-400 shrink-0" />
        {:else}
          <CheckCircle2 class="w-4 h-4 text-emerald-400 shrink-0" />
        {/if}
        <span class="truncate">{toast.text}</span>
      </div>
      <button
        type="button"
        onclick={() => onDismiss(toast.id)}
        class="p-0.5 rounded hover:bg-white/10 transition-colors shrink-0"
      >
        <X class="w-3.5 h-3.5" />
      </button>
    </div>
  {/each}
</div>
