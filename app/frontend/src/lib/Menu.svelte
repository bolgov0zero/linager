<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';

  // A button with a list: a click on an item calls onpick. Nothing stays selected (a menu, not a select).
  // An item can carry a second action as a small icon button at its end (onalt): "alt" on the item turns it on.
  let { items, onpick, onalt, altIcon = '', altTitle = '', icon = '', label, empty = '', small = true, children }: {
    items: { v: string; t: string; alt?: boolean }[];
    onpick: (v: string) => void;
    onalt?: (v: string) => void;
    altIcon?: string;
    altTitle?: string;
    icon?: string;
    label: string;
    empty?: string;
    small?: boolean;
    children?: Snippet;
  } = $props();

  let open = $state(false);
  let btn = $state<HTMLButtonElement | null>(null);
  let pop = $state<HTMLDivElement | null>(null);
  let pos = $state({ left: 0, top: 0 });

  function toggle(): void {
    if (open) {
      open = false;
      return;
    }
    const r = btn!.getBoundingClientRect();
    pos = { left: r.left, top: r.bottom + 4 };
    open = true;
    queueMicrotask(() => {
      if (!pop) return;
      pos = { left: Math.min(r.left, innerWidth - pop.offsetWidth - 8), top: r.bottom + pop.offsetHeight + 12 > innerHeight ? r.top - pop.offsetHeight - 4 : r.bottom + 4 };
    });
  }
</script>

<svelte:window onclick={(e) => open && !pop?.contains(e.target as Node) && !btn?.contains(e.target as Node) && (open = false)} onkeydown={(e) => e.key === 'Escape' && (open = false)} />

<button bind:this={btn} type="button" class="btn" class:sm={small} aria-haspopup="menu" aria-expanded={open} onclick={toggle}>
  {#if icon}<Icon name={icon} size={14} />{/if}{label}<Icon name="chevron-down" size={13} />
</button>
{#if open}
  <div bind:this={pop} class="pop" role="menu" style="left:{pos.left}px;top:{pos.top}px;max-height:60vh;overflow:auto">
    {#each items as it (it.v)}
      {#if it.alt && onalt}
        <div class="mrow">
          <button type="button" role="menuitem" onclick={() => { open = false; onpick(it.v); }}>{it.t}</button>
          <button type="button" class="malt" title={altTitle} aria-label={altTitle} onclick={() => { open = false; onalt(it.v); }}><Icon name={altIcon} size={14} /></button>
        </div>
      {:else}
        <button type="button" role="menuitem" onclick={() => { open = false; onpick(it.v); }}>{it.t}</button>
      {/if}
    {:else}
      <span class="dim" style="padding:6px 9px;font-size:12px">{empty}</span>
    {/each}
  </div>
{/if}
