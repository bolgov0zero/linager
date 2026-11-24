<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type RolloutMode, type UpdatesInfo } from '../api';
  import { t } from '../i18n.svelte';
  import { showToast } from '../toast';
  import Fold from './Fold.svelte';
  import UpdAgentsPanel from './UpdAgentsPanel.svelte';
  import UpdCachePanel from './UpdCachePanel.svelte';
  import UpdHistory from './UpdHistory.svelte';
  import UpdServerPanel from './UpdServerPanel.svelte';
  import { errText, fmtSize } from './upd-format';

  // Updates of the Linager server and of the agents: the server and the agents are updated from here, the cache of the
  // downloaded releases is kept, and the history is listed. The polling is quick while something runs.
  const FAST_MS = 2000;
  const SLOW_MS = 15000;

  let info = $state<UpdatesInfo | null>(null);
  let loadFailed = $state(false);
  /** The server answers no more, but an update was running: the service is restarting. */
  let restarting = $state(false);
  /** Set between starting the server update and the first poll that sees it. */
  let serverPending = $state(false);
  let pendingRunId: string | null = null;
  let checking = $state(false);
  let channelBusy = $state(false);
  let prefetching = $state<string | null>(null);
  let pruning = $state(false);
  let openCli = $state(false);

  const serverRunning = $derived(info?.server.update_run.state === 'running');
  const serverBusy = $derived(Boolean(serverRunning) || serverPending);
  const agentsBusy = $derived(info ? info.agents.some((a) => a.status === 'updating' || a.status === 'queued') : false);
  const rolloutRunning = $derived(info?.rollout?.state === 'running');
  const busy = $derived(restarting || serverBusy || agentsBusy || Boolean(rolloutRunning));

  let inflight = false;
  let destroyed = false;
  let timer: ReturnType<typeof setTimeout> | undefined;

  function applyInfo(next: UpdatesInfo): void {
    if (serverPending && (next.server.update_run.state === 'running' || next.server.update_run.run_id !== pendingRunId)) serverPending = false;
    info = next;
  }
  async function refresh(): Promise<void> {
    if (inflight || destroyed) return;
    inflight = true;
    try {
      applyInfo(await api.updates());
      restarting = false;
      loadFailed = false;
    } catch {
      // the server drops while the service restarts: keep polling quietly
      if (serverBusy || restarting) restarting = true;
      else loadFailed = true;
    } finally {
      inflight = false;
    }
  }
  function scheduleNext(): void {
    clearTimeout(timer);
    if (destroyed || document.hidden) return;
    timer = setTimeout(() => void tick(), busy ? FAST_MS : SLOW_MS);
  }
  async function tick(): Promise<void> {
    await refresh();
    scheduleNext();
  }
  /** Refreshes now and restarts the polling clock. */
  function kick(): void {
    clearTimeout(timer);
    void tick();
  }
  onMount(() => {
    const onVisibility = (): void => {
      if (!document.hidden) kick();
    };
    document.addEventListener('visibilitychange', onVisibility);
    void tick();
    return () => {
      destroyed = true;
      clearTimeout(timer);
      document.removeEventListener('visibilitychange', onVisibility);
    };
  });

  async function check(): Promise<void> {
    checking = true;
    try {
      const next = await api.updatesCheck();
      applyInfo(next);
      showToast(next.server.available && next.server.available !== next.server.current ? t('st.upd.toast.found', { version: next.server.available }) : t('st.upd.toast.none'));
    } catch (e) {
      showToast(t('st.upd.err.request', { error: errText(e) }));
    } finally {
      checking = false;
    }
  }
  async function changeChannel(channel: 'stable' | 'beta'): Promise<void> {
    channelBusy = true;
    try {
      applyInfo(await api.updatesChannel(channel));
    } catch (e) {
      showToast(t('st.upd.err.request', { error: errText(e) }));
    } finally {
      channelBusy = false;
    }
  }
  async function startServerUpdate(version: string): Promise<void> {
    pendingRunId = info?.server.update_run.run_id ?? null;
    serverPending = true;
    try {
      await api.updatesServer(version);
      kick();
    } catch (e) {
      serverPending = false;
      showToast(t('st.upd.err.request', { error: errText(e) }));
    }
  }
  async function prune(): Promise<void> {
    pruning = true;
    try {
      const next = await api.updatesCachePrune();
      applyInfo(next);
      if (next.pruned) showToast(t('st.upd.cache.pruneDone', { n: next.pruned.count, size: fmtSize(next.pruned.bytes) }));
    } catch (e) {
      showToast(t('st.upd.err.prune', { error: errText(e) }));
    } finally {
      pruning = false;
    }
  }
  async function prefetch(version: string): Promise<void> {
    prefetching = version;
    try {
      applyInfo(await api.updatesPrefetch(version));
    } catch (e) {
      showToast(t('st.upd.err.prefetch', { error: errText(e) }));
    } finally {
      prefetching = null;
    }
  }
  async function runAgents(ids: string[] | 'all', mode: RolloutMode, stopOnError: boolean): Promise<boolean> {
    try {
      applyInfo(await api.updatesAgents(ids, mode, stopOnError));
      kick();
      return true;
    } catch (e) {
      showToast(t('st.upd.err.request', { error: errText(e) }));
      return false;
    }
  }
</script>

<div class="set-sec wide">
  <div class="row"><h3 style="margin:0">{t('st.upd.title')}</h3><span class="grow"></span>{#if info}<span class="dim">{t('st.upd.channel', { channel: info.channel })}</span>{/if}</div>

  {#if !info}
    <p class="dim">{loadFailed ? t('st.upd.loadError') : t('st.upd.loading')}</p>
  {:else}
    {#if loadFailed}<div class="err" role="alert">{t('st.upd.loadError')}</div>{/if}
    <div class="two">
      <UpdServerPanel {info} busy={serverBusy} {restarting} {checking} channelBusy={channelBusy || serverBusy} oncheck={check} onchannel={changeChannel} onupdate={startServerUpdate} />
      <UpdCachePanel cache={info.cache} prunable={info.cache_prunable ?? 0} {pruning} {prefetching} onprefetch={prefetch} onprune={prune} />
    </div>
    <UpdAgentsPanel {info} onrun={runAgents} />
    <UpdHistory history={info.history} />
    <Fold title={t('st.upd.cli.title')} bind:open={openCli}>
      <pre class="json mono">linager update --check      {t('st.upd.cli.check')}
linager update              {t('st.upd.cli.update')}
linager agent update        {t('st.upd.cli.agentUpdate')}
linager agent status        {t('st.upd.cli.agentStatus')}</pre>
    </Fold>
  {/if}
</div>
