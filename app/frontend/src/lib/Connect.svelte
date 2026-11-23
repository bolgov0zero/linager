<script lang="ts">
  import { native, NativeError, type AppState, type ProfileView } from './native';
  import { t } from './i18n.svelte';

  let { app, onconnected }: { app: AppState; onconnected: (s: AppState) => void } = $props();

  type Step = 'pick' | 'confirm';
  let step = $state<Step>('pick');
  let address = $state('');
  let fingerprint = $state('');
  let known = $state<ProfileView | null>(null); // a saved server whose certificate changed
  let busy = $state(false);
  let error = $state('');

  function fail(e: unknown): void {
    const code = e instanceof NativeError ? e.code : '';
    const key = `conn.err.${code}`;
    error = t(key) === key ? t('conn.err.generic') : t(key);
  }

  async function check(addr = address): Promise<void> {
    const a = addr.trim();
    if (!a || busy) return;
    busy = true;
    error = '';
    try {
      const { fingerprint: fp } = await native.probe(a);
      address = a;
      fingerprint = fp;
      known = app.profiles.find((p) => p.address === a && p.fingerprint !== fp) ?? null;
      // a server that is already pinned with this very certificate needs no question
      const same = app.profiles.find((p) => p.address === a && p.fingerprint === fp);
      if (same) {
        onconnected(await native.use(same.id));
        return;
      }
      step = 'confirm';
    } catch (e) {
      fail(e);
    } finally {
      busy = false;
    }
  }

  async function trust(): Promise<void> {
    busy = true;
    error = '';
    try {
      onconnected(await native.connect(address, fingerprint));
    } catch (e) {
      fail(e);
    } finally {
      busy = false;
    }
  }
</script>

<div class="center">
  <div class="panel-card">
    <div class="brand"><img class="logo" src="icon.svg" alt="" /><b>Linager</b></div>
    <h1>{t('conn.title')}</h1>

    {#if step === 'pick'}
      <p class="muted" style="font-size:12.5px">{t('conn.lead')}</p>
      {#if app.profiles.length > 0}
        <div class="fl">
          <span>{t('conn.saved')}</span>
          <div class="plist">
            {#each app.profiles as p (p.id)}
              <button type="button" disabled={busy} onclick={() => check(p.address)}>{p.name}<span class="mono">{p.address}</span></button>
            {/each}
          </div>
        </div>
      {/if}
      <form class="fl" onsubmit={(e) => { e.preventDefault(); void check(); }}>
        <span>{t('conn.address')}</span>
        <label class="field"><input bind:value={address} placeholder={t('conn.address.ph')} autocomplete="off" spellcheck="false" /></label>
        {#if error}<div class="err" role="alert">{error}</div>{/if}
        <div class="row end"><button class="btn primary" type="submit" disabled={busy || !address.trim()}>{busy ? t('conn.checking') : t('conn.check')}</button></div>
      </form>
    {:else}
      <div class="fl">
        <span>{t('conn.fp.title')}</span>
        <div class="fp">{fingerprint}</div>
        <small>{address}</small>
      </div>
      {#if known}<div class="warnbox" role="alert">{t('conn.fp.changed')}</div>{:else}<p class="muted" style="font-size:12.5px">{t('conn.fp.hint')}</p>{/if}
      {#if error}<div class="err" role="alert">{error}</div>{/if}
      <div class="row end">
        <button class="btn" type="button" onclick={() => { step = 'pick'; error = ''; }}>{t('conn.back')}</button>
        <button class="btn primary" type="button" disabled={busy} onclick={trust}>{t('conn.fp.trust')}</button>
      </div>
    {/if}
  </div>
</div>
