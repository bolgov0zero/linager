<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';
  import { i18n, setLocale, t } from './i18n.svelte';
  import { currentTheme, setTheme } from './theme';

  let { server, onlogout, children }: { server: string; onlogout: () => void; children: Snippet } = $props();

  let theme = $state(currentTheme());

  function flipTheme(): void {
    theme = theme === 'dark' ? 'light' : 'dark';
    setTheme(theme);
  }
</script>

<div class="app">
  <header class="topbar">
    <div class="brand"><img class="logo" src="icon.svg" alt="" /><b>Linager</b></div>
    <nav class="seg nav-tabs" aria-label="Sections">
      <button type="button" class="on" aria-current="page"><Icon name="layout-grid" size={15} />{t('nav.dashboard')}</button>
      <button type="button" disabled title={t('nav.soon')}><Icon name="square-terminal" size={15} />{t('nav.console')}</button>
    </nav>
    <span class="grow"></span>
    <div class="top-actions">
      <span class="muted mono" style="font-size:11.5px">{server}</span>
      <button class="btn ghost" type="button" onclick={() => setLocale(i18n.locale === 'ru' ? 'en' : 'ru')} title="Language"><span>{i18n.locale === 'ru' ? 'RU' : 'EN'}</span></button>
      <button class="btn icon ghost" type="button" onclick={flipTheme} title={t('nav.theme')}><Icon name={theme === 'dark' ? 'sun' : 'moon'} size={16} /></button>
      <button class="btn icon ghost" type="button" disabled title={t('nav.soon')}><Icon name="settings" size={16} /></button>
      <button class="btn ghost" type="button" onclick={onlogout}><Icon name="log-out" size={15} /><span>{t('nav.logout')}</span></button>
    </div>
  </header>
  <main class="view">{@render children()}</main>
</div>
