<script lang="ts">
  import { onMount } from 'svelte';
  import { api, subscribeLive, type Group, type ServerCard } from './api';
  import Card from './Card.svelte';
  import CustomSelect from './CustomSelect.svelte';
  import Icon from './Icon.svelte';
  import { getPref, setPref } from './prefs';
  import { t } from './i18n.svelte';
  import Mini from './Mini.svelte';
  import { native } from './native';
  import Rows from './Rows.svelte';
  import SearchField from './SearchField.svelte';
  import AddServerModal from './addserver/AddServerModal.svelte';
  import { confirmAsk } from './dialogs.svelte';
  import DeleteServerModal from './DeleteServerModal.svelte';
  import EditBar from './EditBar.svelte';
  import type { RowEdit } from './dash-edit';
  import { isUnauthorized } from './errors';
  import ContextMenu, { type CtxItem } from './ContextMenu.svelte';
  import { hasModule, isAdmin } from './me.svelte';
  import { showToast } from './toast';
  import { openServer } from './route.svelte';
  import { assistantOffered, levelOf, rdpOffered, sparkPath, summarize, type AgentFlag } from './status';

  let { nativeWindows, host = '' }: { nativeWindows: boolean; /** The address of the Linager server (without the port): the agents connect to it. */ host?: string } = $props();

  type Layout = 'cards' | 'mini' | 'rows';
  const LKEY = 'linager.layout';

  let cards = $state<ServerCard[]>([]);
  let groups = $state<Group[]>([]);
  let loaded = $state(false);
  let addOpen = $state(false);
  // the agents that are older than the Linager server (an administrator): a tag on the card
  let agentFlags = $state<Record<string, AgentFlag>>({});
  let menu = $state<{ x: number; y: number; items: CtxItem[] } | null>(null);
  // the edit mode: order, group, take off the dashboard (with a few seconds to undo), delete
  let editing = $state(false);
  let pending = $state<{ card: ServerCard; index: number } | null>(null);
  let dragId = $state<string | null>(null);
  let overId = $state<string | null>(null);
  let orderTimer: ReturnType<typeof setTimeout> | undefined;
  let undoTimer: ReturnType<typeof setTimeout> | undefined;
  const ORDER_SAVE_MS = 400;
  const UNDO_MS = 5_000;
  /** Agents that ask to be approved: the badge of the button. */
  let waiting = $state(0);
  let failed = $state(false);
  let now = $state(Date.now());
  let filter = $state<'all' | 'bad' | 'off'>('all');
  let group = $state('all');
  let q = $state('');
  let layout = $state<Layout>(((): Layout => {
    try {
      const v = getPref(LKEY);
      if (v === 'cards' || v === 'mini' || v === 'rows') return v;
    } catch {
      // no storage
    }
    return 'cards';
  })());

  function setLayout(l: Layout): void {
    layout = l;
    setPref(LKEY, l);
  }

  async function load(): Promise<void> {
    try {
      const [d, g] = await Promise.all([api.dashboard(), api.groups()]);
      const hidden = pending?.card.id; // taken off, waiting for the end of the undo time
      cards = hidden ? d.items.filter((c) => c.id !== hidden) : d.items;
      groups = g;
      failed = false;
    } catch {
      failed = true;
    } finally {
      loaded = true;
    }
  }

  async function loadAgentFlags(): Promise<void> {
    if (!isAdmin()) return;
    try {
      const info = await api.updates();
      const next: Record<string, AgentFlag> = {};
      for (const a of info.agents) if (a.status === 'available' || a.status === 'unsupported') next[a.id] = a.status;
      agentFlags = next;
    } catch {
      // the tags are optional: keep what was there
    }
  }

  /** The right click on a server: the sessions, the page, the group (in the edit mode) and the deletion. */
  function showMenu(e: MouseEvent, card: ServerCard): void {
    e.preventDefault();
    const items: CtxItem[] = [];
    if (hasModule('terminal')) items.push({ key: 'term', label: t('dash.menu.terminal'), icon: 'square-terminal', disabled: !card.online, onselect: () => openIn(card, 'terminal') });
    if (hasModule('desktop') && rdpOffered(card)) items.push({ key: 'rdp', label: t('dash.menu.rdp'), icon: 'monitor', disabled: !card.online, onselect: () => openIn(card, 'rdp') });
    if (hasModule('assistant') && assistantOffered(card.os, card.sample)) items.push({ key: 'asst', label: t('dash.menu.asst'), icon: 'app-window', disabled: !card.online, onselect: () => openIn(card, 'asst') });
    items.push({ key: 'open', label: t('dash.menu.open'), icon: 'external-link', separated: items.length > 0, onselect: () => openServer(card.id) });
    if (isAdmin() && editing && groups.length > 0) {
      groups.forEach((g, i) => items.push({ key: `g${g.id}`, label: t('dash.menu.group', { name: g.name }), icon: card.group_id === g.id ? 'check' : undefined, separated: i === 0, onselect: () => void setCardGroup(card, String(g.id)) }));
      if (card.group_id !== null) items.push({ key: 'g0', label: t('dash.menu.groupNone'), onselect: () => void setCardGroup(card, '0') });
    }
    if (isAdmin()) items.push({ key: 'del', label: t('dash.deleteServer'), icon: 'trash-2', danger: true, separated: true, onselect: () => deleteCard(card) });
    menu = { x: e.clientX, y: e.clientY, items };
  }

  async function loadWaiting(): Promise<void> {
    try {
      waiting = (await api.enrollments()).length;
    } catch (e) {
      if (!isUnauthorized(e)) waiting = 0;
    }
  }
  /** The page of a server by its name (the new server of an install over SSH). */
  function openByName(name: string): void {
    const c = cards.find((x) => x.name === name);
    if (c) openServer(c.id);
  }

  onMount(() => {
    void load();
    if (isAdmin()) void loadWaiting();
    void loadAgentFlags();
    let timer: ReturnType<typeof setTimeout> | undefined;
    // the live stream says that something changed: the list is read again (at most once a second)
    const stop = subscribeLive(() => {
      timer ??= setTimeout(() => {
        timer = undefined;
        void load();
      }, 1000);
    });
    const tick = setInterval(() => (now = Date.now()), 30_000);
    const poll = setInterval(() => {
      void load();
      void loadAgentFlags();
    }, 30_000);
    const waitTimer = setInterval(() => isAdmin() && void loadWaiting(), 10_000);
    return () => {
      stop();
      clearInterval(tick);
      clearInterval(poll);
      clearInterval(waitTimer);
    };
  });

  // ---- edit mode ----
  const indexOf = (id: string): number => cards.findIndex((c) => c.id === id);
  const visibleIndex = (id: string): number => shown.findIndex((c) => c.id === id);

  function scheduleOrderSave(): void {
    clearTimeout(orderTimer);
    orderTimer = setTimeout(() => void persistOrder(), ORDER_SAVE_MS);
  }
  async function persistOrder(): Promise<void> {
    orderTimer = undefined;
    await commitRemoval();
    try {
      await api.dashboardOrder(cards.map((c) => c.id));
    } catch (e) {
      if (!isUnauthorized(e)) showToast(t('dash.orderFailed'));
      void load();
    }
  }
  function moveTo(fromId: string, toId: string): void {
    const from = indexOf(fromId);
    const to = indexOf(toId);
    if (from < 0 || to < 0 || from === to) return;
    const next = cards.slice();
    const [moved] = next.splice(from, 1);
    if (!moved) return;
    next.splice(to, 0, moved);
    cards = next;
    scheduleOrderSave();
  }
  /** Moves a card one step among the cards that are shown (the chosen filter and group), swapping with its visible neighbour. */
  function move(id: string, dir: -1 | 1): void {
    const index = indexOf(id);
    const neighbour = shown[visibleIndex(id) + dir];
    const target = neighbour ? indexOf(neighbour.id) : -1;
    if (index < 0 || target < 0) return;
    const next = cards.slice();
    [next[index], next[target]] = [next[target], next[index]];
    cards = next;
    scheduleOrderSave();
  }
  async function setCardGroup(card: ServerCard, value: string): Promise<void> {
    const groupId = value === '0' ? null : Number(value);
    try {
      await api.serverSetGroup(card.id, groupId);
      cards = cards.map((c) => (c.id === card.id ? { ...c, group_id: groupId } : c));
    } catch (e) {
      if (!isUnauthorized(e)) showToast(t('dash.group.setFailed'));
    }
  }
  async function removeCard(card: ServerCard): Promise<void> {
    if (indexOf(card.id) < 0) return;
    await commitRemoval();
    const at = indexOf(card.id);
    if (at < 0) return;
    cards = cards.filter((c) => c.id !== card.id);
    pending = { card, index: at };
    clearTimeout(undoTimer);
    undoTimer = setTimeout(() => void commitRemoval(), UNDO_MS);
  }
  /** Sends the delayed removal to the server once the undo time has passed. */
  async function commitRemoval(): Promise<void> {
    const current = pending;
    if (!current) return;
    clearTimeout(undoTimer);
    undoTimer = undefined;
    pending = null;
    try {
      await api.dashboardRemove(current.card.id);
    } catch (e) {
      if (!isUnauthorized(e)) showToast(t('dash.removeFailed'));
      void load();
    }
  }
  function undoRemoval(): void {
    const current = pending;
    if (!current) return;
    clearTimeout(undoTimer);
    undoTimer = undefined;
    pending = null;
    const next = cards.slice();
    next.splice(Math.min(current.index, next.length), 0, current.card);
    cards = next;
    scheduleOrderSave();
  }
  let deleting = $state<ServerCard | null>(null);
  const deleteCard = (card: ServerCard): void => {
    deleting = card;
  };
  function toggleEdit(): void {
    if (editing) {
      // leaving the mode saves what is waiting
      if (orderTimer) {
        clearTimeout(orderTimer);
        void persistOrder();
      } else void commitRemoval();
    } else showToast(t('dash.editHint'));
    editing = !editing;
  }
  function dragStart(event: DragEvent, id: string): void {
    dragId = id;
    if (event.dataTransfer) {
      event.dataTransfer.effectAllowed = 'move';
      event.dataTransfer.setData('text/plain', id);
    }
  }
  function dragOver(event: DragEvent, id: string): void {
    if (!dragId) return;
    event.preventDefault();
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'move';
    overId = id;
  }
  function drop(event: DragEvent, id: string): void {
    event.preventDefault();
    if (dragId && dragId !== id) moveTo(dragId, id);
    dragId = null;
    overId = null;
  }
  function dragEnd(): void {
    dragId = null;
    overId = null;
  }
  const rowEdit = $derived<RowEdit | null>(editing ? { groups, move, group: (id, v) => { const c = cards.find((x) => x.id === id); if (c) void setCardGroup(c, v); }, remove: (id) => { const c = cards.find((x) => x.id === id); if (c) void removeCard(c); }, del: (id) => { const c = cards.find((x) => x.id === id); if (c) deleteCard(c); }, dragStart, dragOver, drop, dragEnd, overId: () => overId, dragId: () => dragId } : null);

  function openIn(c: ServerCard, kind: 'terminal' | 'rdp' | 'asst'): void {
    void native.openWindow(kind, c.id, c.name, nativeWindows);
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

<div class="dash">
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
  <SearchField id="q" placeholder={t('dash.search')} bind:value={q} />
  {#if isAdmin()}<button class="btn sm" class:primary={editing} type="button" aria-pressed={editing} onclick={toggleEdit}><Icon name={editing ? 'check' : 'pencil'} size={14} />{editing ? t('dash.done') : t('dash.edit')}</button>
  <button class="btn sm" type="button" onclick={() => (addOpen = true)}><Icon name="plus" size={14} />{t('asv.openDash')}{#if waiting > 0} <span class="badge b-warn">{waiting}</span>{/if}</button>{/if}
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
  <div class="empty"><Icon name="server" size={26} /><p>{t('dash.empty')}</p>{#if isAdmin()}<button class="btn primary" type="button" onclick={() => (addOpen = true)}><Icon name="plus" size={14} />{t('asv.openDash')}</button>{/if}</div>
{:else if shown.length === 0}
  <div class="empty"><Icon name="search" size={26} /><p>{t('dash.nothing')}</p></div>
{:else if layout === 'rows'}
  <div class="tbl-wrap fill"><Rows cards={shown} {now} edit={rowEdit} agents={agentFlags} onmenu={showMenu} onopen={openIn} /></div>
{:else if layout === 'mini'}
  <div class="dash-scroll"><div class="grid mini-grid">{#each shown as c (c.id)}
    <div class="slot" class:editing class:over={editing && overId === c.id && dragId !== c.id} class:dragging={dragId === c.id} draggable={editing} role="presentation" oncontextmenu={(e) => showMenu(e, c)} ondragstart={(e) => dragStart(e, c.id)} ondragover={(e) => dragOver(e, c.id)} ondrop={(e) => drop(e, c.id)} ondragend={dragEnd}>
      <Mini card={c} {editing} agent={agentFlags[c.id] ?? null} onopen={openIn} />
      {#if editing}<EditBar card={c} index={visibleIndex(c.id)} count={shown.length} {groups} onmove={(d) => move(c.id, d)} ongroup={(v) => void setCardGroup(c, v)} onremove={() => void removeCard(c)} ondelete={() => deleteCard(c)} />{/if}
    </div>
  {/each}</div></div>
{:else}
  <div class="dash-scroll"><div class="grid">{#each shown as c (c.id)}
    <div class="slot" class:editing class:over={editing && overId === c.id && dragId !== c.id} class:dragging={dragId === c.id} draggable={editing} role="presentation" oncontextmenu={(e) => showMenu(e, c)} ondragstart={(e) => dragStart(e, c.id)} ondragover={(e) => dragOver(e, c.id)} ondrop={(e) => drop(e, c.id)} ondragend={dragEnd}>
      <Card card={c} {now} {editing} agent={agentFlags[c.id] ?? null} onopen={openIn} />
      {#if editing}<EditBar card={c} index={visibleIndex(c.id)} count={shown.length} {groups} onmove={(d) => move(c.id, d)} ongroup={(v) => void setCardGroup(c, v)} onremove={() => void removeCard(c)} ondelete={() => deleteCard(c)} />{/if}
    </div>
  {/each}</div></div>
{/if}
</div>

{#if addOpen}
  <AddServerModal {host} onclose={() => { addOpen = false; void loadWaiting(); }} onadded={() => { void load(); void loadWaiting(); }} onopen={openByName} onauthlost={() => (addOpen = false)} />
{/if}

{#if pending}
  <div class="undo-bar" role="status" aria-live="polite">
    <span>{t('dash.undo.text', { name: pending.card.name })}</span>
    <button class="btn sm" type="button" onclick={undoRemoval}>{t('dash.undo.button')}</button>
  </div>
{/if}

{#if deleting}
  <DeleteServerModal server={deleting} onclose={() => (deleting = null)} ondeleted={() => { const gone = deleting; deleting = null; if (gone) cards = cards.filter((c) => c.id !== gone.id); void load(); }} />
{/if}

{#if menu}<ContextMenu x={menu.x} y={menu.y} items={menu.items} onclose={() => (menu = null)} />{/if}
