<script lang="ts">
  import Icon from './Icon.svelte';

  let {
    value,
    options,
    icon = '',
    small = false,
    onchange,
  }: { value: string; options: { v: string; t: string }[]; icon?: string; small?: boolean; onchange: (v: string) => void } = $props();

  let open = $state(false);
  let btn = $state<HTMLButtonElement | null>(null);
  let pop = $state<HTMLDivElement | null>(null);
  let pos = $state({ left: 0, top: 0, minWidth: 0 });

  const current = $derived(options.find((o) => o.v === value) ?? options[0]);

  function toggle(): void {
    if (open) {
      open = false;
      return;
    }
    const r = btn!.getBoundingClientRect();
    pos = { left: r.left, top: r.bottom + 4, minWidth: r.width };
    open = true;
    // measured after the list is in the page: it opens upwards when there is no room below
    queueMicrotask(() => {
      if (!pop) return;
      const h = pop.offsetHeight;
      pos = { left: Math.min(r.left, innerWidth - pop.offsetWidth - 8), top: r.bottom + h + 12 > innerHeight ? r.top - h - 4 : r.bottom + 4, minWidth: r.width };
    });
  }

  function pick(v: string): void {
    open = false;
    if (v !== value) onchange(v);
  }

  function outside(e: MouseEvent): void {
    if (open && !pop?.contains(e.target as Node) && !btn?.contains(e.target as Node)) open = false;
  }
</script>

<svelte:window onclick={outside} onkeydown={(e) => e.key === 'Escape' && (open = false)} />

<button bind:this={btn} type="button" class="csel" class:sm={small} aria-haspopup="listbox" aria-expanded={open} onclick={toggle}>
  {#if icon}<Icon name={icon} size={14} />{/if}
  <span>{current?.t}</span>
  <Icon name="chevron-down" size={13} />
</button>

{#if open}
  <div bind:this={pop} class="pop" role="listbox" style="left:{pos.left}px;top:{pos.top}px;min-width:{pos.minWidth}px">
    {#each options as o (o.v)}
      <button type="button" role="option" aria-selected={o.v === value} onclick={() => pick(o.v)}>
        {o.t}{#if o.v === value}<Icon name="check" size={14} />{/if}
      </button>
    {/each}
  </div>
{/if}
