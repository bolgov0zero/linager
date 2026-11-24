<script lang="ts">
  import { serverApi2, type HostUser } from '../api';
  import { errorText } from '../errors';
  import { t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';

  let { serverId, user, onclose, ondone }: { serverId: string; user: HostUser; onclose: () => void; ondone: (msg: string) => void } = $props();
  let removeHome = $state(true);
  let typed = $state('');
  let busy = $state(false);
  let error = $state('');

  async function remove(): Promise<void> {
    if (busy || typed !== user.name) return;
    busy = true;
    error = '';
    try {
      await serverApi2.hostUserDelete(serverId, user.name, removeHome);
      ondone(t('usr.del.done', { user: user.name }));
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={t('usr.del.title', { user: user.name })} {onclose}>
  <p class="warnbox crit">{t('usr.del.irreversible')}</p>
  {#if user.sessions > 0 || user.procs > 0}<p class="warnbox warn">{t('usr.del.running', { s: user.sessions, p: user.procs })}</p>{/if}
  <label class="ck"><input type="checkbox" bind:checked={removeHome} /><span>{t('usr.del.home', { home: user.home })}<br /><span class="dim">{t('usr.del.homeHint')}</span></span></label>
  {#if user.keys > 0}<p class="dim">{t('usr.del.keys', { n: user.keys })}</p>{/if}
  <label class="fl"><span>{t('usr.del.type', { user: user.name })}</span><span class="field"><input bind:value={typed} autocomplete="off" spellcheck="false" /></span></label>
  {#if error}<div class="err" role="alert">{error}</div>{/if}
  {#snippet foot()}
    <span class="spacer"></span>
    <button class="btn" type="button" onclick={onclose}>{t('common.cancel')}</button>
    <button class="btn danger" type="button" disabled={busy || typed !== user.name} onclick={remove}>{t('usr.del.do')}</button>
  {/snippet}
</Modal>
