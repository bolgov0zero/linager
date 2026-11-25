<script module lang="ts">
  export interface CtxItem {
    key: string;
    label: string;
    icon?: string;
    disabled?: boolean;
    danger?: boolean;
    separated?: boolean;
    /** A choice that is in force: the row is highlighted (no tick); for a toggle or one of several modes. Leave it out for a plain action. */
    selected?: boolean;
    /** A short note at the end of the row (a shortcut), in grey. */
    hint?: string;
    /** A number at the start of the row (the number of a session), in grey. */
    lead?: string;
    /** A second line under the label, in grey. */
    sub?: string;
    /** A small badge at the end of the row. */
    badge?: { text: string; tone: 'info' | 'mute' | 'warn' };
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
    <button type="button" role={it.selected === undefined ? 'menuitem' : 'menuitemradio'} aria-checked={it.selected} class:danger={it.danger} class:on={it.selected} class:rich={!!it.sub} disabled={it.disabled} onclick={(e) => { e.stopPropagation(); onclose(); it.onselect(); }}>
      <span class="ci">{#if it.icon}<Icon name={it.icon} size={14} />{/if}</span>{#if it.lead}<span class="lead">{it.lead}</span>{/if}<span class="lbl">{it.label}{#if it.sub}<span class="sub">{it.sub}</span>{/if}</span>{#if it.hint}<span class="hint">{it.hint}</span>{/if}{#if it.badge}<span class="cbadge t-{it.badge.tone}">{it.badge.text}</span>{/if}
    </button>
  {/each}
</div>
