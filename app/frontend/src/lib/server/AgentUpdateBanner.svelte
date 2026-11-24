<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type AgentUpdateRow, type ServerDetail } from '../api';
  import { t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';
  import UpdAgentSteps from '../settings/UpdAgentSteps.svelte';
  import { errText } from '../settings/upd-format';
  import { showToast } from '../toast';

  // The banner of the server page: the agent of this server is older than the version of the Linager server (update it
  // from here), an update of the agent is going on, or it ended badly. Administrators only: the answer of the server
  // decides, a person without the right sees nothing.
  let { server, onchanged }: { server: ServerDetail; onchanged?: () => void } = $props();

  const FAST_MS = 2000;
  const SLOW_MS = 15000;
  const TOTAL_STEPS = 8;

  let row = $state<AgentUpdateRow | null>(null);
  let admin = $state(false);
  let confirmOpen = $state(false);
  let submitting = $state(false);

  const busy = $derived(row?.status === 'updating' || row?.status === 'queued');
  const canUpdate = $derived(server.online && (row?.status === 'available' || row?.status === 'unsupported'));
  const target = $derived(row?.target ?? '');

  let timer: ReturnType<typeof setTimeout> | undefined;
  let destroyed = false;
  let inflight = false;
  let lastStatus: AgentUpdateRow['status'] | null = null;

  /** Stores the row of this server and reports how an update that was in progress ended. */
  function apply(rows: AgentUpdateRow[]): void {
    const next = rows.find((r) => r.id === server.id) ?? null;
    const wasBusy = lastStatus === 'updating' || lastStatus === 'queued';
    if (next && wasBusy) {
      if (next.status === 'current') {
        showToast(t('srv.agent.done', { name: server.name, version: next.version }));
        onchanged?.();
      } else if (next.status === 'rollback' || next.status === 'failed') {
        showToast(t('srv.agent.rolledBack', { name: server.name }));
        onchanged?.();
      }
    }
    lastStatus = next?.status ?? null;
    row = next;
  }
  async function refresh(): Promise<void> {
    if (inflight || destroyed || !admin) return;
    inflight = true;
    try {
      apply((await api.updates()).agents);
    } catch {
      // optional block: keep the previous state and try again on the next tick
    } finally {
      inflight = false;
    }
  }
  function scheduleNext(): void {
    clearTimeout(timer);
    if (destroyed) return;
    timer = setTimeout(() => void tick(), busy ? FAST_MS : SLOW_MS);
  }
  async function tick(): Promise<void> {
    if (!document.hidden) await refresh();
    scheduleNext();
  }
  onMount(() => {
    void api.me().then((me) => {
      admin = (me.role ?? 'admin') === 'admin';
      if (admin) void tick();
    }).catch(() => undefined);
    return () => {
      destroyed = true;
      clearTimeout(timer);
    };
  });

  async function start(): Promise<void> {
    submitting = true;
    try {
      apply((await api.updatesAgents([server.id], 'all', true)).agents);
      confirmOpen = false;
      clearTimeout(timer);
      void tick();
    } catch (e) {
      showToast(t('st.upd.err.request', { error: errText(e) }));
    } finally {
      submitting = false;
    }
  }
</script>

{#if admin && row && (canUpdate || busy || row.status === 'rollback' || row.status === 'failed')}
  <div class="banner agent-banner" role="status">
    {#if row.status === 'available' || row.status === 'unsupported'}
      <span class="msg">{row.status === 'unsupported' ? t('srv.agent.unsupported', { version: row.version }) : t('srv.agent.available', { version: target || row.version })}</span>
      <button class="btn sm" type="button" disabled={!canUpdate || submitting} onclick={() => (confirmOpen = true)}>{t('srv.agent.update')}</button>
    {:else if row.status === 'updating'}
      <span class="msg">{t('st.upd.ag.updating')}: {t('st.upd.ag.step', { n: row.step ?? 0, total: row.steps_total || TOTAL_STEPS, name: row.step_id ? t(`st.upd.agstep.${row.step_id}`) : '' })}{#if row.message}{' · ' + row.message}{/if}</span>
      {#if row.steps?.length}<div class="ab-steps"><UpdAgentSteps steps={row.steps} nowMs={row.now_ms} rollbackAt={row.rollback_at} /></div>{/if}
    {:else if row.status === 'queued'}
      <span class="msg">{t('st.upd.ag.queued')}{#if row.message}: {row.message}{:else if !row.online}: {t('st.upd.ag.waiting')}{/if}</span>
    {:else}
      <span class="msg">{row.status === 'rollback' ? t('st.upd.ag.rollback') : t('st.upd.ag.failed')}{#if row.message}: {row.message}{/if}</span>
      {#if row.steps?.length}<div class="ab-steps"><UpdAgentSteps steps={row.steps} nowMs={row.now_ms} /></div>{/if}
      {#if canUpdate || server.online}<button class="btn sm" type="button" disabled={submitting} onclick={() => (confirmOpen = true)}>{t('srv.agent.update')}</button>{/if}
    {/if}
  </div>
{/if}

{#if confirmOpen}
  <Modal title={t('srv.agent.confirmTitle', { name: server.name })} onclose={() => !submitting && (confirmOpen = false)}>
    <p>{t('srv.agent.confirmText')}</p>
    {#snippet foot()}
      <span class="spacer"></span>
      <button class="btn" type="button" disabled={submitting} onclick={() => (confirmOpen = false)}>{t('common.cancel')}</button>
      <button class="btn primary" type="button" disabled={submitting} onclick={() => void start()}>{t('srv.agent.confirmOk')}</button>
    {/snippet}
  </Modal>
{/if}
