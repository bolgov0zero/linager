<script lang="ts">
  import type { UpdateHistoryRow } from '../api';
  import { t } from '../i18n.svelte';
  import { fmtDateTime } from './upd-format';

  let { history }: { history: UpdateHistoryRow[] } = $props();
  const BADGE: Record<UpdateHistoryRow['status'], { cls: string; key: string }> = {
    success: { cls: 'b-ok', key: 'st.upd.hist.success' },
    failed: { cls: 'b-crit', key: 'st.upd.hist.failed' },
    rolled_back: { cls: 'b-warn', key: 'st.upd.hist.rolledBack' },
  };
</script>

<div class="upd-hist">
  <div class="panel flush" style="margin:0"><table class="tbl compact">
    <thead><tr><th>{t('st.upd.hist.time')}</th><th>{t('st.upd.hist.what')}</th><th>{t('st.upd.hist.versions')}</th><th>{t('st.upd.hist.result')}</th></tr></thead>
    <tbody>
      {#each history as row (row.id)}
        <tr>
          <td class="dim">{fmtDateTime(row.ts)}</td>
          <td>{row.kind === 'server' ? t('st.upd.hist.server') : t('st.upd.hist.agents')}</td>
          <td class="dim mono">{row.from} → {row.to}</td>
          <td><span class="badge {BADGE[row.status].cls}" title={row.detail}>{t(BADGE[row.status].key)}</span></td>
        </tr>
      {:else}
        <tr><td colspan="4" class="dim">{t('st.upd.hist.empty')}</td></tr>
      {/each}
    </tbody>
  </table></div>
</div>
