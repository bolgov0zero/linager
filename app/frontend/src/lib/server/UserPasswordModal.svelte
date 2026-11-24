<script lang="ts">
  import { serverApi2, type HostUser } from '../api';
  import { errorText } from '../errors';
  import { t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';
  import { genPassword } from '../users-util';

  let { serverId, user, onclose, ondone }: { serverId: string; user: HostUser; onclose: () => void; ondone: (msg: string) => void } = $props();
  let mode = $state<'generate' | 'manual'>('generate');
  let generated = $state(genPassword());
  let manual = $state('');
  let busy = $state(false);
  let error = $state('');
  let copied = $state(false);
  const password = $derived(mode === 'generate' ? generated : manual);
  const valid = $derived(password.length >= 8 && password.length <= 128);

  async function copy(): Promise<void> {
    try {
      await navigator.clipboard.writeText(generated);
      copied = true;
      setTimeout(() => (copied = false), 2000);
    } catch {
      // a blocked clipboard: the password stays on screen
    }
  }
  async function save(): Promise<void> {
    if (busy || !valid) return;
    busy = true;
    error = '';
    try {
      await serverApi2.hostUserPassword(serverId, user.name, password);
      ondone(t('usr.pw.done', { user: user.name }));
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={t('usr.pw.title', { user: user.name })} {onclose}>
  <div class="opts">
    <button class="optc" type="button" aria-pressed={mode === 'generate'} onclick={() => (mode = 'generate')}>{t('usr.pw.generate')}<small>{t('usr.pw.generateHint')}</small></button>
    <button class="optc" type="button" aria-pressed={mode === 'manual'} onclick={() => (mode = 'manual')}>{t('usr.pw.manual')}</button>
  </div>
  {#if mode === 'generate'}
    <div class="row">
      <span class="field grow"><input class="mono" readonly value={generated} aria-label={t('usr.pw.generated')} /></span>
      <button class="btn sm" type="button" onclick={copy}>{copied ? t('usr.pw.copied') : t('common.copy')}</button>
      <button class="btn sm" type="button" onclick={() => (generated = genPassword())}>{t('usr.pw.again')}</button>
    </div>
    <p class="dim">{t('usr.pw.once')}</p>
  {:else}
    <span class="field"><input type="password" autocomplete="new-password" bind:value={manual} aria-label={t('usr.pw.new')} /></span>
    <p class="dim">{t('usr.pw.rule')}</p>
  {/if}
  {#if error}<div class="err" role="alert">{error}</div>{/if}
  {#snippet foot()}
    <span class="spacer"></span>
    <button class="btn" type="button" onclick={onclose}>{t('common.cancel')}</button>
    <button class="btn primary" type="button" disabled={busy || !valid} onclick={save}>{t('usr.pw.set')}</button>
  {/snippet}
</Modal>
