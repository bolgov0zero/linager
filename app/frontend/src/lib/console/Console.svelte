<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { api, subscribeLive, type Group, type ServerCard } from '../api';
  import { buildTree, chainOf, type FolderNode } from '../folders';
  import ContextMenu, { type CtxItem } from '../ContextMenu.svelte';
  import Icon from '../Icon.svelte';
  import { t } from '../i18n.svelte';
  import { native } from '../native';
  import { hasModule } from '../me.svelte';
  import { openServer } from '../route.svelte';
  import SearchField from '../SearchField.svelte';
  import AssistantPane from '../assistant/AssistantPane.svelte';
  import DesktopPane from '../desktop/DesktopPane.svelte';
  import { assistantOffered, isWindows, levelOf } from '../status';
  import Terminal from '../Terminal.svelte';
  import { activate, closeTab, con, openTab, passes, setFilter, setMode, syncNames, toggleGroup, type ConMode } from './store.svelte';

  let { nativeWindows }: { nativeWindows: boolean } = $props();

  let cards = $state<ServerCard[]>([]);
  let groups = $state<Group[]>([]);
  let loaded = $state(false);
  let failed = $state(false);
  let q = $state('');
  let filtersOpen = $state(false);
  let menu = $state<{ x: number; y: number; items: CtxItem[] } | null>(null);
  // a session is built when its tab is first on the screen, and then kept alive (a restored tab does not connect by itself)
  let visited = $state<Record<number, boolean>>({});
  let strip = $state<HTMLDivElement | null>(null);
  let overflow = $state(false);

  const MODES: { id: ConMode; icon: string; ready: boolean }[] = [
    { id: 'term', icon: 'square-terminal', ready: true },
    { id: 'rdp', icon: 'monitor', ready: true },
    { id: 'asst', icon: 'app-window', ready: true },
  ];

  /** The server has the mode: a terminal everywhere, the remote desktop on Windows, the Assistant on Windows. */
  function has(c: ServerCard, mode: ConMode): boolean {
    if (mode === 'term') return hasModule('terminal');
    if (mode === 'asst') return hasModule('assistant') && assistantOffered(c.os, c.sample);
    return hasModule('desktop') && isWindows(c.os) && c.sample?.desktop_off !== true;
  }
  const shownCards = $derived(cards.filter((c) => passes(con.filter, isWindows(c.os), c.online) && (!q.trim() || `${c.name} ${c.address}`.toLowerCase().includes(q.trim().toLowerCase()))));
  const counts = $derived(Object.fromEntries(MODES.map((m) => [m.id, shownCards.filter((c) => has(c, m.id)).length])) as Record<ConMode, number>);
  // the sidebar is the tree of the folders: a folder shows how many servers are under it (the subfolders included) and folds
  // as a whole; a folder with nothing to show under it is left out
  type Entry = { k: 'f'; key: string; name: string; depth: number; count: number } | { k: 'u'; key: string; name: string; depth: number; count: number } | { k: 's'; key: string; card: ServerCard; depth: number };
  const entries = $derived.by(() => {
    const visible = shownCards.filter((c) => has(c, con.mode));
    const under = new Map<number, number>();
    for (const c of visible) for (const id of chainOf(groups, c.group_id)) under.set(id, (under.get(id) ?? 0) + 1);
    const out: Entry[] = [];
    const walk = (nodes: FolderNode[]): void => {
      for (const n of nodes) {
        const total = under.get(n.id) ?? 0;
        if (total === 0) continue;
        const key = String(n.id);
        out.push({ k: 'f', key, name: n.name, depth: n.depth, count: total });
        if (!con.collapsed[key]) {
          walk(n.children);
          for (const c of visible) if (c.group_id === n.id) out.push({ k: 's', key: c.id, card: c, depth: n.depth + 1 });
        }
      }
    };
    walk(buildTree(groups));
    const free = visible.filter((c) => c.group_id === null);
    if (free.length > 0) {
      out.push({ k: 'u', key: 'none', name: t('pult.noGroup'), depth: 0, count: free.length });
      if (!con.collapsed.none) for (const c of free) out.push({ k: 's', key: c.id, card: c, depth: 1 });
    }
    return out;
  });
  const filterCount = $derived((con.filter.os !== 'all' ? 1 : 0) + (con.filter.status !== 'all' ? 1 : 0));
  const byId = $derived(new Map(cards.map((c) => [c.id, c])));

  async function load(): Promise<void> {
    try {
      const [d, g] = await Promise.all([api.dashboard(), api.groups()]);
      cards = d.items;
      groups = g;
      failed = false;
      syncNames(new Map(d.items.map((c) => [c.id, c.name])));
    } catch {
      failed = true;
    } finally {
      loaded = true;
    }
  }
  onMount(() => {
    void load();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const stop = subscribeLive(() => {
      timer ??= setTimeout(() => {
        timer = undefined;
        void load();
      }, 800);
    });
    const poll = setInterval(() => void load(), 60_000);
    return () => {
      stop();
      clearInterval(poll);
    };
  });

  // the tab on the screen is built
  $effect(() => {
    if (con.active) visited[con.active] = true;
  });

  function measure(): void {
    overflow = !!strip && strip.scrollWidth > strip.clientWidth + 1;
  }
  $effect(() => {
    void con.tabs.length;
    void tick().then(measure);
  });
  $effect(() => {
    // the active tab stays in view
    const id = con.active;
    void tick().then(() => strip?.querySelector<HTMLElement>(`[data-tab="${id}"]`)?.scrollIntoView({ block: 'nearest', inline: 'nearest' }));
  });

  const modeLabel = (m: ConMode): string => t(`pult.mode.${m}`);
  const open = (c: ServerCard): void => openTab(c.id, c.name, con.mode);
  function rowMenu(c: ServerCard, ev: MouseEvent): void {
    ev.preventDefault();
    menu = { x: ev.clientX, y: ev.clientY, items: [
      { key: 'files', label: t('pult.files'), icon: 'folder', disabled: !c.online, onselect: () => void native.openWindow('files', c.id, c.name, nativeWindows) },
      { key: 'mgr', label: t('term.manager'), icon: 'activity', disabled: !c.online, onselect: () => void native.openWindow('manager', c.id, c.name, nativeWindows) },
      { key: 'page', label: t('pult.page'), icon: 'arrow-up', separated: true, onselect: () => openServer(c.id) },
    ] };
  }
  /** The session moves to a window of its own: one session can be open in one place only, so the tab closes. */
  function inWindow(tab: { id: number; serverId: string; name: string }, kind: 'rdp' | 'asst'): void {
    closeTab(tab.id);
    void native.openWindow(kind, tab.serverId, tab.name, nativeWindows);
  }
  const tabIcon = (m: ConMode): string => MODES.find((x) => x.id === m)!.icon;
