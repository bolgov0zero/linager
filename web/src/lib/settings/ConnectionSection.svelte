<script lang="ts">
  import { onMount } from 'svelte';
  import { api, ApiError, isUnauthorized, type ConnectionState } from '../../api';
  import { t, tr } from '../../i18n';
  import { showToast } from '../toast';

  let { onauthlost }: { onauthlost: () => void } = $props();

  let st = $state<ConnectionState | null>(null);
  let agentPort = $state('');
  let appPort = $state('');
  let loaded = $state(false);
  let busy = $state(false);
  let error = $state('');
  let confirming = $state(false);
  let confirmClose = $state(false);
  let timer: ReturnType<typeof setInterval> | undefined;

  const agentNum = $derived(Number.parseInt(agentPort, 10));
  const appNum = $derived(appPort.trim() === '' ? 0 : Number.parseInt(appPort, 10));
  const validPort = (n: number, zeroOk: boolean): boolean => (zeroOk && n === 0) || (Number.isInteger(n) && n >= 1 && n <= 65535);
  const valid = $derived(validPort(agentNum, false) && validPort(appNum, true) && /^\d*$/.test(agentPort.trim()) && /^\d*$/.test(appPort.trim()));
  const agentChanged = $derived(!!st && agentNum !== st.agent_port);
  const appChanged = $derived(!!st && appNum !== st.app_port);
  const dirty = $derived(agentChanged || appChanged);
  const blocked = $derived(!!st && (st.blockers.length > 0 || st.move !== null));

  function fail(e: unknown): void {
    if (isUnauthorized(e)) {
      onauthlost();
      return;
    }
    const code = e instanceof ApiError ? e.serverMessage : undefined;
    const key = code ? `conn.err.${code}` : '';
    error = key && t(key) !== key ? t(key) : t('conn.err.failed');
  }

  function fill(s: ConnectionState): void {
    st = s;
    agentPort = String(s.agent_port);
    appPort = s.app_port ? String(s.app_port) : '';
  }

  async function load(initial: boolean): Promise<void> {
    try {
      const s = await api.connection();
      if (initial || !dirty) fill(s);
      else st = s; // keep what is being typed; the progress and the blockers still refresh
    } catch (e) {
      fail(e);
    } finally {
      loaded = true;
    }
  }

  async function apply(): Promise<void> {
    if (!st || !valid || !dirty || busy) return;
    if (!confirming) {
      confirming = true;
      return;
    }
    busy = true;
    error = '';
    try {
      fill(await api.connectionSave(agentNum, appNum));
      showToast(t('conn.saved'));
    } catch (e) {
      fail(e);
      await load(true); // a refused change may have left something different from what was typed
    } finally {
      busy = false;
      confirming = false;
    }
  }

  async function closeOld(): Promise<void> {
    if (busy) return;
    if (!confirmClose) {
      confirmClose = true;
      return;
    }
    busy = true;
    error = '';
    try {
      fill(await api.connectionCloseOld());
      showToast(t('conn.move.closed'));
    } catch (e) {
      fail(e);
    } finally {
      busy = false;
      confirmClose = false;
    }
  }

  onMount(() => {
    void load(true);
    // while agents move, the progress is refreshed
    timer = setInterval(() => {
      if (st?.move) void load(false);
    }, 3000);
    return () => clearInterval(timer);
  });
</script>

