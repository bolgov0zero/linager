<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from '../Icon.svelte';

  // A panel whose header opens and closes the body; meta is a short status at the right of the header.
  let { title, meta = '', tone = 'muted', open = $bindable(false), children }: { title: string; meta?: string; tone?: 'ok' | 'warn' | 'crit' | 'muted'; open?: boolean; children: Snippet } = $props();
</script>

<section class="panel fold">
  <button class="fold-h" type="button" aria-expanded={open} onclick={() => (open = !open)}>
    <h4 class="ph" style="margin:0">{title}</h4>
    {#if meta}<span class="badge {tone === 'ok' ? 'b-ok' : tone === 'warn' ? 'b-warn' : tone === 'crit' ? 'b-crit' : 'b-mute'}">{meta}</span>{/if}
    <span class="grow"></span><Icon name={open ? 'chevron-down' : 'chevron-right'} size={14} />
  </button>
  {#if open}<div class="fold-b">{@render children()}</div>{/if}
</section>
