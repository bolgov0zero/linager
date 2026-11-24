<script lang="ts">
  import { ApiError, serverApi2, type SecState } from '../api';
  import { confirmAsk } from '../dialogs.svelte';
  import { t } from '../i18n.svelte';

  // The remote (SSH) sessions with "End" for a session, or for all sessions of a user. Shared by the manager and the Security tab.
  let { serverId, sessions, online, onchanged, flush = false }: { serverId: string; sessions: SecState['sessions']; online: boolean; onchanged: () => Promise<void> | void; flush?: boolean } = $props();

  let note = $state<{ text: string; bad: boolean } | null>(null);
  let busy = $state('');
  let noteTimer: ReturnType<typeof setTimeout> | undefined;

  function say(text: string, bad = false): void {
    note = { text, bad };
    clearTimeout(noteTimer);
    noteTimer = setTimeout(() => (note = null), 5000);
  }

  /** Users with more than one session get a button for all of them. */
  const multi = $derived.by(() => {
    const counts = new Map<string, number>();
    for (const x of sessions) counts.set(x.user, (counts.get(x.user) ?? 0) + 1);
    return [...counts].filter(([, n]) => n > 1).map(([user, count]) => ({ user, count }));
  });

  async function end(target: { id: string; user: string; ip: string } | { user: string; count: number }): Promise<void> {
    const one = 'id' in target;
    const ok = await confirmAsk({
      title: t('sec.end'),
      text: `${one ? t('sec.end.confirm', { user: target.user, ip: target.ip || '—' }) : t('sec.end.confirmUser', { user: target.user, n: target.count })} ${t('sec.end.hint')}`,
      ok: t('sec.end'),
      danger: true,
    });
    if (!ok || busy) return;
    busy = one ? target.id : target.user;
    try {
      const res = await serverApi2.securityTerminate(serverId, one ? { session: target.id } : { user: target.user });
      say(res.terminated.length > 1 ? t('sec.end.doneN', { n: res.terminated.length }) : t('sec.end.done'));
    } catch (e) {
      say(e instanceof ApiError && e.code === 'no such remote session' ? t('sec.end.gone') : t('sec.end.failed'), true);
    } finally {
      busy = '';
      await onchanged();
    }
  }
  const when = (n: number): string => new Date(n * 1000).toLocaleString(undefined, { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' });
</script>

{#if multi.length > 0}
  <div class="row">{#each multi as m (m.user)}<button class="btn sm danger" type="button" disabled={!online || !!busy} onclick={() => end(m)}>{t('sec.endAll', { user: m.user, n: m.count })}</button>{/each}</div>
{/if}
{#if note}<div class="fm-note" class:bad={note.bad}><span>{note.text}</span></div>{/if}
<div class="panel flush" class:fill={flush}><table class="tbl compact fixed">
  <thead><tr><th style="width:24%">{t('sp.proc.user')}</th><th style="width:28%">IP</th><th>{t('mgr.since')}</th><th style="width:130px"></th></tr></thead>
  <tbody>
    {#each sessions as x (x.id)}
      <tr><td class="mono"><b>{x.user}</b></td><td class="mono dim">{x.ip || '—'}</td><td class="dim">{x.since ? when(x.since) : '—'}</td>
        <td class="r"><button class="btn sm danger" type="button" disabled={!online || !!busy} onclick={() => end(x)}>{t('sec.end')}</button></td></tr>
    {:else}
      <tr><td colspan="4" class="dim">{t('sec.sessions.none')}</td></tr>
    {/each}
  </tbody>
</table></div>
