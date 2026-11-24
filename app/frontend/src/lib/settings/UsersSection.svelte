<script lang="ts">
  import { onMount } from 'svelte';
  import { api, ApiError, type Role, type UserAccess, type UserRow } from '../api';
  import { confirmAsk } from '../dialogs.svelte';
  import { isUnauthorized } from '../errors';
  import { i18n, t } from '../i18n.svelte';
  import { showToast } from '../toast';
  import AccessEditor from './AccessEditor.svelte';

  let { onauthlost }: { onauthlost: () => void } = $props();

  const empty = (): UserAccess => ({ servers: [], groups: [], tags: [], modules: [] });

  let users = $state<UserRow[]>([]);
  let loaded = $state(false);
  let failed = $state(false);

  let adding = $state(false);
  let newLogin = $state('');
  let newPassword = $state('');
  let newRole = $state<Role>('admin');
  let newAccess = $state<UserAccess>(empty());
  let addError = $state('');

  /** Changing the role and the access of an existing user. */
  let editing = $state<{ user: UserRow; role: Role; access: UserAccess } | null>(null);
  let pwFor = $state<UserRow | null>(null);
  let resetPw = $state('');
  let actionError = $state('');
  let busy = $state(false);

  async function load(): Promise<void> {
    try {
      users = (await api.users()).users;
      failed = false;
    } catch (e) {
      if (isUnauthorized(e)) onauthlost();
      else failed = true;
    } finally {
      loaded = true;
    }
  }
  onMount(() => void load());

  function errText(e: unknown): string {
    if (e instanceof ApiError) {
      if (e.status === 0) return t('st.common.networkError');
      if (e.status === 409) return e.code === 'this login is taken' ? t('st.users.loginTaken') : (e.code ?? t('common.genericError'));
      if (e.status === 422) return e.code ?? t('common.genericError');
    }
    return t('common.genericError');
  }
  const when = (iso: string): string => new Date(iso).toLocaleString(i18n.locale);

  async function create(): Promise<void> {
    if (busy) return;
    busy = true;
    addError = '';
    try {
      await api.userCreate(newLogin.trim(), newPassword, newRole, newAccess);
      newLogin = newPassword = '';
      newRole = 'admin';
      newAccess = empty();
      adding = false;
      showToast(t('st.users.createdToast'));
      await load();
    } catch (e) {
      if (isUnauthorized(e)) onauthlost();
      else addError = errText(e);
    } finally {
      busy = false;
    }
  }

  function startEdit(user: UserRow): void {
    pwFor = null;
    editing = { user, role: (user.role as Role) ?? 'admin', access: user.access ? { servers: [...user.access.servers], groups: [...user.access.groups], tags: [...(user.access.tags ?? [])], modules: [...user.access.modules] } : empty() };
    actionError = '';
  }
  async function saveEdit(): Promise<void> {
    const e = editing;
    if (!e || busy) return;
    busy = true;
    actionError = '';
    try {
      await api.userAccess(e.user.id, e.role, e.access);
      showToast(t('st.users.accessSaved'));
      editing = null;
      await load();
    } catch (err) {
      if (isUnauthorized(err)) onauthlost();
      else actionError = errText(err);
    } finally {
      busy = false;
    }
  }

  function summary(u: UserRow): string {
    if (!u.access) return '';
    const n = u.access.servers.length + u.access.groups.length + (u.access.tags?.length ?? 0);
    return u.role === 'operator' ? t('st.users.accessSummaryOp', { n, m: u.access.modules.length }) : t('st.users.accessSummaryAud', { n });
  }

  async function run(action: () => Promise<unknown>, done: string): Promise<void> {
    busy = true;
    actionError = '';
    try {
      await action();
      showToast(done);
      await load();
    } catch (e) {
      if (isUnauthorized(e)) onauthlost();
      else showToast(errText(e));
    } finally {
      busy = false;
    }
  }
  async function remove(u: UserRow): Promise<void> {
    if (!(await confirmAsk({ title: t('st.users.delete'), text: t('st.users.confirmDelete', { login: u.login }), ok: t('st.users.delete'), danger: true }))) return;
    await run(() => api.userDelete(u.id), t('st.users.deleted', { login: u.login }));
  }
  async function reset2fa(u: UserRow): Promise<void> {
    if (!(await confirmAsk({ title: t('st.users.reset2fa'), text: t('st.users.confirmReset2fa', { login: u.login }), ok: t('st.users.reset2fa'), danger: true }))) return;
    await run(() => api.userReset2fa(u.id), t('st.users.reset2faDone'));
  }
  async function setPassword(): Promise<void> {
    const u = pwFor;
    if (!u || !resetPw || busy) return;
    busy = true;
    actionError = '';
    try {
      await api.userSetPassword(u.id, resetPw);
      showToast(t('st.users.passwordSet'));
      pwFor = null;
      resetPw = '';
    } catch (e) {
      if (isUnauthorized(e)) onauthlost();
      else actionError = errText(e);
    } finally {
      busy = false;
    }
  }
