<script lang="ts">
  import { onMount } from 'svelte';
  import { t } from '../i18n.svelte';
  import Icon from '../Icon.svelte';
  import { forgetKnown, openPicked } from './store';
  import { VaultError } from './vault';
  import { forgetVaultFile, pickVaultFile, rememberedVaultFile, type Picked } from './vaultFile';

  /**
   * The fields that open the vault: the file and the passphrase. Used by the window "Keys" and by the small window of a form.
   * `compact` is the small window: one field (the lock, the passphrase, the menu "other file / forget", the arrow) instead of the form.
   */
  let { onopened, compact = false }: { onopened?: () => void; compact?: boolean } = $props();

  let picked = $state<Picked | null>(null);
  let pass = $state('');
  let busy = $state(false);
  let error = $state('');
  // the file chosen in the last seven days is offered again: only the passphrase is asked
  let remembered = $state(false);
  let menuOpen = $state(false);
  let input = $state<HTMLInputElement | null>(null);
  // the compact field is asked for the passphrase at once: where the file is known the cursor is in it
  $effect(() => {
    if (compact && picked && input) input.focus();
  });
  // the menu closes on a click elsewhere and on Escape (which then does not close the window under it)
  $effect(() => {
    if (!menuOpen) return;
    const off = (e: PointerEvent): void => {
      if (!(e.target as Element | null)?.closest('.kc-menu, .kc-dots')) menuOpen = false;
    };
    const esc = (e: KeyboardEvent): void => {
      if (e.key !== 'Escape') return;
      e.stopImmediatePropagation();
      menuOpen = false;
    };
    window.addEventListener('pointerdown', off, true);
    window.addEventListener('keydown', esc, true);
    return () => {
      window.removeEventListener('pointerdown', off, true);
      window.removeEventListener('keydown', esc, true);
    };
  });
  onMount(async () => {
    const r = await rememberedVaultFile();
    if (r && !picked) {
      picked = r;
      remembered = true;
    }
  });

  async function choose(): Promise<void> {
    error = '';
    try {
      const p = await pickVaultFile();
      if (p) {
        picked = p;
        remembered = false;
      }
    } catch {
      error = t('common.genericError');
    }
  }
  async function forget(): Promise<void> {
    await forgetVaultFile();
    forgetKnown();
    picked = null;
    remembered = false;
  }
  async function submit(): Promise<void> {
    if (!picked || !pass || busy) return;
    busy = true;
    error = '';
    try {
      await openPicked(picked, pass);
      pass = '';
      picked = null;
      remembered = false;
      onopened?.();
    } catch (e) {
      error = e instanceof VaultError ? t(`keys.err.${e.code}`) : t('common.genericError');
    } finally {
      busy = false;
    }
  }
</script>

{#if compact}
  <form class="kc" onsubmit={(e) => { e.preventDefault(); void submit(); }}>
    <div class="kc-box">
      {#if picked}
        <span class="kc-ic"><Icon name="lock" size={15} /></span>
        <input class="kc-in" type="password" bind:this={input} bind:value={pass} autocomplete="current-password" placeholder={t('keys.compact.ph', { name: picked.name })} aria-label={t('keys.compact.ph', { name: picked.name })} oninput={() => (error = '')} />
        <button class="kc-dots" type="button" title={t('keys.compact.menu')} aria-label={t('keys.compact.menu')} aria-haspopup="menu" aria-expanded={menuOpen} disabled={busy} onclick={() => (menuOpen = !menuOpen)}><Icon name="ellipsis" size={15} /></button>
      {:else}
        <span class="kc-ic dim"><Icon name="folder" size={15} /></span>
        <button class="kc-pick" type="button" onclick={choose}>{t('keys.compact.pick')}</button>
      {/if}
      <button class="kc-go" type="submit" title={busy ? t('keys.open.busy') : t('keys.unlock.go')} aria-label={t('keys.unlock.go')} disabled={!picked || !pass || busy}><Icon name="arrow-right" size={16} /></button>
    </div>
    {#if error}<div class="kc-err" role="alert">{error}</div>{/if}
    {#if menuOpen}
      <div class="kc-menu" role="menu">
        <button type="button" role="menuitem" onclick={() => { menuOpen = false; void choose(); }}>{t('keys.unlock.other')}</button>
        <button type="button" role="menuitem" class="danger" onclick={() => { menuOpen = false; void forget(); }}>{t('keys.file.forget')}</button>
      </div>
    {/if}
  </form>
{:else}
<form class="mdl-stack" onsubmit={(e) => { e.preventDefault(); void submit(); }}>
  {#if remembered && picked}
    <!-- the file is remembered: nothing to open again, only the passphrase and "Unlock" -->
    <div class="kunlock">
      <span class="ki"><Icon name="lock" size={18} /></span>
      <div class="kt"><b class="kn">{picked.name}</b><small class="dim">{t('keys.remembered.hint')}</small></div>
    </div>
    <label class="fl"><span>{t('keys.pass')}</span><span class="field"><input type="password" bind:value={pass} autocomplete="current-password" oninput={() => (error = '')} /></span></label>
    {#if error}<div class="err" role="alert">{error}</div>{/if}
    <div class="krow">
      <button class="btn" type="button" disabled={busy} onclick={choose}>{t('keys.unlock.other')}</button>
      <button class="btn danger" type="button" disabled={busy} onclick={forget}>{t('keys.file.forget')}</button>
      <span class="grow"></span>
      <button class="btn primary" type="submit" disabled={!pass || busy}>{busy ? t('keys.open.busy') : t('keys.unlock.go')}</button>
    </div>
  {:else}
    <div class="fl"><span>{t('keys.file')}</span>
      <div class="row"><span class="fn" class:none={!picked}>{picked?.name ?? t('keys.file.none')}</span><button class="btn" type="button" onclick={choose}>{t('keys.file.pick')}</button></div></div>
    <label class="fl"><span>{t('keys.pass')}</span><span class="field"><input type="password" bind:value={pass} autocomplete="current-password" oninput={() => (error = '')} /></span></label>
    {#if error}<div class="err" role="alert">{error}</div>{/if}
    <button class="btn primary" type="submit" disabled={!picked || !pass || busy}>{busy ? t('keys.open.busy') : t('keys.open.go')}</button>
  {/if}
</form>
{/if}
