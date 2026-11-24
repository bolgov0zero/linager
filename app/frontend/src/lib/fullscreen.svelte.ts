import { native } from './native';

/**
 * The full screen of a pane (the remote desktop or the Assistant): the picture takes the whole window and the window itself goes to
 * the full screen (the system window of the application; in a browser, the full screen of the page). The state is the pane's own:
 * it also ends when the system leaves the full screen (the green button, Esc in a browser) or when the pane is hidden.
 * Call it while a component is being created.
 */
export function createFullscreen(opts: { native: () => boolean; root: () => HTMLElement | null }): { readonly on: boolean; set: (on: boolean) => Promise<void>; toggle: () => Promise<void> } {
  let on = $state(false);

  async function apply(v: boolean): Promise<void> {
    try {
      if (opts.native()) await native.fullscreenSet(v);
      else if (v) await document.documentElement.requestFullscreen();
      else if (document.fullscreenElement) await document.exitFullscreen();
    } catch {
      // the layout alone (a window that cannot change, a browser that refuses)
    }
  }

  async function set(v: boolean): Promise<void> {
    if (v === on) return;
    on = v;
    await apply(v);
  }

  // the system left the full screen on its own
  let timer: ReturnType<typeof setTimeout> | undefined;
  function check(): void {
    clearTimeout(timer);
    timer = setTimeout(async () => {
      if (!on) return;
      try {
        const still = opts.native() ? (await native.fullscreenGet()).on : !!document.fullscreenElement;
        if (!still) on = false;
      } catch {
        // unknown: the pane stays as it is
      }
    }, 350);
  }

  $effect(() => {
    if (!on) return;
    window.addEventListener('resize', check);
    document.addEventListener('fullscreenchange', check);
    const root = opts.root();
    // a hidden pane (another tab) has no size: the window must not stay in the full screen without its panel
    const ro = new ResizeObserver(() => {
      if (root && root.getClientRects().length === 0) void set(false);
    });
    if (root) ro.observe(root);
    return () => {
      window.removeEventListener('resize', check);
      document.removeEventListener('fullscreenchange', check);
      ro.disconnect();
    };
  });
  $effect(() => () => {
    clearTimeout(timer);
    if (on) void apply(false);
  });

  return {
    get on() {
      return on;
    },
    set,
    toggle: () => set(!on),
  };
}
