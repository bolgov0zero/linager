<script lang="ts">
  import Icon from './Icon.svelte';
  import { hasModule } from './me.svelte';
  import { t } from './i18n.svelte';
  import { openServer } from './route.svelte';
  import type { ServerCard } from './api';
  import TagBadges from './TagBadges.svelte';
  import { assistantOffered, CHIP_ICON, chipsOf, isWindows, levelOf, meter, rdpOffered, visibleChips, worstTone, type AgentFlag, type Chip } from './status';

  let { card, editing = false, agent = null, onopen }: { card: ServerCard; editing?: boolean; agent?: AgentFlag; onopen: (c: ServerCard, kind: 'terminal' | 'rdp' | 'asst') => void } = $props();

  const level = $derived(levelOf(card));
  const s = $derived(card.sample);
  const cols = $derived(
    card.online && s
      ? [
          { l: t('card.cpu'), v: Math.round(s.cpu) },
          { l: t('card.ram'), v: Math.round(s.mem_pct) },
          { l: t('card.disk'), v: Math.round(s.disk_pct) },
        ]
      : [
          { l: t('card.cpu'), v: -1 },
          { l: t('card.ram'), v: -1 },
          { l: t('card.disk'), v: -1 },
        ],
  );
  // The footer: what needs a look as small chips (failures, a reboot, updates, Docker images, the agent), and the address when there is
  // nothing. Three chips fit next to the buttons; the rest fold into "+N" (the tooltip lists them).
  const chips = $derived(visibleChips(chipsOf(card, agent), (card.tag_items?.length ?? 0) > 0 ? 2 : 3));
  const chipTitle = (x: Chip): string => t(`tag.${x.id}`, { n: x.n ?? 0 });
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<article class="card mini st-{level}" onclick={(e) => !editing && !(e.target as Element).closest('button') && openServer(card.id)}>
  <header class="c-head">
    <span class="dot {level}"></span>
    <h3>{card.name}</h3>
    <span class="os" class:os-windows={isWindows(card.os)}>{isWindows(card.os) ? 'WIN' : 'LIN'}</span>
  </header>
  <div class="mini-m">
    {#each cols as c (c.l)}
      <div class={c.v < 0 ? '' : `lv-${meter(c.v)}`}><small>{c.l}</small><b>{c.v < 0 ? '—' : c.v}</b><span class="bar"><i style="width:{Math.max(0, Math.min(100, c.v))}%"></i></span></div>
    {/each}
  </div>
  <footer class="mini-f">
    <div class="mchips">
      {#each chips.shown as x (x.id)}
        <span class="mchip t-{x.tone}" class:txt={x.id === 'offline'} title={chipTitle(x)}>{#if CHIP_ICON[x.id]}<Icon name={CHIP_ICON[x.id]} size={11} />{/if}{#if x.id === 'offline'}{t('tag.s.offline')}{:else if x.n}{x.n}{/if}</span>
      {:else}
        <span class="mono dim">{card.address}</span>
      {/each}
      {#if chips.rest.length > 0}<span class="mchip t-{worstTone(chips.rest)} more" title={chips.rest.map(chipTitle).join('; ')}>+{chips.rest.length}</span>{/if}
    </div>
    <TagBadges tags={card.tag_items ?? []} max={2} />
    <div class="acts">
      {#if hasModule('terminal')}      <button class="btn icon xs" type="button" title={t('card.terminal')} disabled={!card.online} onclick={() => onopen(card, 'terminal')}><Icon name="square-terminal" size={14} /></button>{/if}
      {#if hasModule('desktop') && rdpOffered(card)}<button class="btn icon xs" type="button" title={t('card.rdp')} disabled={!card.online} onclick={() => onopen(card, 'rdp')}><Icon name="monitor" size={14} /></button>{/if}
      {#if hasModule('assistant') && assistantOffered(card.os, card.sample)}<button class="btn icon xs" type="button" title={t('card.asst')} disabled={!card.online} onclick={() => onopen(card, 'asst')}><Icon name="app-window" size={14} /></button>{/if}
    </div>
  </footer>
</article>
