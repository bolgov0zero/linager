// A list whose rows are reordered by dragging (a Svelte action). The rows are the direct children of the element; the gesture is
// the same as in Settings → Folders: take a row anywhere except on its buttons, the line shows where it will go, Escape cancels.
// The keyboard has no buttons to press: Alt+Up and Alt+Down move the focused row (give the rows tabindex="0").
// Nothing is added inside the list (the framework owns it): the ghost and the line live in the body and follow the pointer.

export interface SortableOptions {
  /** The row at index `from` goes to index `to` of the list as it will be after the move. */
  onreorder: (from: number, to: number) => void;
}

const SKIP = 'button,a,input,textarea,select,label,[data-nosort]';

/** Where a row dropped at `y` goes: the number of the other rows that lie above it. */
export function dropIndex(mids: readonly number[], y: number): number {
  return mids.filter((m) => m < y).length;
}

export function sortable(node: HTMLElement, options: SortableOptions): { update: (o: SortableOptions) => void; destroy: () => void } {
  let opts = options;
  let start: { x: number; y: number; row: HTMLElement } | null = null;
  let drag: { row: HTMLElement; from: number; to: number } | null = null;
  let ghost: HTMLElement | null = null;
  let line: HTMLElement | null = null;

  const rows = (): HTMLElement[] => [...node.children].filter((e): e is HTMLElement => e instanceof HTMLElement);

  function down(e: PointerEvent): void {
    if (e.button !== 0 || !(e.target instanceof Element) || e.target.closest(SKIP)) return;
    const row = rows().find((r) => r.contains(e.target as Node));
    if (!row) return;
    start = { x: e.clientX, y: e.clientY, row };
    addEventListener('pointermove', move);
    addEventListener('pointerup', up);
    addEventListener('pointercancel', cancel);
  }

  function begin(): void {
    if (!start) return;
    const from = rows().indexOf(start.row);
    drag = { row: start.row, from, to: from };
    start.row.classList.add('sort-src');
    const r = start.row.getBoundingClientRect();
    // the copy of the row sits in a list of its own that has the classes of this one, so that it looks the same
    const copy = start.row.cloneNode(true) as HTMLElement;
    copy.classList.remove('sort-src'); // the copy was made after the row was dimmed
    copy.removeAttribute('tabindex');
    ghost = document.createElement(node.tagName);
    ghost.className = `${node.className} sort-ghost`;
    ghost.append(copy);
    ghost.style.width = `${r.width}px`;
    line = document.createElement('div');
    line.className = 'sort-line';
    document.body.append(ghost, line);
    document.body.classList.add('sorting');
  }

  function move(e: PointerEvent): void {
    if (!drag) {
      if (!start || Math.hypot(e.clientX - start.x, e.clientY - start.y) < 5) return;
      begin();
    }
    if (!drag || !ghost || !line) return;
    const r = drag.row.getBoundingClientRect();
    ghost.style.left = `${node.getBoundingClientRect().left}px`;
    ghost.style.top = `${e.clientY - r.height / 2}px`;
    const others = rows().filter((x) => x !== drag!.row);
    drag.to = dropIndex(others.map((x) => { const b = x.getBoundingClientRect(); return b.top + b.height / 2; }), e.clientY);
    const nb = node.getBoundingClientRect();
    const y = drag.to < others.length ? others[drag.to].getBoundingClientRect().top - 3 : (others.at(-1)?.getBoundingClientRect().bottom ?? nb.top) + 3;
    line.style.cssText = `left:${nb.left}px;width:${nb.width}px;top:${y - 1}px`;
    line.classList.toggle('same', drag.to === drag.from);
  }

  function end(): void {
    removeEventListener('pointermove', move);
    removeEventListener('pointerup', up);
    removeEventListener('pointercancel', cancel);
    drag?.row.classList.remove('sort-src');
    ghost?.remove();
    line?.remove();
    ghost = line = null;
    document.body.classList.remove('sorting');
  }

  function up(): void {
    const d = drag;
    end();
    start = null;
    drag = null;
    if (d && d.to !== d.from) opts.onreorder(d.from, d.to);
  }

  function cancel(): void {
    end();
    start = null;
    drag = null;
  }

  function key(e: KeyboardEvent): void {
    if (!e.altKey || (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') || !(e.target instanceof HTMLElement)) return;
    const list = rows();
    const from = list.indexOf(e.target);
    if (from < 0) return;
    const to = from + (e.key === 'ArrowUp' ? -1 : 1);
    if (to < 0 || to >= list.length) return;
    e.preventDefault();
    opts.onreorder(from, to);
  }
  const esc = (e: KeyboardEvent): void => {
    if (e.key === 'Escape' && drag) cancel();
  };

  node.addEventListener('pointerdown', down);
  node.addEventListener('keydown', key);
  addEventListener('keydown', esc);
  return {
    update: (o) => (opts = o),
    destroy: () => {
      cancel();
      node.removeEventListener('pointerdown', down);
      node.removeEventListener('keydown', key);
      removeEventListener('keydown', esc);
    },
  };
}
