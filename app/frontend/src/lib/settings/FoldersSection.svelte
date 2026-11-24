<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { api, ApiError, type AccessChange, type Group, type UserRow } from '../api';
  import ContextMenu, { type CtxItem } from '../ContextMenu.svelte';
  import { confirmAsk } from '../dialogs.svelte';
  import { isUnauthorized } from '../errors';
  import { impactLines } from '../folder-impact';
  import FolderPickModal from '../FolderPickModal.svelte';
  import { MAX_FOLDER_DEPTH, MAX_FOLDER_NAME, buildTree, moveProblem, subtreeIds, type FolderNode } from '../folders';
  import Icon from '../Icon.svelte';
  import { t } from '../i18n.svelte';
  import { getPref, setPref } from '../prefs';
  import SearchField from '../SearchField.svelte';
  import { showToast } from '../toast';

  // Settings → Folders: the groups of the servers as a tree. A folder is dragged to change its order (between rows) or to go into
  // another folder (onto the row); a server is dragged into a folder the same way. A folder carries access (it covers everything under
  // it), so before a move or a delete that changes who sees what, the server says whom it touches and the administrator confirms.
  let { onauthlost }: { onauthlost: () => void } = $props();

  interface Srv { id: string; name: string; address: string; os: string; group_id: number | null }
  type Row =
    | { key: string; t: 'f'; f: FolderNode }
    | { key: string; t: 's'; s: Srv; depth: number }
    | { key: string; t: 'n'; depth: number; parent: number | null }
    | { key: string; t: 'u' };
  type Drag = { k: 'f' | 's'; id: number | string };
  type Drop = { row: string; pos: 'before' | 'after' | 'first' | 'into' | 'end' | 'bucket'; target: number | null; parent: number | null; before: number | null; ok: boolean; why: string; sig: string };

  const KEY = 'linager.folders.closed';
  let folders = $state<Group[]>([]);
  let servers = $state<Srv[]>([]);
  let users = $state<UserRow[]>([]);
  let loaded = $state(false);
  let filter = $state('');
  let showServers = $state(true);
  let busy = $state(false);
  let closed = $state<Record<string, boolean>>(readClosed());
  let focus = $state('');
  let flash = $state('');
  let renaming = $state<{ id: number; value: string; error: string } | null>(null);
  let draft = $state<{ parent: number | null; value: string; error: string } | null>(null);
  let menu = $state<{ x: number; y: number; items: CtxItem[] } | null>(null);
  let pick = $state<{ kind: 'f' | 's'; id: number | string } | null>(null);
  let undo = $state<{ text: string; run: () => Promise<void> } | null>(null);
  let undoTimer: ReturnType<typeof setTimeout> | undefined;
  let treeEl = $state<HTMLDivElement | null>(null);

  // the drag in progress
  let drag = $state<Drag | null>(null);
  let drop = $state<Drop | null>(null);
  let ghost = $state<{ x: number; y: number; label: string; n: number; icon: string } | null>(null);
  let line = $state<{ top: number; left: number } | null>(null);
  let start: { x: number; y: number; k: 'f' | 's'; id: number | string } | null = null;
  let lastPt = { x: 0, y: 0 };
  let expandTimer: ReturnType<typeof setTimeout> | undefined;
  let scrollRaf = 0;

  function readClosed(): Record<string, boolean> {
    try {
      const v = JSON.parse(getPref(KEY) ?? '{}') as unknown;
      return v && typeof v === 'object' ? (v as Record<string, boolean>) : {};
    } catch {
      return {};
    }
  }
  const saveClosed = (): void => setPref(KEY, JSON.stringify(closed));

  // ---- data ----
  async function load(): Promise<void> {
    try {
      const [g, s, u] = await Promise.all([api.groups(), api.serversAll(), api.users()]);
      folders = g;
      servers = s;
      users = u.users;
    } catch (e) {
      if (isUnauthorized(e)) onauthlost();
      else showToast(t('fld.err.load'));
    } finally {
      loaded = true;
    }
  }
  onMount(() => void load());

  const q = $derived(filter.trim().toLowerCase());
  const tree = $derived(buildTree(folders));
  const byId = $derived(new Map(folders.map((g) => [g.id, g])));
  const hit = (name: string): boolean => !q || name.toLowerCase().includes(q);
  // the number of servers under every folder, and the users given a folder
  const counts = $derived.by(() => {
    const m = new Map<number, number>();
    for (const g of folders) {
      const ids = subtreeIds(folders, g.id);
      m.set(g.id, servers.filter((s) => s.group_id !== null && ids.has(s.group_id)).length);
    }
    return m;
  });
  const grants = $derived.by(() => {
    const m = new Map<number, string[]>();
    for (const u of users) for (const id of u.access?.groups ?? []) m.set(id, [...(m.get(id) ?? []), u.login]);
    return m;
  });
  const free = $derived(servers.filter((s) => s.group_id === null));
  const matches = (f: FolderNode): boolean => hit(f.name) || f.children.some(matches) || (showServers && servers.some((s) => s.group_id === f.id && hit(s.name)));
  const isOpen = (key: string): boolean => (q ? true : !closed[key]);

  const rows = $derived.by(() => {
    const out: Row[] = [];
    const walk = (nodes: FolderNode[], parent: number | null): void => {
      for (const f of nodes) {
        if (q && !matches(f)) continue;
        out.push({ key: `f${f.id}`, t: 'f', f });
        if (isOpen(`f${f.id}`)) {
          walk(f.children, f.id);
          if (showServers) for (const s of servers) if (s.group_id === f.id && hit(s.name)) out.push({ key: `s${s.id}`, t: 's', s, depth: f.depth + 1 });
        }
      }
      if (draft && draft.parent === parent) out.push({ key: 'n', t: 'n', depth: parent === null ? 0 : (byId.get(parent) ? depthOf(parent) + 1 : 0), parent });
    };
    walk(tree, null);
    if (showServers) {
      const list = free.filter((s) => hit(s.name));
      if (!q || list.length) {
        out.push({ key: 'u', t: 'u' });
        if (q || !closed.u) for (const s of list) out.push({ key: `s${s.id}`, t: 's', s, depth: 1 });
      }
    }
    return out;
  });
  function depthOf(id: number): number {
    let d = 0;
    for (let c = byId.get(id); c && c.parent_id != null && d < 99; c = byId.get(c.parent_id)) d++;
    return d;
  }
  const rowDepth = (r: Row): number => (r.t === 'f' ? r.f.depth : r.t === 'u' ? 0 : r.depth);

  const clean = (v: string): string => v.split(/\s+/).filter(Boolean).join(' ');
  const osTag = (s: Srv): string => (/windows/i.test(s.os) ? 'WIN' : 'LIN');
  function parts(name: string): [string, string, string] {
    const i = q ? name.toLowerCase().indexOf(q) : -1;
    return i < 0 ? [name, '', ''] : [name.slice(0, i), name.slice(i, i + q.length), name.slice(i + q.length)];
  }
  function failText(e: unknown): string {
    if (e instanceof ApiError && e.status === 409) return t('fld.err.exists');
    if (e instanceof ApiError && e.status === 422) return t('fld.err.refused', { n: MAX_FOLDER_DEPTH });
    if (e instanceof ApiError && e.status === 404) return t('fld.err.gone');
    return t('fld.err.failed');
  }
  function fail(e: unknown): void {
    if (isUnauthorized(e)) return onauthlost();
    showToast(failText(e));
  }

  // ---- undo ----
  function offerUndo(text: string, run: () => Promise<void>): void {
    clearTimeout(undoTimer);
    undo = { text, run };
    undoTimer = setTimeout(() => (undo = null), 8000);
  }
  async function doUndo(): Promise<void> {
    const u = undo;
    undo = null;
    clearTimeout(undoTimer);
    if (!u) return;
    try {
      await u.run();
      await load();
    } catch (e) {
      fail(e);
    }
  }

  // ---- changes ----
  /** Asks the server whom a change would touch, and the administrator to confirm when it touches anyone. */
  async function confirmImpact(impact: AccessChange[], text: string): Promise<boolean> {
    if (impact.length === 0) return true;
    return confirmAsk({ title: t('fld.impact.title'), text: `${text}\n${impactLines(impact).join('\n')}`, ok: t('fld.move') });
  }
  const nextSibling = (id: number): number | null => {
    const g = byId.get(id);
    const sib = folders.filter((x) => (x.parent_id ?? null) === (g?.parent_id ?? null));
    const i = sib.findIndex((x) => x.id === id);
    return sib[i + 1]?.id ?? null;
  };

  async function moveFolder(id: number, parent: number | null, before: number | null): Promise<void> {
    const f = byId.get(id);
    if (!f || busy) return;
    const was = { parent: f.parent_id ?? null, before: nextSibling(id) };
    if (was.parent === parent && (was.before === before || (before === null && was.before === null))) return;
    busy = true;
    try {
      const plan = await api.groupMove(id, parent, before, true);
      const where = parent === null ? t('fld.toTop') : t('fld.toFolder', { name: byId.get(parent)?.name ?? '' });
      if (!(await confirmImpact(plan.impact, t('fld.impact.folder', { name: f.name, where })))) return;
      await api.groupMove(id, parent, before);
      if (parent !== null) closed[`f${parent}`] = false;
      saveClosed();
      flash = `f${id}`;
      await load();
      offerUndo(t('fld.moved', { name: f.name }), async () => void (await api.groupMove(id, was.parent, was.before)));
    } catch (e) {
      fail(e);
    } finally {
      busy = false;
    }
  }
  async function moveServer(id: string, groupId: number | null): Promise<void> {
    const s = servers.find((x) => x.id === id);
    if (!s || busy || s.group_id === groupId) return;
    busy = true;
    try {
      const plan = await api.serverSetGroup(id, groupId, true);
      if (!(await confirmImpact(plan.impact, t('fld.impact.server', { name: s.name })))) return;
      await api.serverSetGroup(id, groupId);
      if (groupId !== null) closed[`f${groupId}`] = false;
      else closed.u = false;
      saveClosed();
      flash = `s${id}`;
      const was = s.group_id;
      await load();
      offerUndo(t('fld.moved', { name: s.name }), async () => void (await api.serverSetGroup(id, was)));
    } catch (e) {
      fail(e);
    } finally {
      busy = false;
    }
  }

  function startDraft(parent: number | null): void {
    if (parent !== null) {
      closed[`f${parent}`] = false;
      saveClosed();
    }
    renaming = null;
    draft = { parent, value: '', error: '' };
  }
  async function commitDraft(cancel = false): Promise<void> {
    const d = draft;
    if (!d || busy) return;
    const name = clean(d.value);
    if (cancel || !name) {
      draft = null;
      return;
    }
    busy = true;
    try {
      const g = await api.groupCreate(name, d.parent);
      draft = null;
      flash = `f${g.id}`;
      await load();
      offerUndo(t('fld.created', { name }), async () => void (await api.groupDelete(g.id)));
    } catch (e) {
      if (isUnauthorized(e)) return onauthlost();
      d.error = failText(e);
    } finally {
      busy = false;
    }
  }
  function startRename(f: Group): void {
    draft = null;
    renaming = { id: f.id, value: f.name, error: '' };
  }
  async function commitRename(f: Group, cancel = false): Promise<void> {
    const r = renaming;
    if (!r || r.id !== f.id || busy) return;
    const name = clean(r.value);
    if (cancel || !name || name === f.name) {
      renaming = null;
      return;
    }
    busy = true;
    try {
      await api.groupRename(f.id, name);
      renaming = null;
      flash = `f${f.id}`;
      await load();
      offerUndo(t('fld.renamed', { was: f.name, name }), async () => void (await api.groupRename(f.id, f.name)));
    } catch (e) {
      if (isUnauthorized(e)) return onauthlost();
      r.error = failText(e);
    } finally {
      busy = false;
    }
  }
  async function removeFolder(f: Group): Promise<void> {
    if (busy) return;
    busy = true;
    try {
      const plan = await api.groupDelete(f.id, true);
      const empty = plan.subfolders === 0 && plan.servers === 0 && plan.grants.length === 0;
      if (!empty) {
        const parent = f.parent_id === null ? t('fld.toTop') : t('fld.toFolder', { name: byId.get(f.parent_id)?.name ?? '' });
        const lines = [
          plan.subfolders + plan.servers > 0 ? t('fld.delete.moves', { sub: plan.subfolders, srv: plan.servers, where: parent }) : t('fld.delete.empty'),
          plan.grants.length > 0 ? t('fld.delete.grants', { users: plan.grants.join(', ') }) : '',
          ...impactLines(plan.impact),
        ].filter(Boolean);
        if (!(await confirmAsk({ title: t('fld.delete.title', { name: f.name }), text: lines.join('\n'), ok: t('fld.delete'), danger: true }))) return;
      }
      await api.groupDelete(f.id);
      await load();
      if (empty) offerUndo(t('fld.deleted', { name: f.name }), async () => void (await api.groupCreate(f.name, f.parent_id)));
      else showToast(t('fld.deleted', { name: f.name }));
    } catch (e) {
      fail(e);
    } finally {
      busy = false;
    }
  }

  // ---- menu, keys ----
  function rowMenu(r: Row, x: number, y: number): void {
    if (r.t === 'f') {
      const f = r.f;
      menu = {
        x, y,
        items: [
          { key: 'add', label: t('fld.menu.add'), icon: 'folder-plus', onselect: () => startDraft(f.id) },
          { key: 'ren', label: t('fld.menu.rename'), icon: 'pencil', onselect: () => startRename(f) },
          { key: 'mv', label: t('fld.menu.move'), icon: 'folder-input', onselect: () => (pick = { kind: 'f', id: f.id }) },
          { key: 'del', label: t('fld.menu.delete'), icon: 'trash-2', danger: true, separated: true, onselect: () => void removeFolder(f) },
        ],
      };
    } else if (r.t === 's') {
      const s = r.s;
      menu = { x, y, items: [{ key: 'mv', label: t('fld.menu.move'), icon: 'folder-input', onselect: () => (pick = { kind: 's', id: s.id }) }] };
    }
  }
  const rowOf = (key: string): Row | undefined => rows.find((r) => r.key === key);
  async function focusRow(): Promise<void> {
    await tick();
    treeEl?.querySelector<HTMLElement>('[tabindex="0"]')?.focus();
  }
  function onKey(e: KeyboardEvent): void {
    const target = e.target as HTMLElement;
    if (target.tagName === 'INPUT') return;
    const el = target.closest<HTMLElement>('.ft-r');
    const r = el ? rowOf(el.dataset.key ?? '') : undefined;
    if (!el || !r) return;
    const i = rows.indexOf(r);
    const go = (j: number): void => {
      if (rows[j]) {
        focus = rows[j].key;
        void focusRow();
      }
    };
    if (e.key === 'ArrowDown') { e.preventDefault(); go(i + 1); }
    else if (e.key === 'ArrowUp' && !e.altKey) { e.preventDefault(); go(i - 1); }
    else if (e.key === 'ArrowRight' && r.t === 'f') { e.preventDefault(); if (!isOpen(r.key)) { closed[r.key] = false; saveClosed(); } else go(i + 1); }
    else if (e.key === 'ArrowLeft') {
      e.preventDefault();
      if (r.t === 'f' && isOpen(r.key) && !q) { closed[r.key] = true; saveClosed(); }
      else if (r.t === 'f' && r.f.parent_id !== null) go(rows.findIndex((x) => x.t === 'f' && x.f.id === r.f.parent_id));
    } else if (e.key === 'F2' && r.t === 'f') { e.preventDefault(); startRename(r.f); }
    else if ((e.key === 'Delete' || e.key === 'Backspace') && r.t === 'f') { e.preventDefault(); void removeFolder(r.f); }
    else if (e.altKey && (e.key === 'ArrowUp' || e.key === 'ArrowDown') && r.t === 'f') {
      e.preventDefault();
      const sib = folders.filter((x) => (x.parent_id ?? null) === (r.f.parent_id ?? null));
      const j = sib.findIndex((x) => x.id === r.f.id);
      if (e.key === 'ArrowUp' && sib[j - 1]) void moveFolder(r.f.id, r.f.parent_id ?? null, sib[j - 1].id).then(focusRow);
      if (e.key === 'ArrowDown' && sib[j + 1]) void moveFolder(r.f.id, r.f.parent_id ?? null, sib[j + 2]?.id ?? null).then(focusRow);
    } else if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) {
      e.preventDefault();
      const b = el.getBoundingClientRect();
      rowMenu(r, b.left + 60, b.bottom);
    }
  }
  function onClickRow(e: MouseEvent, r: Row): void {
    focus = r.key;
    const el = (e.target as HTMLElement).closest<HTMLElement>('[data-a]');
    if (!el) return;
    const a = el.dataset.a;
    if (a === 'tog') { closed[r.key] = !closed[r.key]; saveClosed(); }
    else if (a === 'add' && r.t === 'f') startDraft(r.f.id);
    else if (a === 'ren' && r.t === 'f') startRename(r.f);
    else if (a === 'menu') { const b = el.getBoundingClientRect(); rowMenu(r, b.left, b.bottom + 4); }
  }

  // ---- drag and drop (pointer events: mouse, pen and touch) ----
  function onPointerDown(e: PointerEvent): void {
    if (e.button !== 0 || q || renaming || draft || busy) return;
    const t0 = e.target as HTMLElement;
    if (t0.closest('button,input,.ft-ra')) return;
    const el = t0.closest<HTMLElement>('.ft-r');
    if (!el || el.dataset.k === 'u' || el.dataset.k === 'n') return;
    start = { x: e.clientX, y: e.clientY, k: el.dataset.k as 'f' | 's', id: el.dataset.k === 'f' ? Number(el.dataset.id) : (el.dataset.id as string) };
    addEventListener('pointermove', onMove);
    addEventListener('pointerup', onUp);
    addEventListener('pointercancel', onCancel);
  }
  function beginDrag(): void {
    if (!start) return;
    drag = { k: start.k, id: start.id };
    document.body.classList.add('ft-dnd');
    const label = start.k === 'f' ? (byId.get(start.id as number)?.name ?? '') : (servers.find((s) => s.id === start!.id)?.name ?? '');
    const n = start.k === 'f' ? (counts.get(start.id as number) ?? 0) + subtreeIds(folders, start.id as number).size - 1 : 0;
    ghost = { x: lastPt.x, y: lastPt.y, label, n, icon: start.k === 'f' ? 'folder' : 'server' };
    scrollLoop();
  }
  function onMove(e: PointerEvent): void {
    lastPt = { x: e.clientX, y: e.clientY };
    if (!drag) {
      if (!start || Math.hypot(e.clientX - start.x, e.clientY - start.y) < 5) return;
      beginDrag();
    }
    update();
  }
  /** Where the drag would land and whether that is allowed. */
  function resolve(d: Drag, el: HTMLElement, y: number): Drop | null {
    const k = el.dataset.k;
    const key = el.dataset.key ?? '';
    if (k === 'n') return null;
    const r = el.getBoundingClientRect();
    const rel = (y - r.top) / r.height;
    let pos: Drop['pos'];
    let target: number | null = null;
    let parent: number | null = null;
    let before: number | null = null;
    let row = key;
    if (d.k === 'f') {
      if (k === 'f') {
        const f = byId.get(Number(el.dataset.id));
        if (!f) return null;
        const expanded = isOpen(key) && (folders.some((g) => g.parent_id === f.id) || (showServers && servers.some((s) => s.group_id === f.id)));
        target = f.id;
        if (rel < 0.28) { pos = 'before'; parent = f.parent_id ?? null; before = f.id; }
        else if (rel > 0.72) {
          if (expanded) { pos = 'first'; parent = f.id; before = folders.find((g) => g.parent_id === f.id && g.id !== d.id)?.id ?? null; }
          else { pos = 'after'; parent = f.parent_id ?? null; before = siblingAfter(f.id, d.id as number); }
        } else { pos = 'into'; parent = f.id; }
      } else if (k === 's') {
        const s = servers.find((x) => x.id === el.dataset.id);
        if (!s) return null;
        if (s.group_id === null) { pos = 'end'; row = 'u'; } else { pos = 'into'; parent = s.group_id; target = s.group_id; row = `f${s.group_id}`; }
      } else { pos = 'end'; }
      let why = '';
      const p = moveProblem(folders, d.id as number, parent);
      if (p === 'self') why = t('fld.why.self');
      else if (p === 'depth') why = t('fld.why.depth', { n: MAX_FOLDER_DEPTH });
      else if (p === 'name') why = t('fld.why.name');
      return { row, pos, target, parent, before, ok: !why, why, sig: [row, pos, target, !why].join('|') };
    }
    // a server goes into a folder: onto a folder row, onto a server of it, or into "no folder"
    if (k === 'f') { pos = 'into'; target = Number(el.dataset.id); }
    else if (k === 's') {
      const s = servers.find((x) => x.id === el.dataset.id);
      if (!s) return null;
      if (s.id === d.id) return null;
      if (s.group_id === null) { pos = 'bucket'; row = 'u'; } else { pos = 'into'; target = s.group_id; row = `f${s.group_id}`; }
    } else pos = 'bucket';
    return { row, pos, target, parent: target, before: null, ok: true, why: '', sig: [row, pos, target, true].join('|') };
  }
  /** The next sibling of a folder, not counting the folder that is being dragged. */
  function siblingAfter(id: number, dragged: number): number | null {
    const g = byId.get(id);
    const sib = folders.filter((x) => (x.parent_id ?? null) === (g?.parent_id ?? null) && x.id !== dragged);
    const i = sib.findIndex((x) => x.id === id);
    return sib[i + 1]?.id ?? null;
  }
  function update(): void {
    if (!drag || !ghost) return;
    const { x, y } = lastPt;
    ghost.x = x;
    ghost.y = y;
    const el = document.elementFromPoint(x, y)?.closest<HTMLElement>('.ft-r') ?? null;
    const d = el && treeEl?.contains(el) ? resolve(drag, el, y) : null;
    if ((d?.sig ?? '') !== (drop?.sig ?? '')) {
      drop = d;
      clearTimeout(expandTimer);
      if (d && d.ok && d.pos === 'into' && d.target !== null && closed[`f${d.target}`]) {
        const id = d.target;
        expandTimer = setTimeout(() => { closed[`f${id}`] = false; saveClosed(); }, 650);
      }
    }
  }
  // the insertion line sits between rows, at the indent of the place the folder would take
  $effect(() => {
    void rows;
    const d = drop;
    if (!d || !treeEl || d.pos === 'into' || d.pos === 'bucket') { line = null; return; }
    void tick().then(() => {
      const el = treeEl?.querySelector<HTMLElement>(`.ft-r[data-key="${d.row}"]`);
      if (!el || !treeEl) { line = null; return; }
      const rr = el.getBoundingClientRect();
      const tr = treeEl.getBoundingClientRect();
      const r = rowOf(d.row);
      const depth = (r ? rowDepth(r) : 0) + (d.pos === 'first' ? 1 : 0);
      // the line lives in the scrolled content of the tree, so the scroll position is added
      line = { top: (d.pos === 'before' || d.pos === 'end' ? rr.top : rr.bottom) - tr.top + treeEl.scrollTop - 1, left: 4 + 18 + depth * 22 + 4 };
    });
  });
  function scrollLoop(): void {
    // the tree scrolls inside its block; near its top or bottom edge a drag scrolls it
    const box = treeEl;
    const tick_ = (): void => {
      if (!drag) return;
      if (box) {
        const r = box.getBoundingClientRect();
        const edge = Math.min(54, r.height / 4);
        let dy = 0;
        if (lastPt.y < r.top + edge) dy = -Math.ceil((r.top + edge - lastPt.y) / 5);
        else if (lastPt.y > r.bottom - edge) dy = Math.ceil((lastPt.y - (r.bottom - edge)) / 5);
        if (dy) {
          box.scrollTop += dy;
          update();
        }
      }
      scrollRaf = requestAnimationFrame(tick_);
    };
    scrollRaf = requestAnimationFrame(tick_);
  }
  function endDrag(): void {
    removeEventListener('pointermove', onMove);
    removeEventListener('pointerup', onUp);
    removeEventListener('pointercancel', onCancel);
    clearTimeout(expandTimer);
    cancelAnimationFrame(scrollRaf);
    document.body.classList.remove('ft-dnd');
    ghost = null;
    line = null;
  }
  function onUp(): void {
    const d = drag;
    const dr = drop;
    endDrag();
    start = null;
    drag = null;
    drop = null;
    if (!d || !dr) return;
    if (!dr.ok) return showToast(dr.why);
    if (d.k === 'f') void moveFolder(d.id as number, dr.parent, dr.before);
    else void moveServer(d.id as string, dr.pos === 'bucket' ? null : dr.target);
  }
  function onCancel(): void {
    endDrag();
    start = null;
    drag = null;
    drop = null;
  }
  const isSrc = (r: Row): boolean => {
    if (!drag) return false;
    if (drag.k === 'f') {
      const ids = subtreeIds(folders, drag.id as number);
      return (r.t === 'f' && ids.has(r.f.id)) || (r.t === 's' && r.s.group_id !== null && ids.has(r.s.group_id));
    }
    return r.t === 's' && r.s.id === drag.id;
  };
  $effect(() => {
    if (!flash) return;
    const k = flash;
    const timer = setTimeout(() => { if (flash === k) flash = ''; }, 1400);
    return () => clearTimeout(timer);
  });

  function focusSelect(node: HTMLInputElement): void {
    node.focus();
    node.select();
  }
  function pickWhy(id: number): string {
    if (!pick || pick.kind !== 'f') return '';
    const p = moveProblem(folders, pick.id as number, id);
    return p === 'self' ? t('fld.why.selfShort') : p === 'depth' ? t('fld.why.depthShort') : p === 'name' ? t('fld.why.nameShort') : '';
  }
  function onPicked(id: number | null): void {
    const p = pick;
    pick = null;
    if (!p) return;
    if (p.kind === 'f') void moveFolder(p.id as number, id, null);
    else void moveServer(p.id as string, id);
  }
  const pickCurrent = $derived(pick ? (pick.kind === 'f' ? (byId.get(pick.id as number)?.parent_id ?? null) : (servers.find((s) => s.id === pick!.id)?.group_id ?? null)) : null);
  const pickTitle = $derived(pick ? t('fld.pick.title', { name: pick.kind === 'f' ? (byId.get(pick.id as number)?.name ?? '') : (servers.find((s) => s.id === pick!.id)?.name ?? '') }) : '');
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && drag && onCancel()} />

