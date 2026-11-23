<script lang="ts">
  import { api, isUnauthorized, type Role, type ServerDetail, type Sample } from '../../api';
  import { locale, tr } from '../../i18n';
  import { subscribeLive } from '../../live';
  import DeleteServerModal from '../DeleteServerModal.svelte';
  import NoteModal from './NoteModal.svelte';
  import { showToast } from '../toast';
  import MetricsTab from './MetricsTab.svelte';
  import OverviewTab from './OverviewTab.svelte';
  import AssistantTab from './tabs/AssistantTab.svelte';
  import DesktopTab from './tabs/DesktopTab.svelte';
  import ManageTab from './tabs/ManageTab.svelte';
  import TerminalTab from './tabs/TerminalTab.svelte';
  import LogsTab from './tabs/LogsTab.svelte';
  import ProcessesTab from './tabs/ProcessesTab.svelte';
  import ServicesTab from './tabs/ServicesTab.svelte';
  import UpdatesTab from './tabs/UpdatesTab.svelte';
  import RebootButton from './tabs/RebootButton.svelte';
  import FilesTab from './tabs/FilesTab.svelte';
  import DiskTab from './tabs/DiskTab.svelte';
  import NetworkTab from './tabs/NetworkTab.svelte';
  import FirewallTab from './tabs/FirewallTab.svelte';
  import SecurityTab from './tabs/SecurityTab.svelte';
  import UsersTab from './tabs/UsersTab.svelte';
  import WinNetworkTab from './tabs/WinNetworkTab.svelte';
  import WinProgramsTab from './tabs/WinProgramsTab.svelte';
  import WinUpdatesTab from './tabs/WinUpdatesTab.svelte';
  import WinUsersTab from './tabs/WinUsersTab.svelte';
  import { osKind } from '../dashboard/status';
  import DockerTab from './tabs/DockerTab.svelte';
  import { formatDateTime } from './format';
  import { serverBar } from './serverbar';
  import { openAssistantWindow, openDesktopWindow, openTermWindow } from '../terminal/windows';
  import AgentUpdateBanner from './AgentUpdateBanner.svelte';
  import SideNav from './SideNav.svelte';
  import SectionsSheet from './SectionsSheet.svelte';
  import Modal from '../Modal.svelte';
  import PixIcon from '../ui/PixIcon.svelte';
  import TabBar, { type TabBarItem } from '../ui/TabBar.svelte';
  import { groupTabs, HISTORY_LIMIT, visibleTabs, type LiveSample, type TabId } from './model';
  import { desktopHidden, rdpOverride } from './rdp';

  let {
    name,
    tab: requestedTab = null,
    ontab,
    onBack,
    onagentchanged,
    role = 'admin',
    userModules = [],
  }: {
    name: string;
    /** Tab from the URL hash; unknown or hidden tabs fall back to the overview. */
    tab?: string | null;
    ontab?: (tab: TabId) => void;
    onBack: () => void;
    /** Called when an agent update of this server finished (refreshes the header counter). */
    onagentchanged?: () => void;
    /** An operator only gets the tabs of the modules that were given, an auditor none. */
    role?: Role;
    userModules?: string[];
  } = $props();

  let detail = $state<ServerDetail | null>(null);
  let status = $state<'loading' | 'ready' | 'missing' | 'error'>('loading');
  let live = $state<LiveSample[]>([]);
  let deleteOpen = $state(false);
  let attempt = $state(0);
  let sectionsOpen = $state(false);
  let menuOpen = $state(false);
  let noteOpen = $state(false);

  const serverId = $derived(detail?.id ?? null);
  const sample = $derived<Sample | null>(detail?.sample ?? null);
  const online = $derived(detail?.online ?? false);
  const isAdmin = $derived(role === 'admin');
  // a terminal as a user needs the terminal module too, on the agent and in the rights of the operator
  const isWin = $derived(osKind(detail?.os ?? '') === 'windows');
  const canDesktop = $derived((detail?.modules ?? []).includes('desktop') && (isAdmin || (role === 'operator' && userModules.includes('desktop'))));
  const canAssistant = $derived((detail?.modules ?? []).includes('assistant') && (isAdmin || (role === 'operator' && userModules.includes('assistant'))));
  const canTerminal = $derived((detail?.modules ?? []).includes('terminal') && (isAdmin || (role === 'operator' && userModules.includes('terminal'))));
  // the remote desktop is off (or the edition has none): the "Desktop" tab is not offered, and it comes back when it is turned on
  const deskHidden = $derived(desktopHidden(detail?.id ?? '', sample, $rdpOverride));
  // a Linux computer has the Assistant only with a graphical console, and no "Management" (that is RDP and Windows policies)
  const tabs = $derived(
    visibleTabs((detail?.modules ?? []).filter((m) => !(m === 'desktop' && deskHidden)), isAdmin ? null : role === 'operator' ? userModules : []).filter(
      (x) => isWin || (x.id !== 'manage' && (x.id !== 'assistant' || sample?.gui === true)),
    ),
  );
  const tab = $derived<TabId>(tabs.find((x) => x.id === requestedTab)?.id ?? 'overview');
  const currentTabKey = $derived(tabs.find((x) => x.id === tab)?.key ?? 'srv.tab.overview');
  const currentGroupKey = $derived(groupTabs(tabs).find((g) => g.tabs.some((x) => x.id === tab))?.key ?? 'srv.group.general');

  /** Phone: the tabs that sit in the bottom bar; everything else is under "Sections". */
  const PINNED: TabId[] = ['overview', 'terminal', 'files', 'services'];
  const barTabs = $derived.by(() => {
    const ids = tabs.map((x) => x.id);
    const picked = PINNED.filter((id) => ids.includes(id));
    for (const id of ids) if (picked.length < 4 && !picked.includes(id)) picked.push(id);
    return picked.slice(0, 4);
  });
  const badges = $derived<Partial<Record<TabId, { text: string; tone: 'warn' | 'crit' }>>>({
    ...((sample?.updates ?? 0) > 0 ? { updates: { text: String(sample?.updates), tone: (sample?.security_updates ?? 0) > 0 ? 'crit' : 'warn' } } : {}),
    ...((sample?.image_updates ?? 0) > 0 ? { docker: { text: String(sample?.image_updates), tone: 'warn' } } : {}),
    ...((sample?.failed_services ?? 0) > 0 ? { services: { text: String(sample?.failed_services), tone: 'crit' } } : {}),
  } as Partial<Record<TabId, { text: string; tone: 'warn' | 'crit' }>>);
  const barItems = $derived<TabBarItem[]>([
    ...barTabs.map((id) => ({
      id,
      label: $tr(tabs.find((x) => x.id === id)?.key ?? ''),
      icon: id,
      current: tab === id,
      badge: badges[id]?.text,
      tone: badges[id]?.tone,
    })),
    { id: '#sections', label: $tr('nav.sections'), icon: 'more', current: !barTabs.includes(tab) },
  ]);

  function onBar(id: string): void {
    if (id === '#sections') sectionsOpen = true;
    else ontab?.(id as TabId);
  }

  // Resolve the server by name, then load its detail. Reruns on name change or retry.
  $effect(() => {
    const wanted = name;
    void attempt;
    let active = true;
    status = 'loading';
    detail = null;
    live = [];

    (async () => {
      try {
        const list = await api.servers();
        const found = list.find((s) => s.name === wanted);
        if (!found) {
          if (active) status = 'missing';
          return;
        }
        const d = await api.server(found.id);
        if (!active) return;
        detail = d;
        live = (d.recent ?? []).map((p) => ({ cpu: p.cpu, mem: p.mem, rx: p.rx, tx: p.tx })).slice(-HISTORY_LIMIT);
        status = 'ready';
      } catch (e) {
        if (!active) return;
        if (isUnauthorized(e)) {
          onBack();
          return;
        }
        status = 'error';
      }
    })();

    return () => {
      active = false;
    };
  });

  // Live updates for this server only. Depends on the id, not on the detail object,
  // so the shared connection is not re-subscribed on every sample.
  $effect(() => {
    const id = serverId;
    if (!id) return;
    return subscribeLive((e) => {
      if (e.type === 'dashboard' || e.server_id !== id || !detail) return;
      if (e.type === 'metrics') {
        const s = e.sample;
        // Mutate in place: tabs depend on individual fields (id, online); replacing the object would
        // restart their streams and polling on every sample.
        detail.online = true;
        detail.sample = s;
        live = [...live, { cpu: s.cpu, mem: s.mem_pct, rx: s.rx_bps, tx: s.tx_bps }].slice(-HISTORY_LIMIT);
      } else if (e.type === 'status') {
        detail.online = e.online;
        detail.last_seen_at = e.last_seen_at;
      }
    });
  });

  // The top bar shows the name, the status and the actions of this server.
  $effect(() => {
    if (!detail) {
      serverBar.set(null);
      return;
    }
    const d = detail;
    serverBar.set({
      detail: d,
      online,
      isAdmin,
      lastSeen: !online && d.last_seen_at ? $tr('srv.lastSeen', { date: formatDateTime(d.last_seen_at, $locale) }) : '',
      hasNote: Boolean(d.note?.trim()),
      onback: onBack,
      onnote: () => (noteOpen = true),
      ondelete: () => (deleteOpen = true),
    });
  });
  $effect(() => () => serverBar.set(null));
