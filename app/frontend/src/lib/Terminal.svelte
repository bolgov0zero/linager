<script lang="ts">
  import { FitAddon } from '@xterm/addon-fit';
  import { Terminal } from '@xterm/xterm';
  import '@xterm/xterm/css/xterm.css';
  import { onMount } from 'svelte';
  import { openTerminal, serverApi2, type Snippet } from './api';
  import CustomSelect from './CustomSelect.svelte';
  import Icon from './Icon.svelte';
  import { t } from './i18n.svelte';
  import Menu from './Menu.svelte';
  import { native } from './native';

  let { id, title, embedded = false, nativeWindows = false, initialUser = '', onwindow }: { id: string; title: string; embedded?: boolean; nativeWindows?: boolean; initialUser?: string; /** The button "in a separate window" (a Console tab gives it, like the Assistant and RDP do). */ onwindow?: () => void } = $props();

  // The terminal is always dark, like the mockup.
  const THEME = { background: '#0a0c0f', foreground: '#d7dbe3', cursor: '#3b82f6', selectionBackground: '#2b2f37' };
  const LIMIT_CODE = 4029;

  let host = $state<HTMLDivElement | null>(null);
  let phase = $state<'connecting' | 'open' | 'closed'>('connecting');
  let note = $state('');
  let attempt = $state(0);
  // svelte-ignore state_referenced_locally
  let user = $state(initialUser);
  let users = $state<string[]>([]);
  let snippets = $state<Snippet[]>([]);
  let current: { send: (s: string) => void; focus: () => void } | null = null;

  onMount(() => {
    // the people of the server (the shell can be opened as one of them) and the snippets of the panel
    serverApi2.hostUsers(id).then((r) => (users = r.users.filter((u) => u.human || u.name === 'root').map((u) => u.name))).catch(() => undefined);
    serverApi2.snippets().then((r) => (snippets = r.snippets)).catch(() => undefined);
  });

  $effect(() => {
    void attempt;
    const as = user;
    if (!host) return;
    if (!embedded) document.title = `${t('term.title')} · ${title}`;
    const term = new Terminal({ fontFamily: '"Medeo Mono", ui-monospace, Menlo, monospace', fontSize: 13, cursorBlink: true, theme: THEME, scrollback: 5000 });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(host);
    fit.fit();
    const enc = new TextEncoder();
    const ws = openTerminal(id, term.cols, term.rows, as);
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
    const send = (d: string): void => {
      if (ws.readyState === WebSocket.OPEN) ws.send(enc.encode(d));
    };
    current = { send, focus: () => term.focus() };
    const data = term.onData(send);
    const size = term.onResize(({ cols, rows }) => ws.readyState === WebSocket.OPEN && ws.send(JSON.stringify({ resize: { cols, rows } })));
    const el = host;
    const ro = new ResizeObserver(() => el.clientWidth > 0 && el.clientHeight > 0 && fit.fit());
    ro.observe(host);
    term.focus();
    return () => {
      current = null;
      ro.disconnect();
      data.dispose();
      size.dispose();
      ws.close();
      term.dispose();
    };
  });

  /** paste: the snippet is only typed into the terminal, without the Enter that runs it. */
  function runSnippet(v: string, paste = false): void {
    if (v === MANAGE) {
      void native.openWindow('settings', 'app', 'Linager', nativeWindows, 'snippets');
      return;
    }
    const sn = snippets.find((x) => String(x.id) === v);
    if (!sn || !current) return;
    current.send(sn.command + (paste ? '' : '\r'));
    current.focus();
    void serverApi2.snippetRan(sn.id, id).catch(() => undefined);
  }
  const MANAGE = '__manage';
  const userOptions = $derived([{ v: '', t: t('term.user.default') }, ...users.map((u) => ({ v: u, t: u }))]);
</script>

<div class="term-win" class:embedded>
  <div class="term-tools">
    <CustomSelect small value={user} options={userOptions} icon="user" onchange={(v) => (user = v)} />
    <Menu icon="file-code" label={t('term.snippets')} items={[...snippets.map((s) => ({ v: String(s.id), t: s.name, alt: true })), { v: MANAGE, t: t('term.snippets.manage'), sep: true }]} empty={t('term.snippets.none')} onpick={(v) => runSnippet(v)} onalt={(v) => runSnippet(v, true)} altIcon="clipboard-paste" altTitle={t('term.snippetPaste')} />
    <span class="spacer"></span>
    {#if phase !== 'open'}<span class="muted" style="font-size:12px">{phase === 'connecting' ? t('term.connecting') : t('term.closed')}</span>{/if}
    {#if phase === 'closed'}<button class="btn sm" type="button" onclick={() => (attempt += 1)}>{t('term.reconnect')}</button>{/if}
    <button class="btn sm icon" type="button" title={t('term.files')} aria-label={t('term.files')} onclick={() => native.openWindow('files', id, title, nativeWindows)}><Icon name="folder" size={14} /></button>
    <button class="btn sm icon" type="button" title={t('term.manager')} aria-label={t('term.manager')} onclick={() => native.openWindow('manager', id, title, nativeWindows)}><Icon name="activity" size={14} /></button>
    {#if onwindow}<button class="btn sm icon" type="button" title={t('pult.inWindow')} aria-label={t('pult.inWindow')} onclick={onwindow}><Icon name="external-link" size={14} /></button>{/if}
  </div>
  <div class="term-host" bind:this={host}></div>
  {#if note}<div class="term-note">{note}</div>{/if}
</div>
