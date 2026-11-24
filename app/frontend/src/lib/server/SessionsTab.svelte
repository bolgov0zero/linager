<script lang="ts">
  import { onMount } from 'svelte';
  import { serverApi2, type SecState, type ServerDetail } from '../api';
  import { errorText } from '../errors';
  import Icon from '../Icon.svelte';
  import { t } from '../i18n.svelte';
  import SessionsList from './SessionsList.svelte';

  // Who is logged in over SSH now (the manager window): the list refreshes every few seconds.
  let { s }: { s: ServerDetail } = $props();
  let sessions = $state<SecState['sessions']>([]);
  let loaded = $state(false);
  let error = $state('');
  const sid = $derived(s.id);
  const online = $derived(s.online);

  async function load(): Promise<void> {
    try {
      sessions = (await serverApi2.security(sid)).sessions;
      error = '';
    } catch (e) {
      error = errorText(e);
    } finally {
      loaded = true;
    }
  }
  onMount(() => {
    void load();
    const i = setInterval(() => void load(), 5000);
    return () => clearInterval(i);
  });
</script>

{#if !loaded}<p class="dim">{t('dash.loading')}</p>{:else}
  <div class="sp-fill">
    <div class="sp-tools"><span class="dim">{t('sec.sessions')}: {sessions.length}</span><span class="grow"></span>
      <button class="btn sm icon" type="button" title={t('sp.refresh')} onclick={load}><Icon name="refresh-cw" size={14} /></button></div>
    {#if error}<div class="err">{error}</div>{/if}
    <SessionsList serverId={sid} {sessions} {online} onchanged={load} flush />
    <p class="dim">{t('sec.end.hint')}</p>
  </div>
{/if}