</script>

<div class="con">
  <aside class="con-side">
    <div class="con-modes" role="group" aria-label={t('pult.modes')}>
      {#each MODES as m (m.id)}
        <button type="button" aria-pressed={con.mode === m.id} disabled={!m.ready} title={m.ready ? modeLabel(m.id) : `${modeLabel(m.id)} · ${t('pult.soon')}`} aria-label={modeLabel(m.id)} onclick={() => setMode(m.id)}>
          <Icon name={m.icon} size={15} /><em>{counts[m.id]}</em>
        </button>
      {/each}
    </div>
    <div class="con-search"><SearchField placeholder={t('pult.search')} bind:value={q} /></div>
    <div class="con-list">
      {#if !loaded}<p class="dim pad">{t('dash.loading')}</p>
      {:else if failed && cards.length === 0}<p class="dim pad">{t('pult.loadFailed')}</p>
      {:else if entries.length === 0}<p class="dim pad">{filterCount > 0 || q ? t('pult.emptyFiltered') : t('pult.empty')}</p>
      {:else}
        {#each entries as e (e.k + e.key)}
          {#if e.k === 's'}
            {@const c = e.card}
            <button class="con-srv" style="--d:{e.depth}" class:off={!c.online} class:on={con.tabs.some((x) => x.serverId === c.id && x.mode === con.mode && x.id === con.active)} type="button" title={c.online ? t('pult.hintOpen') : t('pult.offline')} onclick={() => open(c)} oncontextmenu={(e) => rowMenu(c, e)}>
                  <span class="dot {levelOf(c)}"></span><span class="nm">{c.name}</span><span class="os" class:os-windows={isWindows(c.os)}>{isWindows(c.os) ? 'WIN' : 'LIN'}</span>
                </button>
          {:else}
            <button class="con-gh" style="--d:{e.depth}" type="button" aria-expanded={!con.collapsed[e.key]} onclick={() => toggleGroup(e.key)}><Icon name={con.collapsed[e.key] ? 'chevron-right' : 'chevron-down'} size={13} /><span>{e.name}</span><em>{e.count}</em></button>
          {/if}
        {/each}
      {/if}
    </div>
    <div class="con-foot">
      <button class="btn sm ghost" type="button" aria-expanded={filtersOpen} onclick={() => (filtersOpen = !filtersOpen)}><Icon name="sliders-horizontal" size={14} />{t('pult.filters')}{#if filterCount > 0} <span class="badge b-info">{filterCount}</span>{/if}</button>
      {#if filtersOpen}
        <div class="con-filters">
          <span class="lbl">{t('pult.filter.os')}</span>
          <div class="seg sm">{#each ['all', 'linux', 'windows'] as v (v)}<button type="button" aria-pressed={con.filter.os === v} onclick={() => setFilter({ os: v as 'all' | 'linux' | 'windows' })}>{t(`pult.filter.os.${v}`)}</button>{/each}</div>
          <span class="lbl">{t('pult.filter.status')}</span>
          <div class="seg sm">{#each ['all', 'online', 'offline'] as v (v)}<button type="button" aria-pressed={con.filter.status === v} onclick={() => setFilter({ status: v as 'all' | 'online' | 'offline' })}>{t(`pult.filter.status.${v}`)}</button>{/each}</div>
        </div>
      {/if}
    </div>
  </aside>

  <section class="con-main">
    <div class="con-tabs" role="tablist" aria-label={t('pult.tabs')}>
      {#if overflow}<button class="btn xs" type="button" aria-label={t('pult.tabsLeft')} onclick={() => strip?.scrollBy({ left: -200, behavior: 'smooth' })}><Icon name="chevron-left" size={14} /></button>{/if}
      <div class="strip" bind:this={strip} onscroll={measure}>
        {#each con.tabs as tab (tab.id)}
          <div class="ctab" class:on={tab.id === con.active} data-tab={tab.id} role="tab" aria-selected={tab.id === con.active}>
            <button class="ctab-b" type="button" onclick={() => activate(tab.id)}>
              <span class="dot {byId.get(tab.serverId) ? levelOf(byId.get(tab.serverId)!) : 'off'}"></span><Icon name={tabIcon(tab.mode)} size={13} /><span class="nm">{tab.name}</span>
            </button>
            <button class="ctab-x" type="button" aria-label={t('pult.closeTab')} title={t('pult.closeTab')} onclick={() => closeTab(tab.id)}><Icon name="x" size={12} /></button>
          </div>
        {/each}
      </div>
      {#if overflow}<button class="btn xs" type="button" aria-label={t('pult.tabsRight')} onclick={() => strip?.scrollBy({ left: 200, behavior: 'smooth' })}><Icon name="chevron-right" size={14} /></button>{/if}
    </div>
    <div class="con-panes">
      {#each con.tabs as tab (tab.id)}
        <div class="con-pane" hidden={tab.id !== con.active}>
          {#if visited[tab.id]}
            {#if tab.mode === 'term'}<Terminal id={tab.serverId} title={tab.name} embedded {nativeWindows} />
            {:else if tab.mode === 'rdp'}<DesktopPane serverId={tab.serverId} name={tab.name} address={byId.get(tab.serverId)?.address ?? ''} online={byId.get(tab.serverId)?.online ?? false} {nativeWindows} onwindow={() => inWindow(tab, 'rdp')} />
            {:else if tab.mode === 'asst'}<AssistantPane serverId={tab.serverId} name={tab.name} online={byId.get(tab.serverId)?.online ?? false} paused={tab.id !== con.active} {nativeWindows} onwindow={() => inWindow(tab, 'asst')} />
            {:else}<div class="empty big"><Icon name={tabIcon(tab.mode)} size={28} /><h3>{modeLabel(tab.mode)}</h3><p>{t('pult.soon')}</p></div>{/if}
          {/if}
        </div>
      {/each}
      {#if con.tabs.length === 0}
        <div class="empty big"><Icon name="square-terminal" size={28} /><h3>{t('pult.placeholder')}</h3><p>{t('pult.placeholderSub')}</p></div>
      {/if}
    </div>
  </section>
</div>
{#if menu}<ContextMenu x={menu.x} y={menu.y} items={menu.items} onclose={() => (menu = null)} />{/if}
