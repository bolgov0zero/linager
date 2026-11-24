<script lang="ts">
  import { untrack } from 'svelte';
  import { serverApi2, type HostUser, type HostUserPatch, type HostUsersState } from '../api';
  import CustomSelect from '../CustomSelect.svelte';
  import { errorText } from '../errors';
  import { t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';
  import { HOT_GROUPS, lastAdmin } from '../users-util';

  let { serverId, user, info, onclose, ondone, ondelete }: {
    serverId: string; user: HostUser; info: HostUsersState; onclose: () => void; ondone: (msg: string) => void; ondelete: (u: HostUser) => void;
  } = $props();

  // The draft starts from the account as it is now; only what was changed is sent. Taken once, when the dialog
  // opens: the list is refreshed in the background and must not reset the draft.
  const start = untrack(() => ({ sudo: (user.sudo === 'root' ? 'none' : user.sudo) as 'none' | 'pw' | 'nopw', groups: info.groups.filter((g) => user.groups.includes(g)), shell: user.shell, locked: user.locked }));
  let sudo = $state(start.sudo);
  let groups = $state(start.groups);
  let shell = $state(start.shell);
  let locked = $state(start.locked);
  let busy = $state(false);
  let error = $state('');

  const readonlyAccount = $derived(!user.human);
  const wasAdmin = $derived(user.sudo !== 'none' && user.sudo !== 'root');
  const dropsAdmin = $derived((wasAdmin && sudo === 'none') || (wasAdmin && locked && !user.locked));
  const blocked = $derived(dropsAdmin && lastAdmin(info, user.name));
  const shellOptions = $derived([...new Set([user.shell, ...info.shells])].map((s) => ({ v: s, t: s })));

  const patch = $derived.by<HostUserPatch>(() => {
    const p: HostUserPatch = {};
    if (groups.length !== start.groups.length || groups.some((g) => !start.groups.includes(g))) p.groups = groups;
    if (sudo !== start.sudo) p.sudo = sudo;
    if (shell !== start.shell) p.shell = shell;
    if (locked !== start.locked) p.locked = locked;
    return p;
  });
  const changed = $derived(Object.keys(patch).length > 0);
  const toggle = (g: string): void => {
    groups = groups.includes(g) ? groups.filter((x) => x !== g) : [...groups, g];
  };

  async function apply(): Promise<void> {
    if (busy || !changed || blocked) return;
    busy = true;
    error = '';
    try {
      await serverApi2.hostUserUpdate(serverId, user.name, patch);
      ondone(t('usr.saved', { user: user.name }));
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
  const SUDO = [['none', 'usr.sudo.none', 'usr.sudo.noneHint'], ['pw', 'usr.sudo.pw', 'usr.sudo.pwHint'], ['nopw', 'usr.sudo.nopw', 'usr.sudo.nopwHint']] as const;
</script>

<Modal title={t('usr.props.title', { user: user.name })} {onclose}>
  <div class="sh">{t('usr.props.account')}</div>
  <dl class="kvs">
    <dt>UID / GID</dt><dd>{user.uid} / {user.gid}</dd>
    {#if user.gecos}<dt>{t('usr.props.gecos')}</dt><dd>{user.gecos}</dd>{/if}
    <dt>{t('usr.props.home')}</dt><dd class="mono">{user.home}</dd>
    <dt>{t('usr.props.password')}</dt>
    <dd>{user.has_password ? t('usr.props.pwSet') : t('usr.props.pwNone')}{#if user.has_password && user.pw_changed > 0}<span class="dim">{' · '}{t('usr.props.pwChanged', { date: new Date(user.pw_changed * 1000).toLocaleString() })}</span>{/if}</dd>
    <dt>{t('usr.props.keys')}</dt><dd>{user.keys}</dd>
    <dt>{t('usr.props.now')}</dt><dd>{user.sessions > 0 ? t('usr.props.sessions', { s: user.sessions, p: user.procs }) : t('usr.props.noSessions', { p: user.procs })}</dd>
  </dl>
  {#if readonlyAccount}
    <p class="dim">{t('usr.props.system')}</p>
  {:else}
    <div class="sh">{t('usr.props.sudo')}</div>
    <div class="opts">
      {#each SUDO as [v, label, hint] (v)}
        <button class="optc" type="button" aria-pressed={sudo === v} onclick={() => (sudo = v)}>{t(label)}<small class:crit={v === 'nopw'}>{t(hint)}</small></button>
      {/each}
    </div>
    {#if blocked}<p class="warnbox warn">{t('usr.props.lastAdmin')}</p>{/if}
    <div class="sh">{t('usr.props.groups')}</div>
    <div class="chk-grid">
      {#each info.groups as g (g)}<label class="ck"><input type="checkbox" checked={groups.includes(g)} onchange={() => toggle(g)} />{g}{#if HOT_GROUPS.includes(g)}<em class="hot">{t('usr.props.hot')}</em>{/if}</label>{/each}
    </div>
    <div class="sh">{t('usr.props.login')}</div>
    <div class="fl"><span>{t('usr.props.shell')}</span><CustomSelect value={shell} options={shellOptions} onchange={(v) => (shell = v)} /></div>
    <label class="ck"><input type="checkbox" bind:checked={locked} /><span>{t('usr.props.lock')}<br /><span class="dim">{t('usr.props.lockHint')}</span></span></label>
  {/if}
  <p class="dim">{t('usr.props.audit')}</p>
  {#if error}<div class="err" role="alert">{error}</div>{/if}
  {#snippet foot()}
    <button class="btn danger" type="button" disabled={user.protected || !user.human} onclick={() => ondelete(user)}>{t('usr.props.delete')}</button>
    <span class="spacer"></span>
    <button class="btn" type="button" onclick={onclose}>{t('common.cancel')}</button>
    <button class="btn primary" type="button" disabled={busy || !changed || blocked || readonlyAccount} onclick={apply}>{t('usr.props.apply')}</button>
  {/snippet}
</Modal>
