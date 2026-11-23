<script lang="ts">
  import type { Snippet } from 'svelte';
  import { tr } from '../i18n';
  import PixIcon from './ui/PixIcon.svelte';
  import { dropModal, frontModal, MODAL_Z, modalLevel, pushModal, raiseModal } from './ui/modalstack';
  import { lockScroll } from './ui/scrolllock';
  import { isPhone } from './ui/viewport';

  let {
    title,
    labelId,
    onclose,
    wide = false,
    medium = false,
    flush = false,
    fixed = false,
    onescape,
    actions,
    children,
  }: {
    title: string;
    labelId: string;
    onclose: () => void;
    /** A large dialog (settings). */
    wide?: boolean;
    /** A dialog with two columns of fields. */
    medium?: boolean;
    /** No padding around the content (lists of actions on a phone). */
    flush?: boolean;
    /** A large dialog of a fixed height: the body does not scroll, the content inside it does (Add server). */
    fixed?: boolean;
    /** What the Escape key does; closing the dialog by default. */
    onescape?: () => void;
    /** Buttons in the header, left of the close button. */
    actions?: Snippet;
    children: Snippet;
  } = $props();

  let dialog = $state<HTMLDivElement | undefined>(undefined);

  // A dialog is a floating window: it moves by its header and the page behind it stays alive. On a phone it is a sheet.
  const id = pushModal();
  let offset = $state({ x: 0, y: 0 });
  let dragging = $state(false);
  const z = $derived(MODAL_Z + Math.max(0, $modalLevel(id)));
  $effect(() => () => dropModal(id));

  function startDrag(event: PointerEvent): void {
    if (event.button !== 0 || $isPhone || !dialog) return;
    if ((event.target as HTMLElement).closest('button, a, input, select, textarea')) return;
    event.preventDefault();
    const handle = event.currentTarget as HTMLElement;
    handle.setPointerCapture(event.pointerId);
    const rect = dialog.getBoundingClientRect();
    const start = { px: event.clientX, py: event.clientY, ...offset };
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    dragging = true;
    const move = (e: PointerEvent) => {
      // The header stays reachable: at least 80 px of it stay on the screen and its top never leaves it.
      const left = Math.min(Math.max(rect.left + e.clientX - start.px, 80 - rect.width), vw - 80);
      const top = Math.min(Math.max(rect.top + e.clientY - start.py, 0), vh - 40);
      offset = { x: start.x + left - rect.left, y: start.y + top - rect.top };
    };
    const end = () => {
      dragging = false;
      handle.removeEventListener('pointermove', move);
      handle.removeEventListener('pointerup', end);
      handle.removeEventListener('pointercancel', end);
    };
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', end);
    handle.addEventListener('pointercancel', end);
  }

  $effect(() => {
    dialog?.focus();
  });

  // Phone: the dialog is a sheet over the page; the page behind it must not scroll with the finger.
  $effect(() => {
    if (!$isPhone) return;
    return lockScroll();
  });
</script>

<svelte:window
  onkeydown={(event) => {
    // One Escape closes one dialog: the stack changes between the handlers of the same event, so the first one marks it as taken.
    if (event.key !== 'Escape' || $frontModal !== id || event.defaultPrevented) return;
    event.preventDefault();
    (onescape ?? onclose)();
  }}
/>

