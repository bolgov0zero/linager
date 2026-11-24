<script lang="ts">
  import { onDestroy } from 'svelte';
  import { api, ApiError, type Group, type SSHState } from '../api';
  import CustomSelect from '../CustomSelect.svelte';
  import { folderOptions } from '../folders';
  import { isUnauthorized } from '../errors';
  import { t } from '../i18n.svelte';
  import Fold from '../settings/Fold.svelte';
  import { showToast } from '../toast';
  import StageList from './StageList.svelte';
  import { failureCode, failureText, gateList, gateStr, isBlockedHost, pluralKey, sshStages } from './ssh';

  let { groups, onopen, onadded, onauthlost }: {
    groups: Group[];
    /** Open the page of the new server (the dialog closes). */
    onopen: (name: string) => void;
    /** A server was installed: the dashboard may have changed. */
    onadded: () => void;
    onauthlost: () => void;
  } = $props();

  const FAST_MS = 700;
  const SLOW_MS = 2000;
  const OTHER = '';

  // The form. The secrets (password, key, passphrase, sudo password) exist only in this component: they are not put in a URL,
  // a log or the storage of the window, and they are wiped when the install is over or the dialog closes.
  let host = $state('');
  let port = $state('22');
  let login = $state('');
  let auth = $state<'password' | 'key'>('password');
  let password = $state('');
  let key = $state('');
  let passphrase = $state('');
  let groupId = $state(0);

  let view = $state<'form' | 'session'>('form');
  let starting = $state(false);
  let startError = $state('');
  let st = $state<SSHState | null>(null);
  let expired = $state(false);
  let gateBusy = $state(false);

  // The answers to the gates.
  let sudoPw = $state('');
  let addrPick = $state(OTHER);
  let addrOther = $state('');
  let reinstallOk = $state(false);

  let logOpen = $state(false);
  let logEl = $state<HTMLElement | undefined>(undefined);

  let sessionId: string | null = null;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let alive = true;

  const blocked = $derived(isBlockedHost(host));
  const portNum = $derived(Number(port));
  const portOk = $derived(Number.isInteger(portNum) && portNum >= 1 && portNum <= 65535);
  const canCheck = $derived(!blocked && !starting && host.trim() !== '' && login.trim() !== '' && portOk && (auth === 'password' ? password !== '' : key.trim() !== ''));
  const groupOptions = $derived(folderOptions(groups, t('asv.group.none')));

  const phase = $derived(st?.phase);
  const installing = $derived(phase === 'installing' || phase === 'done' || phase === 'install_failed');
  const stages = $derived(st ? sshStages(st, installing) : []);
  const gate = $derived(st?.gate);
  const failure = $derived(st ? failureCode(st) : { code: '', detail: '' });
  const newName = $derived(st?.new_name ?? '');
  const logLines = $derived(st?.log ?? []);
  const logTotal = $derived(Math.max(st?.log_total ?? 0, logLines.length));

  const candidates = $derived(gateList(gate, 'candidates'));
  const addrOptions = $derived([...candidates.map((c) => ({ v: c, t: c })), { v: OTHER, t: t('asv.ssh.gate.addrOther') }]);
  const addrValue = $derived(addrPick === OTHER ? addrOther.trim() : addrPick);

  function wipeSecrets(): void {
    password = '';
    key = '';
    passphrase = '';
    sudoPw = '';
  }
  function stopPolling(): void {
    clearTimeout(timer);
    timer = undefined;
  }
  function schedule(ms: number): void {
    stopPolling();
    if (!alive || !sessionId) return;
    timer = setTimeout(() => void poll(), ms);
  }
  /** Forget the session: stop polling and tell the server to drop it (and the secrets it holds). */
  function closeSession(): void {
    stopPolling();
    const id = sessionId;
    sessionId = null;
    st = null;
    if (id) void api.sshClose(id).catch(() => {});
  }
  function gotExpired(): void {
    stopPolling();
    sessionId = null;
    st = null;
    expired = true;
    view = 'form';
  }
  function apply(next: SSHState): void {
    st = next;
    if (next.phase === 'done' || next.phase === 'closed') wipeSecrets();
    if (next.phase === 'done') {
      showToast(t('asv.ssh.doneToast', { name: next.new_name ?? next.host }));
      onadded();
    }
    if (next.phase === 'install_failed') logOpen = true;
    if (next.phase === 'done' || next.phase === 'install_failed' || next.phase === 'closed') {
      stopPolling();
      return;
    }
    schedule(next.phase === 'checking' || next.phase === 'installing' ? FAST_MS : SLOW_MS);
  }
  function onError(e: unknown): void {
    if (isUnauthorized(e)) onauthlost();
    else if (e instanceof ApiError && e.status === 404) gotExpired();
    else if (e instanceof ApiError && e.status === 409) schedule(0);
    else {
      showToast(t('common.genericError'));
      schedule(SLOW_MS);
    }
  }
  async function poll(): Promise<void> {
    const id = sessionId;
    if (!id) return;
    try {
      const next = await api.sshState(id);
      if (id === sessionId) apply(next);
    } catch (e) {
      if (id === sessionId) onError(e);
    }
  }
  async function start(): Promise<void> {
    if (!canCheck) return;
    starting = true;
    startError = '';
    expired = false;
    try {
      const res = await api.sshStart({ host: host.trim(), port: portNum, login: login.trim(), auth, password: auth === 'password' ? password : '', key: auth === 'key' ? key : '', passphrase: auth === 'key' ? passphrase : '', group_id: groupId === 0 ? null : groupId });
      sessionId = res.id;
      st = null;
      sudoPw = '';
      logOpen = false;
      view = 'session';
      schedule(0);
    } catch (e) {
      if (isUnauthorized(e)) onauthlost();
      else if (e instanceof ApiError && e.status === 429) startError = t('asv.ssh.tooMany');
      else if (e instanceof ApiError && e.status === 422) startError = e.code ?? t('asv.ssh.startFailed');
      else startError = t('asv.ssh.startFailed');
    } finally {
      starting = false;
    }
  }
  async function answer(accept: boolean, extra: { password?: string; address?: string } = {}): Promise<void> {
    const id = sessionId;
    const g = st?.gate;
    if (!id || !g || gateBusy) return;
    gateBusy = true;
    try {
      const next = await api.sshAnswer(id, { gate: g.kind, accept, ...extra });
      sudoPw = '';
      if (id === sessionId) apply(next);
    } catch (e) {
      if (id === sessionId) onError(e);
    } finally {
      gateBusy = false;
    }
  }
  async function install(): Promise<void> {
    const id = sessionId;
    if (!id || gateBusy) return;
    gateBusy = true;
    try {
      const next = await api.sshInstall(id);
      if (id === sessionId) apply(next);
    } catch (e) {
      if (id === sessionId) onError(e);
    } finally {
      gateBusy = false;
    }
  }
  /** Back to the form with what was typed (the session is dropped on the server). */
  function edit(): void {
    closeSession();
    view = 'form';
  }
  async function recheck(): Promise<void> {
    closeSession();
    view = 'form';
    await start();
  }
  /** "Add another": a clean form; nothing secret stays. */
  function again(): void {
    closeSession();
    wipeSecrets();
    host = '';
    view = 'form';
  }

  // A new question starts with empty answers; the sudo password of the SSH login is the first thing to try.
  let seenGate = '';
  $effect(() => {
    const kind = gate?.kind ?? '';
    const retry = gate?.data?.retry === true;
    const sig = `${kind}:${gateList(gate, 'candidates').join(',')}:${retry}`;
    if (sig === seenGate) return;
    seenGate = sig;
    sudoPw = kind === 'sudo' && !retry && auth === 'password' ? password : '';
    addrPick = candidates[0] ?? OTHER;
    addrOther = '';
    reinstallOk = false;
  });
  // The output stays scrolled to its end.
  $effect(() => {
    void logLines.length;
    void logOpen;
    if (logOpen && logEl) logEl.scrollTop = logEl.scrollHeight;
  });
  onDestroy(() => {
    alive = false;
    stopPolling();
    const id = sessionId;
    sessionId = null;
    if (id) void api.sshClose(id).catch(() => {});
    wipeSecrets();
  });
