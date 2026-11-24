<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { openApt, serverApi, type AptList, type ServerDetail } from '../api';
  import { confirmAsk } from '../dialogs.svelte';
  import Icon from '../Icon.svelte';
  import { t } from '../i18n.svelte';

  let { s }: { s: ServerDetail } = $props();
  let apt = $state<AptList | null>(null);
  let error = $state('');
  let sel = $state<Record<string, boolean>>({});
  let filter = $state<'all' | 'security'>('all');
  // a run (a check or an install): its console text, progress and the state of each package
  let running = $state<'' | 'check' | 'install'>('');
  let progress = $state<{ stage: string; pct: number } | null>(null);
  let states = $state<Record<string, string>>({});
  let failed = $state(false);
  let runErr = $state('');
  let ws: WebSocket | null = null;

  async function load(): Promise<void> {
    try {
      const was = apt;
      apt = await serverApi.apt(s.id);
      error = '';
      // the first list selects what can be installed; a refresh keeps the choice
      const next: Record<string, boolean> = {};
      for (const p of apt.packages) next[p.name] = was ? (sel[p.name] ?? !p.ignored) : !p.ignored;
      sel = next;
      if (apt.busy && !running) attach();
    } catch {
      error = t('sp.err.load');
    }
  }
  onMount(() => void load());
  onDestroy(() => ws?.close());

  const MARK = '\x1e';
  let pending = '';
  /** The agent mixes event lines (MARK + JSON + newline) into the console text. */
  function feed(chunk: string): void {
    let data = pending + chunk;
    pending = '';
    let text = '';
    for (;;) {
      const i = data.indexOf(MARK);
      if (i < 0) {
        text += data;
        break;
      }
      text += data.slice(0, i);
      const j = data.indexOf('\n', i + 1);
      if (j < 0) {
        pending = data.slice(i);
        break;
      }
      try {
        const ev = JSON.parse(data.slice(i + 1, j)) as { t: string; name?: string; state?: string; stage?: string; pct?: number; pkgs?: { name: string }[] };
        if (ev.t === 'queue') for (const p of ev.pkgs ?? []) states[p.name] = 'queued';
        else if (ev.t === 'pkg' && ev.name && ev.state) states[ev.name] = ev.state;
        else if (ev.t === 'progress' && ev.stage) progress = { stage: ev.stage, pct: ev.pct ?? 0 };
      } catch {
        // a damaged event line is dropped
      }
      data = data.slice(j + 1);
    }
  }

  function run(mode: 'update' | 'upgrade' | 'security' | 'packages', names: string[] = []): void {
    if (running) return;
    running = mode === 'update' ? 'check' : 'install';
    progress = null;
    states = {};
    failed = false;
    runErr = '';
    pending = '';
    wire(openApt(s.id, mode, names));
  }
  /** The browser was closed in the middle of a run: the agent replays it. */
  function attach(): void {
    running = 'install';
    wire(openApt(s.id, 'upgrade'));
  }
  function wire(sock: WebSocket): void {
    ws = sock;
    const dec = new TextDecoder();
    let done = false;
    const finish = (): void => {
      if (done) return;
      done = true;
      running = '';
      progress = null;
      void load();
    };
    sock.onmessage = (e: MessageEvent<unknown>) => {
      if (typeof e.data === 'string') {
        try {
          const f = JSON.parse(e.data) as { closed?: unknown; error?: unknown };
          if (f.error !== undefined) {
            failed = true;
            runErr = String(f.error);
          }
          if (f.closed !== undefined) {
            finish();
            sock.close();
          }
        } catch {
          // not JSON: nothing to show
        }
        return;
      }
      feed(dec.decode(new Uint8Array(e.data as ArrayBuffer), { stream: true }));
    };
    sock.onerror = () => (failed = true);
    sock.onclose = finish;
  }

  const shown = $derived(apt ? (filter === 'security' ? apt.packages.filter((p) => p.security) : apt.packages) : []);
  const chosen = $derived((apt?.packages ?? []).filter((p) => sel[p.name]).map((p) => p.name));
  const allOn = $derived(shown.length > 0 && shown.every((p) => sel[p.name]));
  function toggleAll(): void {
    const on = !allOn;
    for (const p of shown) sel[p.name] = on;
  }

  async function install(): Promise<void> {
    if (chosen.length === 0 || running) return;
    if (!(await confirmAsk({ title: t('sp.upd.install'), text: t('sp.upd.install.text', { n: chosen.length, server: s.name }), ok: t('sp.upd.install') }))) return;
    run('packages', chosen);
  }
  const stateTone = (v: string): string => (v === 'done' ? 'b-ok' : v === 'error' ? 'b-crit' : v === 'running' ? 'b-warn' : 'b-mute');
