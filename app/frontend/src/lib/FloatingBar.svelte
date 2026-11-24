<script lang="ts">
  import type { Snippet } from 'svelte';
  import { t } from './i18n.svelte';
  import Icon from './Icon.svelte';

  // The panel of buttons over a full screen picture: an island that can be dragged and folded into a small square with an
  // arrow. Where it stands and whether it is folded is remembered until the application is closed.
  let { children }: { children: Snippet } = $props();

  const saved = remembered;
  let el = $state<HTMLDivElement | null>(null);
  let collapsed = $state(saved.collapsed);
  let pos = $state<{ x: number; y: number } | null>(saved.pos);

  function clamp(x: number, y: number): { x: number; y: number } {
    const p = el?.offsetParent as HTMLElement | null;
    const w = p?.clientWidth ?? window.innerWidth;
    const h = p?.clientHeight ?? window.innerHeight;
    const ew = el?.offsetWidth ?? 0;
    const eh = el?.offsetHeight ?? 0;
    return { x: Math.min(Math.max(0, x), Math.max(0, w - ew)), y: Math.min(Math.max(0, y), Math.max(0, h - eh)) };
  }

  // the first time: the middle of the top edge
  $effect(() => {
    if (!el || pos) return;
    const p = el.offsetParent as HTMLElement | null;
    pos = clamp(((p?.clientWidth ?? window.innerWidth) - el.offsetWidth) / 2, 12);
  });
  // the window changed its size: the panel stays inside
  $effect(() => {
    const on = (): void => {
      if (pos) pos = clamp(pos.x, pos.y);
    };
    window.addEventListener('resize', on);
    return () => window.removeEventListener('resize', on);
  });
  $effect(() => {
    saved.collapsed = collapsed;
    saved.pos = pos;
  });

  let drag: { dx: number; dy: number; sx: number; sy: number; moved: boolean } | null = null;
  function down(e: PointerEvent): void {
    if (e.button !== 0 || !el || !pos) return;
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    drag = { dx: e.clientX - pos.x, dy: e.clientY - pos.y, sx: e.clientX, sy: e.clientY, moved: false };
  }
  function move(e: PointerEvent): void {
    if (!drag) return;
    if (!drag.moved && Math.hypot(e.clientX - drag.sx, e.clientY - drag.sy) < 4) return;
    drag.moved = true;
    pos = clamp(e.clientX - drag.dx, e.clientY - drag.dy);
  }
  function up(): void {
    drag = null;
  }
  function toggleSquare(): void {
    // a drag of the square is not a click
    if (drag?.moved) return;
    collapsed = false;
  }
</script>

<div class="fb" class:sq={collapsed} bind:this={el} style:left="{pos?.x ?? 0}px" style:top="{pos?.y ?? 12}px" style:visibility={pos ? 'visible' : 'hidden'}>
  {#if collapsed}
    <button class="fb-sq" type="button" title={t('fs.show')} aria-label={t('fs.show')} onpointerdown={down} onpointermove={move} onpointerup={up} onclick={toggleSquare}><Icon name="chevron-down" size={16} /></button>
  {:else}
    <span class="fb-grip" role="presentation" title={t('fs.drag')} onpointerdown={down} onpointermove={move} onpointerup={up}><Icon name="grip-horizontal" size={16} /></span>
    {@render children()}
    <button class="btn sm icon" type="button" title={t('fs.hide')} aria-label={t('fs.hide')} onclick={() => (collapsed = true)}><Icon name="chevron-up" size={14} /></button>
  {/if}
</div>

<script lang="ts" module>
  // kept while the application runs: the next full screen finds the panel where it was left
  const remembered: { collapsed: boolean; pos: { x: number; y: number } | null } = { collapsed: false, pos: null };
</script>
