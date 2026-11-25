<script lang="ts">
  import Icon from './Icon.svelte';
  import { hasModule } from './me.svelte';
  import { t } from './i18n.svelte';
  import { openServer } from './route.svelte';
  import type { ServerCard } from './api';
  import TagBadges from './TagBadges.svelte';
  import { assistantOffered, fmtAgo, fmtRate, fmtUptime, isWindows, levelOf, meter, rdpOffered, shortOs, sparkPath, tagsOf, type AgentFlag } from './status';

  let { card, now, editing = false, agent = null, onopen }: { card: ServerCard; now: number; editing?: boolean; agent?: AgentFlag; onopen: (c: ServerCard, kind: 'terminal' | 'rdp' | 'asst') => void } = $props();

  const level = $derived(levelOf(card));
  const s = $derived(card.sample);
  const rows = $derived(
    s && card.online
      ? [
          { l: t('card.cpu'), v: Math.round(s.cpu) },
          { l: t('card.ram'), v: Math.round(s.mem_pct) },
          { l: t('card.disk'), v: Math.round(s.disk_pct) },
        ]
      : null,
  );
  const spark = $derived(sparkPath(card.spark));
  const units = $derived({ d: t('unit.d'), h: t('unit.h'), min: t('unit.min') });
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<article class="card st-{level}" onclick={(e) => !editing && !(e.target as Element).closest('button') && openServer(card.id)}>
  <header class="c-head">
    <span class="dot {level}"></span>
    <h3>{card.name}</h3>
    <span class="os" class:os-windows={isWindows(card.os)}>{isWindows(card.os) ? 'WIN' : 'LIN'}</span>
  </header>
  <p class="c-sub" title={card.os}><span class="mono">{card.address}</span><i>·</i><span class="c-os">{shortOs(card.os)}</span></p>
  <p class="c-ver">{card.agent_version}</p>
  <div class="badges">
    {#each tagsOf(card, agent) as tag (tag.id)}<span class="badge b-{tag.tone}">{t(`tag.${tag.id}`, { n: tag.n ?? 0 })}</span>{/each}
  </div>
  <div class="metrics" class:off={!rows}>
    {#each rows ?? [{ l: t('card.cpu'), v: -1 }, { l: t('card.ram'), v: -1 }, { l: t('card.disk'), v: -1 }] as r (r.l)}
      {#if r.v < 0}
        <div class="m"><span class="ml">{r.l}</span><span class="m-off">{t('card.nodata')}</span></div>
      {:else}
        <div class="m lv-{meter(r.v)}"><span class="ml">{r.l}</span><span class="bar"><i style="width:{Math.min(100, r.v)}%"></i></span><span class="mv">{r.v}%</span></div>
      {/if}
    {/each}
  </div>
  <div class="net">
    {#if card.online && spark.line}
      <svg class="spark net-spark" viewBox="0 0 160 26" preserveAspectRatio="none" aria-hidden="true"><path class="sa" d={spark.area} /><path class="sl" d={spark.line} vector-effect="non-scaling-stroke" /></svg>
    {:else}<span class="net-off"></span>{/if}
    <div class="net-v">
      <span><Icon name="arrow-down" size={11} />{card.online ? fmtRate(s?.rx_bps) : '—'}</span>
      <span><Icon name="arrow-up" size={11} />{card.online ? fmtRate(s?.tx_bps) : '—'}</span>
    </div>
  </div>
  <footer class="c-foot">
    <span class="up">{card.online ? t('card.uptime', { v: s ? fmtUptime(s.uptime, units) : '—' }) : t('card.offline', { v: fmtAgo(card.last_seen_at, now, units) })}</span>
    <TagBadges tags={card.tag_items ?? []} max={3} />
    <div class="acts">
      {#if hasModule('terminal')}      <button class="btn icon sm" type="button" title={t('card.terminal')} disabled={!card.online} onclick={() => onopen(card, 'terminal')}><Icon name="square-terminal" size={14} /></button>{/if}
      {#if hasModule('desktop') && rdpOffered(card)}<button class="btn icon sm" type="button" title={t('card.rdp')} disabled={!card.online} onclick={() => onopen(card, 'rdp')}><Icon name="monitor" size={14} /></button>{/if}
      {#if hasModule('assistant') && assistantOffered(card.os, card.sample)}<button class="btn icon sm" type="button" title={t('card.asst')} disabled={!card.online} onclick={() => onopen(card, 'asst')}><Icon name="app-window" size={14} /></button>{/if}
      <button class="btn icon sm" type="button" title={t('card.page')} onclick={() => openServer(card.id)}><Icon name="external-link" size={14} /></button>
    </div>
  </footer>
</article>