<div class="conn">
  <h3 class="page">{$tr('conn.title')}</h3>
  <p class="muted">{$tr('conn.lead')}</p>

  {#if !loaded}
    <p class="muted">{$tr('common.loading')}</p>
  {:else if st}
    <div class="field">
      <span class="label">{$tr('conn.fp')}</span>
      <code class="mono fp">{st.fingerprint}</code>
      <span class="muted small">{$tr('conn.fp.hint')}</span>
    </div>

    {#if st.blockers.length > 0}
      <div class="warn" role="status">
        <p>{t('conn.blockers', undefined, { min: st.min_agent })}</p>
        <ul>
          {#each st.blockers as a (a.id)}
            <li><b>{a.name}</b> <span class="muted">{t('conn.version', undefined, { v: a.version || '?' })}</span></li>
          {/each}
        </ul>
      </div>
    {/if}

    <label class="field" for="conn-agent">
      <span class="label">{$tr('conn.agent')}</span>
      <input id="conn-agent" class="input port" inputmode="numeric" maxlength="5" disabled={blocked || busy} bind:value={agentPort} oninput={() => (confirming = false)} />
      <span class="muted small">{$tr('conn.agent.hint')}</span>
    </label>

    <label class="field" for="conn-app">
      <span class="label">{$tr('conn.app')} <span class="state" class:up={st.app_up}>{st.app_up ? $tr('conn.app.up') : $tr('conn.app.down')}</span></span>
      <input id="conn-app" class="input port" inputmode="numeric" maxlength="5" disabled={busy} bind:value={appPort} oninput={() => (confirming = false)} />
      <span class="muted small">{$tr('conn.app.hint')}</span>
    </label>

    {#if error}
      <p class="error" role="alert">{error}</p>
    {/if}

    {#if dirty}
      <p class="muted small">{$tr('conn.firewall')}</p>
      {#if confirming}
        <p class="confirm" role="status">
          {#if agentChanged}{t('conn.confirm.agent', undefined, { from: st.agent_port, to: agentNum })}{/if}
          {#if appChanged}{t('conn.confirm.app', undefined, { from: st.app_port || '—', to: appNum || '—' })}{/if}
        </p>
      {/if}
    {/if}
    <div class="row">
      <button class="btn primary" type="button" disabled={busy || !valid || !dirty} onclick={() => void apply()}>
        {confirming ? $tr('conn.confirm') : $tr('conn.apply')}
      </button>
      {#if confirming}
        <button class="btn" type="button" onclick={() => (confirming = false)}>{$tr('common.cancel')}</button>
      {/if}
    </div>

    {#if st.move}
      <div class="move">
        <h4>{$tr('conn.move.title')}</h4>
        <p>{t('conn.move.progress', undefined, { from: st.move.from, to: st.move.to, moved: st.move.moved, total: st.move.total })}</p>
        <progress max={st.move.total || 1} value={st.move.moved}></progress>
        {#if st.move.pending.length > 0}
          <p class="muted small">{$tr('conn.move.pending')}</p>
          <ul>
            {#each st.move.pending as a (a.id)}
              <li>
                <b>{a.name}</b>
                <span class="muted">{t('conn.version', undefined, { v: a.version || '?' })}</span>
                <span class="muted small">
                  {#if !a.supported}{$tr('conn.move.old')}{:else if !a.online}{$tr('conn.move.offline')}{:else}{$tr('conn.move.waiting')}{/if}
                </span>
              </li>
            {/each}
          </ul>
        {/if}
        <p class="muted small">{$tr('conn.move.links')}</p>
        {#if confirmClose}
          <p class="confirm" role="status">{t('conn.move.close.confirm', undefined, { from: st.move.from })}</p>
        {/if}
        <div class="row">
          <button class="btn" class:danger={confirmClose} type="button" disabled={busy} onclick={() => void closeOld()}>
            {confirmClose ? $tr('conn.confirm') : $tr('conn.move.close')}
          </button>
          {#if confirmClose}
            <button class="btn" type="button" onclick={() => (confirmClose = false)}>{$tr('common.cancel')}</button>
          {/if}
        </div>
      </div>
    {/if}
  {/if}
</div>

<style>
  .conn {
    display: flex;
    flex-direction: column;
    gap: 13px;
    min-width: 0;
  }

  h3.page {
    margin: 0;
    font-size: 18px;
    font-weight: 400;
  }

  h4,
  p {
    margin: 0;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 5px;
    min-width: 0;
  }

  .label {
    font-family: var(--font-display);
    font-size: 13px;
  }

  .fp {
    word-break: break-all;
    font-size: 12px;
  }

  .port {
    width: 128px;
  }

  .state {
    margin-left: 7px;
    padding: 0 5px;
    border: 2px solid var(--muted);
    color: var(--muted);
    font-size: 11px;
  }

  .state.up {
    border-color: var(--good);
    color: var(--good);
  }

  .warn,
  .move {
    display: flex;
    flex-direction: column;
    gap: 7px;
    padding: 11px 13px;
    border: 2px solid var(--line);
  }

  .warn {
    border-color: var(--warn);
  }

  ul {
    margin: 0;
    padding-left: 18px;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }

  progress {
    width: 100%;
    height: 14px;
    accent-color: var(--accent);
  }

  .confirm {
    padding: 7px 11px;
    border: 2px solid var(--warn);
    font-size: 13px;
  }

  .row {
    display: flex;
    flex-wrap: wrap;
    gap: 7px;
  }

  @media (max-width: 720px) {
    .port {
      width: 100%;
    }
  }
</style>
