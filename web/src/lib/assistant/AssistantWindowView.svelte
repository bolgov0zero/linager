<script lang="ts">
  import { untrack } from 'svelte';
  import { tr } from '../../i18n';
  import WindowFrame from '../terminal/WindowFrame.svelte';
  import { openQualityWindow, updateTermWindow, type TermWindow } from '../terminal/windows';
  import AssistantBadge from './AssistantBadge.svelte';
  import AssistantSession, { type AssistInfo, type AssistPhase } from './AssistantSession.svelte';
  import { assistInfoShown } from './settings';
  import AssistantStats from './AssistantStats.svelte';
  import AssistantTools from './AssistantTools.svelte';

  let { win, top }: { win: TermWindow; top: boolean } = $props();

  let phase = $state<AssistPhase>('connecting');
  let info = $state<AssistInfo | null>(null);
  let screen = $state<AssistantSession | null>(null);
  // The server never changes for a window; deriving keeps the session from restarting when the store replaces the object.
  const serverId = $derived(win.serverId);
  const serverName = $derived(win.serverName ?? win.name);
  // The session lives in the window record, so that the store can tell which sessions are open already.
  const session = $derived(win.session ?? 'console');

  // How the picture and the window go together. The window follows the size of the picture by default; the person
  // choosing another mode, resizing the window by hand or maximizing it leaves that mode.
  let display = $state<'fit' | 'fill' | 'window' | 'max' | 'real'>('window');
  let boxW = $state(0);
  let boxH = $state(0);
  /** What the window needs around the picture (header, borders, the stats line), measured once. */
  let chrome = $state<{ w: number; h: number } | null>(null);
  let applied = { w: 0, h: 0 };
  const narrow = (): boolean => window.matchMedia('(max-width: 720px)').matches;

  $effect(() => {
    if (!chrome && !win.maximized && boxW > 50 && boxH > 50) chrome = { w: win.w - boxW, h: win.h - boxH };
  });

  /** The picture area the window was fitted for, and whether the picture goes in at 100% or had to be scaled down. */
  let target = $state<{ w: number; h: number } | null>(null);
  let scaleMode = $state<'real' | 'fit'>('real');
  let tries = 0;
  let lastKey = '';

  /**
   * The window gets the size of the picture at 100% and stays inside the page. A picture larger than the page is scaled
   * down as a whole (the window takes its proportions), so that nothing of it is cut off.
   */
  function fitWindow(pw: number, ph: number): void {
    const c = chrome;
    if (!c) return;
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const k = Math.min(1, (vw - 16 - c.w) / pw, (vh - 64 - c.h) / ph);
    const picW = Math.max(1, Math.floor(pw * k));
    const picH = Math.max(1, Math.floor(ph * k));
    const w = Math.max(360, picW + c.w);
    const h = Math.max(220, picH + c.h);
    target = { w: picW, h: picH };
    scaleMode = k >= 1 ? 'real' : 'fit';
    applied = { w, h };
    updateTermWindow(win.key, {
      w,
      h,
      x: Math.max(8, Math.min(win.x, vw - w - 8)),
      y: Math.max(8, Math.min(win.y, vh - h - 56)),
      maximized: false,
    });
  }

  $effect(() => {
    if (display !== 'window' || !info || !chrome || narrow()) return;
    const [pw, ph] = [info.w, info.h];
    const key = `${pw}x${ph}`;
    untrack(() => {
      if (key !== lastKey) {
        lastKey = key;
        tries = 0;
      }
      fitWindow(pw, ph);
    });
  });

  // what the window needs around the picture was measured at the first look; when the real picture area comes out
  // another size (the stats line wrapped in a narrow window), it is measured again and the window is fitted once more
  $effect(() => {
    const [bw, bh] = [boxW, boxH];
    untrack(() => {
      const t = target;
      if (display !== 'window' || !t || win.maximized || narrow() || bw < 50) return;
      if (Math.abs(bw - t.w) <= 1 && Math.abs(bh - t.h) <= 1) {
        tries = 0;
        return;
      }
      if (tries >= 3) return;
      tries++;
      chrome = { w: win.w - bw, h: win.h - bh };
    });
  });

  // the person resized the window by hand: the picture is fitted into it from now on
  $effect(() => {
    const [w, h, m] = [win.w, win.h, win.maximized];
    untrack(() => {
      if (display === 'window' && !m && applied.w && (Math.abs(w - applied.w) > 3 || Math.abs(h - applied.h) > 3)) display = 'fit';
    });
  });

  // the maximize button of the header and the mode "fills the browser" are the same thing
  $effect(() => {
    const m = win.maximized;
    untrack(() => {
      if (m && display !== 'max') display = 'max';
      else if (!m && display === 'max') display = 'fit';
    });
  });
  $effect(() => {
    const d = display;
    untrack(() => {
      if (d === 'max' && !win.maximized) updateTermWindow(win.key, { maximized: true });
      else if (d !== 'max' && win.maximized) updateTermWindow(win.key, { maximized: false });
    });
  });

  // the picture is shown at 100% while the window follows it (scaled down as a whole when the page is too small), fitted into the window otherwise
  $effect(() => {
    screen?.setScale(display === 'window' ? scaleMode : display === 'fill' ? 'fill' : 'fit');
  });

  const statusText = $derived(
    phase === 'connecting' ? $tr('assist.connectingShort') : phase === 'open' ? $tr('assist.open') : $tr('assist.closed'),
  );
</script>

<WindowFrame {win} {top} ariaLabel={$tr('assistwin.title', { name: serverName })} onrefocus={() => screen?.focus()}>
  {#snippet title()}
    <span class="dot" class:on={phase === 'open'} aria-hidden="true"></span>
    <span class="label">{serverName}</span>
    <span class="muted status">· {$tr('assist.name')} · {statusText}{#if phase === 'open'} <AssistantBadge desktop={info?.desktop} />{/if}</span>
  {/snippet}
  {#snippet tools()}
    <AssistantTools {serverId} bind:session={() => session, (v) => updateTermWindow(win.key, { session: v })} {phase} {info} {screen} layout="window" bind:display onsettings={() => openQualityWindow(serverId, serverName)} {serverName} />
  {/snippet}
  <div class="col">
    <div class="screenbox" bind:clientWidth={boxW} bind:clientHeight={boxH}><AssistantSession {serverId} {session} bind:phase bind:info bind:this={screen} /></div>
    {#if $assistInfoShown}
      <AssistantStats {serverId} {phase} {info} />
    {/if}
  </div>
</WindowFrame>

<style>
  .col {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
    border: 2px solid var(--line);
  }

  .screenbox {
    flex: 1;
    min-height: 0;
    display: flex;
  }
</style>
