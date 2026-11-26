<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';
  import { i18n, setLocale, t } from './i18n.svelte';
  import KeysModal from './keys/KeysModal.svelte';
  import { checkVaultKnown, keyState, keysOpen } from './keys/store';
  import { nav, setSection } from './route.svelte';
  import { native } from './native';
  import { startAutoCheck, upd } from './update.svelte';
  import { onMount } from 'svelte';
  import { api, subscribeLive } from './api';
  import { showToast } from './toast';
  import { devs, isAdmin } from './me.svelte';
  import { currentTheme, setTheme } from './theme';

  let { server, onlogout, flush = false, nativeWindows = false, children }: { server: string; onlogout: () => void; flush?: boolean; nativeWindows?: boolean; children: Snippet } = $props();

  /** Settings are a window of their own. */
  const openSettings = (): void => void native.openWindow('settings', devs.pending > 0 ? 'devices' : 'app', 'Linager', nativeWindows);

  let theme = $state(currentTheme());
  // an update of the Linager server is available (an administrator): a mark on the gear, like the one of the application
  let serverNew = $state(false);
  async function checkServer(): Promise<void> {
    if (!isAdmin()) return;
    try {
      const u = (await api.updates()).server;
      serverNew = !!u.available && u.available !== u.current;
    } catch {
      // optional
    }
  }
  // devices that wait for an approval (an administrator): a mark on the gear and a toast when a new one comes
  async function checkDevices(): Promise<void> {
    if (!isAdmin()) return;
    try {
      const n = (await api.devices()).pending;
      if (n > devs.pending) showToast(t('st.dev.newRequest', { n }));
      devs.pending = n;
    } catch {
      // optional
    }
  }
  const keyTitle = (base: string, state: string): string => (state === 'none' ? base : `${base} (${t('keys.state.' + state)})`);
  onMount(() => {
    void checkVaultKnown();
    const stop = startAutoCheck();
    void checkServer();
    const timer = setInterval(() => void checkServer(), 10 * 60_000);
    void checkDevices();
    const stopDevices = subscribeLive(() => void checkDevices(), ['devices']);
    return () => {
      stop();
      stopDevices();
      clearInterval(timer);
    };
  });

  function flipTheme(): void {
    theme = theme === 'dark' ? 'light' : 'dark';
    setTheme(theme);
  }
</script>

<div class="app">
  <header class="topbar">
    <div class="brand"><img class="logo" src="icon.svg" alt="" /><b>Linager</b></div>
    <nav class="seg nav-tabs" aria-label="Sections">
      <button type="button" class:on={nav.section === 'console'} aria-current={nav.section === 'console' ? 'page' : undefined} onclick={() => setSection('console')}><Icon name="square-terminal" size={15} />{t('nav.console')}</button>
      <button type="button" class:on={nav.section === 'dash'} aria-current={nav.section === 'dash' ? 'page' : undefined} onclick={() => setSection('dash')}><Icon name="layout-grid" size={15} />{t('nav.dashboard')}</button>
    </nav>
    <span class="grow"></span>
    <div class="top-actions">
      {#if server}<span class="badge b-ok mono" title={server}>{server}</span>{/if}
      <button class="btn ghost" type="button" onclick={() => setLocale(i18n.locale === 'ru' ? 'en' : 'ru')} title="Language"><span>{i18n.locale === 'ru' ? 'RU' : 'EN'}</span></button>
      <button class="btn icon ghost" type="button" onclick={flipTheme} title={t('nav.theme')}><Icon name={theme === 'dark' ? 'sun' : 'moon'} size={16} /></button>
      <button class="btn icon ghost" type="button" data-keys={$keyState} title={keyTitle(t('keys.app.menu'), $keyState)} aria-label={keyTitle(t('keys.app.menu'), $keyState)} onclick={() => keysOpen.set(true)}><Icon name="key-round" size={16} /></button>
      <button class="btn icon ghost" class:has-mark={upd.info?.newer || serverNew || devs.pending > 0} type="button" title={devs.pending > 0 ? t('st.dev.mark', { n: devs.pending }) : upd.info?.newer ? t('set.upd.mark') : serverNew ? t('set.serverUpdate') : t('nav.settings')} aria-label={t('nav.settings')} onclick={openSettings}><Icon name="settings" size={16} /></button>
      <button class="btn ghost logout" type="button" onclick={onlogout}><Icon name="log-out" size={15} /><span>{t('nav.logout')}</span></button>
    </div>
  </header>
  <main class="view" class:has-sp={flush}>{@render children()}</main>
</div>
{#if $keysOpen}<KeysModal onclose={() => keysOpen.set(false)} />{/if}
