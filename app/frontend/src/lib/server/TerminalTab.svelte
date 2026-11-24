<script lang="ts">
  import type { ServerDetail } from '../api';
  import Terminal from '../Terminal.svelte';
  import { t } from '../i18n.svelte';

  let { s, nativeWindows }: { s: ServerDetail; nativeWindows: boolean } = $props();
</script>

{#if s.online}
  <div class="sp-termbox"><Terminal id={s.id} title={s.name} embedded {nativeWindows} /></div>
{:else}
  <p class="dim">{t('sp.offline')}</p>
{/if}