</script>

<div class="set-sec">
  <div class="row"><h3 style="margin:0">{t('st.users.title')}</h3><span class="grow"></span><button class="btn primary" type="button" onclick={() => (adding = !adding)}>{t('st.users.add')}</button></div>
  <p class="dim">{t('st.users.hint')}</p>

  {#if adding}
    <form class="panel mdl-stack" onsubmit={(e) => { e.preventDefault(); void create(); }}>
      <label class="fl"><span>{t('st.users.login')}</span><span class="field"><input autocomplete="off" autocapitalize="none" spellcheck="false" required bind:value={newLogin} /></span></label>
      <label class="fl"><span>{t('st.users.password')}</span><span class="field"><input type="password" autocomplete="new-password" required bind:value={newPassword} /></span></label>
      <AccessEditor bind:role={newRole} bind:access={newAccess} />
      {#if addError}<div class="err" role="alert">{addError}</div>{/if}
      <div class="row end"><button class="btn" type="button" onclick={() => (adding = false)}>{t('common.cancel')}</button><button class="btn primary" type="submit" disabled={busy}>{t('st.users.create')}</button></div>
    </form>
  {/if}

  {#if failed}<div class="err" role="alert">{t('st.users.loadFailed')}</div>
  {:else if !loaded}<p class="dim">{t('dash.loading')}</p>
  {:else}
    <ul class="set-list col">
      {#each users as u (u.id)}
        <li>
          <div class="urow">
            <div class="info">
              <b>{u.login}</b>
              {#if u.current}<span class="badge b-info">{t('st.users.you')}</span>{/if}
              <span class="badge {u.role === 'admin' ? 'b-mute' : 'b-info'}">{t(`st.users.role.${u.role}`)}</span>
              {#if u.access}<span class="dim">{summary(u)}</span>{/if}
              <span class="badge {u.totp_enabled ? 'b-ok' : 'b-mute'}">{u.totp_enabled ? t('st.users.2fa.on') : t('st.users.2fa.off')}</span>
              <span class="dim">{t('st.users.created', { date: when(u.created_at) })}</span>
            </div>
            {#if !u.current}
              <div class="row">
                <button class="btn sm" type="button" onclick={() => startEdit(u)}>{t('st.users.access')}</button>
                <button class="btn sm" type="button" onclick={() => { editing = null; pwFor = u; resetPw = ''; actionError = ''; }}>{t('st.users.reset')}</button>
                <button class="btn sm" type="button" disabled={busy || !u.totp_enabled} onclick={() => void reset2fa(u)}>{t('st.users.reset2fa')}</button>
                <button class="btn sm danger" type="button" disabled={busy} onclick={() => void remove(u)}>{t('st.users.delete')}</button>
              </div>
            {/if}
          </div>
          {#if editing && editing.user.id === u.id}
            <div class="mdl-stack sub">
              <AccessEditor bind:role={editing.role} bind:access={editing.access} />
              {#if actionError}<div class="err" role="alert">{actionError}</div>{/if}
              <div class="row end"><button class="btn" type="button" onclick={() => (editing = null)}>{t('common.cancel')}</button><button class="btn primary" type="button" disabled={busy} onclick={() => void saveEdit()}>{t('st.acc.apply')}</button></div>
            </div>
          {/if}
          {#if pwFor && pwFor.id === u.id}
            <form class="mdl-stack sub" onsubmit={(e) => { e.preventDefault(); void setPassword(); }}>
              <label class="fl"><span>{t('st.users.newPassword', { login: u.login })}</span><span class="field"><input type="password" autocomplete="new-password" required bind:value={resetPw} /></span></label>
              <p class="dim">{t('st.users.confirmPassword')}</p>
              {#if actionError}<div class="err" role="alert">{actionError}</div>{/if}
              <div class="row end"><button class="btn" type="button" onclick={() => (pwFor = null)}>{t('common.cancel')}</button><button class="btn primary" type="submit" disabled={busy || !resetPw}>{t('st.users.reset')}</button></div>
            </form>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>
