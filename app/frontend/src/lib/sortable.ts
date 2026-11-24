// A list whose rows are reordered by dragging (a Svelte action). The rows are the direct children of the element; the gesture is
// the same as in Settings → Folders: take a row anywhere except on its buttons, the line shows where it will go, Escape cancels.
// The keyboard has no buttons to press: Alt+Up and Alt+Down move the focused row (Alt+Left and Alt+Right in a horizontal list; the
// focus may be on the row or on something inside it). A drag does not end in a click on what was taken.
// Nothing is added inside the list (the framework owns it): the ghost and the line live in the body and follow the pointer.

export interface SortableOptions {
  /** The row at index `from` goes to index `to` of the list as it will be after the move. */
  onreorder: (from: number, to: number) => void;
  /** "y" (the default) for a column of rows, "x" for a row of tabs. */
  axis?: 'x' | 'y';
  /** What does not start a drag (the default is every button, link and field); the tabs let the drag start on the tab button itself. */
  skip?: string;
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
    if (e.button !== 0 || !(e.target instanceof Element) || e.target.closest(opts.skip ?? SKIP)) return;
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
    if (opts.axis === 'x') ghost.style.cssText += ';display:flex;overflow:visible'; // the strip of the tabs is a scrolled flex row
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
    const nb = node.getBoundingClientRect();
    if (opts.axis === 'x') {
      // a row of tabs: near an end of the strip it scrolls
      if (node.scrollWidth > node.clientWidth) {
        if (e.clientX < nb.left + 28) node.scrollLeft -= 14;
        else if (e.clientX > nb.right - 28) node.scrollLeft += 14;
      }
      ghost.style.left = `${e.clientX - r.width / 2}px`;
      ghost.style.top = `${r.top}px`;
      const others = rows().filter((x) => x !== drag!.row);
      drag.to = dropIndex(others.map((x) => { const b = x.getBoundingClientRect(); return b.left + b.width / 2; }), e.clientX);
      const x = drag.to < others.length ? others[drag.to].getBoundingClientRect().left - 3 : (others.at(-1)?.getBoundingClientRect().right ?? nb.left) + 3;
      line.style.cssText = `left:${x - 1}px;width:2px;top:${r.top}px;height:${r.height}px`;
    } else {
      ghost.style.left = `${nb.left}px`;
      ghost.style.top = `${e.clientY - r.height / 2}px`;
      const others = rows().filter((x) => x !== drag!.row);
      drag.to = dropIndex(others.map((x) => { const b = x.getBoundingClientRect(); return b.top + b.height / 2; }), e.clientY);
      const y = drag.to < others.length ? others[drag.to].getBoundingClientRect().top - 3 : (others.at(-1)?.getBoundingClientRect().bottom ?? nb.top) + 3;
      line.style.cssText = `left:${nb.left}px;width:${nb.width}px;top:${y - 1}px`;
    }
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
    if (!d) return;
    // the button that was taken must not click when it is let go
    const swallow = (ev: Event): void => ev.stopPropagation();
    addEventListener('click', swallow, { capture: true, once: true });
    setTimeout(() => removeEventListener('click', swallow, { capture: true }), 0);
    if (d.to !== d.from) opts.onreorder(d.from, d.to);
  }

  function cancel(): void {
    end();
    start = null;
    drag = null;
  }

  function key(e: KeyboardEvent): void {
    const keys = opts.axis === 'x' ? ['ArrowLeft', 'ArrowRight'] : ['ArrowUp', 'ArrowDown'];
    if (!e.altKey || !keys.includes(e.key) || !(e.target instanceof Node)) return;
    const list = rows();
    const from = list.findIndex((r) => r.contains(e.target as Node));
    if (from < 0) return;
    const to = from + (e.key === keys[0] ? -1 : 1);
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
