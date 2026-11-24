<script lang="ts">
  import { ApiError, serverApi2, type FileConflictMode, type FileEntry } from '../api';
  import ContextMenu, { type CtxItem } from '../ContextMenu.svelte';
  import { confirmAsk, promptAsk } from '../dialogs.svelte';
  import FileProps from '../FileProps.svelte';
  import { clipboard, startDownload } from '../files-clip.svelte';
  import Icon from '../Icon.svelte';
  import { t } from '../i18n.svelte';
  import { native } from '../native';
  import { fmtBytes } from '../status';

  let { id, title, nativeWindows, start = '/root', win = false }: { id: string; title: string; nativeWindows: boolean; start?: string; win?: boolean } = $props();

  // svelte-ignore state_referenced_locally
  let path = $state(start);
  let entries = $state<FileEntry[]>([]);
  let error = $state('');
  let loading = $state(false);
  // the folder tree: the folders met so far, and which are open
  let kids = $state<Record<string, string[]>>({});
  let open = $state<Record<string, boolean>>({ '/': true });
  // the selection (names in the folder), the anchor of a range, the context menu, the dialog of properties
  let sel = $state<Record<string, boolean>>({});
  let anchor = '';
  let menu = $state<{ x: number; y: number; items: CtxItem[] } | null>(null);
  let propsPath = $state('');
  let note = $state<{ text: string; bad: boolean } | null>(null);
  let upload = $state<{ name: string; index: number; total: number; pct: number } | null>(null);
  let picker = $state<HTMLInputElement | null>(null);
  let noteTimer: ReturnType<typeof setTimeout> | undefined;
  let dragging = $state(false);

  const join = (p: string, n: string): string => (p === '/' ? '/' + n : p + '/' + n);
  const parent = (p: string): string => p.replace(/\/[^/]+\/?$/, '') || '/';
  const crumbs = $derived(path.split('/').filter(Boolean));
  const chosen = $derived(entries.filter((e) => sel[e.name]));
  const clip = $derived(clipboard.clip && clipboard.clip.serverId === id ? clipboard.clip : null);

  function say(text: string, bad = false): void {
    note = { text, bad };
    clearTimeout(noteTimer);
    noteTimer = setTimeout(() => (note = null), bad ? 8000 : 4000);
  }
  const why = (e: unknown): string => (e instanceof ApiError && e.code ? e.code : t('sp.err.action'));

  async function list(p: string): Promise<FileEntry[]> {
    return (await serverApi2.files(id, p)).entries;
  }

  async function go(p: string, keepSel = false): Promise<boolean> {
    loading = true;
    error = '';
    try {
      const e = await list(p);
      if (p !== path) keepSel = false;
      path = p;
      entries = e.slice().sort((a, b) => (a.type === 'dir' ? 0 : 1) - (b.type === 'dir' ? 0 : 1) || a.name.localeCompare(b.name));
      if (!keepSel) sel = {};
      kids[p] = e.filter((x) => x.type === 'dir').map((x) => x.name).sort();
      // the way to this folder is open in the tree
      let acc = '/';
      open['/'] = true;
      const above = ['/'];
      for (const part of p.split('/').filter(Boolean)) {
        acc = join(acc, part);
        open[acc] = true;
        above.push(acc);
      }
      // the folders on the way are shown with their children (a start deep in the tree has no other way to fill them)
      for (const a of above) {
        if (kids[a] === undefined) {
          list(a).then((x) => (kids[a] = x.filter((y) => y.type === 'dir').map((y) => y.name).sort())).catch(() => undefined);
        }
      }
    } catch {
      error = t('files.err');
      return false;
    } finally {
      loading = false;
    }
    return true;
  }

  async function toggle(p: string): Promise<void> {
    if (open[p]) {
      open[p] = false;
      return;
    }
    open[p] = true;
    if (!kids[p]) {
      try {
        kids[p] = (await list(p)).filter((x) => x.type === 'dir').map((x) => x.name).sort();
      } catch {
        kids[p] = [];
      }
    }
  }

  // the agent works as root: its home folder first, the top of the tree when there is none (Windows)
  $effect(() => {
    const p = start;
    void go(p).then((ok) => !ok && p !== '/' && go('/'));
  });

  function openEntry(e: FileEntry): void {
    if (e.type === 'dir') void go(join(path, e.name));
    else if (e.type === 'file') void native.openWindow('editor', id, e.name, nativeWindows, join(path, e.name));
  }
  const fmtDate = (s: number): string => new Date(s * 1000).toLocaleString(undefined, { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' });

  // ---- selection ----
  function pick(e: FileEntry, ev: MouseEvent): void {
    if (ev.shiftKey && anchor) {
      const a = entries.findIndex((x) => x.name === anchor), b = entries.findIndex((x) => x.name === e.name);
      const [lo, hi] = a < b ? [a, b] : [b, a];
      sel = Object.fromEntries(entries.slice(lo, hi + 1).map((x) => [x.name, true]));
    } else if (ev.metaKey || ev.ctrlKey) {
      sel = { ...sel, [e.name]: !sel[e.name] };
      anchor = e.name;
    } else {
      sel = { [e.name]: true };
      anchor = e.name;
    }
  }

  // ---- names ----
  const checkName = (v: string): string => (!v ? t('files.err.nameEmpty') : v.includes('/') ? t('files.err.nameSlash') : v === '.' || v === '..' ? t('files.err.nameReserved') : '');

  async function create(kind: 'dir' | 'file'): Promise<void> {
    const name = await promptAsk({ title: kind === 'dir' ? t('files.promptDir') : t('files.promptFile'), label: `${t('files.nameLabel')} · ${t('files.inDir', { path })}`, ok: t('files.create'), check: checkName });
    if (name === null) return;
    try {
      if (kind === 'dir') await serverApi2.fileMkdir(id, join(path, name));
      else await serverApi2.fileCreate(id, join(path, name));
      say(t('files.created', { name }));
      await go(path, true);
      if (kind === 'dir') kids[path] = entries.filter((x) => x.type === 'dir').map((x) => x.name).sort();
    } catch (e) {
      say(why(e), true);
    }
  }

  async function rename(e: FileEntry): Promise<void> {
    const name = await promptAsk({ title: t('files.promptRename'), label: t('files.nameLabel'), value: e.name, ok: t('files.rename.submit'), check: (v) => checkName(v) || (v === e.name ? t('files.err.nameEmpty') : '') });
    if (name === null) return;
    try {
      await serverApi2.fileRename(id, join(path, e.name), name);
      say(t('files.renamed', { name }));
      await go(path);
    } catch (er) {
      say(why(er), true);
    }
  }

  async function remove(list: FileEntry[]): Promise<void> {
    if (list.length === 0) return;
    const one = list[0];
    const ok = await confirmAsk(list.length === 1
      ? { title: t('files.deleteTitle'), text: t(one.type === 'dir' ? 'files.deleteDir' : 'files.deleteFile', { path: join(path, one.name) }), ok: t('files.delete'), danger: true }
      : { title: t('files.deleteManyTitle'), text: t('files.deleteMany', { n: list.length }), ok: t('files.delete'), danger: true });
    if (!ok) return;
    let done = 0;
    for (const e of list) {
      try {
        await serverApi2.fileDelete(id, join(path, e.name));
        done++;
      } catch (er) {
        say(`${e.name}: ${why(er)}`, true);
      }
    }
    if (done > 0) say(list.length === 1 ? t('files.deleted', { name: one.name }) : t('files.deletedMany', { n: done }));
    await go(path);
  }

  // ---- clipboard ----
  function setClip(mode: 'cut' | 'copy'): void {
    clipboard.clip = { mode, serverId: id, dir: path, names: chosen.map((e) => e.name) };
    say(t(mode === 'cut' ? 'files.cut' : 'files.copied', { n: chosen.length }));
  }

  async function paste(target: string): Promise<void> {
    const c = clip;
    if (!c) return;
    let done = 0;
    const failed: string[] = [];
    for (const name of c.names) {
      const src = join(c.dir, name);
      if (c.mode === 'cut' && c.dir === target) continue;
      let mode: FileConflictMode = 'fail';
      for (;;) {
        try {
          if (c.mode === 'cut') await serverApi2.fileMove(id, src, target, mode);
          else await serverApi2.fileCopy(id, src, target, mode);
          done++;
          break;
        } catch (e) {
          if (mode === 'fail' && e instanceof ApiError && e.status === 409) {
            // the name is taken: replace it, or keep both under a free name
            mode = (await confirmAsk({ title: t('files.conflict.title'), text: t('files.conflict.text', { name }), ok: t('files.conflict.overwrite'), cancel: t('files.conflict.keep'), danger: true })) ? 'overwrite' : 'keep';
            continue;
          }
          failed.push(name);
          break;
        }
      }
    }
    if (c.mode === 'cut') clipboard.clip = null;
    say(failed.length ? t('files.paste.failed', { list: failed.join(', ') }) : t('files.paste.done', { n: done }), failed.length > 0);
    await go(path);
  }

  // ---- download and upload ----
  async function save(url: string, name: string): Promise<void> {
    say(t('files.downloading', { name }));
    try {
      const to = await startDownload(url, name);
      if (to) say(t('files.downloaded', { path: to }));
      else note = null;
    } catch (e) {
      say(why(e), true);
    }
  }
  const download = (e: FileEntry): Promise<void> => save(serverApi2.fileDownloadUrl(id, join(path, e.name)), e.name);
  function downloadZip(list: FileEntry[]): Promise<void> {
    const names = list.map((e) => e.name);
    return save(serverApi2.fileZipUrl(id, path, names), names.length === 1 ? `${names[0]}.zip` : 'files.zip');
  }

  async function sendFiles(files: File[]): Promise<void> {
    if (upload || files.length === 0) return;
    for (let i = 0; i < files.length; i++) {
      const f = files[i];
      upload = { name: f.name, index: i + 1, total: files.length, pct: 0 };
      try {
        await serverApi2.uploadFile(id, join(path, f.name), f, (p) => upload && (upload.pct = Math.round(p * 100)));
      } catch (e) {
        say(t('files.upload.fail', { name: f.name, error: why(e) }), true);
      }
    }
    upload = null;
    await go(path, true);
  }
  function onPick(): void {
    const f = [...(picker?.files ?? [])];
    if (picker) picker.value = '';
    void sendFiles(f);
  }
  function onDrop(e: DragEvent): void {
    e.preventDefault();
    dragging = false;
    void sendFiles([...(e.dataTransfer?.files ?? [])]);
  }

  // ---- the context menu ----
  function itemsFor(list: FileEntry[]): CtxItem[] {
    const one = list.length === 1 ? list[0] : null;
    const items: CtxItem[] = [];
    if (list.length > 0) {
      if (one) items.push({ key: 'open', label: t('files.menu.open'), icon: one.type === 'dir' ? 'folder' : 'file-text', disabled: one.type !== 'dir' && one.type !== 'file', onselect: () => openEntry(one) });
      if (one && one.type !== 'dir') items.push({ key: 'dl', label: t('files.menu.download'), icon: 'download', disabled: one.type !== 'file', onselect: () => void download(one) });
      else items.push({ key: 'zip', label: t('files.menu.downloadZip'), icon: 'download', onselect: () => void downloadZip(list) });
      items.push({ key: 'cut', label: t('files.menu.cut'), icon: 'scissors', separated: true, onselect: () => setClip('cut') });
      items.push({ key: 'copy', label: t('files.menu.copy'), icon: 'copy', onselect: () => setClip('copy') });
      if (clip) {
        if (one?.type === 'dir') items.push({ key: 'pasteInto', label: t('files.menu.pasteInto'), icon: 'clipboard-paste', onselect: () => void paste(join(path, one.name)) });
        else items.push({ key: 'paste', label: t('files.menu.paste'), icon: 'clipboard-paste', onselect: () => void paste(path) });
      }
      if (one) {
        items.push({ key: 'rename', label: t('files.menu.rename'), icon: 'pencil', separated: true, onselect: () => void rename(one) });
        items.push({ key: 'props', label: t('files.menu.props'), icon: 'info', onselect: () => (propsPath = join(path, one.name)) });
      }
      items.push({ key: 'delete', label: list.length > 1 ? t('files.menu.deleteN', { n: list.length }) : t('files.menu.delete'), icon: 'trash-2', danger: true, separated: true, onselect: () => void remove(list) });
    } else {
      if (clip) items.push({ key: 'paste', label: t('files.menu.paste'), icon: 'clipboard-paste', onselect: () => void paste(path) });
      items.push({ key: 'mkdir', label: t('files.newFolder'), icon: 'folder-plus', separated: !!clip, onselect: () => void create('dir') });
      items.push({ key: 'mkfile', label: t('files.newFile'), icon: 'file-plus', onselect: () => void create('file') });
      items.push({ key: 'upload', label: t('files.upload'), icon: 'upload', onselect: () => picker?.click() });
      items.push({ key: 'refresh', label: t('files.refresh'), icon: 'refresh-cw', onselect: () => void go(path, true) });
      items.push({ key: 'props', label: t('files.menu.props'), icon: 'info', separated: true, onselect: () => (propsPath = path) });
    }
    return items;
  }

  function rowMenu(e: FileEntry, ev: MouseEvent): void {
    ev.preventDefault();
    ev.stopPropagation();
    if (!sel[e.name]) {
      sel = { [e.name]: true };
      anchor = e.name;
    }
    menu = { x: ev.clientX, y: ev.clientY, items: itemsFor(chosen) };
  }
  function areaMenu(ev: MouseEvent): void {
    ev.preventDefault();
    sel = {};
    menu = { x: ev.clientX, y: ev.clientY, items: itemsFor([]) };
  }

  function onKey(ev: KeyboardEvent): void {
    if ((ev.target as HTMLElement).closest('input,textarea')) return;
    if (ev.key === 'Delete' && chosen.length > 0) void remove(chosen);
    else if (ev.key === 'Enter' && chosen.length === 1) openEntry(chosen[0]);
    else if (ev.key === 'F2' && chosen.length === 1) void rename(chosen[0]);
    else if ((ev.metaKey || ev.ctrlKey) && ev.key === 'a') {
      ev.preventDefault();
      sel = Object.fromEntries(entries.map((e) => [e.name, true]));
    }
  }
</script>

{#snippet node(p: string, name: string, depth: number)}
  <div class="tr" class:on={path === p} style="padding-left:{4 + depth * 14}px">
    <button class="tg" class:nil={kids[p] !== undefined && kids[p].length === 0} type="button" aria-label="toggle" onclick={() => toggle(p)}><Icon name={open[p] ? 'chevron-down' : 'chevron-right'} size={13} /></button>
    <button class="tl" type="button" onclick={() => go(p)}><Icon name="folder" size={14} /><span>{name}</span></button>
  </div>
  {#if open[p]}{#each kids[p] ?? [] as k (k)}{@render node(join(p, k), k, depth + 1)}{/each}{/if}
{/snippet}

<div class="fmv" class:fm-win={win}>
  <div class="fm-bar">
    <div class="crumbs"><button type="button" onclick={() => go('/')}>/</button>{#each crumbs as c, i (i)}<i>/</i><button type="button" onclick={() => go('/' + crumbs.slice(0, i + 1).join('/'))}>{c}</button>{/each}</div>
    <span class="grow"></span>
    <button class="btn sm" type="button" disabled={path === '/'} onclick={() => go(parent(path))}><Icon name="folder-up" size={14} />{t('files.up')}</button>
    <button class="btn sm" type="button" onclick={() => create('dir')}><Icon name="folder-plus" size={14} />{t('files.newFolder')}</button>
    <button class="btn sm" type="button" onclick={() => create('file')}><Icon name="file-plus" size={14} />{t('files.newFile')}</button>
    <button class="btn sm" type="button" disabled={!!upload} onclick={() => picker?.click()}><Icon name="upload" size={14} />{t('files.upload')}</button>
    <button class="btn sm icon" type="button" title={t('files.refresh')} aria-label={t('files.refresh')} onclick={() => go(path, true)}><Icon name="refresh-cw" size={14} /></button>
    <input bind:this={picker} type="file" multiple hidden onchange={onPick} />
  </div>
  {#if upload}<div class="fm-note"><span>{t('files.uploading', { name: upload.name, index: upload.index, total: upload.total })}</span><span class="bar"><i style="width:{upload.pct}%"></i></span></div>
  {:else if note}<div class="fm-note" class:bad={note.bad}><span>{note.text}</span></div>{/if}
  <div class="fm-split">
    <div class="fm-tree"><div class="tree">{@render node('/', '/', 0)}</div></div>
    <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_static_element_interactions -->
    <div class="fm-list" class:drop={dragging} tabindex="0" onkeydown={onKey} oncontextmenu={areaMenu} onclick={(e) => e.target === e.currentTarget && (sel = {})}
      ondragover={(e) => { e.preventDefault(); dragging = true; }} ondragleave={() => (dragging = false)} ondrop={onDrop}>
      {#if error}<div class="err" style="margin:10px">{error}</div>{/if}
      <table class="tbl compact">
        <thead><tr><th>{t('files.name')}</th><th>{t('files.rights')}</th><th>{t('files.owner')}</th><th class="r">{t('files.size')}</th><th>{t('files.modified')}</th></tr></thead>
        <tbody>
          {#each entries as e (e.name)}
            <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
            <tr class="fr" class:sel={sel[e.name]} onclick={(ev) => { ev.stopPropagation(); pick(e, ev); }} ondblclick={() => openEntry(e)} oncontextmenu={(ev) => rowMenu(e, ev)}>
              <td><Icon name={e.type === 'dir' ? 'folder' : 'file-text'} size={14} />{e.name}{e.type === 'dir' ? '/' : ''}</td>
              <td class="mono dim">{e.mode_str}</td><td class="dim">{e.owner}</td><td class="mono r">{e.type === 'dir' ? '—' : fmtBytes(e.size)}</td><td class="mono dim">{fmtDate(e.mtime)}</td>
            </tr>
          {:else}
            <tr><td colspan="5" class="dim">{loading ? t('dash.loading') : t('files.empty')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
</div>
{#if menu}<ContextMenu x={menu.x} y={menu.y} items={menu.items} onclose={() => (menu = null)} />{/if}
{#if propsPath}<FileProps {id} path={propsPath} onclose={() => (propsPath = '')} ondone={() => void go(path, true)} />{/if}
