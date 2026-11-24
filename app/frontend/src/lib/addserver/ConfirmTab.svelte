<script lang="ts">
  import { onMount } from 'svelte';
  import { api, ApiError, type AvailableServer, type ConnectInfo, type Enrollment } from '../api';
  import { isUnauthorized } from '../errors';
  import { t } from '../i18n.svelte';
  import { showToast } from '../toast';

  let { active, host, count = $bindable(0), onadded, onauthlost }: {
    /** The tab is on screen; the list of the connected servers is only refreshed then. */
    active: boolean;
    /** The address of the Linager server the application is connected to (without the port). */
    host: string;
    /** How many requests wait (the badge of the tab). */
    count?: number;
    /** A server was approved or put on the dashboard. */
    onadded: () => void;
    onauthlost: () => void;
  } = $props();

  const POLL_MS = 5000;
  const KEY_LENGTH = 9; // XXXX-XXXX

  let items = $state<Enrollment[]>([]);
  let available = $state<AvailableServer[]>([]);
  let connect = $state<ConnectInfo | null>(null);
  let loaded = $state(false);
  let keys = $state<Record<string, string>>({});
  let errors = $state<Record<string, string>>({});
  let busy = $state<Record<string, boolean>>({});

  // The agent port is not the application port, so the address is the one the application connected to; the other
  // names the agent-port certificate was issued for are listed as alternatives.
  const connectTarget = $derived(connect ? `${host}:${connect.port}` : '');
  const otherHosts = $derived((connect?.hosts ?? []).filter((h) => h.toLowerCase() !== host.toLowerCase() && h !== 'localhost' && h !== '127.0.0.1' && h !== '::1'));

  /** Uppercase, strip anything but A-Z/0-9, keep 8 chars and insert the dash after 4. */
  function formatKey(raw: string): string {
    const clean = raw.toUpperCase().replace(/[^A-Z0-9]/g, '').slice(0, 8);
    return clean.length > 4 ? `${clean.slice(0, 4)}-${clean.slice(4)}` : clean;
  }
  function onKeyInput(id: string, event: Event): void {
    const input = event.currentTarget as HTMLInputElement;
    const formatted = formatKey(input.value);
    input.value = formatted;
    keys[id] = formatted;
    errors[id] = '';
  }

  async function loadEnrollments(): Promise<void> {
    try {
      items = await api.enrollments();
      count = items.length;
    } catch (e) {
      if (isUnauthorized(e)) onauthlost();
    } finally {
      loaded = true;
    }
  }
  async function loadAvailable(): Promise<void> {
    try {
      available = await api.availableServers();
    } catch (e) {
      if (isUnauthorized(e)) onauthlost();
    }
  }
  const refresh = (): Promise<unknown> => Promise.all([loadEnrollments(), active ? loadAvailable() : Promise.resolve()]);
  async function loadConnect(): Promise<void> {
    try {
      connect = await api.connectInfo();
    } catch (e) {
      if (isUnauthorized(e)) onauthlost();
    }
  }
  async function copyTarget(): Promise<void> {
    try {
      await navigator.clipboard.writeText(connectTarget);
      showToast(t('st.common.copied'));
    } catch {
      showToast(t('st.common.copyFailed'));
    }
  }

  async function approve(item: Enrollment): Promise<void> {
    const key = item.needs_key ? (keys[item.id] ?? '') : '';
    if ((item.needs_key && key.length !== KEY_LENGTH) || busy[item.id]) return;
    busy[item.id] = true;
    errors[item.id] = '';
    try {
      await api.approveEnrollment(item.id, key);
      showToast(t('asv.confirm.approved', { name: item.hostname }));
      onadded();
      await Promise.all([loadEnrollments(), loadAvailable()]);
    } catch (e) {
      if (isUnauthorized(e)) return onauthlost();
      if (e instanceof ApiError && e.status === 422) errors[item.id] = t('asv.confirm.invalidKey');
      else if (e instanceof ApiError && e.status === 410) {
        showToast(t('asv.confirm.tooMany'));
        await loadEnrollments();
      } else errors[item.id] = t('common.genericError');
    } finally {
      busy[item.id] = false;
    }
  }
  async function reject(item: Enrollment): Promise<void> {
    if (busy[item.id]) return;
    busy[item.id] = true;
    errors[item.id] = '';
    try {
      await api.rejectEnrollment(item.id);
      showToast(t('asv.confirm.rejected'));
      await loadEnrollments();
    } catch (e) {
      if (isUnauthorized(e)) return onauthlost();
      errors[item.id] = t('common.genericError');
    } finally {
      busy[item.id] = false;
    }
  }
  async function add(server: AvailableServer): Promise<void> {
    if (busy[server.id]) return;
    busy[server.id] = true;
    try {
      await api.dashboardAdd(server.id);
      showToast(t('asv.avail.added', { name: server.name }));
      onadded();
      await loadAvailable();
    } catch (e) {
      if (isUnauthorized(e)) return onauthlost();
      if (e instanceof ApiError && e.status === 409) await loadAvailable();
      else showToast(t('common.genericError'));
    } finally {
      busy[server.id] = false;
    }
  }

  // Coming back to the tab shows fresh data at once.
  $effect(() => {
    if (active) void loadAvailable();
  });
  onMount(() => {
    void loadEnrollments();
    void loadConnect();
    const timer = setInterval(() => void refresh(), POLL_MS);
    return () => clearInterval(timer);
  });