</script>

{#if error}<div class="err">{error}</div>{:else if !apt}<p class="dim">{t('dash.loading')}</p>{:else}
  <div class="sp-fill">
    <div class="sp-tools">
      <button class="btn sm" type="button" disabled={!!running || !s.online} onclick={() => run('update')}><Icon name="refresh-cw" size={14} />{t('sp.upd.check')}</button>
      <button class="btn sm primary" type="button" disabled={!!running || !s.online || chosen.length === 0} onclick={install}><Icon name="download" size={14} />{t('sp.upd.install')}{chosen.length > 0 ? ` (${chosen.length})` : ''}</button>
      <button class="chip" type="button" aria-pressed={filter === 'security'} onclick={() => (filter = filter === 'security' ? 'all' : 'security')}>{t('sp.upd.security')}</button>
      <span class="grow"></span>
      <span class="dim">{t('sp.upd.count', { n: apt.packages.length })}{#if apt.last_check}{' · '}{t('sp.upd.checked', { v: new Date(apt.last_check * 1000).toLocaleString() })}{/if}</span>
    </div>
    {#if runErr}<div class="err">{runErr}</div>{/if}
    {#if apt.reboot_required}<div class="banner">{t('sp.reboot.needed')}</div>{/if}
    {#if running}
      <div class="run-line"><span class="dim">{running === 'check' ? t('sp.upd.checking') : t('sp.upd.installing')}{progress ? ` · ${progress.stage === 'download' ? t('sp.upd.download') : t('sp.upd.setup')} ${progress.pct}%` : ''}</span>
        <span class="bar"><i style="width:{progress?.pct ?? 8}%"></i></span></div>
    {/if}
    <div class="panel flush fill"><table class="tbl compact fixed">
      <thead><tr><th class="ck-c" style="width:40px"><input type="checkbox" aria-label={t('sp.upd.all')} checked={allOn} disabled={!!running || shown.length === 0} onchange={toggleAll} /></th><th style="width:30%">{t('sp.upd.package')}</th><th>{t('sp.upd.version')}</th><th style="width:230px"></th></tr></thead>
      <tbody>
        {#each shown as p (p.name)}
          <tr class:dimrow={p.ignored}>
            <td class="ck-c"><input type="checkbox" aria-label={p.name} bind:checked={sel[p.name]} disabled={!!running} /></td>
            <td class="mono"><b>{p.name}</b></td><td class="mono dim" title="{p.current} → {p.candidate}">{p.current} → {p.candidate}</td>
            <td class="r">{#if states[p.name]}<span class="badge {stateTone(states[p.name])}">{t(`sp.upd.st.${states[p.name]}`)}</span> {/if}{#if p.security}<span class="badge b-warn">security</span>{/if}{#if p.reboot} <span class="badge b-mute">reboot</span>{/if}{#if p.ignored} <span class="badge b-mute">{t('sp.upd.ignored')}</span>{/if}</td></tr>
        {:else}
          <tr><td colspan="4" class="dim">{t('sp.upd.none')}</td></tr>
        {/each}
      </tbody>
    </table></div>
  </div>
{/if}
