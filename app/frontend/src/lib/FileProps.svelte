<script lang="ts">
  import { ApiError, serverApi2, type FileStat } from './api';
  import { confirmAsk } from './dialogs.svelte';
  import { t } from './i18n.svelte';
  import { fmtBytes } from './status';

  let { id, path, offline = false, onclose, ondone }: { id: string; path: string; offline?: boolean; onclose: () => void; ondone: () => void } = $props();

  let st = $state<FileStat | null>(null);
  let error = $state('');
  let notice = $state('');
  let busy = $state(false);
  // permission bits: index 0..8 = owner r w x, group r w x, other r w x
  let bits = $state<boolean[]>(Array(9).fill(false));
  let special = $state({ suid: false, sgid: false, sticky: false });
  let octal = $state('');
  let owner = $state('');
  let group = $state('');
  let recursive = $state(false);
  let du = $state<{ size: number; files: number; dirs: number; truncated: boolean } | null>(null);

  const dir = $derived(st?.type === 'dir');
  const locked = $derived(offline || !!st?.protected || st?.type === 'link' || busy);

  function fromMode(m: string): void {
    const n = parseInt(m, 8) || 0;
    bits = Array.from({ length: 9 }, (_, i) => !!(n & (1 << (8 - i))));
    special = { suid: !!(n & 0o4000), sgid: !!(n & 0o2000), sticky: !!(n & 0o1000) };
    octal = (n & 0o7777).toString(8).padStart(4, '0');
  }
  function toNumber(): number {
    let n = 0;
    bits.forEach((b, i) => {
      if (b) n |= 1 << (8 - i);
    });
    if (special.suid) n |= 0o4000;
    if (special.sgid) n |= 0o2000;
    if (special.sticky) n |= 0o1000;
    return n;
  }
  function sync(): void {
    octal = toNumber().toString(8).padStart(4, '0');
  }
  function fromOctal(): void {
    if (/^[0-7]{3,4}$/.test(octal)) fromMode(octal);
  }

  async function load(): Promise<void> {
    try {
      st = await serverApi2.fileStat(id, path);
      fromMode(st.mode);
      owner = st.owner;
      group = st.group;
    } catch {
      error = t('files.props.loadFailed');
    }
  }
  $effect(() => {
    void path;
    void load();
  });

  const when = (s: number): string => (s > 0 ? new Date(s * 1000).toLocaleString() : t('files.props.unknown'));
  const kind = (k: string): string => t(`files.kind.${k}`);

  async function calc(): Promise<void> {
    try {
      du = await serverApi2.fileDu(id, path);
    } catch {
      error = t('files.props.loadFailed');
    }
  }

  async function run(fn: () => Promise<unknown>, done: string): Promise<void> {
    busy = true;
    error = '';
    notice = '';
    try {
      await fn();
      notice = done;
      ondone();
      await load();
    } catch (e) {
      error = e instanceof ApiError && e.code ? e.code : t('sp.err.action');
    } finally {
      busy = false;
    }
  }

  async function applyPerms(): Promise<void> {
    const sp = special.suid || special.sgid || special.sticky;
    const needs = recursive || sp;
    if (needs && !(await confirmAsk({ title: t('files.props.applyPerms'), text: t('files.props.confirmPerms', { extra: `${recursive ? t('files.props.extraRec') : ''}${sp ? t('files.props.extraSpecial') : ''}` }), ok: t('files.props.confirm'), danger: true }))) return;
    await run(() => serverApi2.fileChmod(id, path, octal, recursive, needs), t('files.props.permsDone'));
  }
  async function applyOwner(): Promise<void> {
    if (!(await confirmAsk({ title: t('files.props.applyOwner'), text: t('files.props.confirmOwner', { extra: recursive ? t('files.props.extraRec') : '' }), ok: t('files.props.confirm'), danger: true }))) return;
    await run(() => serverApi2.fileChown(id, path, owner.trim(), group.trim(), recursive), t('files.props.ownerDone'));
  }
  const ROWS = ['owner', 'group', 'other'] as const;
  const COLS = ['read', 'write', 'exec'] as const;
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onclose()} />

