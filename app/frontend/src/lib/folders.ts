// The folders of the servers: a flat list from the server (with parent_id) is turned into a tree here. Every place that shows
// folders (the filter of the dashboard, the Console, the access of a user, the keys) goes through these helpers.
import type { Group } from './api';

export const MAX_FOLDER_DEPTH = 8;
export const MAX_FOLDER_NAME = 40;

export interface FolderNode extends Group {
  /** 0 for a folder of the top level. */
  depth: number;
  /** The names from the top level down, joined by " / ". */
  path: string;
  children: FolderNode[];
}

/** The tree, in the order of the list: the top level first, every folder followed by its subfolders (depth first). */
export function buildTree(list: readonly Group[]): FolderNode[] {
  const byParent = new Map<number | null, Group[]>();
  const ids = new Set(list.map((g) => g.id));
  for (const g of list) {
    // a folder whose parent is unknown (a stale list) is shown on the top level rather than lost
    const p = g.parent_id != null && ids.has(g.parent_id) ? g.parent_id : null;
    byParent.set(p, [...(byParent.get(p) ?? []), g]);
  }
  const make = (g: Group, depth: number, parentPath: string, seen: Set<number>): FolderNode => {
    const path = parentPath ? `${parentPath} / ${g.name}` : g.name;
    const next = new Set(seen).add(g.id);
    return { ...g, depth, path, children: (byParent.get(g.id) ?? []).filter((c) => !next.has(c.id)).map((c) => make(c, depth + 1, path, next)) };
  };
  return (byParent.get(null) ?? []).map((g) => make(g, 0, '', new Set()));
}

/** The tree as one list in display order (a folder, then everything under it). */
export function flatten(list: readonly Group[]): FolderNode[] {
  const out: FolderNode[] = [];
  const walk = (nodes: FolderNode[]): void => {
    for (const n of nodes) {
      out.push(n);
      walk(n.children);
    }
  };
  walk(buildTree(list));
  return out;
}

/** The id of the folder and of every folder under it. */
export function subtreeIds(list: readonly Group[], id: number): Set<number> {
  const out = new Set<number>([id]);
  for (let grew = true; grew; ) {
    grew = false;
    for (const g of list) {
      if (g.parent_id != null && out.has(g.parent_id) && !out.has(g.id)) {
        out.add(g.id);
        grew = true;
      }
    }
  }
  return out;
}

/** The union of the subtrees of several folders. */
export function subtreesOf(list: readonly Group[], ids: Iterable<number>): Set<number> {
  const out = new Set<number>();
  for (const id of ids) for (const x of subtreeIds(list, id)) out.add(x);
  return out;
}

/** The folder and the folders above it, the folder first. */
export function chainOf(list: readonly Group[], id: number | null): number[] {
  const byId = new Map(list.map((g) => [g.id, g]));
  const out: number[] = [];
  for (let cur = id == null ? undefined : byId.get(id); cur && !out.includes(cur.id); cur = cur.parent_id == null ? undefined : byId.get(cur.parent_id)) out.push(cur.id);
  return out;
}

export function pathOf(list: readonly Group[], id: number | null): string {
  if (id == null) return '';
  return flatten(list).find((n) => n.id === id)?.path ?? '';
}

/** How many levels the subtree of a folder has, the folder itself included. */
export function heightOf(list: readonly Group[], id: number): number {
  const kids = list.filter((g) => g.parent_id === id);
  return 1 + Math.max(0, ...kids.map((k) => heightOf(list, k.id)));
}

/** What would stop `id` from being put under `parent` (null = the top level); '' when it is allowed. */
export type MoveProblem = '' | 'self' | 'depth' | 'name';
export function moveProblem(list: readonly Group[], id: number, parent: number | null): MoveProblem {
  const me = list.find((g) => g.id === id);
  if (!me) return '';
  if (parent != null && subtreeIds(list, id).has(parent)) return 'self';
  const top = parent == null ? 0 : chainOf(list, parent).length;
  if (top + heightOf(list, id) > MAX_FOLDER_DEPTH) return 'depth';
  const clash = list.some((g) => g.id !== id && (g.parent_id ?? null) === parent && g.name.toLowerCase() === me.name.toLowerCase());
  return clash ? 'name' : '';
}

/** The entries of a select: a "none" entry first, then the folders in tree order, each with its depth (for the indent) and its path. */
export function folderOptions(list: readonly Group[], none: string, noneValue = '0'): { v: string; t: string; d?: number; b?: string; i?: string }[] {
  // a folder has the icon of a folder (an open one when something lies in it), the entry "none" the icon of all the folders
  const hasKids = new Set(list.map((g) => g.parent_id).filter((x): x is number => x != null));
  return [{ v: noneValue, t: none, i: 'folders' }, ...flatten(list).map((n) => ({ v: String(n.id), t: n.name, d: n.depth, b: n.path, i: hasKids.has(n.id) ? 'folder-open' : 'folder' }))];
}
