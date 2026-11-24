<script lang="ts">
  import { onMount } from 'svelte';
  import { serverApi, type DiskFS, type ServerDetail } from '../api';
  import { t } from '../i18n.svelte';
  import { fmtBytes, meter } from '../status';

  let { s }: { s: ServerDetail } = $props();
  let fs = $state<DiskFS[] | null>(null);
  let error = $state('');

  onMount(async () => {
    try {
      fs = (await serverApi.disk(s.id)).filesystems;
    } catch {
      error = t('sp.err.load');
    }
  });
</script>

{#if error}<div class="err">{error}</div>{:else if !fs}<p class="dim">{t('dash.loading')}</p>{:else}
  <div class="panel flush"><table class="tbl compact">
    <thead><tr><th>{t('sp.disk.mount')}</th><th>{t('sp.disk.device')}</th><th>FS</th><th>{t('sp.disk.size')}</th><th>{t('sp.disk.used')}</th><th>{t('sp.disk.free')}</th><th>{t('sp.disk.growth')}</th></tr></thead>
    <tbody>
      {#each fs as d (d.mount)}
        <tr><td class="mono"><b>{d.mount}</b></td><td class="mono dim">{d.device}</td><td>{d.fs}</td><td>{fmtBytes(d.total)}</td>
          <td><span class="m lv-{meter(d.pct)}"><span class="bar"><i style="width:{Math.min(100, d.pct)}%"></i></span><b>{Math.round(d.pct)}%</b></span></td>
          <td class="mono">{fmtBytes(d.avail)}</td>
          <td class="mono dim">{d.per_day > 0 ? `${fmtBytes(d.per_day)}/${t('unit.d')}${d.days_left > 0 ? ` · ${d.days_left}${t('unit.d')}` : ''}` : '—'}</td></tr>
      {:else}
        <tr><td colspan="7" class="dim">{t('card.nodata')}</td></tr>
      {/each}
    </tbody>
  </table></div>
{/if}
