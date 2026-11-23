import { derived, get, writable } from 'svelte/store';

/** The open dialogs from the back to the front. A dialog is a floating window: a click on it brings it to the front. */
const order = writable<number[]>([]);
let nextId = 1;

/** The base z-index of dialogs; the page, the top bar and the floating terminals stay below the first one. */
export const MODAL_Z = 20;

/** Registers a dialog on the top of the stack; returns its id. */
export function pushModal(): number {
  const id = nextId++;
  order.update((list) => [...list, id]);
  return id;
}

export function dropModal(id: number): void {
  order.update((list) => list.filter((x) => x !== id));
}

/** Brings the dialog to the front. */
export function raiseModal(id: number): void {
  const list = get(order);
  if (list[list.length - 1] === id) return;
  order.set([...list.filter((x) => x !== id), id]);
}

/** The place of a dialog in the stack, for the z-index. */
export const modalLevel = derived(order, (list) => (id: number) => list.indexOf(id));

/** The dialog is the front one (Escape closes only that one). */
export const frontModal = derived(order, (list) => list[list.length - 1] ?? 0);
