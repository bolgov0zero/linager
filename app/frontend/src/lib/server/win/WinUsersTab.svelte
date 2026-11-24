<script lang="ts">
  import { onMount } from 'svelte';
  import { winCall, type ServerDetail, type WinUser, type WinUsersState } from '../../api';
  import ContextMenu, { type CtxItem } from '../../ContextMenu.svelte';
  import { confirmAsk } from '../../dialogs.svelte';
  import { errorText } from '../../errors';
  import Icon from '../../Icon.svelte';
  import { t } from '../../i18n.svelte';
  import Modal from '../../Modal.svelte';
  import SearchField from '../../SearchField.svelte';

  let { s }: { s: ServerDetail } = $props();
  const sid = $derived(s.id);

  let data = $state<WinUsersState | null>(null);
  let error = $state('');
  let busy = $state(false);
  let note = $state<{ text: string; bad: boolean } | null>(null);
  let noteTimer: ReturnType<typeof setTimeout> | undefined;
  type Dialog = { kind: 'add' } | { kind: 'password'; user: WinUser } | { kind: 'groups'; user: WinUser };
  let dialog = $state<Dialog | null>(null);
  let menu = $state<{ x: number; y: number; items: CtxItem[] } | null>(null);
  let fName = $state('');
  let fFull = $state('');
  let fPass = $state('');
  let fGroups = $state<string[]>([]);
  let q = $state('');
  let fEnabled = $state(false);
  let fAdmins = $state(false);
  let fOnline = $state(false);
  const HOT = ['Administrators', 'Администраторы'];

  function say(text: string, bad = false): void {
    note = { text, bad };
    clearTimeout(noteTimer);
    noteTimer = setTimeout(() => (note = null), bad ? 8000 : 4000);
  }
  async function load(): Promise<void> {
    try {
      data = await winCall<WinUsersState>(sid, 'users', 'state');
      error = '';
    } catch (e) {
      error = errorText(e);
    }
  }
  onMount(() => void load());

  /** Runs an action (the confirmation the person gave goes with it). */
  async function act(action: string, params: Record<string, unknown>): Promise<void> {
    busy = true;
    try {
      await winCall(sid, 'users', action, params, true);
      dialog = null;
      await load();
    } catch (e) {
      say(errorText(e), true);
    } finally {
      busy = false;
    }
  }
  async function ask(text: string, danger: boolean, run: () => Promise<void>): Promise<void> {
    if (await confirmAsk({ title: t('win.confirm'), text, ok: t('win.confirm'), danger })) await run();
  }
  function openAdd(): void {
    fName = fFull = fPass = '';
    fGroups = data?.groups.includes('Users') ? ['Users'] : [];
    dialog = { kind: 'add' };
  }
  const toggleGroup = (g: string): void => {
    fGroups = fGroups.includes(g) ? fGroups.filter((x) => x !== g) : [...fGroups, g];
  };
  const sessionsOf = (name: string): number => (data?.sessions ?? []).filter((x) => x.user.toLowerCase() === name.toLowerCase()).length;
  const rows = $derived((data?.users ?? []).filter((u) => {
    if (q.trim() && !`${u.name} ${u.full_name}`.toLowerCase().includes(q.trim().toLowerCase())) return false;
    if (fEnabled && !u.enabled) return false;
    if (fAdmins && !u.groups.some((g) => HOT.includes(g))) return false;
    if (fOnline && sessionsOf(u.name) === 0) return false;
    return true;
  }));
  const when = (n: number): string => (n > 0 ? new Date(n * 1000).toLocaleString() : t('wu.never'));

  function itemsFor(u: WinUser): CtxItem[] {
    return [
      u.enabled
        ? { key: 'off', label: t('wu.disable'), icon: 'lock', disabled: busy || u.builtin, onselect: () => void ask(t('wu.confirm.disable', { name: u.name }), false, () => act('update', { name: u.name, enabled: false })) }
        : { key: 'on', label: t('wu.enable'), icon: 'lock-open', disabled: busy, onselect: () => void act('update', { name: u.name, enabled: true }) },
      { key: 'pw', label: t('wu.setPassword'), icon: 'key-round', disabled: busy, onselect: () => { fPass = ''; dialog = { kind: 'password', user: u }; } },
      { key: 'groups', label: t('wu.editGroups'), icon: 'users', disabled: busy, onselect: () => { fGroups = [...u.groups]; dialog = { kind: 'groups', user: u }; } },
      { key: 'del', label: t('wu.delete'), icon: 'trash-2', danger: true, separated: true, disabled: busy || u.builtin, onselect: () => void ask(t('wu.confirm.delete', { name: u.name }), true, () => act('delete', { name: u.name })) },
    ];
  }
  function openMenu(u: WinUser, ev: MouseEvent): void {
    ev.preventDefault();
    ev.stopPropagation();
    const r = (ev.currentTarget as HTMLElement).getBoundingClientRect();
    menu = { x: ev.type === 'contextmenu' ? ev.clientX : r.left, y: ev.type === 'contextmenu' ? ev.clientY : r.bottom + 2, items: itemsFor(u) };
  }
