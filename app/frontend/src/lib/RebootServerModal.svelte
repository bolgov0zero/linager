<script lang="ts">
  import { serverApi } from './api';
  import CopyButton from './CopyButton.svelte';
  import { errorText } from './errors';
  import { t } from './i18n.svelte';
  import Modal from './Modal.svelte';
  import { showToast } from './toast';

  // Reboot a server: the name is typed to confirm, as for a deletion.
  let { server, onclose }: { server: { id: string; name: string }; onclose: () => void } = $props();

  let typed = $state('');
  let busy = $state(false);
  let error = $state('');
  const can = $derived(!busy && typed.trim() === server.name.trim());

  async function reboot(): Promise<void> {
    if (!can) return;
    busy = true;
    error = '';
    try {
      await serverApi.reboot(server.id);
      showToast(t('reboot.started'));
      onclose();
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={t('reboot.title')} {onclose}>
  <form class="mdl-stack" onsubmit={(e) => { e.preventDefault(); void reboot(); }}>
    <p>{t('reboot.text')}</p>
    <p class="dim copyrow">{t('reboot.server', { name: server.name })}<CopyButton text={server.name} /></p>
    <label class="fl"><span>{t('reboot.typeName', { name: server.name })}</span><span class="field"><input autocomplete="off" spellcheck="false" bind:value={typed} /></span></label>
    {#if error}<div class="err" role="alert">{error}</div>{/if}
    <div class="row end">
      <button class="btn" type="button" disabled={busy} onclick={onclose}>{t('common.cancel')}</button>
      <button class="btn danger" type="submit" disabled={!can}>{t('reboot.confirm')}</button>
    </div>
  </form>
</Modal>
