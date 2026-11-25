<script lang="ts">
  import { i18n, t } from '../i18n.svelte';
  import { fmtBytes } from '../status';
  import Icon from '../Icon.svelte';
  import { autoCheck, checkUpdate, githubToken, installUpdate, setAutoCheck, setChannel, startAutoCheck, upd, updateBusy } from '../update.svelte';
  import { onMount } from 'svelte';
  import { native } from '../native';

  let auto = $state(autoCheck());
  /** The release notes and the token field are folded until asked for: the page keeps to what is needed every time. */
  let notesOpen = $state(false);
  let tokenOpen = $state(false);
  const info = $derived(upd.info);
  const channels = $derived([{ v: 'stable', t: t('set.upd.ch.stable') }, { v: 'beta', t: t('set.upd.ch.beta') }]);
  function pickChannel(v: string): void {
    setChannel(v as 'stable' | 'beta');
    if (upd.info) upd.info = { ...upd.info, channel: v, newer: false, latest: undefined };
    void checkUpdate();
  }
  const phaseText = $derived(
    upd.status.phase === 'downloading' ? t('set.upd.downloading', { pct: upd.status.pct }) : upd.status.phase === 'installing' ? t('set.upd.installing') : upd.status.phase === 'restarting' ? t('set.upd.restarting') : '',
  );
  /** What went wrong with the last check: the limit of GitHub (with the time it ends), or no connection. */
  const failText = $derived.by(() => {
    const p = upd.problem;
    if (p?.kind === 'rate_limited') {
      const when = p.retryAt ? new Date(p.retryAt * 1000).toLocaleTimeString(i18n.locale === 'ru' ? 'ru-RU' : 'en-GB', { hour: '2-digit', minute: '2-digit' }) : '';
      return when ? t('set.upd.rateLimitedAt', { time: when }) : t('set.upd.rateLimited');
    }
    return t('set.upd.checkFailed');
  });
  const reason = $derived(info?.reason === 'headless' ? t('set.upd.headless') : info?.reason === 'dev' ? t('set.upd.dev') : '');
  // this window has its own page: it reads what is known and asks again
  // the GitHub token of the checks (optional): with it the limit of GitHub is 5000 requests an hour for the token, not 60 for the address
  let tokenSet = $state(false);
  let tokenDraft = $state('');
  let tokenBusy = $state(false);
  let tokenError = $state('');
  async function saveToken(): Promise<void> {
    if (!tokenDraft.trim() || tokenBusy) return;
    tokenBusy = true;
    tokenError = '';
    const code = await githubToken.set(tokenDraft.trim());
    tokenBusy = false;
    if (code) {
      tokenError = t(code === 'invalid_token' ? 'set.upd.token.invalid' : code === 'bad_token' ? 'set.upd.token.bad' : 'set.upd.token.network');
      return;
    }
    tokenSet = true;
    tokenDraft = '';
    void checkUpdate();
  }
  async function clearToken(): Promise<void> {
    await githubToken.clear();
    tokenSet = false;
  }
  let login = $state<{ supported: boolean; enabled: boolean } | null>(null);
  let loginBusy = $state(false);
  async function setLogin(on: boolean): Promise<void> {
    loginBusy = true;
    try {
      login = await native.autostartSet(on);
    } catch {
      login = login ? { ...login } : login; // the box goes back to what it was
    } finally {
      loginBusy = false;
    }
  }
  onMount(() => {
    void githubToken.get().then((v) => (tokenSet = v)).catch(() => undefined);
    void native.autostartGet().then((r) => (login = r)).catch(() => undefined);
    const stop = startAutoCheck();
    return stop;
  });
</script>

