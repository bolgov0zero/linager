<script lang="ts">
  import { onMount } from 'svelte';
  import { api, ApiError, type AccessCheck, type AccessConfig, type AccessHost, type AccessLogFilter, type AccessLogRow, type AccessRule, type AccessState } from '../api';
  import CustomSelect from '../CustomSelect.svelte';
  import { isUnauthorized } from '../errors';
  import { i18n, t } from '../i18n.svelte';
  import { showToast } from '../toast';
  import Fold from './Fold.svelte';

  let { onauthlost }: { onauthlost: () => void } = $props();

  const FILTERS: AccessLogFilter[] = ['all', 'denied', 'login', 'fail', 'proxy', 'direct'];
  const RFC1918 = ['10.0.0.0/8', '172.16.0.0/12', '192.168.0.0/16'];

  let srv = $state<AccessState | null>(null);
  let draft = $state<AccessConfig | null>(null);
  let check = $state<AccessCheck | null>(null);
  let loaded = $state(false);
  let busy = $state(false);
  let confirming = $state(false);
  let error = $state('');

  let openAccess = $state(true);
  let openProxy = $state(false);
  let openLog = $state(false);

  let newAddr = $state('');
  let newNote = $state('');
  let addErr = $state('');
  let newProxy = $state('');
  let newProxyNote = $state('');
  let proxyErr = $state('');

  let testIp = $state('');
  let testHow = $state('proxy');

  let hostBusy = $state(false);
  let refreshTick = $state(0); // a name was refreshed: ask the server about the draft again
  let now = $state(Date.now()); // the "N min ago" of the host names

  let filter = $state<AccessLogFilter>('all');
  let rows = $state<AccessLogRow[]>([]);
  let moreRows = $state(false);
  let retention = $state('30');

  const listKey = $derived(draft?.mode === 'black' ? 'black' : 'white');
  const list = $derived(draft ? draft[listKey] : []);
  const dirty = $derived(!!srv && !!draft && JSON.stringify(draft) !== JSON.stringify(srv.config));
  const problems = $derived(check?.problems ?? []);
  const trustAll = $derived(!!draft && draft.proxies.some((p) => p.cidr.endsWith('/0')));
  const hosts = $derived<Record<string, AccessHost>>({ ...(srv?.hosts ?? {}), ...(check?.hosts ?? {}) });
  const hasHosts = $derived(list.some((r) => isHostRule(r)));

  function fail(e: unknown): void {
    if (isUnauthorized(e)) onauthlost();
    else error = t('st.access.failed');
  }

  function clone<T>(v: T): T {
    return JSON.parse(JSON.stringify(v)) as T;
  }

  // ---- addresses ----

  function ip4(s: string): number | null {
    const p = s.split('.');
    if (p.length !== 4) return null;
    let n = 0;
    for (const part of p) {
      if (!/^\d{1,3}$/.test(part) || Number(part) > 255) return null;
      n = n * 256 + Number(part);
    }
    return n;
  }

  /** A valid address or network in the form the server stores ("1.2.3.4" becomes "1.2.3.4/32"), or null. */
  function normalize(raw: string): string | null {
    const s = raw.trim();
    if (!s) return null;
    const [addr, bits, extra] = s.split('/');
    if (extra !== undefined) return null;
    if (addr.includes(':')) {
      if (!/^[0-9a-f:]+$/i.test(addr) || (bits !== undefined && !(/^\d{1,3}$/.test(bits) && Number(bits) <= 128))) return null;
      return `${addr}/${bits ?? 128}`;
    }
    if (ip4(addr) === null || (bits !== undefined && !(/^\d{1,2}$/.test(bits) && Number(bits) <= 32))) return null;
    return `${addr}/${bits ?? 32}`;
  }

  /** A valid host name in the form the server stores (lower case, no final dot), or null. A name has a dot and does not end in a number. */
  function hostName(raw: string): string | null {
    const s = raw.trim().replace(/\.$/, '').toLowerCase();
    if (!s || s.length > 253 || !s.includes('.')) return null;
    const labels = s.split('.');
    if (!labels.every((l) => /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/.test(l))) return null;
    return /^\d/.test(labels[labels.length - 1]) ? null : s;
  }

  function isHostRule(rule: AccessRule): boolean {
    return normalize(rule.cidr) === null && hostName(rule.cidr) !== null;
  }

  /** Whether the network of a list holds the address (IPv4; other families only on an exact match). */
  function holds(cidr: string, addr: string): boolean {
    const [net, bits] = cidr.split('/');
    const a = ip4(addr);
    const n = ip4(net);
    if (a === null || n === null) return net === addr && Number(bits) === (addr.includes(':') ? 128 : 32);
    const len = Number(bits);
    if (len === 0) return true;
    const mask = (0xffffffff << (32 - len)) >>> 0;
    return ((a & mask) >>> 0) === ((n & mask) >>> 0);
  }

  function addTo(target: AccessRule[], cidr: string, note: string): string {
    const norm = normalize(cidr);
    if (!norm) return 'addr';
    if (target.some((r) => r.cidr === norm)) return 'dup';
    target.push({ cidr: norm, note: note.trim() });
    return '';
  }

  // ---- loading ----

  async function loadLog(more = false): Promise<void> {
    try {
      const before = more && rows.length ? rows[rows.length - 1].id : 0;
      const res = await api.accessLog(filter, before);
      rows = more ? [...rows, ...res.rows] : res.rows;
      moreRows = res.rows.length >= 50;
    } catch (e) {
      fail(e);
    }
  }

  async function load(): Promise<void> {
    try {
      srv = await api.access();
      draft = clone(srv.config);
      retention = String(srv.config.retention_days);
      await loadLog();
    } catch (e) {
      fail(e);
    } finally {
      loaded = true;
    }
  }

  onMount(() => {
    void load();
    const timer = setInterval(() => (now = Date.now()), 15000);
    return () => clearInterval(timer);
  });

  // The server judges the draft (what happens to you, whether it may be applied, what happens to the tested address).
  let seq = 0;
  $effect(() => {
    if (!draft) return;
    const body = $state.snapshot(draft);
    void refreshTick;
    const ip = testIp.trim();
    const test = ip ? { ip, proxy: testHow === 'proxy' } : undefined;
    const timer = setTimeout(async () => {
      const mine = ++seq;
      try {
        const res = await api.accessCheck(body, test);
        if (mine === seq) check = res;
      } catch (e) {
        if (isUnauthorized(e)) onauthlost();
      }
    }, 200);
    return () => clearTimeout(timer);
  });

  // ---- actions ----

  function setMode(mode: AccessConfig['mode']): void {
    if (draft) draft.mode = mode;
    addErr = '';
  }

  async function addRule(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!draft || hostBusy) return;
    const host = normalize(newAddr) === null ? hostName(newAddr) : null;
    if (host) {
      // a name is added only when DNS knows it: the server looks it up
      if (draft[listKey].some((r) => r.cidr === host)) {
        addErr = t('st.access.err.dupHost');
        return;
      }
      hostBusy = true;
      try {
        await api.accessResolve(host);
      } catch (e) {
        if (isUnauthorized(e)) onauthlost();
        else addErr = e instanceof ApiError && e.status === 422 ? t('st.access.err.unresolved', { host }) : t('st.access.failed');
        return;
      } finally {
        hostBusy = false;
      }
      draft[listKey].push({ cidr: host, note: newNote.trim() });
      addErr = '';
      newAddr = '';
      newNote = '';
      return;
    }
    const res = addTo(draft[listKey], newAddr, newNote);
    addErr = res ? t(`st.access.err.${res}`) : '';
    if (!res) {
      newAddr = '';
      newNote = '';
    }
  }

  function addRfc(): void {
    if (!draft) return;
    for (const c of RFC1918) addTo(draft.white, c, t('st.access.rfc.note'));
  }

  function removeRule(index: number): void {
    draft?.[listKey].splice(index, 1);
  }

  /** Looks the name up again at once (the server also does it every minute). */
  async function refreshHost(host: string): Promise<void> {
    if (hostBusy) return;
    hostBusy = true;
    try {
      const res = await api.accessResolve(host);
      refreshTick++;
      if (srv) srv.hosts[host] = res.status;
      showToast(res.status.failing ? t('st.access.host.stillDown', { host }) : t('st.access.host.refreshed', { host, ips: res.status.ips.join(', ') }));
    } catch (e) {
      if (isUnauthorized(e)) onauthlost();
      else showToast(t('st.access.err.unresolved', { host }));
    } finally {
      hostBusy = false;
    }
  }

  function addProxy(event: SubmitEvent): void {
    event.preventDefault();
    if (!draft) return;
    if (normalize(newProxy)?.endsWith('/0')) {
      proxyErr = t('st.access.proxy.err.all');
      return;
    }
    if (normalize(newProxy) === null && hostName(newProxy)) {
      proxyErr = t('st.access.proxy.err.host');
      return;
    }
    const res = addTo(draft.proxies, newProxy, newProxyNote);
    proxyErr = res ? t(`st.access.err.${res}`) : '';
    if (!res) {
      newProxy = '';
      newProxyNote = '';
    }
  }

  /** Trust the connection this very request came from (the proxy in front of the panel), drop "trust everybody". */
  function trustPeer(): void {
    if (!draft || !srv) return;
    draft.proxies = draft.proxies.filter((p) => !p.cidr.endsWith('/0'));
    addTo(draft.proxies, srv.you.remote_addr, '');
  }

  function toList(ip: string): void {
    if (!draft || draft.mode === 'off') return;
    const res = addTo(draft[listKey], ip, '');
    if (res === 'dup') showToast(t('st.access.log.exists'));
    else if (!res) {
      openAccess = true;
      showToast(t('st.access.log.added', { ip }));
    }
  }

  async function apply(): Promise<void> {
    if (!draft || busy) return;
    busy = true;
    error = '';
    try {
      srv = await api.accessSave($state.snapshot(draft));
      draft = clone(srv.config);
      confirming = false;
      showToast(t('st.access.applied'));
      await loadLog();
    } catch (e) {
      fail(e);
    } finally {
      busy = false;
    }
  }

  function revert(): void {
    if (srv) draft = clone(srv.config);
    confirming = false;
    addErr = proxyErr = '';
  }

  async function setRetention(value: string): Promise<void> {
    try {
      srv = await api.accessRetention(Number(value));
      if (draft) draft.retention_days = srv.config.retention_days;
      showToast(t('st.access.log.retentionSaved', { n: value }));
    } catch (e) {
      fail(e);
    }
  }

  async function setFilter(f: AccessLogFilter): Promise<void> {
    filter = f;
    await loadLog();
  }

  // ---- text ----

  function reasonText(code: string): string {
    return t(`st.access.reason.${code}`);
  }

  function problemText(p: { code: string; ip?: string; reason?: string; host?: string }): string {
    return t(`st.access.problem.${p.code}`, { ip: p.ip ?? '', host: p.host ?? '', reason: p.reason ? reasonText(p.reason) : '' });
  }

  function ago(ts: number): string {
    const s = Math.max(0, Math.round((now - ts * 1000) / 1000));
    if (s < 60) return t('st.access.ago.s', { n: s });
    if (s < 3600) return t('st.access.ago.m', { n: Math.floor(s / 60) });
    return t('st.access.ago.h', { n: Math.floor(s / 3600) });
  }

  /** The line under the addresses of a name: when they were checked, or what is wrong. */
  function hostLine(h: AccessHost | undefined): { text: string; tone: 'ok' | 'warn' } {
    if (!h || !h.checked_at) return { text: t('st.access.host.never'), tone: 'warn' };
    if (h.expired) return { text: t('st.access.host.expired', { ago: ago(h.checked_at) }), tone: 'warn' };
    if (h.failing) return { text: t('st.access.host.failing', { ago: ago(h.checked_at) }), tone: 'warn' };
    if (now - h.checked_at * 1000 < 5000) return { text: t('st.access.host.okNow'), tone: 'ok' };
    return { text: t('st.access.host.ok', { ago: ago(h.checked_at) }), tone: 'ok' };
  }

  function formatTime(ts: number): string {
    return new Date(ts * 1000).toLocaleString(i18n.locale === 'en' ? 'en-GB' : 'ru-RU', {
      day: '2-digit',
      month: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  }

  const accessMeta = $derived(
    !srv ? '' : srv.config.mode === 'off' ? t('st.access.meta.off') : t(`st.access.meta.${srv.config.mode}`, { n: srv.config[srv.config.mode].length }),
  );
  const proxyMeta = $derived(
    !srv ? '' : srv.config.proxies.some((p) => p.cidr.endsWith('/0')) ? t('st.access.proxy.metaAll') : srv.config.proxies.length ? t('st.access.proxy.metaN', { n: srv.config.proxies.length }) : t('st.access.proxy.metaNone'),
  );
  const proxyTone = $derived(!srv ? 'muted' : srv.config.proxies.length === 0 || srv.config.proxies.some((p) => p.cidr.endsWith('/0')) ? 'warn' : 'ok');
  const peerListed = $derived(!!srv && !!draft && !trustAll && draft.proxies.some((p) => holds(p.cidr, srv!.you.remote_addr)));
  const peerUnlisted = $derived(!!srv && srv.you.via_proxy && !peerListed && !trustAll);
  const modeCount = $derived(draft && draft.mode !== 'off' ? draft[draft.mode].length : 0);
  const modeOptions = ['off', 'white', 'black'] as const;
  const retentionOptions = $derived(['7', '30', '90'].map((n) => ({ v: n, t: t('st.access.log.days', { n }) })));
  const howOptions = $derived([
    { v: 'proxy', t: t('st.access.test.proxy') },
    { v: 'direct', t: t('st.access.test.direct') },
  ]);
</script>

<div class="set-sec access">
  <h3>{t('st.access.title')}</h3>
  <p class="dim">{t('st.access.lead')}</p>

  {#if !loaded}
    <p class="dim">{t('dash.loading')}</p>
  {:else if srv && draft}
    {#if error}<div class="err" role="alert">{error}</div>{/if}

    <Fold title={t('st.access.restrict')} meta={accessMeta} tone={srv.config.mode === 'off' ? 'muted' : 'ok'} bind:open={openAccess}>
      <div class="radios" role="radiogroup" aria-label={t('st.access.mode.label')}>
        {#each modeOptions as m (m)}
          <label class="opt" class:on={draft.mode === m}>
            <input type="radio" name="access-mode" value={m} checked={draft.mode === m} onchange={() => setMode(m)} />
            <span><b>{t(`st.access.mode.${m}`)}</b><small class="dim">{t(`st.access.mode.${m}.hint`)}</small></span>
          </label>
        {/each}
      </div>

      {#if draft.mode !== 'off'}
        <div class="sec">
          <h4 class="ph">{t(`st.access.list.${listKey}`, { n: list.length })}</h4>
          {#if list.length}
            <div class="panel flush"><table class="tbl compact">
              <thead><tr><th>{t('st.access.col.addrHost')}</th><th>{t('st.access.col.now')}</th><th>{t('st.access.col.note')}</th><th></th></tr></thead>
              <tbody>
                {#each list as rule, i (rule.cidr)}
                  {@const host = isHostRule(rule)}
                  {@const info = hosts[rule.cidr]}
                  {@const line = host ? hostLine(info) : null}
                  <tr>
                    <td class="nw mono">{rule.cidr}{#if host}<span class="badge b-info ddns" title={t('st.access.host.badgeHint')}>DDNS</span>{/if}</td>
                    <td>
                      {#if host && line}
                        <div class="mono">{#each info?.ips ?? [] as ip (ip)}<div>{ip}</div>{/each}</div>
                        <div class="{line.tone === 'ok' ? 'okt' : 'warnt'}">{line.text}</div>
                      {:else}<span class="dim">—</span>{/if}
                    </td>
                    <td>{rule.note || '—'}</td>
                    <td class="r nw">
                      {#if host}<button class="btn sm" type="button" disabled={hostBusy} aria-label={`${t('st.access.host.refresh')} ${rule.cidr}`} onclick={() => void refreshHost(rule.cidr)}>{t('st.access.host.refresh')}</button>{' '}{/if}
                      <button class="btn sm" type="button" aria-label={`${t('st.access.remove')} ${rule.cidr}`} onclick={() => removeRule(i)}>{t('st.access.remove')}</button>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table></div>
            {#if hasHosts}<p class="warnbox warn">{t('st.access.host.note')}</p>{/if}
          {:else}
            <p class="dim">{t('st.access.empty')}</p>
          {/if}
          <form class="row add" autocomplete="off" onsubmit={addRule}>
            <span class="field grow2"><input placeholder={t('st.access.addr.ph')} aria-label={t('st.access.addr.aria')} bind:value={newAddr} /></span>
            <span class="field grow"><input placeholder={t('st.access.note.ph')} aria-label={t('st.access.note.ph')} maxlength="100" bind:value={newNote} /></span>
            <button class="btn primary" type="submit" disabled={hostBusy}>{t('st.access.add')}</button>
            {#if draft.mode === 'white'}<button class="btn" type="button" onclick={addRfc}>{t('st.access.rfc')}</button>{/if}
          </form>
          {#if addErr}<div class="err" role="alert">{addErr}</div>{/if}
        </div>

        <label class="sw-row">
          <span><b>{t('st.access.onlyProxy')}</b><small>{t('st.access.onlyProxy.hint')}</small></span>
          <span class="sw"><input type="checkbox" role="switch" bind:checked={draft.only_proxy} /><i></i></span>
        </label>

        <div class="sec">
          <h4 class="ph">{t('st.access.test.title')}</h4>
          <div class="row">
            <span class="field grow"><input placeholder="198.51.100.20" aria-label={t('st.access.test.aria')} bind:value={testIp} /></span>
            <CustomSelect value={testHow} options={howOptions} onchange={(v) => (testHow = v)} />
          </div>
          <div aria-live="polite">
            {#if !testIp.trim()}
              <span class="dim">{t('st.access.test.enter')}</span>
            {:else if check?.test}
              <span class="badge {check.test.allowed ? 'b-ok' : 'b-crit'}">{t(check.test.allowed ? 'st.access.test.allowed' : 'st.access.test.denied')}</span>
              {check.test.ip} — {reasonText(check.test.reason)}{check.test.ignored_xff ? `; ${t('st.access.test.ignored')}` : ''}
            {/if}
          </div>
        </div>

        {#if check && problems.length === 0}
          <p class="okbox">{t('st.access.you.ok', { ip: check.you.ip, reason: reasonText(check.you.reason) })}</p>
        {/if}
      {/if}
    </Fold>

    <Fold title={t('st.access.proxy.title')} meta={proxyMeta} tone={proxyTone} bind:open={openProxy}>
      {#if trustAll}<p class="warnbox warn">{t('st.access.proxy.trustAll')}</p>{/if}
      {#if srv.you.via_proxy}
        <div class="row wrapr">
          <span class="dim">{t('st.access.proxy.seen', { peer: srv.you.remote_addr, xff: srv.you.forwarded_for })}</span>
          {#if peerListed}<span class="okt">{t('st.access.proxy.listed')}</span>
          {:else}<button class="btn sm primary" type="button" onclick={trustPeer}>{t('st.access.proxy.addPeer', { peer: srv.you.remote_addr })}</button>{/if}
        </div>
      {:else}
        <p class="dim">{t('st.access.proxy.direct', { peer: srv.you.remote_addr })}</p>
      {/if}
      {#if peerUnlisted}<p class="warnbox crit">{t('st.access.proxy.unlisted', { peer: srv.you.remote_addr })}</p>{/if}

      <div class="sec">
        <h4 class="ph">{t('st.access.proxy.list', { n: draft.proxies.length })}</h4>
        {#if draft.proxies.length}
          <div class="panel flush"><table class="tbl compact">
            <thead><tr><th>{t('st.access.col.addr')}</th><th>{t('st.access.col.note')}</th><th></th></tr></thead>
            <tbody>
              {#each draft.proxies as rule, i (rule.cidr)}
                <tr><td class="nw mono">{rule.cidr}</td><td>{rule.note || '—'}</td><td class="r"><button class="btn sm" type="button" aria-label={`${t('st.access.remove')} ${rule.cidr}`} onclick={() => draft?.proxies.splice(i, 1)}>{t('st.access.remove')}</button></td></tr>
              {/each}
            </tbody>
          </table></div>
        {:else}<p class="dim">{t('st.access.proxy.empty')}</p>{/if}
        <form class="row add" autocomplete="off" onsubmit={addProxy}>
          <span class="field grow"><input placeholder={t('st.access.proxy.addr.ph')} aria-label={t('st.access.addr.aria')} bind:value={newProxy} /></span>
          <span class="field grow"><input placeholder={t('st.access.note.ph')} aria-label={t('st.access.note.ph')} maxlength="100" bind:value={newProxyNote} /></span>
          <button class="btn primary" type="submit">{t('st.access.add')}</button>
        </form>
        {#if proxyErr}<div class="err" role="alert">{proxyErr}</div>{/if}
      </div>
    </Fold>

    <Fold title={t('st.access.log.title')} meta={srv.denied_24h ? t('st.access.log.metaDenied', { n: srv.denied_24h }) : t('st.access.log.metaNone')} tone={srv.denied_24h ? 'warn' : 'muted'} bind:open={openLog}>
      <div class="row wrapr">
        {#each FILTERS as f (f)}
          <button class="chip" type="button" aria-pressed={filter === f} onclick={() => void setFilter(f)}>{t(`st.access.log.f.${f}`)}</button>
        {/each}
        <span class="grow"></span>
        <span class="row dim">{t('st.access.log.retention')}<CustomSelect small value={retention} options={retentionOptions} onchange={(v) => { retention = v; void setRetention(v); }} /></span>
      </div>
      <div class="panel flush"><table class="tbl compact">
        <thead><tr><th>{t('st.access.log.col.time')}</th><th>{t('st.access.log.col.client')}</th><th>{t('st.access.log.col.how')}</th><th>{t('st.access.log.col.event')}</th><th>{t('st.access.log.col.login')}</th><th></th></tr></thead>
        <tbody>
          {#each rows as r (r.id)}
            <tr>
              <td class="nw">{formatTime(r.ts)}</td>
              <td class="nw mono">{r.client_ip}</td>
              <td class="nw">
                {#if r.via_proxy}{t('st.access.log.via', { proxy: r.remote_addr })}{#if !r.proxy_trusted} <span class="badge b-warn" title={t('st.access.log.untrustedHint')}>{t('st.access.log.untrusted')}</span>{/if}
                {:else}{t('st.access.log.direct')}{/if}
              </td>
              <td class="nw"><span class="badge {r.event === 'login.ok' ? 'b-ok' : r.event === 'login.fail' ? 'b-warn' : r.event === 'denied' ? 'b-crit' : 'b-mute'}">{t(`st.access.log.e.${r.event}`)}</span></td>
              <td>{r.login || '—'}</td>
              <td class="r">{#if srv.config.mode !== 'off'}<button class="btn sm" type="button" onclick={() => toList(r.client_ip)}>{t(srv.config.mode === 'black' ? 'st.access.log.toBlack' : 'st.access.log.toWhite')}</button>{/if}</td>
            </tr>
          {:else}
            <tr><td colspan="6" class="dim">{t('st.access.log.empty')}</td></tr>
          {/each}
        </tbody>
      </table></div>
      {#if moreRows}<div><button class="btn sm" type="button" onclick={() => void loadLog(true)}>{t('st.access.log.more')}</button></div>{/if}
      <p class="dim">{t('st.access.log.foot')}</p>
    </Fold>

    {#if dirty}
      <div class="applybar" role="region" aria-live="polite">
        {#if confirming}
          <div class="msg">
            <b>{t('st.access.confirm.title')}</b>
            {t('st.access.confirm.text', { mode: t(`st.access.mode.${draft.mode}`), n: modeCount })}{draft.only_proxy && draft.mode !== 'off' ? t('st.access.confirm.onlyProxy') : ''}
            {#if check && draft.mode !== 'off'}<br />{t('st.access.confirm.you', { ip: check.you.ip })}{/if}
            <br /><span class="dim">{t('st.access.confirm.audit')}</span>
          </div>
          <button class="btn" type="button" disabled={busy} onclick={() => (confirming = false)}>{t('st.access.cancel')}</button>
          <button class="btn primary" type="button" disabled={busy} onclick={() => void apply()}>{t('st.access.confirm.ok')}</button>
        {:else}
          <div class="msg">
            {#if problems.length}
              <span class="critt">{t('st.access.cannotApply')}</span>
              {#each problems as p (p.code + (p.host ?? ''))}<br />{problemText(p)}{/each}
            {:else}{t('st.access.dirty')}{/if}
          </div>
          <button class="btn" type="button" onclick={revert}>{t('st.access.revert')}</button>
          <button class="btn primary" type="button" disabled={busy || problems.length > 0 || !check} onclick={() => (confirming = true)}>{t('st.access.apply')}</button>
        {/if}
      </div>
    {/if}
  {/if}
</div>