<div class="set-sec ft-sec">
  <h3>{t('fld.title')}</h3>
  <p class="dim">{t('fld.hint', { n: MAX_FOLDER_DEPTH })}</p>
  <div class="ft-bar">
    <button class="btn primary" type="button" disabled={busy} onclick={() => startDraft(null)}><Icon name="folder-plus" size={15} />{t('fld.add')}</button>
    <SearchField placeholder={t('fld.search')} bind:value={filter} spellcheck={false} />
    <button class="btn ft-tgl" type="button" aria-pressed={showServers} onclick={() => (showServers = !showServers)}><Icon name={showServers ? 'check' : 'minus'} size={14} />{t('fld.showServers')}</button>
    <button class="btn icon" type="button" title={t('fld.expandAll')} aria-label={t('fld.expandAll')} onclick={() => { closed = {}; saveClosed(); }}><Icon name="chevrons-up-down" size={15} /></button>
    <button class="btn icon" type="button" title={t('fld.collapseAll')} aria-label={t('fld.collapseAll')} onclick={() => { closed = { u: true, ...Object.fromEntries(folders.map((g) => [`f${g.id}`, true])) }; saveClosed(); }}><Icon name="chevrons-down-up" size={15} /></button>
    <span class="ft-meta">{t('fld.meta', { f: folders.length, s: servers.length })}</span>
  </div>

  <!-- svelte-ignore a11y_no_noninteractive_element_interactions, a11y_interactive_supports_focus -->
  <div class="ft" role="tree" aria-label={t('fld.title')} bind:this={treeEl} onpointerdown={onPointerDown} onkeydown={onKey} oncontextmenu={(e) => { const el = (e.target as HTMLElement).closest<HTMLElement>('.ft-r'); const r = el ? rowOf(el.dataset.key ?? '') : undefined; if (r && r.t !== 'u' && r.t !== 'n') { e.preventDefault(); rowMenu(r, e.clientX, e.clientY); } }}>
    {#if !loaded}
      <div class="ft-empty">{t('dash.loading')}</div>
    {:else if folders.length === 0 && !draft && (!showServers || servers.length === 0)}
      <div class="ft-empty"><Icon name="folders" size={28} /><div>{t('fld.empty')}</div><button class="btn primary" type="button" onclick={() => startDraft(null)}><Icon name="folder-plus" size={15} />{t('fld.addFirst')}</button></div>
    {:else if rows.length === 0}
      <div class="ft-empty"><Icon name="search" size={24} /><div>{t('fld.nothing')}</div></div>
    {:else}
      {#each rows as r (r.key)}
        {#if r.t === 'f'}
          {@const f = r.f}
          {@const open = isOpen(r.key)}
          {@const has = f.children.length > 0 || (showServers && servers.some((s) => s.group_id === f.id))}
          {@const gs = grants.get(f.id) ?? []}
          {@const dim = drop && drop.row === r.key}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <div class="ft-r f" class:src={isSrc(r)} class:into={dim && drop?.ok && drop.pos === 'into'} class:bad={dim && !drop?.ok} class:flash={flash === r.key} class:kb={focus === r.key} data-k="f" data-id={f.id} data-key={r.key} tabindex={focus === r.key ? 0 : -1} role="treeitem" aria-selected={focus === r.key} aria-level={f.depth + 1} aria-expanded={has ? open : undefined} onclick={(e) => onClickRow(e, r)} ondblclick={(e) => { if (!(e.target as HTMLElement).closest('button,input')) startRename(f); }}>
            <span class="ft-gr" title={t('fld.drag')}><Icon name="grip-vertical" size={14} /></span>
            {#each Array(f.depth) as _, i (i)}<span class="ft-g"></span>{/each}
            <button class="ft-cv" class:open class:nil={!has} type="button" data-a="tog" tabindex="-1" aria-label={open ? t('fld.collapse') : t('fld.expand')}><Icon name="chevron-right" size={14} /></button>
            <span class="ft-ico"><Icon name={open && has ? 'folder-open' : 'folder'} size={16} /></span>
            {#if renaming && renaming.id === f.id}
              <input class="ft-rn" class:err={renaming.error} use:focusSelect maxlength={MAX_FOLDER_NAME} aria-label={t('fld.rename')} bind:value={renaming.value} oninput={() => renaming && (renaming.error = '')} onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); void commitRename(f); } else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); void commitRename(f, true); } }} onblur={() => void commitRename(f)} />
              {#if renaming.error}<span class="ft-er">{renaming.error}</span>{/if}
            {:else}
              {@const [a, m, b] = parts(f.name)}
              <span class="ft-nm">{a}{#if m}<mark>{m}</mark>{b}{/if}</span>
              <span class="ft-sp"></span>
              {#if gs.length}<span class="ft-pill acc" title={t('fld.grants', { users: gs.join(', ') })}><Icon name="user" size={12} />{gs.length}</span>{/if}
              <span class="ft-pill" title={t('fld.countTitle')}><Icon name="server" size={12} />{counts.get(f.id) ?? 0}</span>
              <span class="ft-ra">
                <button class="btn xs" type="button" data-a="add" title={t('fld.menu.add')} aria-label={t('fld.menu.add')}><Icon name="folder-plus" size={15} /></button>
                <button class="btn xs" type="button" data-a="ren" title={t('fld.menu.rename')} aria-label={t('fld.menu.rename')}><Icon name="pencil" size={14} /></button>
                <button class="btn xs" type="button" data-a="menu" title={t('fld.more')} aria-label={t('fld.more')}><Icon name="ellipsis" size={15} /></button>
              </span>
            {/if}
          </div>
        {:else if r.t === 's'}
          {@const s = r.s}
          {@const [a, m, b] = parts(s.name)}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <div class="ft-r s" class:src={isSrc(r)} class:flash={flash === r.key} class:kb={focus === r.key} data-k="s" data-id={s.id} data-key={r.key} tabindex={focus === r.key ? 0 : -1} role="treeitem" aria-selected={focus === r.key} aria-level={r.depth + 1} onclick={(e) => onClickRow(e, r)}>
            <span class="ft-gr" title={t('fld.drag')}><Icon name="grip-vertical" size={14} /></span>
            {#each Array(r.depth) as _, i (i)}<span class="ft-g"></span>{/each}
            <span class="ft-cv nil"></span>
            <span class="ft-ico"><Icon name="server" size={15} /></span>
            <span class="ft-nm">{a}{#if m}<mark>{m}</mark>{b}{/if}</span>
            <span class="ft-os" class:win={osTag(s) === 'WIN'}>{osTag(s)}</span>
            <span class="ft-addr">{s.address}</span>
            <span class="ft-sp"></span>
            <span class="ft-ra"><button class="btn xs" type="button" data-a="menu" title={t('fld.more')} aria-label={t('fld.more')}><Icon name="ellipsis" size={15} /></button></span>
          </div>
        {:else if r.t === 'n'}
          <div class="ft-r n" data-k="n" data-key="n">
            <span class="ft-gr"></span>
            {#each Array(r.depth) as _, i (i)}<span class="ft-g"></span>{/each}
            <span class="ft-cv nil"></span>
            <span class="ft-ico"><Icon name="folder" size={16} /></span>
            {#if draft}
              <input class="ft-rn" class:err={draft.error} use:focusSelect maxlength={MAX_FOLDER_NAME} placeholder={t('fld.newName')} aria-label={t('fld.newName')} bind:value={draft.value} oninput={() => draft && (draft.error = '')} onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); void commitDraft(); } else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); void commitDraft(true); } }} onblur={() => void commitDraft(!draft?.value.trim())} />
              {#if draft.error}<span class="ft-er">{draft.error}</span>{/if}
            {/if}
          </div>
        {:else}
          {@const dim = drop && drop.row === 'u'}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <div class="ft-r u" class:into={dim && drop?.ok && drop.pos === 'bucket'} class:bad={dim && !drop?.ok} class:kb={focus === 'u'} data-k="u" data-key="u" tabindex={focus === 'u' ? 0 : -1} role="treeitem" aria-selected={focus === 'u'} aria-level="1" aria-expanded={free.length ? !closed.u : undefined} onclick={(e) => onClickRow(e, r)}>
            <span class="ft-gr"></span>
            <button class="ft-cv" class:open={q || !closed.u} class:nil={free.length === 0} type="button" data-a="tog" tabindex="-1" aria-label={closed.u ? t('fld.expand') : t('fld.collapse')}><Icon name="chevron-right" size={14} /></button>
            <span class="ft-ico"><Icon name="folder-input" size={16} /></span>
            <span class="ft-nm">{t('fld.noFolder')}</span>
            <span class="ft-sp"></span>
            <span class="ft-pill" title={t('fld.noFolderTitle')}><Icon name="server" size={12} />{free.length}</span>
          </div>
        {/if}
      {/each}
    {/if}
    {#if line}<div class="ft-line" class:bad={drop && !drop.ok} style="top:{line.top}px;left:{line.left}px;right:8px"></div>{/if}
  </div>

  <div class="ft-help">
    <span><Icon name="grip-vertical" size={13} /><span>{t('fld.help.drag')}</span></span>
    <span><kbd>F2</kbd>{t('fld.help.rename')}</span>
    <span><kbd>Alt</kbd>+<kbd>↑</kbd><kbd>↓</kbd>{t('fld.help.order')}</span>
    <span><kbd>Del</kbd>{t('fld.help.delete')}</span>
  </div>
</div>

{#if ghost}
  <div class="ft-ghost" class:bad={drop && !drop.ok} style="left:{ghost.x}px;top:{ghost.y}px"><Icon name={ghost.icon} size={15} /><span>{ghost.label}</span>{#if ghost.n}<b title={t('fld.withContent')}>+{ghost.n}</b>{/if}{#if drop && !drop.ok}<span class="why">{drop.why}</span>{/if}</div>
{/if}
{#if undo}
  <div class="undo-bar" role="status" aria-live="polite"><span>{undo.text}</span><button class="btn sm" type="button" onclick={() => void doUndo()}>{t('fld.undo')}</button></div>
{/if}
{#if menu}<ContextMenu x={menu.x} y={menu.y} items={menu.items} onclose={() => (menu = null)} />{/if}
{#if pick}
  <FolderPickModal title={pickTitle} {folders} current={pickCurrent} noneLabel={pick.kind === 'f' ? t('fld.top') : t('fld.noFolder')} why={pickWhy} onpick={onPicked} onclose={() => (pick = null)} />
{/if}
