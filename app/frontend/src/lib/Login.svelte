<script lang="ts">
  import { onMount } from 'svelte';
  import { api, ApiError, type Session } from './api';
  import { native, type AppState, type ProfileView } from './native';
  import { t } from './i18n.svelte';

  let { profile, onsignedin, onchange }: { profile: ProfileView; onsignedin: (s: Session) => void; onchange: (s: AppState) => void } = $props();

  // svelte-ignore state_referenced_locally
  let login = $state(profile.login);
  let password = $state('');
  let totp = $state('');
  let needTotp = $state(false);
  let remember = $state(false);
  let busy = $state(false);
  let error = $state('');

  async function submit(): Promise<void> {
    if (busy || !login.trim() || !password) return;
    busy = true;
    error = '';
    try {
      const s = await api.login(login.trim(), password, needTotp ? totp.trim() : undefined);
      await native.rememberLogin(login.trim());
      if (remember) await native.saveSecret(profile.id, password).catch(() => undefined);
      onsignedin(s);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401 && e.code === 'totp_required') {
        needTotp = true;
        error = totp ? t('login.err.totp') : '';
      } else if (e instanceof ApiError && e.status === 401) error = t('login.err.bad');
      else error = t('login.err.generic');
    } finally {
      busy = false;
    }
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
    <h1>{t('login.title')}</h1>
    <p class="muted" style="font-size:12.5px">{#if profile.name !== profile.address}{profile.name} · {/if}<span class="mono">{profile.address}</span></p>
    <label class="fl"><span>{t('login.login')}</span><span class="field"><input bind:value={login} autocomplete="username" spellcheck="false" /></span></label>
    <label class="fl"><span>{t('login.password')}</span><span class="field"><input type="password" bind:value={password} autocomplete="current-password" /></span></label>
    {#if needTotp}
      <label class="fl"><span>{t('login.totp')}</span><span class="field"><input bind:value={totp} inputmode="numeric" autocomplete="one-time-code" maxlength="12" /></span></label>
    {/if}
    <label class="ck"><input type="checkbox" bind:checked={remember} />{t('login.remember')}</label>
    {#if error}<div class="err" role="alert">{error}</div>{/if}
    <div class="row">
      <button class="btn ghost sm" type="button" onclick={other}>{t('login.other')}</button>
      <button class="btn ghost sm" type="button" onclick={forget}>{t('login.forget')}</button>
      <span class="spacer"></span>
      <button class="btn primary" type="submit" disabled={busy || !login.trim() || !password}>{busy ? t('login.busy') : t('login.submit')}</button>
    </div>
  </form>
</div>
