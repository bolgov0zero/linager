<script lang="ts">
  import { onMount } from 'svelte';
  import { api, ApiError, type Session, type TotpSetup } from '../api';
  import { confirmAsk } from '../dialogs.svelte';
  import { isUnauthorized } from '../errors';
  import { i18n, t } from '../i18n.svelte';
  import { showToast } from '../toast';

  let { session, onrefresh, onauthlost }: { session: Session; onrefresh: () => Promise<void>; onauthlost: () => void } = $props();

  const MIN_PASSWORD = 12;

  // ---- password ----
  /** The rows open in place: the form of a row shows under it, and the closed rows are one line each. */
  let pwOpen = $state(false);
  let disOpen = $state(false);
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
      pwOpen = false;
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
      disOpen = false;
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
</script>

<div class="set-sec prof">
  <div class="pf-who"><span class="pf-av" aria-hidden="true">{[...session.login][0]?.toUpperCase() ?? '?'}</span><b>{session.login}</b>{#if session.role}<span class="badge b-mute">{t(`st.users.role.${session.role}`)}</span>{/if}</div>

  <div class="pf-list">
    <div class="pfr">
      <b>{t('st.security.password')}</b><span class="grow"></span>
      <button class="btn sm" type="button" aria-expanded={pwOpen} onclick={() => { pwOpen = !pwOpen; pwError = ''; }}>{pwOpen ? t('common.cancel') : t('st.profile.change')}</button>
    </div>
    {#if pwOpen}
      <form class="pfx" onsubmit={(e) => { e.preventDefault(); void changePassword(); }}>
        <!-- the login is here so a password manager updates the right entry -->
        <input type="text" name="username" autocomplete="username" value={session.login} hidden readonly />
        <div class="pf-fg">
          <label class="pf-f">{t('st.profile.pw.current')}<span class="field"><input type="password" autocomplete="current-password" required bind:value={curPw} /></span></label>
          <label class="pf-f">{t('st.profile.pw.new')}<span class="field"><input type="password" autocomplete="new-password" required placeholder={t('st.profile.pw.min')} bind:value={newPw} /></span></label>
          <label class="pf-f">{t('st.profile.pw.repeat')}<span class="field"><input type="password" autocomplete="new-password" required bind:value={newPw2} /></span></label>
          <button class="btn primary" type="submit" disabled={pwBusy}>{t('st.profile.pw.submit')}</button>
        </div>
        {#if pwError}<div class="err" role="alert">{pwError}</div>{/if}
      </form>
    {/if}

    <div class="pfr">
      <b>{t('st.profile.2fa.title')}</b>
      <span class="badge {session.totp_enabled ? 'b-ok' : 'b-mute'}">{session.totp_enabled ? t('st.profile.on') : t('st.profile.off')}</span>
      <span class="grow"></span>
      {#if session.totp_enabled}
        <button class="btn sm danger" type="button" aria-expanded={disOpen} onclick={() => { disOpen = !disOpen; tfError = ''; }}>{disOpen ? t('common.cancel') : t('st.profile.disable')}</button>
      {:else if setup}
        <button class="btn sm" type="button" onclick={() => { setup = null; tfError = ''; }}>{t('common.cancel')}</button>
      {:else}
        <button class="btn sm primary" type="button" disabled={tfBusy} onclick={() => void startSetup()}>{t('st.profile.enable')}</button>
      {/if}
    </div>
    {#if session.totp_enabled && disOpen}
      <form class="pfx" onsubmit={(e) => { e.preventDefault(); void disable(); }}>
        <div class="pf-fg">
          <label class="pf-f">{t('st.security.password')}<span class="field"><input type="password" autocomplete="current-password" required bind:value={disablePw} /></span></label>
          <button class="btn danger" type="submit" disabled={tfBusy || !disablePw}>{t('st.security.disable')}</button>
        </div>
        {#if tfError}<div class="err" role="alert">{tfError}</div>{/if}
      </form>
    {:else if !session.totp_enabled && setup}
      <form class="pfx" onsubmit={(e) => { e.preventDefault(); void enable(); }}>
        <div class="pf-fg">
          <div class="pf-f grow">{t('st.security.secret')}
            <span class="field pf-secret"><b class="mono secret">{groupSecret(setup.secret)}</b><button class="linkbtn" type="button" onclick={() => void copy(setup?.secret.replace(/\s+/g, '') ?? '')}>{t('st.common.copy')}</button></span>
          </div>
          <label class="pf-f">{t('st.profile.code')}<span class="field"><input class="mono" inputmode="numeric" autocomplete="one-time-code" maxlength="6" placeholder="123456" bind:value={code} /></span></label>
          <button class="btn primary" type="submit" disabled={tfBusy || code.trim().length !== 6}>{t('st.profile.enable')}</button>
        </div>
        <span class="dim pf-small">{t('st.profile.orLink')} <button class="linkbtn" type="button" onclick={() => void copy(setup?.uri ?? '')}>{t('st.profile.copyLink')}</button></span>
        {#if tfError}<div class="err" role="alert">{tfError}</div>{/if}
      </form>
    {:else if !session.totp_enabled && tfError}
      <div class="pfx"><div class="err" role="alert">{tfError}</div></div>
    {/if}

    {#if session.totp_enabled || codes}
      <div class="pfr">
        <b>{t('st.recovery.title')}</b>
        {#if codes}<span class="badge b-ok">{t('st.profile.newCodes')}</span>
        {:else}<span class="badge {recoveryLeft <= 2 ? 'b-warn' : 'b-ok'}" title={recoveryLeft <= 2 ? t('st.recovery.low') : ''}>{t('st.recovery.leftShort', { n: recoveryLeft })}</span>{/if}
        <span class="grow"></span>
        {#if !codes}<button class="btn sm" type="button" aria-expanded={regenOpen} onclick={() => { regenOpen = !regenOpen; regenPw = ''; regenError = ''; }}>{regenOpen ? t('common.cancel') : t('st.profile.newOnes')}</button>{/if}
      </div>
      {#if codes}
        <div class="pfx">
          <div class="warnbox warn">{t('st.recovery.shown')}</div>
          <ul class="pf-codes mono">{#each codes as c (c)}<li>{c}</li>{/each}</ul>
          <div class="row">
            <button class="btn sm" type="button" onclick={() => void copy(codes?.join('\n') ?? '')}>{t('st.recovery.copy')}</button>
            <button class="btn sm" type="button" onclick={downloadCodes}>{t('st.recovery.download')}</button>
            <span class="grow"></span>
            <button class="btn primary" type="button" onclick={() => (codes = null)}>{t('st.recovery.saved')}</button>
          </div>
        </div>
      {:else if regenOpen}
        <form class="pfx" onsubmit={(e) => { e.preventDefault(); void regenerate(); }}>
          <div class="pf-fg">
            <label class="pf-f">{t('st.security.password')}<span class="field"><input type="password" autocomplete="current-password" required bind:value={regenPw} /></span></label>
            <button class="btn primary" type="submit" disabled={regenBusy || !regenPw}>{t('st.profile.newOnes')}</button>
          </div>
          <span class="dim pf-small">{t('st.recovery.regenerateHint')}</span>
          {#if regenError}<div class="err" role="alert">{regenError}</div>{/if}
        </form>
      {/if}
    {/if}
  </div>
</div>