<div class="dlg-scrim" role="presentation" onclick={(e) => e.target === e.currentTarget && onclose()}>
  <div class="dlg props" role="dialog" aria-modal="true" aria-label={t('files.props.title')}>
    <h3>{t('files.props.title')}{#if st}{' · '}<span class="mono">{st.name}</span>{/if}</h3>
    {#if error}<div class="err" role="alert">{error}</div>{/if}
    {#if st}
      <div class="pr-grid">
        <span>{t('files.props.path')}</span><b class="mono">{st.path}</b>
        <span>{t('files.props.type')}</span><b>{kind(st.type)}{#if st.mime} · {st.mime}{/if}</b>
        <span>{t('files.props.size')}</span><b>{fmtBytes(st.size)}</b>
        {#if dir}
          <span>{t('files.props.content')}</span>
          <b>{#if du}{du.truncated ? t('files.props.calcPartial', { size: fmtBytes(du.size), files: du.files, dirs: du.dirs }) : t('files.props.calcResult', { size: fmtBytes(du.size), files: du.files, dirs: du.dirs })}{:else}<button class="btn sm" type="button" onclick={calc}>{t('files.props.calc')}</button>{/if}</b>
        {/if}
        {#if st.link_target}<span>{t('files.props.target')}</span><b class="mono">{st.link_target}</b>{/if}
        <span>{t('files.props.modified')}</span><b>{when(st.mtime)}</b>
        <span>{t('files.props.accessed')}</span><b>{when(st.atime)}</b>
        <span>{t('files.props.created')}</span><b>{when(st.btime)}</b>
      </div>
      {#if st.protected}<div class="banner">{t('files.props.system')}</div>{:else if st.type === 'link'}<p class="dim">{t('files.props.linkNote')}</p>{:else if offline}<p class="dim">{t('files.props.offline')}</p>{/if}
      <div class="pr-perm">
        <div class="pr-h">{t('files.props.perms')}</div>
        <div class="pr-bits">
          <span></span>{#each COLS as c (c)}<em>{t(`files.props.${c}`)}</em>{/each}
          {#each ROWS as r, ri (r)}
            <span>{t(`files.props.${r}`)}</span>
            {#each COLS as c, ci (c)}<input type="checkbox" aria-label="{r} {c}" bind:checked={bits[ri * 3 + ci]} disabled={locked} onchange={sync} />{/each}
          {/each}
        </div>
        <div class="row">
          <label class="ck"><input type="checkbox" bind:checked={special.suid} disabled={locked} onchange={sync} />{t('files.props.suid')}</label>
          <label class="ck"><input type="checkbox" bind:checked={special.sgid} disabled={locked} onchange={sync} />{t('files.props.sgid')}</label>
          <label class="ck"><input type="checkbox" bind:checked={special.sticky} disabled={locked} onchange={sync} />{t('files.props.sticky')}</label>
        </div>
        <div class="row">
          <span class="dim">{t('files.props.octal')}</span>
          <span class="field" style="width:80px"><input class="mono" bind:value={octal} maxlength="4" disabled={locked} oninput={fromOctal} /></span>
          <span class="grow"></span>
          <button class="btn sm" type="button" disabled={locked || !/^[0-7]{3,4}$/.test(octal)} onclick={applyPerms}>{t('files.props.applyPerms')}</button>
        </div>
        <div class="row">
          <span class="field"><input bind:value={owner} placeholder={t('files.props.owner')} aria-label={t('files.props.owner')} disabled={locked} spellcheck="false" /></span>
          <span class="field"><input bind:value={group} placeholder={t('files.props.group')} aria-label={t('files.props.group')} disabled={locked} spellcheck="false" /></span>
          <span class="grow"></span>
          <button class="btn sm" type="button" disabled={locked || !owner.trim() || !group.trim()} onclick={applyOwner}>{t('files.props.applyOwner')}</button>
        </div>
        {#if dir}<label class="ck"><input type="checkbox" bind:checked={recursive} disabled={locked} />{t('files.props.recursive')}</label>{/if}
        {#if notice}<div class="ok-t">{notice}</div>{/if}
      </div>
    {:else if !error}<p class="dim">{t('dash.loading')}</p>{/if}
    <div class="row end"><button class="btn" type="button" onclick={onclose}>{t('files.props.close')}</button></div>
  </div>
</div>
