<script lang="ts">
  import { serverApi } from './api';
  import { errorText } from './errors';
  import { t } from './i18n.svelte';
  import Modal from './Modal.svelte';

  // Delete a server: the agent can be uninstalled from the server at the same time (on by default; an offline server keeps
  // the choice on but locked: the agent is removed when it comes back), and the name is typed to confirm.
  let { server, onclose, ondeleted }: {
    server: { id: string; name: string; online: boolean };
    onclose: () => void;
    ondeleted: () => void;
  } = $props();

  let uninstall = $state(true);
  let typed = $state('');
  let busy = $state(false);
  let error = $state('');
  const canDelete = $derived(!busy && typed.trim() === server.name.trim());

  async function confirmDelete(): Promise<void> {
    if (!canDelete) return;
    busy = true;
    error = '';
    try {
      await serverApi.deleteServer(server.id, server.name, uninstall);
      ondeleted();
    } catch (e) {
      error = errorText(e) === t('common.genericError') ? t('delete.failed') : errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={t('delete.title')} {onclose}>
  <form class="mdl-stack" onsubmit={(e) => { e.preventDefault(); void confirmDelete(); }}>
    <p>{t('delete.text')}</p>
    <label class="ck"><input type="checkbox" bind:checked={uninstall} disabled={!server.online} />{t('delete.uninstall')}</label>
    {#if !server.online}<p class="dim">{t('delete.uninstallOffline')}</p>{/if}
    <label class="fl"><span>{t('delete.typeName', { name: server.name })}</span><span class="field"><input autocomplete="off" spellcheck="false" bind:value={typed} /></span></label>
    {#if error}<div class="err" role="alert">{error}</div>{/if}
    <div class="row end">
      <button class="btn" type="button" disabled={busy} onclick={onclose}>{t('common.cancel')}</button>
      <button class="btn danger" type="submit" disabled={!canDelete}>{busy ? t('delete.deleting') : t('delete.confirm')}</button>
    </div>
  </form>
</Modal>
