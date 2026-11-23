<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { api, isUnauthorized, type Group, type LiveEvent, type ServerCard, type ServerDetail } from '../../api';
  import { t, tr } from '../../i18n';
  import { subscribeLive } from '../../live';
  import { assistantOffered, levelOf, osKind } from '../dashboard/status';
  import AssistantTab from '../server/tabs/AssistantTab.svelte';
  import DesktopTab from '../server/tabs/DesktopTab.svelte';
  import TerminalTab from '../server/tabs/TerminalTab.svelte';
  import { desktopHidden, rdpOverride } from '../server/rdp';
  import { closeTermWindow, focusTermWindow, openFilesWindow, restoreFilesWindows, termWindows, updateTermWindow, type TermWindow } from '../terminal/windows';
  import ContextMenu, { type MenuItem } from '../ui/ContextMenu.svelte';
  import PixIcon from '../ui/PixIcon.svelte';
  import {
    closePultTab,
    openPultTab,
    pultActive,
    passesFilter,
    pultCollapsed,
    pultFilter,
    pultMode,
    pultShown,
    pultTabs,
    syncPultServers,
    type PultMode,
  } from './store';

  /**
   * The Console ("Пульт"): a side list of servers with a mode switch (terminal, RDP, Assistant), tabs of the open sessions
   * and the session of the tab on the screen. It lives in the app next to the pages, not inside them: the sessions
   * stay connected while the person looks at other views or pages, and a hidden tab only stops being drawn.
   */
  let {
    shown,
    role,
    modules,
    reloadKey,
    onopen,
    onauthlost,
  }: {
    /** The Console is the view on the screen (otherwise it is only kept alive, invisible). */
    shown: boolean;
    role: string;
    /** The modules of an operator. */
    modules: string[];
    reloadKey: number;
    onopen: (name: string, tab?: string) => void;
    onauthlost: () => void;
  } = $props();

  const LIVE_RELOAD_MS = 500;
  const SAFETY_RELOAD_MS = 60_000;

  let items = $state<ServerCard[]>([]);
  let groups = $state<Group[]>([]);
  let loaded = $state(false);
  let loadFailed = $state(false);

  const MODES: { id: PultMode; module: string; icon: string }[] = [
    { id: 'term', module: 'terminal', icon: 'terminal' },
    { id: 'rdp', module: 'desktop', icon: 'desktop' },
    { id: 'asst', module: 'assistant', icon: 'assistant' },
  ];

  const allowed = (mode: PultMode): boolean => {
    const m = MODES.find((x) => x.id === mode)!.module;
    return role === 'admin' || (role === 'operator' && modules.includes(m));
  };
  const canFiles = $derived(role === 'admin' || (role === 'operator' && modules.includes('files')));

  /** The server has the mode: a terminal everywhere, the remote desktop on Windows (only while RDP is on), the Assistant on Windows and on a Linux computer with a graphical console. */
  function has(card: ServerCard, mode: PultMode): boolean {
    if (!allowed(mode)) return false;
    if (mode === 'term') return true;
    if (mode === 'asst') return assistantOffered(card.os, card.sample);
    if (osKind(card.os) !== 'windows') return false;
    return !desktopHidden(card.id, card.sample, $rdpOverride);
  }

  let filtersOpen = $state(false);
  const filterCount = $derived(($pultFilter.os !== 'all' ? 1 : 0) + ($pultFilter.status !== 'all' ? 1 : 0));
  const shownItems = $derived(items.filter((c) => passesFilter($pultFilter, osKind(c.os) === 'windows', c.online)));

  const counts = $derived(Object.fromEntries(MODES.map((m) => [m.id, shownItems.filter((c) => has(c, m.id)).length])) as Record<PultMode, number>);

  const sections = $derived.by(() => {
    const list: { key: string; name: string; cards: ServerCard[] }[] = [];
    for (const g of groups) list.push({ key: String(g.id), name: g.name, cards: shownItems.filter((c) => c.group_id === g.id && has(c, $pultMode)) });
    list.push({ key: 'none', name: $tr('pult.noGroup'), cards: shownItems.filter((c) => c.group_id === null && has(c, $pultMode)) });
    return list.filter((s) => s.cards.length > 0);
  });

  const byId = $derived(new Map(items.map((c) => [c.id, c])));
  const activeTab = $derived($pultTabs.find((x) => x.id === $pultActive));

  // ---- data ----

  let reloadTimer: ReturnType<typeof setTimeout> | undefined;

  async function load(): Promise<void> {
    try {
      const [res, list] = await Promise.all([api.dashboard(), api.groups()]);
      items = res.items;
      groups = list;
      loadFailed = false;
      syncPultServers(res.items.map((c) => ({ id: c.id, name: c.name })));
    } catch (e) {
      if (isUnauthorized(e)) {
        onauthlost();
        return;
      }
      loadFailed = true;
    } finally {
      loaded = true;
    }
  }

  function scheduleLoad(): void {
    clearTimeout(reloadTimer);
    reloadTimer = setTimeout(() => void load(), LIVE_RELOAD_MS);
  }

  function onLive(event: LiveEvent): void {
    if (event.type === 'dashboard') {
      scheduleLoad();
      return;
    }
    const index = items.findIndex((c) => c.id === event.server_id);
    const current = items[index];
    if (index < 0 || !current) {
      scheduleLoad();
      return;
    }
    if (event.type === 'metrics') {
      items = items.map((c, i) => (i === index ? { ...current, online: true, last_seen_at: new Date().toISOString(), sample: event.sample } : c));
    } else if (event.type === 'status') {
      items = items.map((c, i) => (i === index ? { ...current, online: event.online, last_seen_at: event.last_seen_at } : c));
    }
  }

  let seenReload = -1;
  $effect(() => {
    if (reloadKey !== seenReload) {
      seenReload = reloadKey;
      void load();
    }
  });

  onMount(() => {
    restoreFilesWindows();
    const unsubscribe = subscribeLive(onLive);
    const safety = setInterval(() => void load(), SAFETY_RELOAD_MS);
    return () => {
      unsubscribe();
      clearInterval(safety);
      clearTimeout(reloadTimer);
    };
  });

  // The dock of the floating windows steps aside while the Console is on the screen: the side list has the window list.
  $effect(() => {
    pultShown.set(shown);
    return () => pultShown.set(false);
  });

  // ---- place: the Console fills what is left under the top bar ----

  let top = $state(62);
  function measure(): void {
    const bar = document.querySelector('header');
    if (bar) top = Math.round(bar.getBoundingClientRect().bottom);
  }
  onMount(() => {
    measure();
    window.addEventListener('resize', measure);
    return () => window.removeEventListener('resize', measure);
  });
  $effect(() => {
    if (shown) measure();
  });

  // ---- the side list ----

  function pick(card: ServerCard): void {
    if (!card.online) return;
    openPultTab(card.id, card.name, $pultMode);
  }

  function toggleGroup(name: string): void {
    pultCollapsed.update((m) => ({ ...m, [name]: !m[name] }));
  }

  let menu = $state<{ x: number; y: number; items: MenuItem[] } | null>(null);
  let menuFor = $state<string | null>(null);

  function showMenu(event: MouseEvent, card: ServerCard): void {
    event.preventDefault();
    menuFor = card.id;
    menu = {
      x: event.clientX,
      y: event.clientY,
      items: [
        { key: 'files', label: $tr('pult.files'), disabled: !card.online || !canFiles, onselect: () => openFilesWindow(card.id, card.name) },
        { key: 'page', label: $tr('pult.page'), separated: true, onselect: () => onopen(card.name) },
      ],
    };
  }

  function closeMenu(): void {
    menu = null;
    menuFor = null;
  }

  const hasTab = (card: ServerCard): boolean => $pultTabs.some((x) => x.serverId === card.id && x.mode === $pultMode);

  // ---- tabs ----

  let tabsEl = $state<HTMLDivElement | undefined>(undefined);
  let overflow = $state(false);
  let atStart = $state(true);
  let atEnd = $state(true);

  function scrollState(): void {
    const el = tabsEl;
    if (!el) return;
    overflow = el.scrollWidth > el.clientWidth + 1;
    atStart = el.scrollLeft <= 1;
    atEnd = el.scrollLeft + el.clientWidth >= el.scrollWidth - 1;
  }

  $effect(() => {
    void $pultTabs.length;
    void $pultActive;
    void shown;
    void tick().then(() => {
      scrollState();
      tabsEl?.querySelector('[aria-selected="true"]')?.scrollIntoView({ inline: 'nearest', block: 'nearest' });
    });
  });

  const modeOf = (m: PultMode): string => (m === 'term' ? 'terminal' : m === 'rdp' ? 'desktop' : 'assistant');

  /** The session component needs a server record; the parts the three sessions use all come from the card. */
  function detailOf(card: ServerCard): ServerDetail {
    return {
      modules: [],
      id: card.id,
      name: card.name,
      hostname: card.name,
      address: card.address,
      os: card.os,
      agent_version: card.agent_version,
      online: card.online,
      last_seen_at: card.last_seen_at,
      enrolled_at: '',
      system: null,
      sample: card.sample,
      spark: card.spark,
      recent: [],
    };
  }

  // ---- the windows list ----

  const topKey = $derived(
    $termWindows.filter((w) => !w.minimized).reduce<{ key: number; z: number } | null>((best, w) => (!best || w.z > best.z ? { key: w.key, z: w.z } : best), null)?.key ?? null,
  );

  function winLabel(w: TermWindow): string {
    switch (w.kind) {
      case 'files':
        return `${w.name} · ${$tr('pult.winFiles')}`;
      case 'editor':
        return `${w.serverName ?? ''} · ${w.name}`;
      case 'desktop':
        return `${w.name} · ${$tr('pult.winDesktop')}`;
      case 'assistant':
        return `${w.name} · ${$tr('pult.winAssistant')}`;
      case 'dispatcher':
        return `${w.name} · ${$tr('pult.winDispatcher')}`;
      case 'quality':
        return `${w.name} · ${$tr('pult.winQuality')}`;
      default:
        return `${w.name} · ${$tr('pult.winTerminal')}`;
    }
  }

  const winIcon = (w: TermWindow): string => (w.kind === 'files' ? 'folder' : w.kind === 'editor' ? 'note' : w.kind === 'desktop' ? 'desktop' : w.kind === 'assistant' ? 'assistant' : w.kind === 'quality' ? 'gear' : w.kind === 'dispatcher' ? 'servers' : 'terminal');

  /** Click on a row: a minimized window opens, the one in front is minimized, any other comes to the front. */
  function toggleWindow(w: TermWindow): void {
    if (!w.minimized && w.key === topKey) updateTermWindow(w.key, { minimized: true });
    else focusTermWindow(w.key);
  }