<div class="set-sec app">
<section class="panel blk">
  <h4 class="ph">{t('set.upd.title')}</h4>
  <div class="vrow">
    <b class="ubig mono">{info?.current ?? '—'}</b>
    {#if info?.supported && info.newer}
      <span class="dim">→</span><b class="ubig mono new">{info.latest ?? ''}</b><span class="badge b-warn">{t('set.upd.availShort')}{#if info.size} · {fmtBytes(info.size)}{/if}</span>
    {:else if info?.supported && info.checked_at && !upd.failed}
      <span class="badge b-ok">{t('set.upd.latestShort')}</span>
    {/if}
    <span class="grow"></span>
    {#if info?.supported && info.newer}
      {#if updateBusy()}
        <span class="bar lv-ok upd-bar"><i style="width:{upd.status.pct}%"></i></span><span class="dim">{phaseText}</span>
      {:else}
        <button class="btn primary" type="button" onclick={installUpdate}>{t('set.upd.install')}</button>
      {/if}
    {:else if info?.supported}
      <button class="btn" type="button" disabled={upd.checking || updateBusy()} onclick={checkUpdate}>{upd.checking ? t('set.upd.checking') : t('set.upd.check')}</button>
    {/if}
  </div>
  {#if reason}<p class="dim">{reason}</p>{/if}
  {#if info?.supported}
    {#if info.newer && info.notes?.length}
      <button class="linkbtn" type="button" aria-expanded={notesOpen} onclick={() => (notesOpen = !notesOpen)}>{t('set.upd.whatsNew')}<Icon name={notesOpen ? 'chevron-up' : 'chevron-down'} size={12} /></button>
      {#if notesOpen}<ul class="upd-notes">{#each info.notes as n (n)}<li>{n}</li>{/each}</ul>{/if}
    {/if}
    {#if upd.status.phase === 'error'}<div class="err" role="alert">{t('set.upd.failed')}: {upd.status.error}</div>{/if}
    {#if upd.failed}<div class="err" role="alert">{failText}</div>{/if}
    <hr class="sep" />
    <div class="ctl">
      <span class="alb">{t('set.upd.channel')}<button class="q" type="button" aria-label={t('set.upd.tipChannel')} data-tip={t('set.upd.tipChannel')}>?</button></span>
      <span class="seg" role="radiogroup" aria-label={t('set.upd.channel')}>
        {#each channels as c (c.v)}<button type="button" role="radio" aria-checked={info.channel === c.v} class:on={info.channel === c.v} onclick={() => info.channel !== c.v && pickChannel(c.v)}>{c.t}</button>{/each}
      </span>
      <span class="grow"></span>
      {#if info.newer}<button class="btn" type="button" disabled={upd.checking || updateBusy()} onclick={checkUpdate}>{upd.checking ? t('set.upd.checking') : t('set.upd.check')}</button>{/if}
      <label class="ck"><input type="checkbox" bind:checked={auto} onchange={() => setAutoCheck(auto)} />{t('set.upd.auto')}</label>
    </div>
    <div class="ctl" style="gap:10px">
      <button class="linkbtn" type="button" aria-expanded={tokenOpen} onclick={() => (tokenOpen = !tokenOpen)}>{tokenSet ? t('set.upd.token.setLink') : t('set.upd.token.addLink')}</button>
      <button class="q" type="button" aria-label={t('set.upd.tipToken')} data-tip={t('set.upd.tipToken')}>?</button>
    </div>
    {#if tokenOpen}
      <div class="tokrow">
        {#if tokenSet}
          <div class="row"><span class="ok-t">{t('set.upd.token.set')}</span><span class="grow"></span><button class="btn sm danger" type="button" onclick={clearToken}>{t('set.upd.token.remove')}</button></div>
        {:else}
          <form class="row" autocomplete="off" onsubmit={(e) => { e.preventDefault(); void saveToken(); }}>
            <span class="field grow"><input type="password" autocomplete="off" spellcheck="false" placeholder="github_pat_…" aria-label={t('set.upd.token.title')} bind:value={tokenDraft} oninput={() => (tokenError = '')} /></span>
            <button class="btn" type="submit" disabled={tokenBusy || !tokenDraft.trim()}>{t('common.save')}</button>
          </form>
        {/if}
        {#if tokenError}<div class="err" role="alert">{tokenError}</div>{/if}
      </div>
    {/if}
  {/if}
</section>

{#if login?.supported}
  <section class="panel blk">
    <h4 class="ph">{t('set.tray.title')}</h4>
    <div class="ctl" style="gap:10px">
      <label class="ck"><input type="checkbox" disabled={loginBusy} checked={login.enabled} onchange={(e) => void setLogin(e.currentTarget.checked)} />{t('set.tray.login')}</label>
      <button class="q" type="button" aria-label={t('set.tray.tip')} data-tip={t('set.tray.tip')}>?</button>
    </div>
  </section>
{/if}
</div>
