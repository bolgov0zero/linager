<script lang="ts">
  import { onMount } from 'svelte';
  import { api, ApiError, type PasskeyRow, type Session, type TotpSetup } from '../api';
  import { confirmAsk } from '../dialogs.svelte';
  import { isUnauthorized } from '../errors';
  import { i18n, t } from '../i18n.svelte';
  import { showToast } from '../toast';

  let { session, onrefresh, onauthlost }: { session: Session; onrefresh: () => Promise<void>; onauthlost: () => void } = $props();

  const MIN_PASSWORD = 12;

  // ---- password ----
  let curPw = $state('');
  let newPw = $state('');
  let newPw2 = $state('');
  let pwBusy = $state(false);
  let pwError = $state('');

  // ---- 2FA ----
  let setup = $state<TotpSetup | null>(null);
  let code = $state('');
  let disablePw = $state('');
  let tfBusy = $state(false);
  let tfError = $state('');

  // ---- recovery codes (shown once) ----
  let codes = $state<string[] | null>(null);
  let regenOpen = $state(false);
  let regenPw = $state('');
  let regenBusy = $state(false);
  let regenError = $state('');

  // ---- passkeys: they cannot be made in the application, an old one can be removed ----
  let passkeys = $state<PasskeyRow[]>([]);
  let removing = $state<PasskeyRow | null>(null);
  let removePw = $state('');
  let pkBusy = $state(false);
  let pkError = $state('');

  const recoveryLeft = $derived(session.recovery_left ?? 0);

  function failText(e: unknown, key: string): string {
    if (e instanceof ApiError) {
      if (e.status === 0) return t('st.common.networkError');
      if (e.status === 429) return e.code ?? t('common.genericError');
      if (e.status >= 400 && e.status < 500) return e.status === 403 || e.status === 422 ? t(key) : (e.code ?? t(key));
    }
    return t('common.genericError');
  }
  /** A lost session is reported once; true when the error was that. */
  function handle(e: unknown): boolean {
    if (!isUnauthorized(e)) return false;
    onauthlost();
    return true;
  }
  const when = (iso: string): string => new Date(iso).toLocaleString(i18n.locale);

  async function loadPasskeys(): Promise<void> {
    try {
      passkeys = (await api.passkeys()).passkeys;
    } catch (e) {
      handle(e);
    }
  }
  onMount(() => void loadPasskeys());

  async function changePassword(): Promise<void> {
    if (pwBusy) return;
    pwError = '';
    if (newPw.length < MIN_PASSWORD) {
      pwError = t('st.profile.password.tooShort');
      return;
    }
    if (newPw !== newPw2) {
      pwError = t('st.profile.password.mismatch');
      return;
    }
    pwBusy = true;
    try {
      await api.changePassword(curPw, newPw);
      curPw = newPw = newPw2 = '';
      showToast(t('st.profile.password.done'));
    } catch (e) {
      if (!handle(e)) pwError = failText(e, 'st.profile.password.wrong');
    } finally {
      pwBusy = false;
    }
  }

  async function startSetup(): Promise<void> {
    tfBusy = true;
    tfError = '';
    try {
      setup = await api.totpSetup();
      code = '';
    } catch (e) {
      if (!handle(e)) tfError = t('common.genericError');
    } finally {
      tfBusy = false;
    }
  }
  async function enable(): Promise<void> {
    const value = code.trim();
    if (!value || tfBusy) return;
    tfBusy = true;
    tfError = '';
    try {
      const res = await api.totpEnable(value);
      setup = null;
      code = '';
      codes = res.recovery_codes;
      showToast(t('st.security.enabled'));
      await onrefresh();
    } catch (e) {
      if (!handle(e)) tfError = failText(e, 'st.security.invalidCode');
    } finally {
      tfBusy = false;
    }
  }
  async function disable(): Promise<void> {
    if (!disablePw || tfBusy) return;
    if (!(await confirmAsk({ title: t('st.security.disable'), text: t('st.security.totpOn'), ok: t('st.security.disable'), danger: true }))) return;
    tfBusy = true;
    tfError = '';
    try {
      await api.totpDisable(disablePw);
      disablePw = '';
      codes = null;
      showToast(t('st.security.disabled'));
      await onrefresh();
    } catch (e) {
      if (!handle(e)) tfError = failText(e, 'st.security.invalidPassword');
    } finally {
      tfBusy = false;
    }
  }
  async function copy(text: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(text);
      showToast(t('st.common.copied'));
    } catch {
      showToast(t('st.common.copyFailed'));
    }
  }
  const groupSecret = (secret: string): string => secret.replace(/\s+/g, '').match(/.{1,4}/g)?.join(' ') ?? secret;

  async function regenerate(): Promise<void> {
    if (!regenPw || regenBusy) return;
    regenBusy = true;
    regenError = '';
    try {
      codes = (await api.recoveryRegenerate(regenPw)).recovery_codes;
      regenPw = '';
      regenOpen = false;
      await onrefresh();
    } catch (e) {
      if (!handle(e)) regenError = failText(e, 'st.security.invalidPassword');
    } finally {
      regenBusy = false;
    }
  }
  function downloadCodes(): void {
    if (!codes) return;
    const text = `Linager recovery codes for ${session.login}\n\n${codes.join('\n')}\n`;
    const url = URL.createObjectURL(new Blob([text], { type: 'text/plain' }));
    const a = document.createElement('a');
    a.href = url;
    a.download = 'linager-recovery-codes.txt';
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 10_000);
  }

  async function removePasskey(): Promise<void> {
    const target = removing;
    if (!target || !removePw || pkBusy) return;
    pkBusy = true;
    pkError = '';
    try {
      await api.passkeyDelete(target.id, removePw);
      removing = null;
      removePw = '';
      showToast(t('st.passkeys.removed'));
      await loadPasskeys();
      await onrefresh();
    } catch (e) {
      if (!handle(e)) pkError = failText(e, 'st.security.invalidPassword');
    } finally {
      pkBusy = false;
    }
  }