</script>

<div class="pult" class:off={!shown} style:top="{top}px" aria-hidden={!shown}>
  <aside class="side">
    <div class="modes" role="group" aria-label={$tr('pult.modes')}>
      {#each MODES as m (m.id)}
        <button
          class="mode"
          type="button"
          aria-pressed={$pultMode === m.id}
          title={$tr(`pult.mode.${m.id}`)}
          aria-label={`${$tr(`pult.mode.${m.id}`)}: ${counts[m.id]}`}
          onclick={() => pultMode.set(m.id)}><PixIcon name={m.icon} /><span class="cnt">{counts[m.id]}</span></button>
      {/each}
    </div>
    <div class="list">
      {#if loadFailed && !loaded}
        <p class="empty small">{$tr('pult.loadFailed')}</p>
      {:else if loaded && sections.length === 0}
        <p class="empty small">{filterCount > 0 ? $tr('pult.emptyFiltered') : $tr('pult.empty')}</p>
      {/if}
      {#each sections as sec (sec.key)}
        <div class="grp" class:collapsed={$pultCollapsed[sec.name]}>
          <button class="grp-h" type="button" aria-expanded={!$pultCollapsed[sec.name]} onclick={() => toggleGroup(sec.name)}>
            <span>{sec.name} · {sec.cards.length}</span><span class="chev" aria-hidden="true"></span>
          </button>
          {#if !$pultCollapsed[sec.name]}
            {#each sec.cards as card (card.id)}
              {@const lvl = levelOf(card)}
              <button
                class="srv"
                class:off={!card.online}
                class:ctx={menuFor === card.id}
                class:openx={hasTab(card)}
                type="button"
                aria-current={activeTab?.serverId === card.id && activeTab?.mode === $pultMode ? 'true' : undefined}
                title={card.online ? $tr('pult.hintOpen') : $tr('pult.offline')}
                onclick={() => pick(card)}
                oncontextmenu={(e) => showMenu(e, card)}>
                <span class="dot {lvl}" aria-hidden="true"></span>
                <span class="nm">{card.name}</span>
                <span class="os" class:win={osKind(card.os) === 'windows'}>{osKind(card.os) === 'windows' ? $tr('pult.os.win') : $tr('pult.os.lin')}</span>
              </button>
            {/each}
          {/if}
        </div>
      {/each}
    </div>
    {#if $termWindows.length > 0}
      <div class="wins" aria-label={$tr('pult.windows')}>
        <div class="wh">{$tr('pult.winsCount', { n: $termWindows.length })}</div>
        {#each $termWindows as w (w.key)}
          <div class="wrow" class:min={w.minimized} class:cur={!w.minimized && w.key === topKey}>
            <button class="tg" type="button" title={w.minimized ? $tr('pult.winRestore') : $tr('pult.winMinimize')} onclick={() => toggleWindow(w)}>
              <PixIcon name={winIcon(w)} /><span class="wn">{winLabel(w)}</span>{#if w.minimized}<i class="st">{$tr('pult.winMinimized')}</i>{/if}
            </button>
            <button class="cl" type="button" aria-label={$tr('pult.winClose')} onclick={() => closeTermWindow(w.key)}><PixIcon name="x" /></button>
          </div>
        {/each}
      </div>
    {/if}
    <div class="filters" class:open={filtersOpen}>
      <button class="fh" type="button" aria-expanded={filtersOpen} aria-controls="pult-filters" onclick={() => (filtersOpen = !filtersOpen)}>
        <span>{$tr('pult.filters')}{#if filterCount > 0} · {filterCount}{/if}</span><span class="chev" aria-hidden="true"></span>
      </button>
      {#if filtersOpen}
        <div class="fb" id="pult-filters">
          <div class="fg" role="group" aria-label={$tr('pult.filter.os')}>
            <span class="fl">{$tr('pult.filter.os')}</span>
            {#each ['all', 'linux', 'windows'] as v (v)}
              <button class="fo" type="button" aria-pressed={$pultFilter.os === v} onclick={() => pultFilter.update((f) => ({ ...f, os: v as typeof f.os }))}
                >{$tr(`pult.filter.os.${v}`)}</button>
            {/each}
          </div>
          <div class="fg" role="group" aria-label={$tr('pult.filter.status')}>
            <span class="fl">{$tr('pult.filter.status')}</span>
            {#each ['all', 'online', 'offline'] as v (v)}
              <button class="fo" type="button" aria-pressed={$pultFilter.status === v} onclick={() => pultFilter.update((f) => ({ ...f, status: v as typeof f.status }))}
                >{$tr(`pult.filter.status.${v}`)}</button>
            {/each}
          </div>
        </div>
      {/if}
    </div>
  </aside>

  <section class="main">
    <div class="tabbar" class:ovf={overflow}>
      <button class="scr l" type="button" aria-label={$tr('pult.tabsLeft')} disabled={atStart} onclick={() => tabsEl?.scrollBy({ left: -220 })}>&lt;</button>
      <div class="tabs" role="tablist" aria-label={$tr('pult.tabs')} bind:this={tabsEl} onscroll={scrollState}>
        {#each $pultTabs as tab (tab.id)}
          {@const card = byId.get(tab.serverId)}
          <div class="tab" role="tab" tabindex="0" aria-selected={tab.id === $pultActive} onclick={() => pultActive.set(tab.id)} onkeydown={(e) => (e.key === 'Enter' || e.key === ' ') && pultActive.set(tab.id)}>
            <span class="dotm {card ? levelOf(card) : 'off'}" aria-hidden="true"></span>
            <PixIcon name={modeOf(tab.mode)} />
            <span class="tn">{tab.name}</span>
            <button class="x" type="button" aria-label={$tr('pult.closeTab')} onclick={(e) => { e.stopPropagation(); closePultTab(tab.id); }}><PixIcon name="x" /></button>
          </div>
        {/each}
      </div>
      <button class="scr r" type="button" aria-label={$tr('pult.tabsRight')} disabled={atEnd} onclick={() => tabsEl?.scrollBy({ left: 220 })}>&gt;</button>
    </div>
    <div class="stage">
      {#each $pultTabs as tab (tab.id)}
        {@const card = byId.get(tab.serverId)}
        {#if card}
          <div class="pane" class:front={tab.id === $pultActive} role="tabpanel">
            {#if tab.mode === 'term'}
              <TerminalTab server={detailOf(card)} docked />
            {:else if tab.mode === 'rdp'}
              <DesktopTab server={detailOf(card)} docked />
            {:else}
              <AssistantTab server={detailOf(card)} docked paused={!shown || tab.id !== $pultActive} />
            {/if}
          </div>
        {/if}
      {/each}
      {#if !activeTab}
        <div class="placeholder">
          <PixIcon name="servers" />
          <p>{$tr('pult.placeholder')}</p>
          <p class="small muted">{$tr('pult.placeholderSub')}</p>
        </div>
      {/if}
    </div>
  </section>
</div>

{#if menu}
  <ContextMenu x={menu.x} y={menu.y} items={menu.items} label={$tr('pult.menu')} onclose={closeMenu} />
{/if}

<style>
  /* The Console is fixed under the top bar and takes the rest of the window. Hidden, it keeps its place (the sessions
     keep their size) and is only invisible. */
  .pult {
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 1;
    display: grid;
    grid-template-columns: 252px minmax(0, 1fr);
    background: var(--surface);
  }

  .pult.off {
    visibility: hidden;
    pointer-events: none;
  }

  .side {
    display: flex;
    flex-direction: column;
    min-height: 0;
    border-right: 2px solid var(--line);
    background: var(--surface);
  }

  .modes {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    border-bottom: 2px solid var(--line);
  }

  .mode {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    height: 36px;
    padding: 0 4px;
    background: none;
    border: 0;
    border-right: 2px solid var(--line);
    cursor: pointer;
    font-size: 13px;
    color: var(--ink);
  }

  .mode:last-child {
    border-right: 0;
  }

  .mode:hover {
    background: var(--seg-off);
  }

  .mode[aria-pressed='true'] {
    background: var(--ink);
    color: var(--surface);
  }

  .list {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding: 4px 0 8px;
  }

  .empty {
    margin: 12px;
    color: var(--muted);
  }

  .grp-h {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    height: 28px;
    padding: 0 12px;
    margin-top: 4px;
    background: none;
    border: 0;
    cursor: pointer;
    color: var(--muted);
    font-family: var(--font-display);
    font-size: 12px;
    letter-spacing: 0.1em;
    text-transform: uppercase;
  }

  .grp-h:hover {
    color: var(--ink);
  }

  .chev {
    width: 0;
    height: 0;
    border-left: 4px solid transparent;
    border-right: 4px solid transparent;
    border-top: 5px solid currentColor;
  }

  .grp.collapsed .chev {
    transform: rotate(-90deg);
  }

  .srv {
    position: relative;
    display: flex;
    align-items: center;
    gap: 9px;
    width: 100%;
    height: 30px;
    padding: 0 12px 0 16px;
    border: 2px solid transparent;
    background: none;
    cursor: pointer;
    text-align: left;
    font-size: 13px;
    color: var(--ink);
  }

  .srv:hover {
    background: var(--seg-off);
  }

  .srv .dot {
    flex: none;
    width: 9px;
    height: 9px;
    background: var(--good);
  }

  .srv .dot.warn {
    background: var(--warn);
  }

  .srv .dot.crit {
    background: var(--crit);
  }

  .srv .dot.off {
    background: transparent;
    border: 2px solid var(--muted);
  }

  .srv .nm {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .srv .os {
    flex: none;
    font-family: var(--font-display);
    font-size: 10px;
    color: var(--os-linux);
    border: 1px solid currentColor;
    padding: 0 3px;
  }

  /* The colours of the badges are the same as on the cards and in the grid: Linux orange, Windows blue. */
  .srv .os.win {
    color: var(--accent);
  }

  .srv.off {
    color: var(--muted);
    cursor: not-allowed;
  }

  .srv.openx::after {
    content: '';
    position: absolute;
    right: 3px;
    top: 50%;
    width: 5px;
    height: 5px;
    margin-top: -2px;
    background: var(--accent);
  }

  .srv[aria-current='true'] {
    background: var(--ink);
    color: var(--surface);
    box-shadow: 3px 3px 0 var(--shade);
  }

  .srv[aria-current='true'] .os {
    color: var(--surface);
  }

  .srv.ctx {
    outline: 2px dashed var(--accent);
    outline-offset: -2px;
  }

  /* the filters: a folding panel at the very bottom of the side list */
  .filters {
    flex: none;
    border-top: 2px solid var(--line);
    background: var(--bg);
  }

  .fh {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    height: 30px;
    padding: 0 12px;
    background: none;
    border: 0;
    cursor: pointer;
    color: var(--muted);
    font-family: var(--font-display);
    font-size: 12px;
    letter-spacing: 0.1em;
    text-transform: uppercase;
  }

  .fh:hover {
    background: var(--seg-off);
  }

  .filters.open .fh {
    color: var(--ink);
  }

  .filters .chev {
    transform: rotate(180deg); /* the panel opens upwards */
  }

  .filters.open .chev {
    transform: none;
  }

  .fb {
    display: grid;
    gap: 6px;
    padding: 4px 12px 10px;
    animation: filters-in 0.12s ease-out;
  }

  .fg {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }

  .fl {
    flex: 0 0 100%;
    color: var(--muted);
    font-size: 11px;
  }

  .fo {
    flex: 1 1 auto;
    height: 26px;
    padding: 0 6px;
    white-space: nowrap;
    background: var(--surface);
    border: 2px solid var(--line);
    cursor: pointer;
    font-size: 12px;
    color: var(--ink);
  }

  .fo:hover {
    background: var(--seg-off);
  }

  .fo[aria-pressed='true'] {
    background: var(--ink);
    color: var(--surface);
  }

  @keyframes filters-in {
    from {
      opacity: 0;
      transform: translateY(6px);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .fb {
      animation: none;
    }
  }

  /* the windows list: a separate area at the bottom of the side list, the servers scroll above it */
  .wins {
    flex: none;
    max-height: 176px;
    overflow: auto;
    border-top: 2px solid var(--line);
    background: var(--bg);
  }

  .wh {
    position: sticky;
    top: 0;
    display: flex;
    align-items: center;
    height: 26px;
    padding: 0 12px;
    background: var(--bg);
    color: var(--muted);
    font-family: var(--font-display);
    font-size: 12px;
    letter-spacing: 0.1em;
    text-transform: uppercase;
  }

  .wrow {
    display: flex;
    align-items: stretch;
    height: 30px;
    border: 2px solid transparent;
    font-size: 12px;
  }

  .wrow:hover {
    background: var(--seg-off);
  }

  .wrow:not(.min) {
    box-shadow: inset 3px 0 0 var(--accent);
  }

  .wrow.cur {
    background: var(--seg-off);
  }

  .wrow.min {
    color: var(--muted);
  }

  .wrow .tg {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 6px 0 12px;
    background: none;
    border: 0;
    cursor: pointer;
    font: inherit;
    color: inherit;
    text-align: left;
  }

  .wrow .wn {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .wrow .st {
    flex: none;
    font-style: normal;
    font-size: 10px;
    font-family: var(--font-display);
    color: var(--muted);
  }

  .wrow .cl {
    flex: none;
    width: 28px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: none;
    border: 0;
    cursor: pointer;
    color: inherit;
    opacity: 0.7;
  }

  .wrow .cl:hover {
    background: var(--crit);
    color: #fff;
    opacity: 1;
  }

  .wrow :global(.pix),
  .tab :global(.pix),
  .mode :global(.pix) {
    width: 14px;
    height: 14px;
  }

  .wrow .cl :global(.pix) {
    width: 10px;
    height: 10px;
  }

  /* tabs and the session */
  .main {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
  }

  .tabbar {
    display: flex;
    align-items: stretch;
    min-width: 0;
    background: var(--bg);
  }

  .tabs {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: flex-end;
    min-height: 38px;
    padding: 0 8px;
    overflow-x: auto;
    border-bottom: 2px solid var(--line);
    scrollbar-width: none;
    scroll-behavior: smooth;
  }

  .tabs::-webkit-scrollbar {
    display: none;
  }

  .scr {
    display: none;
    flex: none;
    width: 30px;
    border: 0;
    border-bottom: 2px solid var(--line);
    background: var(--surface);
    cursor: pointer;
    font-family: var(--font-display);
    font-size: 16px;
    line-height: 1;
    color: var(--ink);
  }

  .scr.l {
    border-right: 2px solid var(--line);
  }

  .scr.r {
    border-left: 2px solid var(--line);
  }

  .scr:hover:not(:disabled) {
    background: var(--ink);
    color: var(--surface);
  }

  .scr:disabled {
    opacity: 0.35;
    cursor: default;
  }

  .tabbar.ovf .scr {
    display: block;
  }

  .tab {
    position: relative;
    top: 2px;
    display: flex;
    align-items: center;
    gap: 8px;
    height: 34px;
    padding: 0 6px 0 10px;
    margin-right: -2px;
    border: 2px solid var(--line);
    border-bottom: 0;
    background: var(--surface);
    cursor: pointer;
    font-size: 13px;
    white-space: nowrap;
  }

  .tab:hover {
    background: var(--seg-off);
  }

  .tab[aria-selected='true'] {
    z-index: 2;
    background: var(--ink);
    color: var(--surface);
  }

  .dotm {
    flex: none;
    width: 8px;
    height: 8px;
    background: var(--good);
  }

  .dotm.warn {
    background: var(--warn);
  }

  .dotm.crit {
    background: var(--crit);
  }

  .dotm.off {
    background: var(--muted);
  }

  .tab .x {
    width: 20px;
    height: 20px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border: 0;
    background: none;
    cursor: pointer;
    color: inherit;
    opacity: 0.7;
  }

  .tab .x:hover {
    opacity: 1;
    background: var(--crit);
    color: #fff;
  }

  .tab .x :global(.pix) {
    width: 10px;
    height: 10px;
  }

  .stage {
    position: relative;
    flex: 1;
    min-height: 0;
    background: var(--surface);
  }

  /* Every open session is kept in place; only the one in front is visible (and hidden panes keep their size). */
  .pane {
    position: absolute;
    inset: 0;
    padding: 8px 10px;
    visibility: hidden;
    pointer-events: none;
    overflow: hidden;
  }

  .pane.front {
    visibility: inherit;
    pointer-events: auto;
  }

  .placeholder {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    color: var(--muted);
    text-align: center;
  }

  .placeholder :global(.pix) {
    width: 54px;
    height: 54px;
    color: var(--ink);
  }

  .placeholder p {
    margin: 0;
  }
</style>
