<script lang="ts">
  import { onMount } from 'svelte';
  import { ApiError, serverApi, subscribeLive, type ServerDetail } from '../api';
  import { confirmAsk } from '../dialogs.svelte';
  import DeleteServerModal from '../DeleteServerModal.svelte';
  import AgentUpdateBanner from './AgentUpdateBanner.svelte';
  import Icon from '../Icon.svelte';
  import { t } from '../i18n.svelte';
  import { native } from '../native';
  import { openDashboard, openServer } from '../route.svelte';
  import { assistantOffered, isWindows, levelOf } from '../status';
  import DiskTab from './DiskTab.svelte';
  import DockerTab from './DockerTab.svelte';
  import FirewallTab from './FirewallTab.svelte';
  import NetworkTab from './NetworkTab.svelte';
  import UsersTab from './UsersTab.svelte';
  import WinNetworkTab from './win/WinNetworkTab.svelte';
  import WinProgramsTab from './win/WinProgramsTab.svelte';
  import WinUpdatesTab from './win/WinUpdatesTab.svelte';
  import WinUsersTab from './win/WinUsersTab.svelte';
  import SecurityTab from './SecurityTab.svelte';
  import FilesTab from './FilesTab.svelte';
  import LogsTab from './LogsTab.svelte';
  import MetricsTab from './MetricsTab.svelte';
  import OverviewTab from './OverviewTab.svelte';
  import ProcessesTab from './ProcessesTab.svelte';
  import ServicesTab from './ServicesTab.svelte';
  import TerminalTab from './TerminalTab.svelte';
  import UpdatesTab from './UpdatesTab.svelte';
  import AssistantPane from '../assistant/AssistantPane.svelte';
  import DesktopPane from '../desktop/DesktopPane.svelte';

  let { id, tab, nativeWindows }: { id: string; tab: string; nativeWindows: boolean } = $props();

  let s = $state<ServerDetail | null>(null);
  let missing = $state(false);
  let collapsed = $state(false);
  let closedGroups = $state<Record<string, boolean>>({});

  interface Item {
    key: string;
    icon: string;
    label: string;
    ready: boolean;
    window?: 'terminal' | 'rdp' | 'asst';
  }
  const win = $derived(s ? isWindows(s.os) : false);
  const groups = $derived<{ key: string; items: Item[] }[]>([
    { key: 'general', items: [
      { key: 'overview', icon: 'layout-grid', label: t('sp.overview'), ready: true },
      { key: 'metrics', icon: 'activity', label: t('sp.metrics'), ready: true },
      { key: 'terminal', icon: 'square-terminal', label: win ? 'PowerShell' : t('sp.terminal'), ready: true, window: 'terminal' },
      ...(win && s?.sample?.desktop_off !== true ? [{ key: 'desktop', icon: 'monitor', label: 'RDP', ready: true, window: 'rdp' as const }] : []),
      ...(assistantOffered(s?.os ?? '', s?.sample) ? [{ key: 'assistant', icon: 'app-window', label: t('sp.assistant'), ready: true, window: 'asst' as const }] : []),
    ] },
    { key: 'system', items: [
      { key: 'services', icon: 'list-checks', label: t('sp.services'), ready: true },
      { key: 'processes', icon: 'cpu', label: t('sp.processes'), ready: true },
      { key: 'logs', icon: 'scroll-text', label: t('sp.logs'), ready: !win },
      { key: 'updates', icon: 'download', label: t('sp.updates'), ready: true },
    ] },
    { key: 'fs', items: [
      { key: 'files', icon: 'folder', label: t('sp.files'), ready: true },
      { key: 'disk', icon: 'hard-drive', label: t('sp.disk'), ready: true },
      ...(win ? [{ key: 'programs', icon: 'layout-grid', label: t('sp.programs'), ready: true }] : []),
    ] },
    { key: 'net', items: win
      ? [{ key: 'network', icon: 'network', label: t('sp.network'), ready: true }, { key: 'users', icon: 'users', label: t('sp.users'), ready: true }]
      : [{ key: 'network', icon: 'network', label: t('sp.network'), ready: true }, { key: 'firewall', icon: 'shield-check', label: t('sp.firewall'), ready: true },
         { key: 'security', icon: 'lock', label: t('sp.security'), ready: true }, { key: 'users', icon: 'users', label: t('sp.users'), ready: true }] },
    ...(win ? [] : [{ key: 'apps', items: [{ key: 'docker', icon: 'container', label: 'Docker', ready: true }] }]),
  ]);
  /** A session of the remote desktop or the Assistant can be open in one place only: when its tab is on the screen it makes way for the window. */
  function inWindow(it: Item): void {
    if (!it.window || !s) return;
    if (it.window !== 'terminal' && current.key === it.key) openServer(id, 'overview');
    void native.openWindow(it.window, id, s.name, nativeWindows);
  }
  const flat = $derived(groups.flatMap((g) => g.items));
  const current = $derived(flat.find((i) => i.key === tab && i.ready) ?? flat[0]);
  const group = $derived(groups.find((g) => g.items.includes(current))!);
  const level = $derived(s ? levelOf({ ...s, spark: s.spark, group_id: null }) : 'off');
  const reboot = $derived(!!s?.sample?.reboot_required);

  async function load(): Promise<void> {
    try {
      const d = await serverApi.detail(id);
      // the record is updated in place: replacing it would restart everything that reads it (the terminal, the files)
      if (s) {
        for (const k of Object.keys(s)) if (!(k in d)) delete (s as unknown as Record<string, unknown>)[k];
        Object.assign(s, d);
      } else s = d;
      missing = false;
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) missing = true;
    }
  }

  onMount(() => {
    void load();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const stop = subscribeLive(() => {
      timer ??= setTimeout(() => {
        timer = undefined;
        void load();
      }, 1500);
    });
    const poll = setInterval(() => void load(), 15_000);
    return () => {
      stop();
      clearInterval(poll);
    };
  });

  async function doReboot(): Promise<void> {
    if (!s || !(await confirmAsk({ title: t('sp.reboot'), text: t('sp.reboot.text', { name: s.name }), ok: t('sp.reboot'), danger: true }))) return;
    try {
      await serverApi.reboot(id);
    } catch {
      // the next refresh shows what happened
    }
  }

  let deleting = $state(false);
  function pick(it: Item): void {
    if (!it.ready) return;
    openServer(id, it.key);
  }
