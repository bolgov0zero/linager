<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';
  import { i18n, setLocale, t } from './i18n.svelte';
  import KeysModal from './keys/KeysModal.svelte';
  import { keysOpen } from './keys/store';
  import { nav, setSection } from './route.svelte';
  import { native } from './native';
  import { startAutoCheck, upd } from './update.svelte';
  import { onMount } from 'svelte';
  import { currentTheme, setTheme } from './theme';

  let { server, onlogout, flush = false, nativeWindows = false, children }: { server: string; onlogout: () => void; flush?: boolean; nativeWindows?: boolean; children: Snippet } = $props();

  /** Settings are a window of their own. */
  const openSettings = (): void => void native.openWindow('settings', 'app', 'Linager', nativeWindows);

  let theme = $state(currentTheme());
  onMount(() => startAutoCheck());

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
      <button class="btn icon ghost" type="button" title={t('keys.app.menu')} aria-label={t('keys.app.menu')} onclick={() => keysOpen.set(true)}><Icon name="key-round" size={16} /></button>
      <button class="btn icon ghost" class:has-mark={upd.info?.newer} type="button" title={upd.info?.newer ? t('set.upd.mark') : t('nav.settings')} aria-label={t('nav.settings')} onclick={openSettings}><Icon name="settings" size={16} /></button>
      <button class="btn ghost logout" type="button" onclick={onlogout}><Icon name="log-out" size={15} /><span>{t('nav.logout')}</span></button>
    </div>
  </header>
  <main class="view" class:has-sp={flush}>{@render children()}</main>
</div>
{#if $keysOpen}<KeysModal onclose={() => keysOpen.set(false)} />{/if}
