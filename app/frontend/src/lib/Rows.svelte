<script lang="ts">
  import Icon from './Icon.svelte';
  import { t } from './i18n.svelte';
  import type { ServerCard } from './api';
  import { fmtAgo, fmtUptime, isWindows, levelOf, meter, tagsOf } from './status';

  let { cards, now, onterminal }: { cards: ServerCard[]; now: number; onterminal: (c: ServerCard) => void } = $props();
  const units = $derived({ d: t('unit.d'), h: t('unit.h'), min: t('unit.min') });
</script>

<table class="tbl">
  <thead><tr><th>{t('rows.server')}</th><th>IP</th><th>{t('rows.system')}</th><th>{t('card.cpu')}</th><th>{t('card.ram')}</th><th>{t('card.disk')}</th><th>{t('rows.state')}</th><th>Uptime</th><th></th></tr></thead>
  <tbody>
    {#each cards as c (c.id)}
      {@const lv = levelOf(c)}
      {@const s = c.sample}
      <tr class="st-{lv}">
        <td><span class="dot {lv}"></span><b>{c.name}</b> <span class="os" class:os-windows={isWindows(c.os)}>{isWindows(c.os) ? 'WIN' : 'LIN'}</span></td>
        <td class="mono">{c.address}</td>
        <td>{c.os}</td>
        {#each [s?.cpu, s?.mem_pct, s?.disk_pct] as v, i (i)}
          <td>
            {#if c.online && v !== undefined}
              <span class="m lv-{meter(v)}"><span class="bar"><i style="width:{Math.min(100, v)}%"></i></span><b>{Math.round(v)}%</b></span>
            {:else}<span class="dim">—</span>{/if}
          </td>
        {/each}
        <td>{#each tagsOf(c) as tag (tag.id)}<span class="badge b-{tag.tone}">{t(`tag.${tag.id}`, { n: tag.n ?? 0 })}</span> {/each}</td>
        <td class="mono dim">{c.online ? (s ? fmtUptime(s.uptime, units) : '—') : fmtAgo(c.last_seen_at, now, units)}</td>
        <td class="r"><button class="btn icon sm" type="button" title={t('card.terminal')} disabled={!c.online} onclick={() => onterminal(c)}><Icon name="square-terminal" size={15} /></button></td>
      </tr>
    {/each}
  </tbody>
</table>