</script>

{#if missing}
  <div class="center"><div class="panel-card"><h1>{t('sp.missing')}</h1><button class="btn" type="button" onclick={openDashboard}>{t('sp.back')}</button></div></div>
{:else if !s}
  <div class="center"><span class="muted">{t('dash.loading')}</span></div>
{:else}
  <div class="sp" class:collapsed>
    <aside class="sp-nav">
      <div class="sp-id">
        <button class="btn ghost sm back" type="button" onclick={openDashboard} title={t('sp.back')}><Icon name="arrow-left" size={14} /><span>{t('sp.back')}</span></button>
        <div class="sp-name"><span class="dot {level}"></span><b>{s.name}</b><span class="os" class:os-windows={win}>{win ? 'WIN' : 'LIN'}</span></div>
        <div class="sp-ip mono">{s.address}</div>
      </div>
      <div class="sp-groups">
        {#each groups as g (g.key)}
          <div class="sg">
            <button class="sg-h" type="button" onclick={() => (closedGroups[g.key] = !closedGroups[g.key])}><span>{t(`sp.group.${g.key}`)}</span><Icon name={closedGroups[g.key] ? 'chevron-right' : 'chevron-down'} size={13} /></button>
            {#if !closedGroups[g.key]}
              {#each g.items as it (it.key)}
                <div class="si-row">
                  <button class="si" type="button" aria-current={current.key === it.key} disabled={!it.ready} title={it.ready ? it.label : `${it.label} · ${t('nav.soon')}`} onclick={() => pick(it)}>
                    <Icon name={it.icon} size={15} /><span>{it.label}</span>
                    {#if it.key === 'updates' && s.sample && s.sample.updates > 0}<em class="cnt">{s.sample.updates}</em>{/if}
                  </button>
                  {#if it.window}
                    <button class="si-w" type="button" title={t('sp.inWindow')} disabled={!s.online} onclick={() => inWindow(it)}><Icon name="app-window" size={13} /></button>
                  {/if}
                </div>
              {/each}
            {/if}
          </div>
        {/each}
      </div>
      <div class="sp-foot"><span class="mono">agent {s.agent_version}</span>
        <button class="btn xs" type="button" onclick={() => (collapsed = !collapsed)} title={collapsed ? t('sp.expand') : t('sp.collapse')}><Icon name={collapsed ? 'panel-left-open' : 'panel-left-close'} size={15} /></button>
      </div>
    </aside>

    <section class="sp-content">
      <div class="sp-ph">
        <div><small>{t(`sp.group.${group.key}`)}</small><h1>{current.label}</h1></div>
        <span class="grow"></span>
        <button class="btn sm" type="button" onclick={() => native.openWindow('note', id, s!.name, nativeWindows)}><Icon name="sticky-note" size={14} />{t('sp.note')}{#if s.note}<i class="nd"></i>{/if}</button>
        <button class="btn sm" type="button" disabled={!s.online} onclick={doReboot}><Icon name="power" size={14} />{t('sp.reboot')}</button>
        <button class="btn sm danger" type="button" onclick={() => (deleting = true)} title={t('sp.delete')}><Icon name="trash-2" size={14} /></button>
      </div>
      <AgentUpdateBanner server={s} onchanged={() => void load()} />
      {#if reboot}<div class="banner"><Icon name="triangle-alert" size={14} />{t('sp.reboot.needed')}</div>{/if}
      <div class="sp-body">
        {#key current.key}
          {#if current.key === 'overview'}<OverviewTab {s} />
          {:else if current.key === 'metrics'}<MetricsTab {s} />
          {:else if current.key === 'terminal'}<TerminalTab {s} {nativeWindows} />
          {:else if current.key === 'desktop'}<div class="sp-asst"><DesktopPane serverId={s.id} name={s.name} address={s.address} online={s.online} {nativeWindows} /></div>
          {:else if current.key === 'assistant'}<div class="sp-asst"><AssistantPane serverId={s.id} name={s.name} online={s.online} {nativeWindows} /></div>
          {:else if current.key === 'services'}<ServicesTab {s} />
          {:else if current.key === 'processes'}<ProcessesTab {s} />
          {:else if current.key === 'logs'}<LogsTab {s} />
          {:else if current.key === 'updates'}{#if win}<WinUpdatesTab {s} />{:else}<UpdatesTab {s} />{/if}
          {:else if current.key === 'programs'}<WinProgramsTab {s} />
          {:else if current.key === 'disk'}<DiskTab {s} />
          {:else if current.key === 'files'}<FilesTab {s} {nativeWindows} />
          {:else if current.key === 'security'}<SecurityTab {s} />
          {:else if current.key === 'users'}{#if win}<WinUsersTab {s} />{:else}<UsersTab {s} {nativeWindows} />{/if}
          {:else if current.key === 'network'}{#if win}<WinNetworkTab {s} />{:else}<NetworkTab {s} />{/if}
          {:else if current.key === 'firewall'}<FirewallTab {s} />
          {:else if current.key === 'docker'}<DockerTab {s} />
          {/if}
        {/key}
      </div>
    </section>
  </div>
{/if}

{#if deleting && s}
  <DeleteServerModal server={s} onclose={() => (deleting = false)} ondeleted={() => { deleting = false; openDashboard(); }} />
{/if}
