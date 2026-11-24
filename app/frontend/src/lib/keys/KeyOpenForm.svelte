<script lang="ts">
  import { onMount } from 'svelte';
  import { t } from '../i18n.svelte';
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
  <div class="fl"><span>{t('keys.file')}</span>
    <div class="row"><span class="fn" class:none={!picked}>{picked?.name ?? t('keys.file.none')}</span><button class="btn" type="button" onclick={choose}>{t('keys.file.pick')}</button>{#if remembered}<button class="btn" type="button" onclick={forget}>{t('keys.file.forget')}</button>{/if}</div>
    {#if remembered}<small class="dim">{t('keys.file.remembered')}</small>{/if}</div>
  <label class="fl"><span>{t('keys.pass')}</span><span class="field"><input type="password" bind:value={pass} autocomplete="current-password" oninput={() => (error = '')} /></span></label>
  {#if error}<div class="err" role="alert">{error}</div>{/if}
  <button class="btn primary" type="submit" disabled={!picked || !pass || busy}>{busy ? t('keys.open.busy') : t('keys.open.go')}</button>
</form>
