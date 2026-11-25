// "Picture 100%" shows the remote screen pixel for pixel, and a screen bigger than the window is scrolled by the mouse: when the
// pointer comes to an edge of the window, the picture moves smoothly that way (and stands still in the middle, so the pointer stays
// exact where it is needed). There are no scroll bars. The speed grows from nothing at the inner border of the zone to `max`
// pixels per frame at the edge itself.

/** The scroll speed (pixels per frame, negative to the start) for a pointer at `pos` in a box of `size`; 0 in the middle. */
export function edgeSpeed(pos: number, size: number, zone = 56, max = 22): number {
  if (size <= zone * 2) return 0; // a box with no middle does not scroll: it would run all the time
  if (pos < zone) return -Math.round(max * Math.min(1, (zone - Math.max(0, pos)) / zone));
  if (pos > size - zone) return Math.round(max * Math.min(1, (Math.min(size, pos) - (size - zone)) / zone));
  return 0;
}

export interface EdgeScrollOptions {
  /** The element that scrolls (found each time: the remote desktop client makes its own late). */
  scroller: () => HTMLElement | null;
  /** The mode "100%" is in force. */
  enabled: () => boolean;
  /** The picture moved under a pointer that stood still: the remote pointer is to be told where it is now. */
  moved?: (e: { clientX: number; clientY: number }) => void;
}

/** Starts the edge scrolling inside `host` (the box of the picture); returns the function that stops it. */
export function attachEdgeScroll(host: HTMLElement, o: EdgeScrollOptions): () => void {
  let last: { clientX: number; clientY: number } | null = null;
  let raf = 0;

  function tick(): void {
    raf = 0;
    const sc = o.scroller();
    if (!last || !sc || !o.enabled()) return;
    const r = host.getBoundingClientRect();
    const vx = edgeSpeed(last.clientX - r.left, r.width);
    const vy = edgeSpeed(last.clientY - r.top, r.height);
    if (vx === 0 && vy === 0) return;
    const x = sc.scrollLeft;
    const y = sc.scrollTop;
    sc.scrollBy({ left: vx, top: vy, behavior: 'instant' });
    if (sc.scrollLeft !== x || sc.scrollTop !== y) o.moved?.(last);
    raf = requestAnimationFrame(tick);
  }
  const move = (e: PointerEvent): void => {
    last = { clientX: e.clientX, clientY: e.clientY };
    if (!raf && o.enabled()) raf = requestAnimationFrame(tick);
  };
  const leave = (): void => {
    last = null;
    if (raf) cancelAnimationFrame(raf);
    raf = 0;
  };
  host.addEventListener('pointermove', move);
  host.addEventListener('pointerleave', leave);
  return () => {
    host.removeEventListener('pointermove', move);
    host.removeEventListener('pointerleave', leave);
    leave();
  };
}
