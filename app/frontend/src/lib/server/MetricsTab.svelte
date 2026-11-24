<script lang="ts">
  import { onMount } from 'svelte';
  import { serverApi, type History, type MetricKey, type Period, type ServerDetail } from '../api';
  import Icon from '../Icon.svelte';
  import { t } from '../i18n.svelte';
  import { startDownload } from '../files-clip.svelte';
  import { fmtRate } from '../status';
  import { showToast } from '../toast';

  let { s }: { s: ServerDetail } = $props();

  const PERIODS: Period[] = ['1h', '24h', '7d', '30d'];
  let period = $state<Period>('24h');
  let h = $state<History | null>(null);
  let failed = $state(false);

  async function load(): Promise<void> {
    try {
      h = await serverApi.history(s.id, period);
      failed = false;
    } catch {
      failed = true;
    }
  }
  $effect(() => {
    void period;
    void load();
  });
  onMount(() => {
    const i = setInterval(() => void load(), 30_000);
    return () => clearInterval(i);
  });

  async function exportCsv(): Promise<void> {
    try {
      await startDownload(`api/servers/${encodeURIComponent(s.id)}/metrics.csv?period=${period}`, `${s.name}-${period}.csv`);
    } catch {
      showToast(t('common.genericError'));
    }
  }

  const CHARTS: { key: MetricKey; title: string; pct: boolean }[] = [
    { key: 'cpu', title: 'CPU', pct: true },
    { key: 'mem', title: 'RAM', pct: true },
    { key: 'disk', title: t('card.disk'), pct: true },
    { key: 'rx', title: t('sp.net.rx'), pct: false },
  ];

  function pts(key: MetricKey, pct: boolean): { line: string; area: string } {
    const a = h?.points ?? [];
    if (a.length === 0) return { line: '', area: '' };
    const get = (p: (typeof a)[number]): number => p[`${key}_avg` as 'cpu_avg'];
    const max = pct ? 100 : Math.max(1, ...a.map(get));
    const w = 640, ht = 150;
    const step = a.length > 1 ? w / (a.length - 1) : w;
    const line = 'M' + a.map((p, i) => `${(i * step).toFixed(1)},${(ht - (get(p) / max) * (ht - 14) - 7).toFixed(1)}`).join(' L');
    return { line, area: `${line} L${w},${ht} L0,${ht}Z` };
  }
  const fmt = (key: MetricKey, v: number | undefined): string => (v === undefined ? '—' : key === 'rx' ? fmtRate(v) : `${Math.round(v)}%`);

  // the pointer over a chart: the marker and the tooltip; with "sync" every chart follows the pointer
  let sync = $state(true);
  let hov = $state<{ key: MetricKey; frac: number } | null>(null);
  function move(e: PointerEvent, key: MetricKey): void {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    hov = { key, frac: Math.min(1, Math.max(0, (e.clientX - r.left) / r.width)) };
  }
  const idxOf = (frac: number): number => (h && h.points.length > 0 ? Math.round(frac * (h.points.length - 1)) : -1);
  function tip(key: MetricKey): { frac: number; time: string; avg: string; max: string } | null {
    if (!h || !hov || (!sync && hov.key !== key)) return null;
    const i = idxOf(hov.frac);
    const p = h.points[i];
    if (!p) return null;
    const d = new Date(p.ts * 1000);
    const time = period === '1h' || period === '24h' ? d.toTimeString().slice(0, 8) : d.toLocaleString(undefined, { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
    return { frac: h.points.length > 1 ? i / (h.points.length - 1) : 0, time, avg: fmt(key, p[`${key}_avg` as 'cpu_avg']), max: fmt(key, p[`${key}_max` as 'cpu_max']) };
  }
  const ticks = $derived.by(() => {
    if (!h) return [];
    const n = 4;
    return Array.from({ length: n }, (_, i) => {
      const ts = (h!.from + ((h!.to - h!.from) * i) / (n - 1)) * 1000;
      const d = new Date(ts);
      return period === '1h' || period === '24h' ? d.toTimeString().slice(0, 5) : d.toLocaleDateString(undefined, { day: '2-digit', month: '2-digit' });
    });
  });
</script>

<div class="sp-tools">
  <div class="seg sm">{#each PERIODS as p (p)}<button type="button" class:on={period === p} aria-pressed={period === p} onclick={() => (period = p)}>{p}</button>{/each}</div>
  <label class="ck"><input type="checkbox" bind:checked={sync} />{t('sp.metrics.sync')}</label>
  <span class="dim">{t('sp.metrics.hint')}</span>
  <span class="grow"></span>
  <button class="btn sm" type="button" onclick={() => void exportCsv()}><Icon name="download" size={14} />{t('srv.metrics.csv')}</button>
</div>

{#if failed && !h}
  <p class="dim">{t('sp.err.load')}</p>
{:else}
  <div class="charts">
    {#each CHARTS as c (c.key)}
      {@const p = pts(c.key, c.pct)}
      {@const tp = tip(c.key)}
      <section class="panel">
        <div class="ch-h"><h4>{c.title}</h4><b class="mono">{fmt(c.key, h?.stats[c.key]?.avg)} <span class="dim">avg</span> · {fmt(c.key, h?.stats[c.key]?.max)} <span class="dim">max</span></b></div>
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div class="ch-wrap" onpointermove={(e) => move(e, c.key)} onpointerleave={() => (hov = null)}>
        <svg class="chart" viewBox="0 0 640 150" preserveAspectRatio="none" aria-hidden="true">
          {#each [0.25, 0.5, 0.75] as k (k)}<line class="gl" x1="0" x2="640" y1={150 * k} y2={150 * k} vector-effect="non-scaling-stroke" />{/each}
          {#if p.line}<path class="sa" d={p.area} /><path class="sl" d={p.line} vector-effect="non-scaling-stroke" />{/if}
        </svg>
        {#if tp}
          <i class="ch-cur" style="left:{tp.frac * 100}%"></i>
          <div class="ch-tip" style="{tp.frac > 0.6 ? `right:${(1 - tp.frac) * 100}%;margin-right:8px` : `left:${tp.frac * 100}%;margin-left:8px`}"><small>{tp.time}</small><span>{tp.avg} <small>avg</small> · {tp.max} <small>max</small></span></div>
        {/if}
        </div>
        <div class="ch-x mono dim">{#each ticks as tk, i (i)}<span>{tk}</span>{/each}</div>
      </section>
    {/each}
  </div>
{/if}