</script>

{#if error && !data}<div class="err">{error}</div>{:else if !data}<p class="dim">{t('win.loading')}</p>{:else}
  <div class="win-page">
    <div class="sp-tools">
      <SearchField placeholder={t('wu.search')} bind:value={q} />
      <button class="chip" type="button" aria-pressed={fEnabled} onclick={() => (fEnabled = !fEnabled)}>{t('wu.f.enabled')}</button>
      <button class="chip" type="button" aria-pressed={fAdmins} onclick={() => (fAdmins = !fAdmins)}>{t('wu.f.admins')}</button>
      <button class="chip" type="button" aria-pressed={fOnline} onclick={() => (fOnline = !fOnline)}>{t('wu.f.online')}</button>
      <span class="grow"></span>
      <button class="btn sm icon" type="button" title={t('win.refresh')} onclick={load}><Icon name="refresh-cw" size={14} /></button>
      <button class="btn sm primary" type="button" disabled={!s.online} onclick={openAdd}><Icon name="user-plus" size={14} />{t('wu.add')}</button>
    </div>
    {#if note}<div class="fm-note" class:bad={note.bad}><span>{note.text}</span></div>{/if}
    <div class="tw" style="max-height:none"><table class="tbl compact">
      <thead><tr><th>{t('wu.name')}</th><th>{t('wu.state')}</th><th>{t('wu.groups')}</th><th>{t('wu.lastLogon')}</th><th>{t('wu.now')}</th><th></th></tr></thead>
      <tbody>
        {#each rows as u (u.sid)}
          <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
          <tr oncontextmenu={(e) => openMenu(u, e)}>
            <td><b>{u.name}</b>{#if u.builtin} <span class="badge b-mute">{t('wu.builtin')}</span>{/if}<div class="dim sub">{u.full_name || u.description}</div></td>
            <td><span class="badge {u.enabled ? 'b-ok' : 'b-mute'}">{u.enabled ? t('wu.enabled') : t('wu.disabled')}</span></td>
            <td class="grp">{#each u.groups as g (g)}<span class="gtag" class:hot={HOT.includes(g)}>{g}</span>{:else}<span class="dim">—</span>{/each}</td>
            <td class="dim">{u.last_logon > 0 ? when(u.last_logon) : t('wu.never')}</td>
            <td>{#if sessionsOf(u.name) > 0}<span class="ok-t"><span class="dot ok"></span> {t('wu.sessionsN', { n: sessionsOf(u.name) })}</span>{:else}<span class="dim">—</span>{/if}</td>
            <td class="r"><button class="btn icon sm" type="button" title={t('wu.menu')} aria-label={t('wu.menu')} onclick={(e) => openMenu(u, e)}><Icon name="ellipsis" size={14} /></button></td>
          </tr>
        {:else}<tr><td colspan="6" class="dim">{t('wu.empty')}</td></tr>{/each}
      </tbody>
    </table></div>
    <p class="dim">{t('wu.legend', { shown: rows.length, total: data.users.length })}</p>
    <h4 class="ph">{t('wu.sessions')}</h4>
    {#if data.sessions.length === 0}<p class="dim">{t('wu.noSessions')}</p>{:else}
      <div class="tw" style="max-height:none"><table class="tbl compact">
        <thead><tr><th>{t('wu.name')}</th><th>{t('wu.sessionType')}</th><th>{t('wu.state')}</th><th>ID</th><th></th></tr></thead>
        <tbody>{#each data.sessions as x (x.id)}
          <tr><td><b>{x.user}</b></td><td>{t(`wu.type.${x.type}`)}</td><td><span class="badge {x.state === 'active' ? 'b-ok' : 'b-mute'}">{x.state}</span></td><td class="mono">#{x.id}</td>
            <td class="r"><button class="btn sm danger" type="button" disabled={busy} onclick={() => ask(t('wu.confirm.logoff', { name: x.user }), true, () => act('logoff', { id: x.id }))}>{t('wu.logoff')}</button></td></tr>
        {/each}</tbody>
      </table></div>
    {/if}
  </div>
  {#if menu}<ContextMenu x={menu.x} y={menu.y} items={menu.items} onclose={() => (menu = null)} />{/if}
{/if}

{#if dialog?.kind === 'add'}
  <Modal title={t('wu.addTitle')} onclose={() => (dialog = null)}>
    <label class="fl"><span>{t('wu.name')}</span><span class="field"><input bind:value={fName} maxlength="20" autocomplete="off" spellcheck="false" /></span></label>
    <label class="fl"><span>{t('wu.fullName')}</span><span class="field"><input bind:value={fFull} autocomplete="off" /></span></label>
    <label class="fl"><span>{t('wu.password')}</span><span class="field"><input type="password" bind:value={fPass} autocomplete="new-password" /></span></label>
    <p class="dim">{t('wu.pwHint')}</p>
    <div class="fl"><span>{t('wu.groups')}</span><div class="chk-grid">{#each data?.groups ?? [] as g (g)}<label class="ck"><input type="checkbox" checked={fGroups.includes(g)} onchange={() => toggleGroup(g)} />{g}</label>{/each}</div></div>
    {#snippet foot()}<span class="spacer"></span><button class="btn" type="button" onclick={() => (dialog = null)}>{t('win.cancel')}</button>
      <button class="btn primary" type="button" disabled={busy || !fName.trim() || !fPass} onclick={() => act('add', { name: fName.trim(), full_name: fFull, password: fPass, groups: fGroups })}>{t('wu.add')}</button>{/snippet}
  </Modal>
{:else if dialog?.kind === 'password'}
  {@const user = dialog.user}
  <Modal title={user.name} onclose={() => (dialog = null)}>
    <label class="fl"><span>{t('wu.newPassword')}</span><span class="field"><input type="password" bind:value={fPass} autocomplete="new-password" /></span></label>
    {#snippet foot()}<span class="spacer"></span><button class="btn" type="button" onclick={() => (dialog = null)}>{t('win.cancel')}</button>
      <button class="btn primary" type="button" disabled={busy || !fPass} onclick={() => act('passwd', { name: user.name, password: fPass })}>{t('win.save')}</button>{/snippet}
  </Modal>
{:else if dialog?.kind === 'groups'}
  {@const user = dialog.user}
  <Modal title={t('wu.groupsTitle', { name: user.name })} onclose={() => (dialog = null)}>
    <div class="chk-grid">{#each data?.groups ?? [] as g (g)}<label class="ck"><input type="checkbox" checked={fGroups.includes(g)} onchange={() => toggleGroup(g)} />{g}</label>{/each}</div>
    {#snippet foot()}<span class="spacer"></span><button class="btn" type="button" onclick={() => (dialog = null)}>{t('win.cancel')}</button>
      <button class="btn primary" type="button" disabled={busy} onclick={() => act('update', { name: user.name, groups: fGroups })}>{t('win.save')}</button>{/snippet}
  </Modal>
{/if}
