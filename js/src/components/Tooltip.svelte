<!-- GoTTY - Interactive Tooltip Component -->
<!-- Authors: Google Gemini 4 Argon & the original GoTTY project -->
<script lang="ts">
  import type { Snippet } from "svelte";

  interface Props {
    text: string;
    position?: "top" | "bottom" | "left" | "right";
    children: Snippet;
  }

  let { text, position = "bottom", children }: Props = $props();
  let visible = $state(false);
  let timer: ReturnType<typeof setTimeout> | null = null;

  function show() {
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      visible = true;
    }, 120);
  }

  function hide() {
    if (timer) clearTimeout(timer);
    visible = false;
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="relative inline-flex items-center"
  onmouseenter={show}
  onmouseleave={hide}
  onfocusin={show}
  onfocusout={hide}
>
  {@render children()}

  {#if visible && text}
    <div
      role="tooltip"
      class="absolute z-50 pointer-events-none px-2 py-1 text-[11px] font-medium leading-none text-[#cdd6f4] bg-[#181825] border border-[#45475a] rounded shadow-2xl whitespace-nowrap {
        position === 'bottom'
          ? 'top-full mt-2 left-1/2 -translate-x-1/2'
          : position === 'top'
          ? 'bottom-full mb-2 left-1/2 -translate-x-1/2'
          : position === 'right'
          ? 'left-full ml-2 top-1/2 -translate-y-1/2'
          : 'right-full mr-2 top-1/2 -translate-y-1/2'
      }"
    >
      {text}
      <span
        class="absolute w-1.5 h-1.5 bg-[#181825] border border-[#45475a] rotate-45 {
          position === 'bottom'
            ? '-top-1 left-1/2 -translate-x-1/2 border-b-0 border-r-0'
            : position === 'top'
            ? '-bottom-1 left-1/2 -translate-x-1/2 border-t-0 border-l-0'
            : position === 'right'
            ? '-left-1 top-1/2 -translate-y-1/2 border-t-0 border-r-0'
            : '-right-1 top-1/2 -translate-y-1/2 border-b-0 border-l-0'
        }"
      ></span>
    </div>
  {/if}
</div>
