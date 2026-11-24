<script module lang="ts">
  export interface CtxItem {
    key: string;
    label: string;
    icon?: string;
    disabled?: boolean;
    danger?: boolean;
    separated?: boolean;
    onselect: () => void;
  }
</script>

<script lang="ts">
  import Icon from './Icon.svelte';

  // A menu at the place of a right click: the items are drawn in the `.pop` look, it closes on any click outside, Escape and scroll.
  let { x, y, items, onclose }: { x: number; y: number; items: CtxItem[]; onclose: () => void } = $props();

  let el = $state<HTMLDivElement | null>(null);
  let pos = $state({ left: 0, top: 0 });
  // svelte-ignore state_referenced_locally
  pos = { left: x, top: y };

  /** The pointer has left the menu (a few pixels of slack): it closes. */
  function away(e: MouseEvent): void {
    if (!el) return;
    const r = el.getBoundingClientRect();
    const m = 24;
    if (e.clientX < r.left - m || e.clientX > r.right + m || e.clientY < r.top - m || e.clientY > r.bottom + m) onclose();
  }

  // keep it inside the window
  $effect(() => {
    if (!el) return;
    pos = { left: Math.max(4, Math.min(x, innerWidth - el.offsetWidth - 6)), top: Math.max(4, Math.min(y, innerHeight - el.offsetHeight - 6)) };
  });
</script>

<svelte:window
  onpointerdowncapture={(e) => !el?.contains(e.target as Node) && onclose()}
  onclickcapture={(e) => !el?.contains(e.target as Node) && onclose()}
  oncontextmenucapture={(e) => !el?.contains(e.target as Node) && onclose()}
  onkeydown={(e) => e.key === 'Escape' && onclose()}
  onmousemove={away}
  onblur={onclose}
  onresize={onclose}
/>

<div bind:this={el} class="pop ctx" role="menu" style="left:{pos.left}px;top:{pos.top}px">
  {#each items as it (it.key)}
    {#if it.separated}<hr />{/if}
    <button type="button" role="menuitem" class:danger={it.danger} disabled={it.disabled} onclick={(e) => { e.stopPropagation(); onclose(); it.onselect(); }}>
      <span class="ci">{#if it.icon}<Icon name={it.icon} size={14} />{/if}</span>{it.label}
    </button>
  {/each}
</div>
