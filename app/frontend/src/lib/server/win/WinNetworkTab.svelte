<script lang="ts">
  import { onMount } from 'svelte';
  import { ApiError, winCall, type ServerDetail, type WinAdapter, type WinNetApply, type WinNetState } from '../../api';
  import CustomSelect from '../../CustomSelect.svelte';
  import { confirmAsk } from '../../dialogs.svelte';
  import { errorText } from '../../errors';
  import Icon from '../../Icon.svelte';
  import { t } from '../../i18n.svelte';

  let { s }: { s: ServerDetail } = $props();
  const sid = $derived(s.id);

  let data = $state<WinNetState | null>(null);
  let error = $state('');
  let busy = $state(false);
  let editing = $state<WinAdapter | null>(null);
  let opened = $state<number[]>([]);
  let pending = $state<{ id: string; deadline: number } | null>(null);
  let left = $state(0);
  let note = $state<{ text: string; bad: boolean } | null>(null);
  let noteTimer: ReturnType<typeof setTimeout> | undefined;
  let fMode = $state<'dhcp' | 'static'>('dhcp');
  let fIp = $state('');
  let fPrefix = $state(24);
  let fGw = $state('');
  let fDns = $state('');
  const modeOptions = $derived([{ v: 'dhcp', t: t('wn.dhcp') }, { v: 'static', t: t('wn.static') }]);

  function say(text: string, bad = false): void {
    note = { text, bad };
    clearTimeout(noteTimer);
    noteTimer = setTimeout(() => (note = null), bad ? 8000 : 4000);
  }
  async function load(): Promise<void> {
    try {
      data = await winCall<WinNetState>(sid, 'network', 'state');
      error = '';
      if (data.pending) pending = data.pending;
    } catch (e) {
      error = errorText(e);
    }
  }
  onMount(() => void load());

  // the countdown of a change that waits for its confirmation
  $effect(() => {
    if (!pending) return;
    const tick = (): void => {
      left = Math.max(0, pending ? pending.deadline - Math.floor(Date.now() / 1000) : 0);
      if (left === 0) {
        pending = null;
        say(t('wn.reverted'));
        void load();
      }
    };
    tick();
    const i = setInterval(tick, 1000);
    return () => clearInterval(i);
  });

  const toggle = (i: number): void => {
    opened = opened.includes(i) ? opened.filter((x) => x !== i) : [...opened, i];
  };
  function edit(a: WinAdapter): void {
    editing = a;
    fMode = a.dhcp ? 'dhcp' : 'static';
    const [ip, pl] = (a.ipv4[0] ?? '').split('/');
    fIp = ip ?? '';
    fPrefix = Number(pl) || 24;
    fGw = a.gateway === '0.0.0.0' ? '' : a.gateway;
    fDns = a.dns.join(', ');
  }
  async function apply(): Promise<void> {
    if (!editing) return;
    const req: WinNetApply = { index: editing.index, mode: fMode, ...(fMode === 'static' ? { ip: fIp.trim(), prefix: fPrefix, gateway: fGw.trim() } : {}), dns: fDns.split(/[,\s]+/).filter(Boolean) };
    if (!(await confirmAsk({ title: t('win.confirm'), text: t('wn.confirmApply'), ok: t('wn.apply'), danger: true }))) return;
    busy = true;
    try {
      const r = await winCall<{ id: string; deadline: number }>(sid, 'network', 'apply', { ...req }, true);
      pending = { id: r.id, deadline: r.deadline };
      editing = null;
    } catch (e) {
      // the answer may never arrive if the change cut the connection: the agent undoes it by itself
      if (e instanceof ApiError && e.status !== 0 && e.status < 500) say(errorText(e), true);
      else say(t('wn.lost'), true);
    } finally {
      busy = false;
    }
  }
  async function settle(action: 'confirm' | 'rollback'): Promise<void> {
    const p = pending;
    if (!p) return;
    try {
      await winCall(sid, 'network', action, { id: p.id });
      pending = null;
      say(t(action === 'confirm' ? 'wn.kept' : 'wn.reverted'));
      await load();
    } catch (e) {
      say(errorText(e), true);
    }
  }
</script>

