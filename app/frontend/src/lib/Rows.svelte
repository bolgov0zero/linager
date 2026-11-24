<script lang="ts">
  import Icon from './Icon.svelte';
  import { t } from './i18n.svelte';
  import { getPref, setPref } from './prefs';
  import { openServer } from './route.svelte';
  import type { ServerCard } from './api';
  import { fmtAgo, fmtUptime, isWindows, levelOf, meter, tagsOf } from './status';

  let { cards, now, onterminal }: { cards: ServerCard[]; now: number; onterminal: (c: ServerCard) => void } = $props();
  const units = $derived({ d: t('unit.d'), h: t('unit.h'), min: t('unit.min') });

  // a click on a header sorts by the column, a second one reverses, a third one returns to the order of the server
  type Key = 'name' | 'ip' | 'os' | 'cpu' | 'mem' | 'disk' | 'state' | 'uptime';
  const SKEY = 'linager.rows.sort';
  let sort = $state<{ key: Key | ''; dir: 1 | -1 }>(((): { key: Key | ''; dir: 1 | -1 } => {
    try {
      const v = JSON.parse(getPref(SKEY) ?? 'null') as { key: Key; dir: 1 | -1 } | null;
      if (v && typeof v.key === 'string') return v;
    } catch {
      // no storage
    }
    return { key: '', dir: 1 };
  })());

  function setSort(k: Key): void {
    sort = sort.key !== k ? { key: k, dir: 1 } : sort.dir === 1 ? { key: k, dir: -1 } : { key: '', dir: 1 };
    setPref(SKEY, JSON.stringify(sort));
  }

  const LEVEL = { crit: 0, warn: 1, ok: 2, off: 3 } as const;
  const ipNum = (a: string): number => a.split('.').reduce((n, p) => n * 256 + (Number(p) || 0), 0);
  /** The value of a column; an absent number (offline) sorts as the lowest. */
  function val(c: ServerCard, k: Key): number | string {
    const s = c.sample;
    switch (k) {
      case 'name': return c.name.toLowerCase();
      case 'ip': return /^\d+\.\d+\.\d+\.\d+$/.test(c.address) ? ipNum(c.address) : c.address;
      case 'os': return c.os.toLowerCase();
      case 'cpu': return c.online && s ? s.cpu : -1;
      case 'mem': return c.online && s ? s.mem_pct : -1;
      case 'disk': return c.online && s ? s.disk_pct : -1;
      case 'state': return LEVEL[levelOf(c)];
      case 'uptime': return c.online && s ? s.uptime : -1;
    }
  }
  const sorted = $derived.by(() => {
    const k = sort.key;
    if (!k) return cards;
    return cards.slice().sort((a, b) => {
      const x = val(a, k), y = val(b, k);
      const r = typeof x === 'number' && typeof y === 'number' ? x - y : String(x).localeCompare(String(y), undefined, { numeric: true });
      return r * sort.dir || a.name.localeCompare(b.name);
    });
  });
  const COLS: { k: Key; label: () => string }[] = [
    { k: 'name', label: () => t('rows.server') }, { k: 'ip', label: () => 'IP' }, { k: 'os', label: () => t('rows.system') },
    { k: 'cpu', label: () => t('card.cpu') }, { k: 'mem', label: () => t('card.ram') }, { k: 'disk', label: () => t('card.disk') },
    { k: 'state', label: () => t('rows.state') }, { k: 'uptime', label: () => 'Uptime' },
  ];
</script>

<table class="tbl">
  <thead><tr>
    {#each COLS as c (c.k)}
      <th class="sortable" aria-sort={sort.key === c.k ? (sort.dir === 1 ? 'ascending' : 'descending') : 'none'}>
        <button type="button" onclick={() => setSort(c.k)}>{c.label()}{#if sort.key === c.k}<Icon name={sort.dir === 1 ? 'arrow-up' : 'arrow-down'} size={12} />{/if}</button>
      </th>
    {/each}
    <th></th>
  </tr></thead>
  <tbody>
    {#each sorted as c (c.id)}
      {@const lv = levelOf(c)}
      {@const s = c.sample}
      <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
      <tr class="st-{lv}" onclick={(e) => !(e.target as Element).closest('button') && openServer(c.id)}>
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
        <td class="r"><button class="btn icon sm" type="button" title={t('card.terminal')} disabled={!c.online} onclick={() => onterminal(c)}><Icon name="square-terminal" size={14} /></button></td>
      </tr>
    {/each}
  </tbody>
</table>
