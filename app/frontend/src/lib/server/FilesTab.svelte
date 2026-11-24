<script lang="ts">
  import type { ServerDetail } from '../api';
  import { t } from '../i18n.svelte';
  import FilesView from './FilesView.svelte';

  let { s, nativeWindows }: { s: ServerDetail; nativeWindows: boolean } = $props();
</script>

{#if s.online}
  <div class="fm-page"><FilesView id={s.id} title={s.name} {nativeWindows} start={s.os.startsWith('Windows') ? '/C:/' : '/root'} /></div>
{:else}
  <p class="dim">{t('sp.offline')}</p>
{/if}
