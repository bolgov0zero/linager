<script lang="ts">
  import { untrack } from 'svelte';
  import { serverApi2, type F2bBan, type F2bEvent } from '../api';
  import { confirmAsk } from '../dialogs.svelte';
  import { errorText } from '../errors';
  import { t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';
  import SearchField from '../SearchField.svelte';

  let { serverId, bans, onchanged, onclose }: { serverId: string; bans: F2bBan[]; onchanged: () => Promise<void>; onclose: () => void } = $props();

  let tab = $state<'now' | 'hist'>('now');
  let search = $state('');
  let selected = $state<string[]>([]);
  let busy = $state(false);
  let error = $state('');
  let notice = $state('');
  let events = $state<F2bEvent[]>([]);
  let total = $state(0);
  let logPath = $state('');
  let histLoaded = $state(false);
  let histError = $state(false);

  const q = $derived(search.trim());
  const shownBans = $derived(q ? bans.filter((b) => b.ip.includes(q)) : bans);
  const shownEvents = $derived(q ? events.filter((e) => e.ip.includes(q)) : events);
  const bannedSet = $derived(new Set(bans.map((b) => b.ip)));
  const allSelected = $derived(shownBans.length > 0 && shownBans.every((b) => selected.includes(b.ip)));
  const when = (n: number): string => new Date(n * 1000).toLocaleString(undefined, { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' });

  async function loadHistory(): Promise<void> {
    try {
      const h = await serverApi2.securityBans(serverId, 200);
      events = h.events;
      total = h.total;
      logPath = h.log;
      histError = !h.available;
    } catch {
      histError = true;
    } finally {
      histLoaded = true;
    }
  }
  $effect(() => {
    if (tab === 'hist' && !histLoaded) untrack(() => void loadHistory());
  });

  const toggle = (ip: string): void => {
    selected = selected.includes(ip) ? selected.filter((x) => x !== ip) : [...selected, ip];
  };
  const toggleAll = (): void => {
    selected = allSelected ? [] : shownBans.map((b) => b.ip);
  };

  async function unban(target: { ips: string[] } | { all: true }, doneKey: string, n: number): Promise<void> {
    if (busy) return;
    if ('all' in target && !(await confirmAsk({ title: t('sec.f2b.unbanAll'), text: t('sec.f2b.unbanAllText', { n }), ok: t('sec.f2b.unbanAll'), danger: true }))) return;
    busy = true;
    error = '';
    try {
      await serverApi2.securityUnban(serverId, target);
      notice = t(doneKey, { n });
      selected = [];
      histLoaded = false;
      await onchanged();
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
  async function banAgain(ip: string): Promise<void> {
    if (busy) return;
    busy = true;
    error = '';
    try {
      await serverApi2.securityBan(serverId, ip, 0);
      notice = t('sec.f2b.banned.done', { ip });
      histLoaded = false;
      await onchanged();
      if (tab === 'hist') await loadHistory();
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
  /** When the ban of the address ends, as a relative text. */
  function until(b: F2bBan): string {
    if (!b.unban_at) return t('sec.f2b.bans.never');
    const left = b.unban_at - Math.floor(Date.now() / 1000);
    if (left <= 0) return t('sec.f2b.bans.soon');
    if (left < 3600) return t('sec.f2b.bans.inMin', { n: Math.max(1, Math.round(left / 60)) });
    if (left < 86400) return t('sec.f2b.bans.inH', { n: Math.round(left / 3600) });
    return t('sec.f2b.bans.inD', { n: Math.round(left / 86400) });
  }
</script>

<Modal title={t('sec.f2b.bansTitle')} wide {onclose}>
  <div class="row">
    <button class="chip" type="button" aria-pressed={tab === 'now'} onclick={() => (tab = 'now')}>{t('sec.f2b.bans.now')}<em>{bans.length}</em></button>
    <button class="chip" type="button" aria-pressed={tab === 'hist'} onclick={() => (tab = 'hist')}>{t('sec.f2b.bans.hist')}{#if histLoaded}<em>{total}</em>{/if}</button>
    <span class="grow"></span>
    <SearchField placeholder={t('sec.f2b.bans.search')} bind:value={search} spellcheck={false} />
  </div>
  {#if tab === 'now'}
    {#if shownBans.length === 0}
      <p class="dim">{bans.length === 0 ? t('sec.f2b.bans.none') : t('sec.f2b.bans.nothing')}</p>
    {:else}
      <div class="tw"><table class="tbl compact">
        <thead><tr><th class="ck-c"><input type="checkbox" aria-label={t('sec.f2b.bans.all')} checked={allSelected} onchange={toggleAll} /></th><th>{t('sec.col.ip')}</th><th>{t('sec.f2b.bans.jail')}</th><th>{t('sec.f2b.bans.since')}</th><th>{t('sec.f2b.bans.until')}</th><th class="r">{t('sec.col.attempts')}</th><th></th></tr></thead>
        <tbody>
          {#each shownBans as b (b.jail + b.ip)}
            <tr><td class="ck-c"><input type="checkbox" aria-label={b.ip} checked={selected.includes(b.ip)} onchange={() => toggle(b.ip)} /></td>
              <td class="mono">{b.ip}</td><td>{b.jail}</td><td class="dim">{b.banned_at ? when(b.banned_at) : '—'}</td><td>{until(b)}</td><td class="mono r">{b.attempts || '—'}</td>
              <td class="r"><button class="btn sm" type="button" disabled={busy} onclick={() => unban({ ips: [b.ip] }, 'sec.f2b.unbanned', 1)}>{t('sec.f2b.unban')}</button></td></tr>
          {/each}
        </tbody>
      </table></div>
      <div class="row">
        <span class="dim">{t('sec.f2b.bans.selected', { n: selected.length })}</span><span class="grow"></span>
        <button class="btn sm" type="button" disabled={busy || selected.length === 0} onclick={() => unban({ ips: selected }, 'sec.f2b.unbannedN', selected.length)}>{t('sec.f2b.unbanSelected')}</button>
        <button class="btn sm danger" type="button" disabled={busy || bans.length === 0} onclick={() => unban({ all: true }, 'sec.f2b.unbannedN', bans.length)}>{t('sec.f2b.unbanAll')}</button>
      </div>
    {/if}
  {:else if !histLoaded}
    <p class="dim">{t('dash.loading')}</p>
  {:else if histError}
    <p class="dim">{t('sec.f2b.hist.error', { path: logPath || '/var/log/fail2ban.log' })}</p>
  {:else if shownEvents.length === 0}
    <p class="dim">{t('sec.f2b.hist.none')}</p>
  {:else}
    <div class="tw"><table class="tbl compact">
      <thead><tr><th>{t('sec.col.time')}</th><th>{t('sec.f2b.hist.event')}</th><th>{t('sec.col.ip')}</th><th>{t('sec.f2b.bans.jail')}</th><th></th></tr></thead>
      <tbody>
        {#each shownEvents as e, i (i)}
          <tr><td class="dim">{when(e.time)}</td><td><span class="badge {e.event === 'ban' ? 'b-crit' : 'b-ok'}">{t(e.event === 'ban' ? 'sec.f2b.hist.ban' : 'sec.f2b.hist.unban')}</span></td>
            <td class="mono">{e.ip}</td><td>{e.jail}</td>
            <td class="r">{#if e.event === 'ban'}<span class="dim">{t(bannedSet.has(e.ip) ? 'sec.f2b.hist.active' : 'sec.f2b.hist.over')}</span>{:else if !bannedSet.has(e.ip)}<button class="btn sm" type="button" disabled={busy} onclick={() => banAgain(e.ip)}>{t('sec.f2b.hist.again')}</button>{/if}</td></tr>
        {/each}
      </tbody>
    </table></div>
    <p class="dim">{t('sec.f2b.hist.shown', { n: shownEvents.length, total, path: logPath })}</p>
  {/if}
  {#if error}<div class="err" role="alert">{error}</div>{:else if notice}<div class="ok-t">{notice}</div>{/if}
  {#snippet foot()}
    <span class="dim">{t('sec.f2b.bans.audit')}</span><span class="spacer"></span>
    <button class="btn" type="button" onclick={onclose}>{t('common.close')}</button>
  {/snippet}
</Modal>