</script>

<div class="asv-tab">
  {#if view === 'form'}
    <h3>{t('asv.tab.ssh')}</h3>
    <p class="dim">{t('asv.ssh.lead')}</p>
    {#if expired}<p class="warnbox warn">{t('asv.ssh.expired')}</p>{/if}
    <form class="asv-sec" autocomplete="off" onsubmit={(e) => { e.preventDefault(); void start(); }}>
      <h4 class="ph">{t('asv.ssh.where')}</h4>
      <div class="panel mdl-stack">
        <div class="asv-grid">
          <label class="fl"><span>{t('asv.ssh.host')}</span><span class="field"><input class="mono" bind:value={host} spellcheck="false" autocomplete="off" autocapitalize="off" /></span></label>
          <label class="fl"><span>{t('asv.ssh.port')}</span><span class="field"><input class="mono" bind:value={port} inputmode="numeric" autocomplete="off" /></span></label>
          <label class="fl"><span>{t('asv.ssh.login')}</span><span class="field"><input class="mono" bind:value={login} spellcheck="false" autocomplete="off" autocapitalize="off" /></span></label>
        </div>
        {#if blocked}<p class="critt">{t('asv.ssh.blocked')}</p>{/if}
        <div class="row">
          <span class="dim">{t('asv.ssh.auth')}</span>
          <button class="chip" type="button" aria-pressed={auth === 'password'} onclick={() => (auth = 'password')}>{t('asv.ssh.authPassword')}</button>
          <button class="chip" type="button" aria-pressed={auth === 'key'} onclick={() => (auth = 'key')}>{t('asv.ssh.authKey')}</button>
        </div>
        {#if auth === 'password'}
          <label class="fl"><span>{t('asv.ssh.password')}</span><span class="field"><input class="mono" type="password" bind:value={password} autocomplete="new-password" /></span></label>
        {:else}
          <label class="fl"><span>{t('asv.ssh.key')}</span><textarea class="mono area" rows="4" bind:value={key} placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" spellcheck="false" autocomplete="off"></textarea></label>
          <label class="fl"><span>{t('asv.ssh.passphrase')}</span><span class="field"><input class="mono" type="password" bind:value={passphrase} autocomplete="new-password" /></span></label>
        {/if}
        <div class="fl"><span>{t('asv.ssh.group')}</span><CustomSelect value={String(groupId)} options={groupOptions} onchange={(v) => (groupId = Number(v))} /></div>
        <p class="warnbox warn">{t('asv.ssh.secrets')}</p>
        {#if startError}<div class="err" role="alert">{startError}</div>{/if}
        <div class="row end"><button class="btn primary" type="submit" disabled={!canCheck}>{t('asv.ssh.check')}</button></div>
      </div>
    </form>
  {:else}
    <h3>{#if installing}{t('asv.ssh.installTitle', { host: st?.host ?? host })}{:else}{t('asv.ssh.checkTitle', { target: `${st?.login ?? login}@${st?.host ?? host}` })}{/if}</h3>
    <div class="panel mdl-stack">
      {#if !st}
        <p class="dim">{t('asv.ssh.starting')}</p>
      {:else}
        <StageList {stages} nowMs={st.now_ms} label={t(installing ? 'asv.ssh.stagesInstall' : 'asv.ssh.stagesCheck')} />

        {#if gate?.kind === 'hostkey'}
          <div class="warnbox warn mdl-stack">
            <b>{t('asv.ssh.gate.hostkeyTitle')}</b>
            <p>{t('asv.ssh.gate.hostkeyText', { type: gateStr(gate, 'type') })}</p>
            <p class="mono">{gateStr(gate, 'fingerprint')}</p>
            <p>{t('asv.ssh.gate.hostkeyWarn')}</p>
            <div class="row"><button class="btn sm primary" type="button" disabled={gateBusy} onclick={() => void answer(true)}>{t('asv.ssh.gate.trust')}</button><button class="btn sm" type="button" disabled={gateBusy} onclick={() => void answer(false)}>{t('asv.ssh.gate.cancel')}</button></div>
          </div>
        {:else if gate?.kind === 'sudo'}
          <form class="warnbox warn mdl-stack" onsubmit={(e) => { e.preventDefault(); if (sudoPw) void answer(true, { password: sudoPw }); }}>
            <p><b>{t('asv.ssh.gate.sudoTitle')}</b> {t('asv.ssh.gate.sudoText', { login: st.login })}</p>
            {#if gate.data?.retry === true}<p class="critt">{t('asv.ssh.gate.sudoRetry')}</p>{/if}
            <label class="fl"><span>{t('asv.ssh.gate.sudoField')}</span><span class="field"><input class="mono" type="password" bind:value={sudoPw} autocomplete="new-password" /></span></label>
            <div class="row"><button class="btn sm primary" type="submit" disabled={gateBusy || !sudoPw}>{t('asv.ssh.gate.continue')}</button><button class="btn sm" type="button" disabled={gateBusy} onclick={() => void answer(false)}>{t('asv.ssh.gate.cancel')}</button></div>
          </form>
        {:else if gate?.kind === 'address'}
          <div class="warnbox warn mdl-stack">
            <b>{t('asv.ssh.gate.addrTitle')}</b>
            <p>{t('asv.ssh.gate.addrText', { port: gateStr(gate, 'port') })}</p>
            <CustomSelect value={addrPick} options={addrOptions} onchange={(v) => (addrPick = v)} />
            {#if addrPick === OTHER}<span class="field"><input class="mono" bind:value={addrOther} placeholder={t('asv.ssh.gate.addrPlaceholder')} spellcheck="false" autocomplete="off" aria-label={t('asv.ssh.gate.addrOther')} /></span>{/if}
            <div class="row"><button class="btn sm primary" type="button" disabled={gateBusy || !addrValue} onclick={() => void answer(true, { address: addrValue })}>{t('asv.ssh.gate.retry')}</button><button class="btn sm" type="button" disabled={gateBusy} onclick={() => void answer(false)}>{t('asv.ssh.gate.cancel')}</button></div>
          </div>
        {:else if gate?.kind === 'reinstall'}
          <div class="warnbox warn mdl-stack">
            <b>{t('asv.ssh.gate.reinstallTitle')}</b>
            <p>{t('asv.ssh.gate.reinstallText')}</p>
            <label class="ck"><input type="checkbox" bind:checked={reinstallOk} />{t('asv.ssh.gate.reinstallCheck')}</label>
            <div class="row"><button class="btn sm primary" type="button" disabled={gateBusy || !reinstallOk} onclick={() => void answer(true)}>{t('asv.ssh.gate.continue')}</button><button class="btn sm" type="button" disabled={gateBusy} onclick={() => void answer(false)}>{t('asv.ssh.gate.cancel')}</button></div>
          </div>
        {/if}

        {#if phase === 'ready'}
          <div class="okbox mdl-stack">
            <b>{t('asv.ssh.ready')}</b>
            <dl class="facts one">
              <div><dt>{t('asv.ssh.readyHost')}</dt><dd>{st.login}@{st.host}:{st.port}</dd></div>
              {#if st.server}<div><dt>{t('asv.ssh.readyAgent')}</dt><dd>{st.server}</dd></div>{/if}
              {#if st.mode}<div><dt>{t('asv.ssh.readyRights')}</dt><dd>{t(`asv.ssh.mode.${st.mode}`)}</dd></div>{/if}
              <div><dt>{t('asv.ssh.readyApprove')}</dt><dd>{t('asv.ssh.readyApproveText')}</dd></div>
              <div><dt>{t('asv.ssh.readyName')}</dt><dd>{t('asv.ssh.readyNameText')}</dd></div>
            </dl>
            <div class="row"><button class="btn primary" type="button" disabled={gateBusy} onclick={() => void install()}>{t('asv.ssh.install')}</button><button class="btn" type="button" onclick={edit}>{t('asv.ssh.edit')}</button></div>
          </div>
        {:else if phase === 'check_failed'}
          <div class="err mdl-stack">
            <b>{t('asv.ssh.checkFailed')}</b>
            <p>{failureText(failure.code, auth, { login: st.login, detail: failure.detail })}</p>
            <div class="row"><button class="btn sm" type="button" onclick={edit}>{t('asv.ssh.edit')}</button><button class="btn sm" type="button" onclick={() => void recheck()}>{t('asv.ssh.recheck')}</button></div>
          </div>
        {:else if phase === 'install_failed'}
          <div class="err mdl-stack">
            <b>{t('asv.ssh.installFailed')}</b>
            <p>{failureText(failure.code, auth, { login: st.login, detail: failure.detail, server: st.server ?? '' })}</p>
            <div class="row"><button class="btn sm" type="button" onclick={() => void recheck()}>{t('asv.ssh.retryAll')}</button><button class="btn sm" type="button" onclick={edit}>{t('asv.ssh.edit')}</button></div>
          </div>
        {:else if phase === 'done'}
          <div class="okbox mdl-stack">
            <p><b>{t('asv.ssh.done', { name: st.new_name ?? st.host })}</b> {t('asv.ssh.doneText')}</p>
            <div class="row">{#if newName}<button class="btn sm primary" type="button" onclick={() => onopen(newName)}>{t('asv.ssh.open')}</button>{/if}<button class="btn sm" type="button" onclick={again}>{t('asv.ssh.again')}</button></div>
          </div>
        {:else if phase === 'closed'}
          <p class="warnbox warn">{t('asv.ssh.expired')}</p>
          <div class="row"><button class="btn sm" type="button" onclick={edit}>{t('asv.ssh.edit')}</button></div>
        {/if}

        {#if installing}
          <Fold title={t('asv.ssh.log')} meta={t(`asv.ssh.logLines.${pluralKey(logTotal, 'ru')}`, { n: logTotal })} bind:open={logOpen}>
            {#if logTotal > logLines.length}<p class="dim">{t('asv.ssh.logTrimmed', { total: logTotal })}</p>{/if}
            <pre class="json mono sshlog" bind:this={logEl}>{#each logLines as line, i (i)}<span class="ln" class:cmdl={line.startsWith('$ ')}>{line}{'\n'}</span>{/each}</pre>
          </Fold>
        {/if}
      {/if}
    </div>
  {/if}
</div>
