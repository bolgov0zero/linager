<script lang="ts">
  import { onMount } from 'svelte';
  import { t } from '../i18n.svelte';
  import Icon from '../Icon.svelte';
  import { openPicked } from './store';
  import { VaultError } from './vault';
  import { forgetVaultFile, pickVaultFile, rememberedVaultFile, type Picked } from './vaultFile';

  /** The fields that open the vault: the file and the passphrase. Used by the window "Keys" and by the small window of a form. */
  let { onopened }: { onopened?: () => void } = $props();

  let picked = $state<Picked | null>(null);
  let pass = $state('');
  let busy = $state(false);
  let error = $state('');
  // the file chosen in the last seven days is offered again: only the passphrase is asked
  let remembered = $state(false);
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
