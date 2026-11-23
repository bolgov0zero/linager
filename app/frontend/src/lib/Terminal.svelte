<script lang="ts">
  import { FitAddon } from '@xterm/addon-fit';
  import { Terminal } from '@xterm/xterm';
  import '@xterm/xterm/css/xterm.css';
  import { openTerminal } from './api';
  import { t } from './i18n.svelte';

  let { id, title }: { id: string; title: string } = $props();

  // The terminal is always dark, like the mockup.
  const THEME = { background: '#0a0c0f', foreground: '#d7dbe3', cursor: '#3b82f6', selectionBackground: '#2b2f37' };
  const LIMIT_CODE = 4029;

  let host = $state<HTMLDivElement | null>(null);
  let phase = $state<'connecting' | 'open' | 'closed'>('connecting');
  let note = $state('');
  let attempt = $state(0);

  $effect(() => {
    void attempt; // a new attempt starts a new session
    if (!host) return;
    document.title = `${t('term.title')} · ${title}`;
    const term = new Terminal({ fontFamily: '"Medeo Mono", ui-monospace, Menlo, monospace', fontSize: 13, cursorBlink: true, theme: THEME, scrollback: 5000, allowProposedApi: false });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(host);
    fit.fit();
    const enc = new TextEncoder();
    const ws = openTerminal(id, term.cols, term.rows);
    phase = 'connecting';
    note = '';
    ws.onopen = () => {
      phase = 'open';
      ws.send(JSON.stringify({ resize: { cols: term.cols, rows: term.rows } }));
    };
    ws.onmessage = (ev: MessageEvent<unknown>) => {
      if (typeof ev.data === 'string') {
        try {
          const m = JSON.parse(ev.data) as { error?: unknown; closed?: unknown };
          if (typeof m.error === 'string') note = m.error;
          else if (typeof m.closed === 'string') note = m.closed;
        } catch {
          // not JSON
        }
        return;
      }
      term.write(new Uint8Array(ev.data as ArrayBuffer));
    };
    ws.onclose = (ev) => {
      phase = 'closed';
      if (ev.code === LIMIT_CODE) note = t('term.limit');
    };
    const data = term.onData((d) => ws.readyState === WebSocket.OPEN && ws.send(enc.encode(d)));
    const size = term.onResize(({ cols, rows }) => ws.readyState === WebSocket.OPEN && ws.send(JSON.stringify({ resize: { cols, rows } })));
    const el = host;
    const ro = new ResizeObserver(() => el.clientWidth > 0 && el.clientHeight > 0 && fit.fit());
    ro.observe(host);
    term.focus();
    return () => {
      ro.disconnect();
      data.dispose();
      size.dispose();
      ws.close();
      term.dispose();
    };
  });
</script>

<div class="term-win">
  <div class="term-bar"><b>{title}</b><span>·</span><span>{phase === 'connecting' ? t('term.connecting') : phase === 'closed' ? t('term.closed') : ''}</span><span class="spacer"></span>
    {#if phase === 'closed'}<button class="btn sm" type="button" onclick={() => (attempt += 1)}>{t('term.reconnect')}</button>{/if}
  </div>
  <div class="term-host" bind:this={host}></div>
  {#if note}<div class="term-note">{note}</div>{/if}
</div>
