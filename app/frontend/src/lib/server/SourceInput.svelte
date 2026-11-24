<script lang="ts">
  import type { FwSet } from '../api';

  // An address or a network, or the name of an address list (@name): the lists are offered while a name is typed.
  let { value = $bindable(''), lists, placeholder = '', ariaLabel, oninput }: { value: string; lists: FwSet[]; placeholder?: string; ariaLabel: string; oninput?: () => void } = $props();

  let open = $state(false);
  let active = $state(-1);
  const q = $derived(value.trim().replace(/^@/, '').toLowerCase());
  const shown = $derived(/^[a-z0-9_]*$/.test(q) ? lists.filter((x) => x.name.includes(q)) : []);
  const visible = $derived(open && shown.length > 0);

  function pick(name: string): void {
    value = '@' + name;
    open = false;
    active = -1;
    oninput?.();
  }
  function onkey(e: KeyboardEvent): void {
    if (e.key === 'Escape' && open) {
      e.stopPropagation();
      open = false;
    } else if (shown.length && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
      e.preventDefault();
      open = true;
      active = (active + (e.key === 'ArrowDown' ? 1 : -1) + shown.length) % shown.length;
    } else if (e.key === 'Enter' && visible && active >= 0) {
      e.preventDefault();
      pick(shown[active].name);
    }
  }
</script>

<span class="srcin">
  <span class="field"><input class="mono" autocomplete="off" spellcheck="false" {placeholder} aria-label={ariaLabel} bind:value onfocus={() => (open = true)} onblur={() => setTimeout(() => (open = false), 120)} oninput={() => { open = true; active = -1; oninput?.(); }} onkeydown={onkey} /></span>
  {#if visible}
    <div class="pop srcpop" role="listbox">
      {#each shown as l, i (l.name)}<button type="button" role="option" aria-selected={i === active} onmousedown={(e) => { e.preventDefault(); pick(l.name); }}><span class="mono">@{l.name}</span><span class="dim">{l.family === 'ipv6' ? 'IPv6' : 'IPv4'} · {l.elements.length}</span></button>{/each}
    </div>
  {/if}
</span>
