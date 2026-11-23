<script lang="ts">
  import { onMount } from 'svelte';
  import { api, subscribeLive, type Group, type ServerCard } from './api';
  import Card from './Card.svelte';
  import CustomSelect from './CustomSelect.svelte';
  import Icon from './Icon.svelte';
  import { t } from './i18n.svelte';
  import Mini from './Mini.svelte';
  import { native } from './native';
  import Rows from './Rows.svelte';
  import { levelOf, sparkPath, summarize } from './status';

  let { nativeWindows }: { nativeWindows: boolean } = $props();

  type Layout = 'cards' | 'mini' | 'rows';
  const LKEY = 'linager.layout';

  let cards = $state<ServerCard[]>([]);
  let groups = $state<Group[]>([]);
  let loaded = $state(false);
  let failed = $state(false);
  let now = $state(Date.now());
  let filter = $state<'all' | 'bad' | 'off'>('all');
  let group = $state('all');
  let q = $state('');
  let layout = $state<Layout>(((): Layout => {
    try {
      const v = localStorage.getItem(LKEY);
      if (v === 'cards' || v === 'mini' || v === 'rows') return v;
    } catch {
      // no storage
    }
    return 'cards';
  })());

  function setLayout(l: Layout): void {
    layout = l;
    try {
      localStorage.setItem(LKEY, l);
    } catch {
      // no storage
    }
  }

  async function load(): Promise<void> {
    try {
      const [d, g] = await Promise.all([api.dashboard(), api.groups()]);
      cards = d.items;
      groups = g;
      failed = false;
    } catch {
      failed = true;
    } finally {
      loaded = true;
    }
  }

  onMount(() => {
    void load();
    let timer: ReturnType<typeof setTimeout> | undefined;
    // the live stream says that something changed: the list is read again (at most once a second)
    const stop = subscribeLive(() => {
      timer ??= setTimeout(() => {
        timer = undefined;
        void load();
      }, 1000);
    });
    const tick = setInterval(() => (now = Date.now()), 30_000);
    const poll = setInterval(() => void load(), 30_000);
    return () => {
      stop();
      clearInterval(tick);
      clearInterval(poll);
    };
  });

  function terminal(c: ServerCard): void {
    void native.openWindow('terminal', c.id, c.name, nativeWindows);
  }

  const totals = $derived(summarize(cards));
  const counts = $derived({ all: cards.length, bad: cards.filter((c) => c.online && levelOf(c) !== 'ok').length, off: cards.filter((c) => !c.online).length });
  const shown = $derived(
    cards.filter((c) => {
      const lv = levelOf(c);
      if (filter === 'bad' && !(c.online && lv !== 'ok')) return false;
      if (filter === 'off' && c.online) return false;
      if (group !== 'all' && String(c.group_id ?? 'none') !== group) return false;
      const needle = q.trim().toLowerCase();
      return !needle || `${c.name} ${c.address}`.toLowerCase().includes(needle);
    }),
  );
  const groupOptions = $derived([{ v: 'all', t: t('dash.group.all') }, ...groups.map((g) => ({ v: String(g.id), t: g.name }))]);
  const cpuSpark = $derived.by(() => {
    const online = cards.filter((c) => c.online && c.spark.length > 0);
    if (online.length === 0) return null;
    const n = Math.min(...online.map((c) => c.spark.length));
    return sparkPath(Array.from({ length: n }, (_, i) => online.reduce((a, c) => a + c.spark[c.spark.length - n + i], 0) / online.length), 120, 36);
  });
</script>

<section class="tiles" aria-label="Summary">
  <div class="tile tone-{totals.offline > 0 ? 'crit' : 'ok'}">
    <div class="tile-h"><span>{t('dash.tile.online')}</span><Icon name="server" size={14} /></div>
    <div class="tile-v">{totals.online}<small>/{totals.total}</small></div>
    <div class="tile-s">{t('dash.tile.offline', { n: totals.offline })}</div>
  </div>
  <div class="tile tone-{totals.critical > 0 ? 'crit' : totals.problems > 0 ? 'warn' : 'ok'}">
    <div class="tile-h"><span>{t('dash.tile.attention')}</span><Icon name="triangle-alert" size={14} /></div>
    <div class="tile-v">{totals.problems}</div>
    <div class="tile-s">{t('dash.tile.critical', { n: totals.critical })}</div>
  </div>
  <div class="tile">
    <div class="tile-h"><span>{t('dash.tile.updates')}</span><Icon name="download" size={14} /></div>
    <div class="tile-v">{totals.updates}</div>
    <div class="tile-s">{t('dash.tile.reboot', { n: totals.rebootPending })}</div>
  </div>
  <div class="tile">
    <div class="tile-h"><span>{t('dash.tile.cpu')}</span><Icon name="activity" size={14} /></div>
    <div class="tile-v">{totals.cpuAvg === null ? '—' : Math.round(totals.cpuAvg)}{#if totals.cpuAvg !== null}<small>%</small>{/if}</div>
    <div class="tile-s">{t('dash.tile.cpuSub')}</div>
    {#if cpuSpark}<svg class="spark" viewBox="0 0 120 36" preserveAspectRatio="none" aria-hidden="true"><path class="sa" d={cpuSpark.area} /><path class="sl" d={cpuSpark.line} vector-effect="non-scaling-stroke" /></svg>{/if}
  </div>
</section>

<div class="toolbar">
  <div class="chips">
    {#each [['all', t('dash.filter.all')], ['bad', t('dash.filter.bad')], ['off', t('dash.filter.off')]] as [k, label] (k)}
      <button class="chip" type="button" aria-pressed={filter === k} onclick={() => (filter = k as typeof filter)}>{label}<em>{counts[k as keyof typeof counts]}</em></button>
    {/each}
  </div>
  <span class="grow"></span>
  <CustomSelect value={group} options={groupOptions} icon="folders" onchange={(v) => (group = v)} />
  <label class="field"><Icon name="search" size={14} /><input id="q" placeholder={t('dash.search')} bind:value={q} /></label>
  <div class="seg">
    <button type="button" aria-pressed={layout === 'cards'} title={t('dash.layout.cards')} onclick={() => setLayout('cards')}><Icon name="layout-grid" size={14} /></button>
    <button type="button" aria-pressed={layout === 'mini'} title={t('dash.layout.mini')} onclick={() => setLayout('mini')}><Icon name="layers" size={14} /></button>
    <button type="button" aria-pressed={layout === 'rows'} title={t('dash.layout.rows')} onclick={() => setLayout('rows')}><Icon name="rows-3" size={14} /></button>
  </div>
</div>

{#if !loaded}
  <div class="empty"><p>{t('dash.loading')}</p></div>
{:else if failed && cards.length === 0}
  <div class="empty"><Icon name="wifi-off" size={26} /><p>{t('dash.err')}</p></div>
{:else if cards.length === 0}
  <div class="empty"><Icon name="server" size={26} /><p>{t('dash.empty')}</p></div>
{:else if shown.length === 0}
  <div class="empty"><Icon name="search" size={26} /><p>{t('dash.nothing')}</p></div>
{:else if layout === 'rows'}
  <div class="tbl-wrap"><Rows cards={shown} {now} onterminal={terminal} /></div>
{:else if layout === 'mini'}
  <div class="grid mini-grid">{#each shown as c (c.id)}<Mini card={c} onterminal={terminal} />{/each}</div>
{:else}
  <div class="grid">{#each shown as c (c.id)}<Card card={c} {now} onterminal={terminal} />{/each}</div>
{/if}