</script>

<div class="asv-tab">
  <h3>{t('asv.tab.confirm')}</h3>

  <div class="asv-sec">
    <h4 class="ph">{t('asv.confirm.pending')}</h4>
    {#if !loaded}<p class="dim">{t('dash.loading')}</p>
    {:else if items.length === 0}<p class="dim">{t('asv.confirm.empty')}</p>{/if}
    {#each items as item (item.id)}
      <div class="panel mdl-stack">
        <div class="row between">
          <div class="row"><b class="mono">{item.hostname}</b><span class="dim mono">{item.address}</span></div>
          <span class="badge {item.via ? 'b-info' : 'b-mute'}">{item.via ? t('asv.confirm.badgeLink') : t('asv.confirm.badgeManual')}</span>
        </div>
        <dl class="facts one">
          <div><dt>{t('asv.confirm.system')}</dt><dd>{item.os}{item.agent_version ? ` · ${t('asv.confirm.agent', { version: item.agent_version })}` : ''}</dd></div>
          <div><dt>{t('asv.confirm.from')}</dt><dd class="mono">{item.remote_addr}</dd></div>
          <div><dt>{t('asv.confirm.fingerprint')}</dt><dd class="mono">{item.key_fingerprint}</dd></div>
          {#if item.needs_key}<div><dt>{t('asv.confirm.code')}</dt><dd><b class="mono code">{item.verify_code}</b> <span class="dim">{t('asv.confirm.codeHint')}</span></dd></div>{/if}
        </dl>
        {#if item.needs_key}
          <label class="fl"><span>{t('asv.confirm.keyLabel')}</span><span class="field"><input class="mono" autocomplete="off" autocapitalize="characters" spellcheck="false" maxlength={KEY_LENGTH} placeholder={t('asv.confirm.keyPlaceholder')} value={keys[item.id] ?? ''} oninput={(e) => onKeyInput(item.id, e)} /></span></label>
        {:else}<p class="warnbox warn">{t('asv.confirm.viaInfo', { via: item.via ?? '' })}</p>{/if}
        {#if errors[item.id]}<div class="err" role="alert">{errors[item.id]}</div>{/if}
        <div class="row end">
          <button class="btn" type="button" disabled={busy[item.id]} onclick={() => void reject(item)}>{t('asv.confirm.reject')}</button>
          <button class="btn primary" type="button" disabled={busy[item.id] || (item.needs_key && (keys[item.id] ?? '').length !== KEY_LENGTH)} onclick={() => void approve(item)}>{t('asv.confirm.approve')}</button>
        </div>
      </div>
    {/each}
  </div>

  <div class="asv-sec">
    <h4 class="ph">{t('asv.avail.title')}</h4>
    {#if available.length === 0}<p class="dim">{t('asv.avail.empty')}</p>{/if}
    {#each available as server (server.id)}
      <div class="panel row between">
        <div class="row"><b class="mono">{server.name}</b><span class="dim mono">{server.address} · {server.os}</span></div>
        <button class="btn sm" type="button" disabled={busy[server.id]} onclick={() => void add(server)}>{t('asv.avail.add')}</button>
      </div>
    {/each}
  </div>

  {#if connect}
    <div class="asv-sec">
      <h4 class="ph">{t('asv.connect.title')}</h4>
      <div class="panel mdl-stack">
        <p class="dim">{t('asv.connect.hint')}</p>
        <dl class="facts one">
          <div><dt>{t('asv.connect.address')}</dt><dd class="mono">{host}</dd></div>
          <div><dt>{t('asv.connect.port')}</dt><dd class="mono">{connect.port}</dd></div>
          <div><dt>{t('asv.connect.fingerprint')}</dt><dd class="mono">{connect.fingerprint}</dd></div>
        </dl>
        {#if otherHosts.length > 0}<p class="dim mono">{t('asv.connect.other', { list: otherHosts.join(', ') })}</p>{/if}
        <div class="row"><button class="btn sm" type="button" onclick={() => void copyTarget()}>{t('asv.connect.copy')}</button><span class="dim mono">{connectTarget}</span></div>
      </div>
    </div>
  {/if}
</div>