{#if error && !data}<div class="err">{error}</div>{:else if !data}<p class="dim">{t('win.loading')}</p>{:else}
  <div class="net-page">
    <div class="row"><h4 class="ph" style="margin:0">{t('wn.title')}</h4><span class="grow"></span><button class="btn sm icon" type="button" title={t('win.refresh')} onclick={load}><Icon name="refresh-cw" size={14} /></button></div>
    {#if note}<div class="fm-note" class:bad={note.bad}><span>{note.text}</span></div>{/if}
    {#if pending}
      <section class="pending"><div class="count"><b>{left}</b><span>{t('net.pending.seconds')}</span></div>
        <div class="pend-body"><h4>{t('wn.pending', { n: left })}</h4>
          <div class="row"><button class="btn primary" type="button" onclick={() => settle('confirm')}>{t('wn.keep')}</button><button class="btn danger" type="button" onclick={() => settle('rollback')}>{t('wn.revert')}</button></div></div></section>
    {/if}
    {#each data.adapters as a (a.index)}
      {@const open = opened.includes(a.index) || editing?.index === a.index}
      <section class="panel iface">
        <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
        <div class="ih click" onclick={() => toggle(a.index)}>
          <h4 class="mono">{a.name}</h4>
          <span class="badge {a.up ? 'b-ok' : 'b-mute'}">{a.up ? 'UP' : 'DOWN'}</span><span class="badge b-mute">{a.virtual ? t('wn.virtual') : t('wn.physical')}</span><span class="badge b-mute">{a.dhcp ? t('wn.dhcp') : t('wn.static')}</span>
          <span class="grow"></span>{#if !open}<span class="mono dim">{a.ipv4[0] ?? '—'}</span>{/if}<Icon name={open ? 'chevron-down' : 'chevron-right'} size={14} />
        </div>
        {#if open}
          <dl class="facts">
            <div><dt>{t('wn.desc')}</dt><dd>{a.description}</dd></div><div><dt>{t('wn.mac')}</dt><dd class="mono">{a.mac || '—'}</dd></div><div><dt>MTU</dt><dd>{a.mtu}</dd></div>
            <div><dt>{t('wn.mode')}</dt><dd>{a.dhcp ? t('wn.dhcp') : t('wn.static')}</dd></div><div><dt>IPv4</dt><dd class="mono">{a.ipv4.join(', ') || '—'}</dd></div>
            <div><dt>{t('wn.gateway')}</dt><dd class="mono">{a.gateway && a.gateway !== '0.0.0.0' ? a.gateway : '—'}</dd></div><div><dt>DNS</dt><dd class="mono">{a.dns.join(', ') || '—'}</dd></div>
            <div><dt>{t('wn.speed')}</dt><dd>{a.speed_mbps ? `${a.speed_mbps} Mbit/s` : '—'}</dd></div>
          </dl>
          {#if editing?.index !== a.index}
            <div class="row"><button class="btn sm" type="button" disabled={busy || !!pending || !s.online} onclick={() => edit(a)}>{t('wn.edit')}</button></div>
          {:else}
            <div class="mdl-stack">
              <p class="warnbox warn">{t('wn.warnCarrier')}</p>
              <div class="fl"><span>{t('wn.mode')}</span><CustomSelect value={fMode} options={modeOptions} onchange={(v) => (fMode = v as 'dhcp' | 'static')} /></div>
              {#if fMode === 'static'}
                <div class="two"><label class="fl"><span>{t('wn.ip')}</span><span class="field"><input class="mono" bind:value={fIp} placeholder="192.168.1.20" spellcheck="false" /></span></label>
                  <label class="fl"><span>{t('wn.prefix')}</span><span class="field"><input type="number" min="1" max="30" bind:value={fPrefix} /></span></label></div>
                <label class="fl"><span>{t('wn.gateway')}</span><span class="field"><input class="mono" bind:value={fGw} placeholder="192.168.1.1" spellcheck="false" /></span></label>
              {/if}
              <label class="fl"><span>{t('wn.dns')}</span><span class="field"><input class="mono" bind:value={fDns} placeholder="1.1.1.1, 8.8.8.8" spellcheck="false" /></span></label>
              <div class="row"><button class="btn primary" type="button" disabled={busy} onclick={apply}>{t('wn.apply')}</button><button class="btn" type="button" onclick={() => (editing = null)}>{t('win.cancel')}</button></div>
            </div>
          {/if}
        {/if}
      </section>
    {/each}
  </div>
{/if}
