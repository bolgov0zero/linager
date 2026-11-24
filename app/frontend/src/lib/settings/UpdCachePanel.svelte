<script lang="ts">
  import type { UpdatesInfo } from '../api';
  import { confirmAsk } from '../dialogs.svelte';
  import { t } from '../i18n.svelte';
  import { fmtDate, fmtSize } from './upd-format';

  let { cache, prunable, pruning, prefetching, onprefetch, onprune }: {
    cache: UpdatesInfo['cache'];
    /** Cached releases that "delete old" would remove. */
    prunable: number;
    pruning: boolean;
    onprune: () => void;
    /** Version currently being downloaded, or null. */
    prefetching: string | null;
    onprefetch: (version: string) => void;
  } = $props();

  async function prune(): Promise<void> {
    if (await confirmAsk({ title: t('st.upd.cache.pruneTitle'), text: t('st.upd.cache.pruneText', { n: prunable }), ok: t('st.upd.cache.pruneConfirm'), danger: true })) onprune();
  }
</script>

<section class="panel">
  <h4 class="ph">{t('st.upd.cache.title')}</h4>
  <div class="panel flush" style="margin:0"><table class="tbl compact">
    <thead><tr><th>{t('st.upd.cache.version')}</th><th>{t('st.upd.cache.size')}</th><th>{t('st.upd.cache.loaded')}</th><th>{t('st.upd.cache.sign')}</th></tr></thead>
    <tbody>
      {#each cache as entry (entry.version)}
        <tr>
          <td class="mono"><b>{entry.version}</b></td>
          <td class="dim">{entry.downloaded ? fmtSize(entry.size) : '—'}</td>
          <td class="dim">{entry.downloaded ? fmtDate(entry.downloaded_at) : t('st.upd.cache.notLoaded')}</td>
          <td>
            {#if !entry.downloaded}
              <button class="btn sm" type="button" disabled={prefetching !== null} onclick={() => onprefetch(entry.version)}>{prefetching === entry.version ? t('st.upd.cache.prefetching') : t('st.upd.cache.prefetch')}</button>
            {:else if entry.verified}<span class="badge b-ok">{t('st.upd.cache.verified')}</span>
            {:else}<span class="dim">—</span>{/if}
          </td>
        </tr>
      {:else}
        <tr><td colspan="4" class="dim">{t('st.upd.cache.empty')}</td></tr>
      {/each}
    </tbody>
  </table></div>
  {#if prunable > 0}<div class="row end" style="margin-top:10px"><button class="btn sm" type="button" disabled={pruning || prefetching !== null} onclick={() => void prune()}>{t('st.upd.cache.prune', { n: prunable })}</button></div>{/if}
  <p class="dim" style="margin-top:8px">{t('st.upd.cache.note')}</p>
</section>
