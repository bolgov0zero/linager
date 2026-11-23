<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { api, ApiError, type Session } from './api';
  import { native, type AppState, type ProfileView } from './native';
  import { t } from './i18n.svelte';

  let { profile, onsignedin, onchange }: { profile: ProfileView; onsignedin: (s: Session) => void; onchange: (s: AppState) => void } = $props();

  type Step = 'password' | 'totp';

  // svelte-ignore state_referenced_locally
  let login = $state(profile.login);
  let password = $state('');
  let step = $state<Step>('password');
  let code = $state('');
  let recovery = $state(false);
  let remember = $state(false);
  let busy = $state(false);
  let error = $state('');
  let codeEl = $state<HTMLInputElement | null>(null);

  /** What is sent as the code: an authenticator shows "123 456", so a one-time code is reduced to its digits;
   *  a recovery code (EV3M8-CKX9D) keeps its letters and loses only the spaces. */
  function cleanCode(): string {
    return recovery ? code.replace(/\s+/g, '').toUpperCase() : code.replace(/\D/g, '');
  }

  async function submit(): Promise<void> {
    if (busy || !login.trim() || !password) return;
    const second = step === 'totp';
    if (second && !cleanCode()) return;
    busy = true;
    error = '';
    try {
      const s = await api.login(login.trim(), password, second ? cleanCode() : undefined);
      // what is kept for next time must not turn a good sign-in into a failed one
      await native.rememberLogin(login.trim()).catch(() => undefined);
      if (remember) await native.saveSecret(profile.id, password).catch(() => undefined);
      onsignedin(s);
    } catch (e) {
      if (!second && e instanceof ApiError && e.status === 401 && e.code === 'totp_required') {
        // the password is right, the second factor is next
        step = 'totp';
        code = '';
        recovery = false;
        await tick();
        codeEl?.focus();
      } else if (e instanceof ApiError && e.status === 401) {
        error = second ? t('login.err.code') : t('login.err.bad');
        if (second) code = '';
      } else if (e instanceof ApiError && e.status === 429) {
        error = t('login.err.locked');
      } else {
        // the status and the code of the server, so that a failure can be told apart (503 not connected, 403 refused, ...)
        const why = e instanceof ApiError ? `HTTP ${e.status}${e.code ? `: ${e.code}` : ''}` : e instanceof Error ? e.message : String(e);
        error = `${t('login.err.generic')} (${why})`;
      }
    } finally {
      busy = false;
    }
  }

  // sign in as soon as the sixth digit is typed or pasted
  function onCode(): void {
    if (recovery) return;
    code = code.replace(/\D/g, '').slice(0, 6);
    if (code.length === 6) void submit();
  }

  function toggleRecovery(): void {
    recovery = !recovery;
    code = '';
    error = '';
    codeEl?.focus();
  }

  function back(): void {
    step = 'password';
    code = '';
    error = '';
    recovery = false;
  }

  onMount(async () => {
    // a saved password signs in by itself; the 2FA code, when it is on, is still asked
    if (profile.saved && profile.login) {
      try {
        password = (await native.secret(profile.id)).password;
        remember = true;
        await submit();
      } catch {
        // no saved password after all: the form stays
      }
    }
  });

  async function other(): Promise<void> {
    onchange(await native.disconnect());
  }

  async function forget(): Promise<void> {
    onchange(await native.forget(profile.id));
  }
</script>

<div class="center">
  <form class="panel-card" onsubmit={(e) => { e.preventDefault(); void submit(); }}>
    <div class="brand"><img class="logo" src="icon.svg" alt="" /><b>Linager</b></div>
    <h1>{step === 'totp' ? t('login.2fa.title') : t('login.title')}</h1>
    <p class="muted" style="font-size:12.5px">{#if profile.name !== profile.address}{profile.name} · {/if}<span class="mono">{profile.address}</span></p>

    <div class="stack" hidden={step === 'totp'}>
      <label class="fl"><span>{t('login.login')}</span><span class="field"><input bind:value={login} autocomplete="username" spellcheck="false" /></span></label>
      <label class="fl"><span>{t('login.password')}</span><span class="field"><input type="password" bind:value={password} autocomplete="current-password" /></span></label>
      <label class="ck"><input type="checkbox" bind:checked={remember} />{t('login.remember')}</label>
    </div>

    {#if step === 'totp'}
      <p class="muted" style="font-size:12.5px">{t('login.2fa.for', { login: login.trim() })}</p>
      <label class="fl">
        <span>{recovery ? t('login.recovery') : t('login.totp')}</span>
        <span class="field">
          <input
            class="mono"
            bind:this={codeEl}
            bind:value={code}
            oninput={onCode}
            inputmode={recovery ? 'text' : 'numeric'}
            autocomplete="one-time-code"
            autocapitalize="characters"
            spellcheck="false"
            maxlength={recovery ? 20 : 12}
            aria-label={t('login.totp')}
          />
        </span>
      </label>
      <button class="btn ghost sm" type="button" style="justify-self:start" onclick={toggleRecovery}>{recovery ? t('login.useTotp') : t('login.useRecovery')}</button>
    {/if}

    {#if error}<div class="err" role="alert">{error}</div>{/if}

    <div class="row">
      {#if step === 'totp'}
        <button class="btn" type="button" disabled={busy} onclick={back}>{t('conn.back')}</button>
      {:else}
        <button class="btn ghost sm" type="button" onclick={other}>{t('login.other')}</button>
        <button class="btn ghost sm" type="button" onclick={forget}>{t('login.forget')}</button>
      {/if}
      <span class="spacer"></span>
      <button class="btn primary" type="submit" disabled={busy || !login.trim() || !password || (step === 'totp' && !cleanCode())}>
        {busy ? t('login.busy') : step === 'totp' ? t('login.2fa.submit') : t('login.submit')}
      </button>
    </div>
  </form>
</div>