</script>

<section class="srv" data-tab={tab}>
  <!-- Phone header: back, the section as a title, server and its state under it, the actions in a sheet. -->
  <header class="phead">
    <button class="btn ic" type="button" aria-label={$tr('srv.back')} onclick={onBack}><PixIcon name="back" size={16} /></button>
    <div class="pt">
      <h1>{detail ? $tr(currentTabKey) : name}</h1>
      <p class="mono">
        {#if detail}<span class="sq" class:online aria-hidden="true"></span>{detail.name} · {detail.address} · {online ? $tr('srv.online') : $tr('srv.offline')}{:else}{name}{/if}
      </p>
    </div>
    {#if detail}
      <button class="btn ic note" class:has={Boolean(detail.note?.trim())} type="button" aria-label={$tr('srv.note.open')} onclick={() => (noteOpen = true)}><PixIcon name="note" size={16} /></button>
    {/if}
    {#if detail && isAdmin}
      <button class="btn ic" type="button" aria-label={$tr('srv.menu')} onclick={() => (menuOpen = true)}><PixIcon name="more" size={16} /></button>
    {/if}
  </header>

  {#if status === 'loading'}
    <p class="muted">{$tr('common.loading')}</p>
  {:else if status === 'missing'}
    <p class="card">{$tr('srv.notFound')}</p>
  {:else if status === 'error' || !detail}
    <div class="card state">
      <p class="error" role="alert">{$tr('srv.loadFailed')}</p>
      <div class="row">
        <button class="btn" type="button" onclick={() => attempt++}>{$tr('srv.retry')}</button>
      </div>
    </div>
  {:else}
    {#if isAdmin}<AgentUpdateBanner server={detail} onchanged={onagentchanged} />{/if}

    {#if sample?.reboot_required}
      <p class="banner" role="status">{$tr('srv.rebootRequired')}</p>
    {/if}

    <div class="body">
      <SideNav
        {tabs}
        current={tab}
        onselect={(id) => ontab?.(id)}
        updates={sample?.updates ?? 0}
        securityUpdates={sample?.security_updates ?? 0}
        failedServices={sample?.failed_services ?? 0}
        imageUpdates={sample?.image_updates ?? 0}
        agentVersion={detail.agent_version ? `agent ${detail.agent_version}` : ''}
        onterminalwindow={canTerminal && online ? () => openTermWindow(detail!.id, detail!.name) : undefined}
        ondesktopwindow={canDesktop && online && !deskHidden ? () => openDesktopWindow(detail!.id, detail!.name, detail!.address) : undefined}
        onassistantwindow={canAssistant && online ? () => openAssistantWindow(detail!.id, detail!.name) : undefined}
      />
      <div class="main">
        <div class="crumb">{$tr(currentGroupKey)}</div>
        <h2 class="page-title">{$tr(currentTabKey)}</h2>
    <div class="tabpanel" role="tabpanel">
      {#if tab === 'overview'}
        <OverviewTab server={detail} {sample} {live} {online} />
      {:else if tab === 'metrics'}
        <MetricsTab serverId={detail.id} />
      {:else if tab === 'files'}
        <FilesTab server={detail} />
      {:else if tab === 'disk'}
        <DiskTab server={detail} />
      {:else if tab === 'network'}
        {#if isWin}<WinNetworkTab server={detail} />{:else}<NetworkTab server={detail} />{/if}
      {:else if tab === 'firewall'}
        <FirewallTab server={detail} />
      {:else if tab === 'docker'}
        <DockerTab server={detail} />
      {:else if tab === 'terminal'}
        <TerminalTab server={detail} />
      {:else if tab === 'desktop'}
        <DesktopTab server={detail} />
      {:else if tab === 'assistant'}
        <AssistantTab server={detail} />
      {:else if tab === 'manage'}
        <ManageTab server={detail} {canDesktop} />
      {:else if tab === 'services'}
        <ServicesTab server={detail} />
      {:else if tab === 'updates'}
        {#if isWin}<WinUpdatesTab server={detail} />{:else}<UpdatesTab server={detail} />{/if}
      {:else if tab === 'programs'}
        <WinProgramsTab server={detail} />
      {:else if tab === 'processes'}
        <ProcessesTab server={detail} />
      {:else if tab === 'security'}
        <SecurityTab server={detail} />
      {:else if tab === 'users'}
        {#if isWin}<WinUsersTab server={detail} />{:else}<UsersTab server={detail} canTerminal={canTerminal} />{/if}
      {:else if tab === 'logs'}
        <LogsTab server={detail} />
      {:else}
        <p class="soon">{$tr('srv.soon')}</p>
      {/if}
    </div>
      </div>
    </div>
  {/if}
</section>

{#if detail}
  <TabBar items={barItems} label={$tr('srv.tabsLabel')} onselect={onBar} />
{/if}

{#if sectionsOpen}
  <SectionsSheet
    {tabs}
    current={tab}
    {badges}
    onclose={() => (sectionsOpen = false)}
    onselect={(id) => {
      sectionsOpen = false;
      ontab?.(id);
    }} />
{/if}

{#if menuOpen && detail}
  <Modal title={detail.name} labelId="srv-menu-title" onclose={() => (menuOpen = false)}>
    <RebootButton server={detail} />
    <button class="btn danger" type="button" onclick={() => { menuOpen = false; deleteOpen = true; }}>{$tr('srv.delete')}</button>
  </Modal>
{/if}

{#if noteOpen && detail}
  <NoteModal
    server={detail}
    canEdit={isAdmin}
    onclose={() => (noteOpen = false)}
    onsaved={(n) => {
      if (!detail) return;
      detail.note = n.note;
      detail.note_updated_at = n.note_updated_at;
      detail.note_updated_by = n.note_updated_by;
    }}
    onauthlost={onBack}
  />
{/if}

{#if deleteOpen && detail}
  <DeleteServerModal
    server={detail}
    onclose={() => (deleteOpen = false)}
    ondeleted={() => {
      deleteOpen = false;
      showToast($tr('srv.deleted'));
      onBack();
    }}
    onauthlost={onBack}
  />
{/if}

<style>
  .srv {
    display: flex;
    flex-direction: column;
    gap: 11px;
    max-width: 1296px;
    margin: 0 auto;
  }

  .banner {
    margin: 0;
    padding: 7px 11px;
    border: 2px solid var(--warn);
    color: var(--warn);
  }

  .state {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 11px;
  }

  /* Sidebar with the tabs on the left, the panel on the right. */
  .body {
    display: flex;
    align-items: flex-start;
    gap: 18px;
    margin-top: 5px;
  }

  .main {
    flex: 1 1 auto;
    min-width: 0;
  }

  .crumb {
    font-family: var(--font-display);
    font-size: 11px;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--muted);
  }

  .page-title {
    margin: 2px 0 11px;
    font-family: var(--font-display);
    font-size: 23px;
    font-weight: 400;
    line-height: 1.2;
  }

  .tabpanel {
    background: var(--surface);
    border: 2px solid var(--line);
    box-shadow: 4px 4px 0 var(--shade);
    padding: 14px;
    min-width: 0;
  }

  .soon {
    margin: 0;
    padding: 43px 0;
    text-align: center;
    color: var(--muted);
    font-family: var(--font-display);
    font-size: 18px;
  }

  .phead {
    display: none;
  }

  @media (max-width: 860px) {
    .body {
      flex-direction: column;
      align-items: stretch;
      gap: 13px;
    }
  }

  @media (max-width: 720px) {
    .srv {
      gap: 11px;
      padding-bottom: calc(var(--bar-h) + var(--safe-b) + 14px);
    }

    /* One header (the global bar is hidden on a server page); the side menu became the bottom bar. */
    .crumb,
    .page-title {
      display: none;
    }

    .phead {
      position: sticky;
      top: 0;
      z-index: 10;
      display: flex;
      align-items: center;
      gap: 7px;
      min-height: 56px;
      padding: 5px 11px 5px 7px;
      padding-top: calc(5px + var(--safe-t));
      background: var(--surface);
      border-bottom: 2px solid var(--line);
    }

    .phead .btn {
      flex: none;
      width: 44px;
      padding: 0;
    }

    .phead .btn:first-child {
      border-color: transparent;
      box-shadow: none;
      background: none;
    }

    .pt {
      flex: 1;
      min-width: 0;
    }

    .pt h1 {
      overflow: hidden;
      margin: 0;
      font-family: var(--font-display);
      font-size: 20px;
      line-height: 1.15;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .pt p {
      display: flex;
      align-items: center;
      gap: 5px;
      overflow: hidden;
      margin: 0;
      color: var(--muted);
      font-size: 10px;
      white-space: nowrap;
    }

    .pt .sq {
      flex: none;
      width: 8px;
      height: 8px;
      background: var(--muted);
    }

    .pt .sq.online {
      background: var(--good);
    }

    .srv > :global(:not(.phead)) {
      margin-left: 11px;
      margin-right: 11px;
    }

    .body {
      margin-top: 0;
    }

    /* No frame around the tab: its blocks are the cards, so the width is not spent twice. */
    .tabpanel {
      padding: 0;
      background: none;
      border: 0;
      box-shadow: none;
    }
  }
</style>