<div class="modal-layer" style:z-index={z}>
  {#if $isPhone}
    <button class="backdrop" type="button" tabindex="-1" aria-label={$tr('common.close')} onclick={onclose}
    ></button>
  {/if}
  <div
    class="dialog"
    class:wide
    class:medium
    class:fixed
    class:dragging
    role="dialog"
    aria-modal="false"
    aria-labelledby={labelId}
    tabindex="-1"
    bind:this={dialog}
    style:transform={offset.x || offset.y ? `translate(${offset.x}px, ${offset.y}px)` : undefined}
    onpointerdowncapture={() => raiseModal(id)}
  >
    <header class="dialog-head" role="presentation" onpointerdown={startDrag}>
      <h2 id={labelId}>{title}</h2>
      <div class="dialog-tools">
        {#if actions}{@render actions()}{/if}
        <button class="btn sm icon" type="button" aria-label={$tr('common.close')} onclick={onclose}><PixIcon name="x" size={16} /></button>
      </div>
    </header>
    <div class="dialog-body" class:flush class:fixed>
      {@render children()}
    </div>
  </div>
</div>

<style>
  .modal-layer {
    position: fixed;
    inset: 0;
    display: flex;
    align-items: flex-start;
    justify-content: center;
    padding: 43px 14px 14px;
    pointer-events: none; /* the page behind a dialog stays alive; only the dialog takes the pointer */
  }

  .backdrop {
    position: absolute;
    inset: 0;
    border: 0;
    padding: 0;
    cursor: default;
    background: color-mix(in srgb, var(--ink) 55%, transparent);
  }

  .dialog {
    position: relative;
    display: flex;
    flex-direction: column;
    width: 100%;
    max-width: 504px;
    max-height: calc(100dvh - 58px);
    pointer-events: auto;
    background: var(--surface);
    border: 2px solid var(--line);
    box-shadow: 4px 4px 0 var(--shade);
    outline: none;
  }

  .dialog.dragging {
    user-select: none;
  }

  .dialog.medium {
    max-width: 828px;
  }

  .dialog.wide {
    max-width: min(1224px, 100%);
  }

  .dialog.fixed {
    max-width: min(1008px, 100%);
    height: min(684px, calc(100dvh - 58px));
  }

  .dialog-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 7px;
    padding: 5px 7px 5px 11px;
    border-bottom: 2px solid var(--line);
    background: var(--seg-off);
    flex: none;
    cursor: grab;
    touch-action: none;
  }

  .dialog.dragging .dialog-head {
    cursor: grabbing;
  }

  .dialog-tools {
    display: flex;
    align-items: stretch; /* the pencil and the cross are of one height */
    gap: 4px;
  }

  .dialog-tools :global(.btn:not(.icon)) {
    padding: 2px 7px;
    font-size: 11px;
    line-height: 1.4;
    box-shadow: 2px 2px 0 var(--shade);
  }

  .dialog-head h2 {
    margin: 0;
    font-size: 14px;
    line-height: 1.1;
  }

  .dialog-body {
    padding: 14px;
    display: flex;
    flex-direction: column;
    gap: 14px;
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
  }

  .dialog-body.fixed {
    flex: 1 1 auto;
    min-height: 0;
    padding: 0;
    gap: 0;
    overflow: hidden;
  }

  /* Phones: every dialog is a sheet from the bottom edge (a large one fills the screen); only its body scrolls. */
  @media (max-width: 720px) {
    .modal-layer {
      align-items: flex-end;
      padding: 0;
      overflow: hidden;
      pointer-events: auto;
    }

    .dialog-head {
      cursor: auto;
      touch-action: auto;
    }

    .dialog,
    .dialog.medium,
    .dialog.wide {
      display: flex;
      flex-direction: column;
      max-width: none;
      max-height: 92dvh;
      border-width: 2px 0 0;
      box-shadow: 0 -4px 0 var(--shade);
    }

    .dialog.wide,
    .dialog.fixed {
      height: calc(100dvh - 11px);
      max-height: none;
    }

    .dialog.fixed {
      max-width: none;
    }

    .dialog-head {
      flex: none;
      min-height: 52px;
      padding: 4px 5px 4px 13px;
    }

    .dialog-head .btn {
      width: 44px;
      padding: 0;
    }

    .dialog-body {
      flex: 1 1 auto;
      min-height: 0;
      gap: 11px;
      overflow-y: auto;
      overscroll-behavior: contain;
      padding: 11px 11px 0;
    }

    /* The room under the content (home indicator): a spacer, not padding, so that a pinned button bar can sit
       exactly on the bottom edge of the sheet. */
    .dialog-body::after {
      content: '';
      flex: none;
      height: var(--safe-b);
    }

    .dialog-body.flush {
      padding: 0;
      gap: 0;
    }

    .dialog-body.flush::after {
      height: var(--safe-b);
    }

    .dialog-body.fixed {
      padding: 0;
      overflow: hidden;
    }

    .dialog-body.fixed::after {
      display: none;
    }

    /* The buttons at the end of a dialog stay in reach while its content scrolls. */
    .dialog-body > :global(.row.end:last-child),
    .dialog-body > :global(form > .row.end:last-child) {
      position: sticky;
      bottom: 0;
      z-index: 2;
      flex-wrap: nowrap;
      margin: auto -11px 0;
      padding: 9px 14px calc(9px + var(--safe-b));
      background: var(--surface);
      border-top: 2px solid var(--line);
    }

    .dialog-body:has(> :global(.row.end:last-child))::after,
    .dialog-body:has(> :global(form > .row.end:last-child))::after {
      display: none;
    }

    .dialog-body > :global(.row.end:last-child > .btn),
    .dialog-body > :global(form > .row.end:last-child > .btn) {
      flex: 1;
    }
  }
</style>
