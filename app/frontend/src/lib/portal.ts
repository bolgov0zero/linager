// A Svelte action that moves an element out of its place in the markup, to the nearest ancestor of `near` that matches `into`
// (or to the body). Why: `position: fixed` is measured from the viewport only while no ancestor has a transform, a filter or a
// backdrop-filter; the floating bar over the full screen picture has both, so a popup opened inside it was drawn shifted by the
// place of the bar (the unlock window of the keys stood at the side of the key button). Outside the bar its coordinates, taken
// from getBoundingClientRect of the button, are right.
export interface PortalOptions {
  /** The element the popup belongs to (its button). */
  near: HTMLElement | null;
  /** The ancestor of `near` to move into (default: the pane of the full screen, so that the popup stays over the picture). */
  into?: string;
}

export function portal(node: HTMLElement, o: PortalOptions): { update: (o: PortalOptions) => void; destroy: () => void } {
  const move = (opts: PortalOptions): void => {
    const target = (opts.near?.closest(opts.into ?? '.asst-pane') as HTMLElement | null) ?? document.body;
    if (node.parentElement !== target) target.appendChild(node);
  };
  move(o);
  return {
    update: move,
    destroy: () => node.remove(),
  };
}