</script>

<div class="set-sec">
  <h3>{t('st.profile.you', { login: session.login })}</h3>

  <section class="panel">
    <h4 class="ph">{t('st.profile.password.title')}</h4>
    <form class="mdl-stack" onsubmit={(e) => { e.preventDefault(); void changePassword(); }}>
      <!-- the login is here so a password manager updates the right entry -->
      <input type="text" name="username" autocomplete="username" value={session.login} hidden readonly />
      <label class="fl"><span>{t('st.profile.password.current')}</span><span class="field"><input type="password" autocomplete="current-password" required bind:value={curPw} /></span></label>
      <label class="fl"><span>{t('st.profile.password.new')}</span><span class="field"><input type="password" autocomplete="new-password" required bind:value={newPw} /></span></label>
      <label class="fl"><span>{t('st.profile.password.repeat')}</span><span class="field"><input type="password" autocomplete="new-password" required bind:value={newPw2} /></span></label>
      {#if pwError}<div class="err" role="alert">{pwError}</div>{/if}
      <div class="row end"><button class="btn primary" type="submit" disabled={pwBusy}>{t('st.profile.password.submit')}</button></div>
    </form>
  </section>

  <section class="panel">
    <h4 class="ph">{t('st.profile.2fa.title')}</h4>
    {#if session.totp_enabled}
      <p class="dim">{t('st.security.totpOn')}</p>
      <form class="mdl-stack" onsubmit={(e) => { e.preventDefault(); void disable(); }}>
        <label class="fl"><span>{t('st.security.password')}</span><span class="field"><input type="password" autocomplete="current-password" required bind:value={disablePw} /></span></label>
        {#if tfError}<div class="err" role="alert">{tfError}</div>{/if}
        <div class="row end"><button class="btn danger" type="submit" disabled={tfBusy || !disablePw}>{t('st.security.disable')}</button></div>
      </form>
    {:else if setup}
      <div class="mdl-stack">
        <p class="dim">{t('st.security.setupHint')}</p>
        <div class="fl"><span>{t('st.security.secret')}</span><b class="mono secret">{groupSecret(setup.secret)}</b></div>
        <div class="fl"><span>{t('st.security.uri')}</span><span class="field"><input class="mono" readonly value={setup.uri} /></span></div>
        <div class="row"><button class="btn" type="button" onclick={() => void copy(setup?.uri ?? '')}>{t('st.common.copy')}</button></div>
        <form class="mdl-stack" onsubmit={(e) => { e.preventDefault(); void enable(); }}>
          <label class="fl"><span>{t('st.security.code')}</span><span class="field"><input class="mono" inputmode="numeric" autocomplete="one-time-code" maxlength="6" bind:value={code} /></span></label>
          {#if tfError}<div class="err" role="alert">{tfError}</div>{/if}
          <div class="row end"><button class="btn primary" type="submit" disabled={tfBusy || code.trim().length !== 6}>{t('st.security.confirmEnable')}</button></div>
        </form>
      </div>
    {:else}
      <p class="dim">{t('st.security.totpOff')}</p>
      {#if tfError}<div class="err" role="alert" style="margin-top:8px">{tfError}</div>{/if}
      <div class="row end" style="margin-top:10px"><button class="btn primary" type="button" disabled={tfBusy} onclick={() => void startSetup()}>{t('st.security.enable')}</button></div>
    {/if}
  </section>

  {#if session.totp_enabled || codes}
    <section class="panel">
      <h4 class="ph">{t('st.recovery.title')}</h4>
      {#if codes}
        <p class="warnbox warn">{t('st.recovery.shown')}</p>
        <ul class="codes mono">{#each codes as c (c)}<li>{c}</li>{/each}</ul>
        <div class="row">
          <button class="btn" type="button" onclick={() => void copy(codes?.join('\n') ?? '')}>{t('st.recovery.copy')}</button>
          <button class="btn" type="button" onclick={downloadCodes}>{t('st.recovery.download')}</button>
          <button class="btn primary" type="button" onclick={() => (codes = null)}>{t('st.recovery.saved')}</button>
        </div>
      {:else}
        <p class="dim">{t('st.recovery.hint')}</p>
        <p class:warnt={recoveryLeft <= 2}>{t('st.recovery.left', { n: recoveryLeft })}{#if recoveryLeft <= 2}{' · ' + t('st.recovery.low')}{/if}</p>
        {#if regenOpen}
          <form class="mdl-stack" style="margin-top:10px" onsubmit={(e) => { e.preventDefault(); void regenerate(); }}>
            <p class="dim">{t('st.recovery.regenerateHint')}</p>
            <label class="fl"><span>{t('st.security.password')}</span><span class="field"><input type="password" autocomplete="current-password" required bind:value={regenPw} /></span></label>
            {#if regenError}<div class="err" role="alert">{regenError}</div>{/if}
            <div class="row end">
              <button class="btn" type="button" onclick={() => { regenOpen = false; regenPw = ''; regenError = ''; }}>{t('common.cancel')}</button>
              <button class="btn primary" type="submit" disabled={regenBusy || !regenPw}>{t('st.recovery.regenerate')}</button>
            </div>
          </form>
        {:else}
          <div class="row end"><button class="btn" type="button" onclick={() => (regenOpen = true)}>{t('st.recovery.regenerate')}</button></div>
        {/if}
      {/if}
    </section>
  {/if}

  <section class="panel">
    <h4 class="ph">{t('st.passkeys.title')}</h4>
    <p class="dim">{t('st.passkeys.appNote')}</p>
    {#if passkeys.length === 0}
      <p class="dim" style="margin-top:8px">{t('st.passkeys.empty')}</p>
    {:else}
      <ul class="set-list">
        {#each passkeys as pk (pk.id)}
          <li>
            <div class="grow snip"><b>{pk.name}</b><span class="dim">{t('st.passkeys.created', { date: when(pk.created_at) })} · {pk.last_used_at ? t('st.passkeys.used', { date: when(pk.last_used_at) }) : t('st.passkeys.never')}</span></div>
            <button class="btn sm danger" type="button" onclick={() => { removing = pk; removePw = ''; pkError = ''; }}>{t('st.passkeys.remove')}</button>
          </li>
        {/each}
      </ul>
    {/if}
    {#if removing}
      <form class="mdl-stack" style="margin-top:10px" onsubmit={(e) => { e.preventDefault(); void removePasskey(); }}>
        <p>{t('st.passkeys.removeTitle', { name: removing.name })}</p>
        <label class="fl"><span>{t('st.passkeys.confirmPassword')}</span><span class="field"><input type="password" autocomplete="current-password" required bind:value={removePw} /></span></label>
        <div class="row end">
          <button class="btn" type="button" onclick={() => (removing = null)}>{t('common.cancel')}</button>
          <button class="btn danger" type="submit" disabled={pkBusy || !removePw}>{t('st.passkeys.remove')}</button>
        </div>
      </form>
    {/if}
    {#if pkError}<div class="err" role="alert" style="margin-top:8px">{pkError}</div>{/if}
  </section>
</div>
