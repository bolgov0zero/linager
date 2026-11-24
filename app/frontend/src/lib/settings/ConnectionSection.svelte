<script lang="ts">
  import { onMount } from 'svelte';
  import { api, ApiError, type ConnectionState } from '../api';
  import { isUnauthorized } from '../errors';
  import { t } from '../i18n.svelte';
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

  const agentNum = $derived(Number.parseInt(agentPort, 10));
  const appNum = $derived(appPort.trim() === '' ? 0 : Number.parseInt(appPort, 10));
  const validPort = (n: number, zeroOk: boolean): boolean => (zeroOk && n === 0) || (Number.isInteger(n) && n >= 1 && n <= 65535);
  const valid = $derived(validPort(agentNum, false) && validPort(appNum, true) && /^\d*$/.test(agentPort.trim()) && /^\d*$/.test(appPort.trim()));
  const agentChanged = $derived(!!st && agentNum !== st.agent_port);
  const appChanged = $derived(!!st && appNum !== st.app_port);
  const dirty = $derived(agentChanged || appChanged);
  const blocked = $derived(!!st && (st.blockers.length > 0 || st.move !== null));

  function fail(e: unknown): void {
    if (isUnauthorized(e)) return onauthlost();
    const code = e instanceof ApiError ? e.code : undefined;
    const key = code ? `st.conn.err.${code}` : '';
    error = key && t(key) !== key ? t(key) : t('st.conn.err.failed');
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
      showToast(t('st.conn.saved'));
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
      showToast(t('st.conn.move.closed'));
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
    const timer = setInterval(() => {
      if (st?.move) void load(false);
    }, 3000);
    return () => clearInterval(timer);
  });
</script>

<div class="set-sec">
  <h3>{t('st.conn.title')}</h3>
  <p class="dim">{t('st.conn.lead')}</p>

  {#if !loaded}
    <p class="dim">{t('dash.loading')}</p>
  {:else if st}
    <div class="fl"><span>{t('st.conn.fp')}</span><code class="mono fpbox">{st.fingerprint}</code><small class="dim">{t('st.conn.fp.hint')}</small></div>

    {#if st.blockers.length > 0}
      <div class="warnbox warn" role="status">
        <p>{t('st.conn.blockers', { min: st.min_agent })}</p>
        <ul class="plain">{#each st.blockers as a (a.id)}<li><b>{a.name}</b> <span class="dim">{t('st.conn.version', { v: a.version || '?' })}</span></li>{/each}</ul>
      </div>
    {/if}

    <label class="fl"><span>{t('st.conn.agent')}</span><span class="field port"><input inputmode="numeric" maxlength="5" disabled={blocked || busy} bind:value={agentPort} oninput={() => (confirming = false)} /></span><small class="dim">{t('st.conn.agent.hint')}</small></label>
    <label class="fl"><span>{t('st.conn.app')} <span class="badge {st.app_up ? 'b-ok' : 'b-mute'}">{st.app_up ? t('st.conn.app.up') : t('st.conn.app.down')}</span></span><span class="field port"><input inputmode="numeric" maxlength="5" disabled={busy} bind:value={appPort} oninput={() => (confirming = false)} /></span><small class="dim">{t('st.conn.app.hint')}</small></label>

    {#if error}<div class="err" role="alert">{error}</div>{/if}
    {#if dirty}
      <p class="dim">{t('st.conn.firewall')}</p>
      {#if confirming}
        <p class="warnbox warn" role="status">
          {#if agentChanged}{t('st.conn.confirm.agent', { from: st.agent_port, to: agentNum })}{/if}
          {#if appChanged}{t('st.conn.confirm.app', { from: st.app_port || '—', to: appNum || '—' })}{/if}
        </p>
      {/if}
    {/if}
    <div class="row">
      <button class="btn primary" type="button" disabled={busy || !valid || !dirty} onclick={() => void apply()}>{confirming ? t('st.conn.confirm') : t('st.conn.apply')}</button>
      {#if confirming}<button class="btn" type="button" onclick={() => (confirming = false)}>{t('common.cancel')}</button>{/if}
    </div>

    {#if st.move}
      <section class="panel mdl-stack">
        <h4 class="ph" style="margin:0">{t('st.conn.move.title')}</h4>
        <p>{t('st.conn.move.progress', { from: st.move.from, to: st.move.to, moved: st.move.moved, total: st.move.total })}</p>
        <span class="bar lv-ok"><i style="width:{Math.min(100, (st.move.moved / (st.move.total || 1)) * 100)}%"></i></span>
        {#if st.move.pending.length > 0}
          <p class="dim">{t('st.conn.move.pending')}</p>
          <ul class="plain">
            {#each st.move.pending as a (a.id)}
              <li><b>{a.name}</b> <span class="dim">{t('st.conn.version', { v: a.version || '?' })}</span> <span class="dim">{#if !a.supported}{t('st.conn.move.old')}{:else if !a.online}{t('st.conn.move.offline')}{:else}{t('st.conn.move.waiting')}{/if}</span></li>
            {/each}
          </ul>
        {/if}
        <p class="dim">{t('st.conn.move.links')}</p>
        {#if confirmClose}<p class="warnbox warn" role="status">{t('st.conn.move.close.confirm', { from: st.move.from })}</p>{/if}
        <div class="row">
          <button class="btn" class:danger={confirmClose} type="button" disabled={busy} onclick={() => void closeOld()}>{confirmClose ? t('st.conn.confirm') : t('st.conn.move.close')}</button>
          {#if confirmClose}<button class="btn" type="button" onclick={() => (confirmClose = false)}>{t('common.cancel')}</button>{/if}
        </div>
      </section>
    {/if}
  {/if}
</div>
