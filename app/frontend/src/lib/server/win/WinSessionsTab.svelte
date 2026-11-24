<script lang="ts">
  import { onMount } from 'svelte';
  import { winCall, type ServerDetail, type WinSession, type WinUsersState } from '../../api';
  import { confirmAsk } from '../../dialogs.svelte';
  import { errorText } from '../../errors';
  import Icon from '../../Icon.svelte';
  import { t } from '../../i18n.svelte';

  // Who is signed in to a Windows computer now (the manager window): the sessions of the console and of the remote desktop, with
  // "Sign out". The SSH list of SessionsTab is for Linux: a Windows agent has no module for it.
  let { s }: { s: ServerDetail } = $props();
  const sid = $derived(s.id);
  let sessions = $state<WinSession[]>([]);
  let loaded = $state(false);
  let error = $state('');
  let busy = $state(false);

  async function load(): Promise<void> {
    try {
      sessions = (await winCall<WinUsersState>(sid, 'users', 'state')).sessions;
      error = '';
    } catch (e) {
      error = errorText(e);
    } finally {
      loaded = true;
    }
  }
  async function logoff(x: WinSession): Promise<void> {
    if (!(await confirmAsk({ title: t('win.confirm'), text: t('wu.confirm.logoff', { name: x.user }), ok: t('wu.logoff'), danger: true }))) return;
    busy = true;
    try {
      await winCall(sid, 'users', 'logoff', { id: x.id }, true);
      error = '';
      await load();
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
  onMount(() => {
    void load();
    const i = setInterval(() => void load(), 10000);
    return () => clearInterval(i);
  });
</script>

{#if !loaded}<p class="dim">{t('dash.loading')}</p>{:else}
  <div class="sp-fill">
    <div class="sp-tools"><span class="dim">{t('sec.sessions')}: {sessions.length}</span><span class="grow"></span>
      <button class="btn sm icon" type="button" title={t('sp.refresh')} aria-label={t('sp.refresh')} onclick={load}><Icon name="refresh-cw" size={14} /></button></div>
    {#if error}<div class="err">{error}</div>{/if}
    <div class="tw" style="max-height:none"><table class="tbl compact">
      <thead><tr><th>{t('wu.name')}</th><th>{t('wu.sessionType')}</th><th>{t('wu.state')}</th><th>ID</th><th></th></tr></thead>
      <tbody>
        {#each sessions as x (x.id)}
          <tr><td><b>{x.user}</b></td><td>{t(`wu.type.${x.type}`)}</td><td><span class="badge {x.state === 'active' ? 'b-ok' : 'b-mute'}">{x.state}</span></td><td class="mono">#{x.id}</td>
            <td class="r"><button class="btn sm danger" type="button" disabled={busy || !s.online} onclick={() => void logoff(x)}>{t('wu.logoff')}</button></td></tr>
        {:else}<tr><td colspan="5" class="dim">{t('wu.noSessions')}</td></tr>{/each}
      </tbody>
    </table></div>
    <p class="dim">{t('mgr.win.hint')}</p>
  </div>
{/if}
