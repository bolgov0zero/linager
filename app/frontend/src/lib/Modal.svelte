<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';
  import { t } from './i18n.svelte';

  // A dialog over the page for a form: the title, the body and a row of buttons. It closes with the cross and Escape;
  // a click outside does not close it (a typed text must not be lost by a slip).
  let { title, onclose, wide = false, children, foot }: { title: string; onclose: () => void; wide?: boolean; children: Snippet; foot?: Snippet } = $props();
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onclose()} />

<div class="dlg-scrim" role="presentation">
  <div class="dlg mdl" class:wide role="dialog" aria-modal="true" aria-label={title}>
    <div class="mdl-h"><h3>{title}</h3><button class="btn xs" type="button" aria-label={t('common.close')} onclick={onclose}><Icon name="x" size={14} /></button></div>
    <div class="mdl-b">{@render children()}</div>
    {#if foot}<div class="mdl-f">{@render foot()}</div>{/if}
  </div>
</div>
