<script lang="ts">
  import { onMount } from 'svelte';
  import { serverApi2, type HostUser, type HostUsersState, type ServerDetail } from '../api';
  import ContextMenu, { type CtxItem } from '../ContextMenu.svelte';
  import { confirmAsk } from '../dialogs.svelte';
  import { errorText } from '../errors';
  import Icon from '../Icon.svelte';
  import SearchField from '../SearchField.svelte';
  import { t } from '../i18n.svelte';
  import { native } from '../native';
  import { filterUsers, HOT_GROUPS, lastAdmin, sudoTone } from '../users-util';
  import UserAddModal from './UserAddModal.svelte';
  import UserDeleteModal from './UserDeleteModal.svelte';
  import UserModal from './UserModal.svelte';
  import UserPasswordModal from './UserPasswordModal.svelte';

  let { s, nativeWindows = false }: { s: ServerDetail; nativeWindows?: boolean } = $props();

  let data = $state<HostUsersState | null>(null);
  let error = $state('');
  let note = $state<{ text: string; bad: boolean } | null>(null);
  let noteTimer: ReturnType<typeof setTimeout> | undefined;

  const sid = $derived(s.id);
  const online = $derived(s.online);

  async function load(): Promise<void> {
    try {
      data = await serverApi2.hostUsers(sid);
      error = '';
    } catch (e) {
      error = errorText(e);
    }
  }
  onMount(() => {
    void load();
    const i = setInterval(() => void load(), 30_000);
    return () => clearInterval(i);
  });

  function say(text: string, bad = false): void {
    note = { text, bad };
    clearTimeout(noteTimer);
    noteTimer = setTimeout(() => (note = null), bad ? 8000 : 4000);
  }

  let fHuman = $state(false);
  let fSystem = $state(false);
  let fSudo = $state(false);
  let fOnline = $state(false);
  let q = $state('');
  const rows = $derived(data ? filterUsers(data.users, { human: fHuman, system: fSystem, sudo: fSudo, online: fOnline, q }) : []);
  const sums = $derived.by(() => {
    const us = data?.users ?? [];
    return { human: us.filter((u) => u.human).length, sudo: us.filter((u) => u.sudo !== 'none').length, sessions: us.reduce((a, u) => a + u.sessions, 0), locked: us.filter((u) => u.human && u.locked).length };
  });

  let editing = $state<HostUser | null>(null);
  let adding = $state(false);
  let deleting = $state<HostUser | null>(null);
  let password = $state<HostUser | null>(null);
  let menu = $state<{ x: number; y: number; items: CtxItem[] } | null>(null);

  const done = (msg: string): void => {
    editing = null;
    adding = false;
    deleting = null;
    password = null;
    say(msg);
    void load();
  };

  async function lockOrKill(kind: 'lock' | 'unlock' | 'kill', u: HostUser): Promise<void> {
    const ok = await confirmAsk({ title: t(`usr.confirm.${kind}.title`, { user: u.name }), text: `${t(`usr.confirm.${kind}.text`, { user: u.name })} ${t(`usr.confirm.${kind}.hint`)}`, ok: t(`usr.confirm.${kind}.do`), danger: kind !== 'unlock' });
    if (!ok) return;
    try {
      if (kind === 'kill') await serverApi2.hostUserKill(sid, u.name);
      else await serverApi2.hostUserUpdate(sid, u.name, { locked: kind === 'lock' });
      say(t(`usr.confirm.${kind}.done`, { user: u.name }));
      void load();
    } catch (e) {
      say(errorText(e), true);
    }
  }

  function itemsFor(u: HostUser): CtxItem[] {
    const system = !u.human;
    const only = data ? lastAdmin(data, u.name) && u.sudo !== 'none' : false;
    return [
      { key: 'props', label: t('usr.menu.props'), icon: 'info', onselect: () => (editing = u) },
      { key: 'lock', label: u.locked ? t('usr.menu.unlock') : t('usr.menu.lock'), icon: u.locked ? 'lock-open' : 'lock', disabled: system || (!u.locked && only), onselect: () => void lockOrKill(u.locked ? 'unlock' : 'lock', u) },
      { key: 'pw', label: t('usr.menu.password'), icon: 'key-round', disabled: system, onselect: () => (password = u) },
      { key: 'kill', label: t('usr.menu.kill'), icon: 'x', disabled: system || u.sessions === 0, onselect: () => void lockOrKill('kill', u) },
      { key: 'del', label: t('usr.menu.delete'), icon: 'trash-2', danger: true, separated: true, disabled: !u.human || u.protected, onselect: () => (deleting = u) },
    ];
  }
  function openMenu(u: HostUser, ev: MouseEvent): void {
    ev.preventDefault();
    ev.stopPropagation();
    const r = (ev.currentTarget as HTMLElement).getBoundingClientRect();
    menu = { x: ev.type === 'contextmenu' ? ev.clientX : r.left, y: ev.type === 'contextmenu' ? ev.clientY : r.bottom + 2, items: itemsFor(u) };
  }
  const terminal = (u: HostUser): void => void native.openWindow('terminal', sid, s.name, nativeWindows, u.name);
  const when = (n: number): string => new Date(n * 1000).toLocaleString(undefined, { day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' });
</script>

{#if error && !data}<div class="err">{error}</div>{:else if !data}<p class="dim">{t('dash.loading')}</p>{:else}
  <div class="sp-fill">
    <div class="sums">
      <div class="sum"><b>{sums.human}</b><span>{t('usr.sum.human')}</span></div>
      <div class="sum"><b>{sums.sudo}</b><span>{t('usr.sum.sudo')}</span></div>
      <div class="sum"><b>{sums.sessions}</b><span>{t('usr.sum.sessions')}</span></div>
      <div class="sum"><b>{sums.locked}</b><span>{t('usr.sum.locked')}</span></div>
    </div>
    <div class="sp-tools">
      <SearchField placeholder={t('usr.search')} bind:value={q} />
      <button class="chip" type="button" aria-pressed={fHuman} onclick={() => (fHuman = !fHuman)}>{t('usr.f.human')}</button>
      <button class="chip" type="button" aria-pressed={fSystem} onclick={() => (fSystem = !fSystem)}>{t('usr.f.system')}</button>
      <button class="chip" type="button" aria-pressed={fSudo} onclick={() => (fSudo = !fSudo)}>{t('usr.f.sudo')}</button>
      <button class="chip" type="button" aria-pressed={fOnline} onclick={() => (fOnline = !fOnline)}>{t('usr.f.online')}</button>
      <span class="grow"></span>
      <button class="btn sm primary" type="button" disabled={!online} onclick={() => (adding = true)}><Icon name="user-plus" size={14} />{t('usr.add')}</button>
    </div>
    {#if note}<div class="fm-note" class:bad={note.bad}><span>{note.text}</span></div>{/if}
    <div class="panel flush fill"><table class="tbl compact fixed">
      <thead><tr>
        <th style="width:20%">{t('usr.col.user')}</th><th style="width:60px">UID</th><th style="width:130px">{t('usr.col.rights')}</th><th>{t('usr.col.groups')}</th>
        <th style="width:90px">{t('usr.col.shell')}</th><th style="width:175px">{t('usr.col.last')}</th><th style="width:118px">{t('usr.col.now')}</th><th style="width:92px"></th>
      </tr></thead>
      <tbody>
        {#each rows as u (u.name)}
          <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
          <tr class="fr" onclick={() => (editing = u)} oncontextmenu={(e) => openMenu(u, e)}>
            <td><b>{u.name}</b>{#if u.locked && u.human} <span class="badge b-warn">{t('usr.badge.locked')}</span>{/if}{#if u.protected} <span class="badge b-mute">{t('usr.badge.protected')}</span>{/if}
              <div class="dim sub">{u.gecos || (u.human ? '' : t('usr.system'))}</div></td>
            <td class="mono dim">{u.uid}</td>
            <td>{#if u.sudo === 'none'}<span class="dim">—</span>{:else}<span class="badge b-{sudoTone(u.sudo) === 'muted' ? 'mute' : sudoTone(u.sudo)}">{t(`usr.sudo.badge.${u.sudo}`)}</span>{/if}</td>
            <td class="grp">{#each u.groups as g (g)}<span class="gtag" class:hot={HOT_GROUPS.includes(g)}>{g}</span>{:else}<span class="dim">—</span>{/each}</td>
            <td class="mono dim" title={u.shell}>{u.shell.split('/').pop()}</td>
            <td>{#if u.last_login > 0}{when(u.last_login)}{#if u.last_from}<div class="dim sub">{t('usr.from', { ip: u.last_from })}</div>{/if}{:else}<span class="dim">{t('usr.never')}</span>{/if}</td>
            <td>{#if u.sessions > 0}<span class="ok-t"><span class="dot ok"></span> {t('usr.sessionsN', { n: u.sessions })}</span>{:else}<span class="dim">—</span>{/if}</td>
            <td class="r"><span class="r-btns">
              <button class="btn icon sm" type="button" disabled={u.no_shell || !online} title={u.no_shell ? t('usr.noShell') : t('usr.terminal')} onclick={(e) => { e.stopPropagation(); terminal(u); }}><Icon name="square-terminal" size={14} /></button>
              <button class="btn icon sm" type="button" title={t('usr.menu')} aria-label={t('usr.menu')} onclick={(e) => openMenu(u, e)}><Icon name="ellipsis" size={14} /></button>
            </span></td>
          </tr>
        {:else}
          <tr><td colspan="8" class="dim">{t('usr.empty')}</td></tr>
        {/each}
      </tbody>
    </table></div>
    <p class="dim">{t('usr.legend', { shown: rows.length, total: data.users.length })}</p>
  </div>
  {#if menu}<ContextMenu x={menu.x} y={menu.y} items={menu.items} onclose={() => (menu = null)} />{/if}
  {#if editing}<UserModal serverId={sid} user={editing} info={data} onclose={() => (editing = null)} ondone={done} ondelete={(u) => { editing = null; deleting = u; }} />{/if}
  {#if adding}<UserAddModal serverId={sid} info={data} onclose={() => (adding = false)} ondone={done} />{/if}
  {#if deleting}<UserDeleteModal serverId={sid} user={deleting} onclose={() => (deleting = null)} ondone={done} />{/if}
  {#if password}<UserPasswordModal serverId={sid} user={password} onclose={() => (password = null)} ondone={done} />{/if}
{/if}
