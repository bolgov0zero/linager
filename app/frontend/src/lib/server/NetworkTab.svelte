<script lang="ts">
  import { onMount } from 'svelte';
  import { ApiError, serverApi2, type ApplyWarning, type NetChange, type NetInterface, type NetworkInfo, type ServerDetail } from '../api';
  import CustomSelect from '../CustomSelect.svelte';
  import { confirmAsk } from '../dialogs.svelte';
  import { errorText } from '../errors';
  import Icon from '../Icon.svelte';
  import { t, i18n } from '../i18n.svelte';
  import { ifaceAsChange } from '../net-draft';
  import { fmtBytes } from '../status';
  import NetDnsModal from './NetDnsModal.svelte';
  import NetIfaceModal from './NetIfaceModal.svelte';
  import PendingBanner from './PendingBanner.svelte';

  let { s }: { s: ServerDetail } = $props();

  let info = $state<NetworkInfo | null>(null);
  let loadFailed = $state(false);
  let draft = $state<NetChange[]>([]);
  let editing = $state<NetInterface | null>(null);
  let showDns = $state(false);
  let busy = $state(false);
  let actionError = $state('');
  let note = $state<{ text: string; bad: boolean } | null>(null);
  let noteTimer: ReturnType<typeof setTimeout> | undefined;
  let open = $state<Record<string, boolean>>({});
  // pending-change bookkeeping for the notice after it ends
  let lastPending: string | null = null;
  let confirmed: string | null = null;
  let seq = 0;

  const sid = $derived(s.id);
  const nf = $derived(i18n.locale === 'ru' ? 'ru-RU' : 'en-US');
  const ifaces = $derived(info?.interfaces ?? []);
  const ifaceMap = $derived(new Map<string, NetInterface>(ifaces.map((i) => [i.name, i])));
  const draftBy = $derived(new Map<string, NetChange>(draft.map((c) => [c.iface, c])));
  const editable = $derived(info?.editable ?? false);
  const pending = $derived(info?.pending ?? null);
  const isNetplan = $derived(info?.backend === 'netplan');
  const canApply = $derived(draft.length > 0 && !pending && !busy && !editing);
  const sourceKey = $derived(info?.backend === 'ifupdown' ? 'net.source.ifupdown' : info?.backend === 'netplan' ? 'net.source.netplan' : 'net.source.unknown');

  function say(text: string, bad = false): void {
    note = { text, bad };
    clearTimeout(noteTimer);
    noteTimer = setTimeout(() => (note = null), bad ? 8000 : 4000);
  }

  async function load(): Promise<void> {
    const my = ++seq;
    try {
      const res = await serverApi2.network(sid);
      if (my !== seq) return;
      loadFailed = false;
      info = res;
      if (res.pending) lastPending = res.pending.id;
      else if (lastPending !== null) {
        say(t(confirmed === lastPending ? 'net.toast.confirmed' : 'net.toast.rolledBack'));
        lastPending = confirmed = null;
      }
      if (!editing && !busy) draft = res.draft?.changes ?? [];
    } catch {
      if (my === seq) loadFailed = true;
    }
  }
  onMount(() => {
    void load();
    const i = setInterval(() => void load(), 3000);
    return () => clearInterval(i);
  });

  const modeLabel = (m: string): string => (m === 'static' ? t('net.mode.static') : m === 'dhcp' ? t('net.mode.dhcp') : m === 'manual' ? t('net.mode.manual') : '—');
  const describe = (c: NetChange): string => t('net.summary', { mode: modeLabel(c.mode), address: c.address || '—', gateway: c.gateway || '—', mtu: c.mtu, state: c.up ? 'UP' : 'DOWN' });
  const fmt = (n: number): string => n.toLocaleString(nf);
  const mainAddr = (i: NetInterface): string => (i.addresses.length > 0 ? i.addresses[0] + (i.addresses.length > 1 ? ` +${i.addresses.length - 1}` : '') : '—');
  const isOpen = (i: NetInterface): boolean => i.primary || (open[i.name] ?? false);
  const KIND: Record<NetInterface['kind'], string> = { physical: 'net.kind.physical', virtual: 'net.kind.virtual', loopback: 'net.kind.loopback' };

  async function apply(confirm: boolean): Promise<void> {
    if (busy) return;
    busy = true;
    actionError = '';
    try {
      await serverApi2.networkApply(sid, confirm);
      draft = [];
      await load();
    } catch (e) {
      const w = e instanceof ApiError && e.status === 409 ? ((e.body as { warnings?: ApplyWarning[] } | null)?.warnings ?? []) : [];
      if (w.length > 0) {
        busy = false;
        const ok = await confirmAsk({ title: t('net.warn.title'), text: `${t('net.warn.text')}\n\n${w.map((x) => `• ${x.message}`).join('\n')}`, ok: t('net.warn.confirm'), danger: true });
        if (ok) await apply(true);
        return;
      }
      actionError = errorText(e);
    } finally {
      busy = false;
    }
  }

  async function reset(): Promise<void> {
    if (busy) return;
    busy = true;
    try {
      await serverApi2.networkResetDraft(sid);
      draft = [];
      say(t('net.draft.reset'));
    } catch (e) {
      actionError = errorText(e);
    } finally {
      busy = false;
    }
  }

  // ---- the configuration as text ----
  let showConfig = $state(false);
  let editingConfig = $state(false);
  let configText = $state('');
  let showEffective = $state(false);
  let npFile = $state('');
  let npNew = $state(false);
  let newName = $state('');
  const canEditConfig = $derived(info?.config_editable ?? editable);
  const npFiles = $derived(info?.netplan_files ?? []);
  const npCurrent = $derived(npFiles.find((f) => f.name === npFile) ?? null);
  const original = $derived(isNetplan ? (npCurrent?.text ?? '') : (info?.config_preview ?? ''));
  const fileNameOk = $derived(!npNew || /^[A-Za-z0-9][A-Za-z0-9._-]{0,60}\.yaml$/.test(newName));

  function defaultNetplanFile(): string {
    const primary = info?.interfaces.find((i) => i.primary)?.name;
    const matching = primary ? npFiles.filter((f) => new RegExp(`\\b${primary}\\b`).test(f.text)) : [];
    return (matching.at(-1) ?? npFiles.at(-1))?.name ?? '';
  }
  function pickFile(name: string): void {
    npFile = name;
    npNew = false;
    configText = npFiles.find((f) => f.name === name)?.text ?? '';
  }
  function startNewFile(): void {
    npNew = true;
    npFile = '';
    newName = t('net.config.newFileDefault');
    configText = 'network:\n  version: 2\n';
  }
  function startConfigEdit(): void {
    actionError = '';
    editingConfig = true;
    if (isNetplan) {
      if (npFiles.length === 0) startNewFile();
      else pickFile(defaultNetplanFile());
    } else configText = info?.config_preview ?? '';
  }
  async function applyConfig(): Promise<void> {
    if (busy) return;
    if (!(await confirmAsk({ title: t('net.config.confirmTitle'), text: t('net.config.confirmText'), ok: t('net.config.confirmButton'), danger: true }))) return;
    busy = true;
    actionError = '';
    try {
      await serverApi2.networkApplyConfig(sid, configText, isNetplan ? (npNew ? newName : npFile) : undefined);
      editingConfig = false;
      await load();
    } catch (e) {
      actionError = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

{#if !info}
  {#if loadFailed}<div class="err">{t('net.loadFailed')} <button class="btn sm" type="button" onclick={load}>{t('net.retry')}</button></div>{:else}<p class="dim">{t('dash.loading')}</p>{/if}
{:else}
  <div class="net-page">
    <p class="dim">{t('net.note', { source: t(sourceKey) })}</p>
    {#if !editable}<div class="banner">{t('net.netplanNotice')}</div>{/if}
    {#if note}<div class="fm-note" class:bad={note.bad}><span>{note.text}</span></div>{/if}
    {#if pending}<PendingBanner serverId={sid} {pending} lost={loadFailed} onconfirmed={(id) => (confirmed = id)} onchanged={load} />{/if}

    {#if draft.length > 0 && !pending}
      <section class="panel">
        <div class="row"><h4 class="ph" style="margin:0">{t('net.draft.title')}</h4><span class="grow"></span>
          <button class="btn sm primary" type="button" disabled={!canApply} onclick={() => apply(false)}>{t('net.draft.apply')}</button>
          <button class="btn sm" type="button" disabled={busy} onclick={reset}>{t('net.draft.discard')}</button></div>
        <p class="dim">{t('net.draft.banner')}</p>
        <ul class="changes">{#each draft as c (c.iface)}{@const cur = ifaceMap.get(c.iface)}<li><span class="mono">{c.iface}</span>: {describe(c)}{#if cur} <span class="dim">({t('net.draft.was')}: {describe(ifaceAsChange(cur))})</span>{/if}</li>{/each}</ul>
      </section>
    {/if}
    {#if actionError}<div class="err" role="alert">{actionError}</div>{/if}

    {#each ifaces as i (i.name)}
      {@const inDraft = draftBy.has(i.name)}
      <section class="panel iface">
        <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions, a11y_no_static_element_interactions -->
        <div class="ih" class:click={!i.primary} onclick={() => !i.primary && (open[i.name] = !isOpen(i))}>
          <h4 class="mono">{i.name}</h4>
          <span class="badge {i.state === 'UP' ? 'b-ok' : 'b-mute'}">{i.state}</span>
          {#if KIND[i.kind]}<span class="badge b-mute">{t(KIND[i.kind])}</span>{/if}
          {#if i.primary}<span class="badge b-warn">{t('net.badge.primary')}</span>{/if}
          {#if inDraft}<span class="badge b-info">{t('net.badge.inDraft')}</span>{/if}
          <span class="grow"></span>
          {#if !isOpen(i)}<span class="mono dim">{mainAddr(i)} · {fmtBytes(i.rx_bytes)} / {fmtBytes(i.tx_bytes)}</span>{/if}
          {#if !i.primary}<Icon name={isOpen(i) ? 'chevron-down' : 'chevron-right'} size={14} />{/if}
        </div>
        {#if isOpen(i)}
          <dl class="facts">
            <div><dt>{t('net.row.mac')}</dt><dd class="mono">{i.mac || '—'}</dd></div>
            <div><dt>{t('net.row.mtu')}</dt><dd>{i.mtu}</dd></div>
            <div><dt>{t('net.row.mode')}</dt><dd>{modeLabel(i.mode)}</dd></div>
            <div><dt>{t('net.row.ipv4')}</dt><dd class="mono">{i.addresses.join(', ') || '—'}</dd></div>
            <div><dt>{t('net.row.gateway')}</dt><dd class="mono">{i.gateway || '—'}</dd></div>
            <div><dt>{t('net.row.speed')}</dt><dd>{i.speed_mbps > 0 ? t('net.speed', { v: i.speed_mbps }) : '—'}</dd></div>
            <div><dt>{t('net.row.traffic')}</dt><dd class="mono">{fmtBytes(i.rx_bytes)} / {fmtBytes(i.tx_bytes)}</dd></div>
            <div><dt>{t('net.row.packets')}</dt><dd class="mono">{fmt(i.rx_packets)} / {fmt(i.tx_packets)}</dd></div>
            <div><dt>{t('net.row.errors')}</dt><dd class="mono">{fmt(i.rx_errors)} / {fmt(i.tx_errors)}</dd></div>
            <div><dt>{t('net.row.drops')}</dt><dd class="mono">{fmt(i.rx_dropped)} / {fmt(i.tx_dropped)}</dd></div>
          </dl>
          {#if editable && i.editable}<div class="row"><button class="btn sm" type="button" disabled={busy || pending !== null} onclick={() => (editing = i)}>{t('net.edit')}</button></div>
          {:else if !i.editable}<p class="dim">{t('net.readonly')}</p>{/if}
        {/if}
      </section>
    {:else}
      <p class="dim">{t('net.ifaces.empty')}</p>
    {/each}

    <div class="two" style="align-items:start">
      <section class="panel">
        <div class="row"><h4 class="ph" style="margin:0">{t('net.dns.title')}</h4><span class="grow"></span>
          {#if info.dns_editable}<button class="btn sm" type="button" disabled={busy || pending !== null} onclick={() => (showDns = true)}>{t('net.edit')}</button>{/if}</div>
        <dl class="facts one">
          <div><dt>{t('net.dns.servers')}</dt><dd class="mono">{info.dns.nameservers.join(', ') || '—'}</dd></div>
          <div><dt>{t('net.dns.search')}</dt><dd class="mono">{info.dns.search.join(', ') || '—'}</dd></div>
        </dl>
      </section>
      <section class="panel flush">
        <h4 class="ph" style="margin:13px 14px 8px">{t('net.routes.title')}</h4>
        {#if info.routes.length === 0}<p class="dim" style="padding:0 14px 12px">{t('net.routes.empty')}</p>{:else}
          <table class="tbl compact"><thead><tr><th>{t('net.col.dst')}</th><th>{t('net.col.gateway')}</th><th>{t('net.col.dev')}</th><th>{t('net.col.metric')}</th><th>{t('net.col.proto')}</th></tr></thead>
            <tbody>{#each info.routes as r, i (i)}<tr><td class="mono">{r.dst}</td><td class="mono dim">{r.gateway || '—'}</td><td class="mono">{r.dev}</td><td class="mono dim">{r.metric}</td><td class="dim">{r.protocol}</td></tr>{/each}</tbody></table>
        {/if}
      </section>
    </div>

    <section class="panel">
      <div class="row"><h4 class="ph" style="margin:0">{t('net.config.title')}</h4><span class="grow"></span>
        {#if canEditConfig && !editingConfig}<button class="btn sm" type="button" disabled={busy || pending !== null} onclick={startConfigEdit}>{t('net.config.edit')}</button>{/if}
        {#if !editingConfig}<button class="btn sm" type="button" onclick={() => (showConfig = !showConfig)}>{showConfig ? t('net.config.hide') : t('net.config.show')}</button>{/if}</div>
      {#if editingConfig}
        <p class="dim">{t(isNetplan ? 'net.config.hintNetplan' : 'net.config.hint')}</p>
        {#if isNetplan}
          <div class="row">
            {#if npNew}<span class="field grow"><input class="mono" maxlength="64" placeholder={t('net.config.newFileName')} aria-label={t('net.config.newFileName')} bind:value={newName} /></span>
            {:else}<CustomSelect small value={npFile} options={npFiles.map((f) => ({ v: f.name, t: f.name }))} onchange={pickFile} />{/if}
            <button class="btn sm" type="button" disabled={npNew} onclick={startNewFile}>{t('net.config.newFile')}</button>
          </div>
          {#if !fileNameOk}<div class="err">{t('net.config.fileInvalid')}</div>{/if}
          {#if npCurrent?.cloud_init}<div class="banner">{t('net.config.cloudInit')}</div>{/if}
        {/if}
        <textarea class="ta cfg" rows="20" spellcheck="false" aria-label={t('net.config.title')} bind:value={configText}></textarea>
        <div class="row end">
          <button class="btn" type="button" disabled={busy} onclick={() => (editingConfig = false)}>{t('net.config.cancel')}</button>
          <button class="btn primary" type="button" disabled={busy || !fileNameOk || configText.trim() === '' || configText.trim() === original.trim()} onclick={applyConfig}>{t('net.config.apply')}</button>
        </div>
      {:else if showConfig}
        <pre class="pv">{info.config_preview}</pre>
      {/if}
      {#if isNetplan}
        <div class="row"><button class="btn sm" type="button" onclick={() => (showEffective = !showEffective)}>{showEffective ? t('net.config.effectiveHide') : t('net.config.effective')}</button></div>
        {#if showEffective}<pre class="pv">{info.netplan_effective || t('net.config.effectiveEmpty')}</pre>{/if}
      {/if}
    </section>
  </div>

  {#if editing}<NetIfaceModal serverId={sid} iface={editing} {draft} onsaved={(c) => { draft = c; editing = null; say(t('net.draft.saved')); }} onclose={() => (editing = null)} />{/if}
  {#if showDns}<NetDnsModal serverId={sid} nameservers={info.dns.nameservers} search={info.dns.search} onclose={() => (showDns = false)} onapplied={() => { showDns = false; void load(); }} />{/if}
{/if}
