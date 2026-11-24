<script lang="ts">
  import { onMount } from 'svelte';
  import { api, ApiError, type AccessHost, type AccessLogRow, type AccessMode, type AccessRule, type AppAccessCheck, type AppAccessConfig, type AppAccessState } from '../api';
  import { isUnauthorized } from '../errors';
  import { i18n, t } from '../i18n.svelte';
  import { showToast } from '../toast';
  import Fold from './Fold.svelte';

  // Settings, "Application access": who may connect to the application port of the server. Its own white or black list of
  // addresses, apart from the admin panel. The port is a direct connection, so the list judges the address of the
  // connection itself.
  let { onauthlost }: { onauthlost: () => void } = $props();

  const FILTERS = ['all', 'denied', 'login', 'fail'] as const;
  type Filter = (typeof FILTERS)[number];
  const RFC1918 = ['10.0.0.0/8', '172.16.0.0/12', '192.168.0.0/16'];
  const MODES: AccessMode[] = ['off', 'white', 'black'];

  let srv = $state<AppAccessState | null>(null);
  let draft = $state<AppAccessConfig | null>(null);
  let check = $state<AppAccessCheck | null>(null);
  let loaded = $state(false);
  let busy = $state(false);
  let confirming = $state(false);
  let error = $state('');

  let openList = $state(true);
  let openLog = $state(false);

  let newAddr = $state('');
  let newNote = $state('');
  let addErr = $state('');
  let testIp = $state('');
  let hostBusy = $state(false);
  let refreshTick = $state(0); // a name was refreshed: ask the server about the draft again
  let now = $state(Date.now()); // the "N min ago" of the host names

  let filter = $state<Filter>('all');
  let rows = $state<AccessLogRow[]>([]);
  let moreRows = $state(false);

  const listKey = $derived(draft?.mode === 'black' ? 'black' : 'white');
  const list = $derived(draft ? draft[listKey] : []);
  const dirty = $derived(!!srv && !!draft && JSON.stringify(draft) !== JSON.stringify(srv.config));
  const problems = $derived(check?.problems ?? []);
  const hosts = $derived<Record<string, AccessHost>>({ ...(srv?.hosts ?? {}), ...(check?.hosts ?? {}) });
  const hasHosts = $derived(list.some((r) => isHostRule(r)));
  const modeCount = $derived(draft && draft.mode !== 'off' ? draft[draft.mode].length : 0);

  function fail(e: unknown): void {
    if (isUnauthorized(e)) onauthlost();
    else error = t('aa.failed');
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
      const res = await api.appAccessLog(filter, before);
      rows = more ? [...rows, ...res.rows] : res.rows;
      moreRows = res.rows.length >= 50;
    } catch (e) {
      fail(e);
    }
  }

  async function load(): Promise<void> {
    try {
      srv = await api.appAccess();
      draft = clone(srv.config);
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
    const timer = setTimeout(async () => {
      const mine = ++seq;
      try {
        const res = await api.appAccessCheck(body, ip || undefined);
        if (mine === seq) check = res;
      } catch (e) {
        if (isUnauthorized(e)) onauthlost();
      }
    }, 200);
    return () => clearTimeout(timer);
  });

  // ---- actions ----

  function setMode(mode: AccessMode): void {
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
        addErr = t('aa.err.dupHost');
        return;
      }
      hostBusy = true;
      try {
        await api.accessResolve(host);
      } catch (e) {
        if (isUnauthorized(e)) onauthlost();
        else addErr = e instanceof ApiError && e.status === 422 ? t('aa.err.unresolved', { host }) : t('aa.failed');
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
    addErr = res ? t(`aa.err.${res}`) : '';
    if (!res) {
      newAddr = '';
      newNote = '';
    }
  }

  function addRfc(): void {
    if (!draft) return;
    for (const c of RFC1918) addTo(draft.white, c, t('aa.rfc.note'));
  }

  /** Looks the name up again at once (the server also does it every minute). */
  async function refreshHost(host: string): Promise<void> {
    if (hostBusy) return;
    hostBusy = true;
    try {
      const res = await api.accessResolve(host);
      refreshTick++;
      if (srv) srv.hosts[host] = res.status;
      showToast(res.status.failing ? t('aa.host.stillDown', { host }) : t('aa.host.refreshed', { host, ips: res.status.ips.join(', ') }));
    } catch (e) {
      if (isUnauthorized(e)) onauthlost();
      else showToast(t('aa.err.unresolved', { host }));
    } finally {
      hostBusy = false;
    }
  }

  function removeRule(index: number): void {
    draft?.[listKey].splice(index, 1);
  }

  function toList(ip: string): void {
    if (!draft || draft.mode === 'off') return;
    const res = addTo(draft[listKey], ip, '');
    if (res === 'dup') showToast(t('aa.log.exists'));
    else if (!res) {
      openList = true;
      showToast(t('aa.log.added', { ip }));
    }
  }

  async function apply(): Promise<void> {
    if (!draft || busy) return;
    busy = true;
    error = '';
    try {
      srv = await api.appAccessSave($state.snapshot(draft));
      draft = clone(srv.config);
      confirming = false;
      showToast(t('aa.applied'));
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
    addErr = '';
  }

  async function setFilter(f: Filter): Promise<void> {
    filter = f;
    await loadLog();
  }

  // ---- text ----

  const reasonText = (code: string): string => t(`aa.reason.${code}`);
  const problemText = (p: { code: string; ip?: string; reason?: string; host?: string }): string => t(`aa.problem.${p.code}`, { ip: p.ip ?? '', host: p.host ?? '', reason: p.reason ? reasonText(p.reason) : '' });

  function ago(ts: number): string {
    const s = Math.max(0, Math.round((now - ts * 1000) / 1000));
    if (s < 60) return t('aa.ago.s', { n: s });
    if (s < 3600) return t('aa.ago.m', { n: Math.floor(s / 60) });
    return t('aa.ago.h', { n: Math.floor(s / 3600) });
  }

  /** The line under the addresses of a name: when they were checked, or what is wrong. */
  function hostLine(h: AccessHost | undefined): { text: string; tone: 'ok' | 'warn' } {
    if (!h || !h.checked_at) return { text: t('aa.host.never'), tone: 'warn' };
    if (h.expired) return { text: t('aa.host.expired', { ago: ago(h.checked_at) }), tone: 'warn' };
    if (h.failing) return { text: t('aa.host.failing', { ago: ago(h.checked_at) }), tone: 'warn' };
    if (now - h.checked_at * 1000 < 5000) return { text: t('aa.host.okNow'), tone: 'ok' };
    return { text: t('aa.host.ok', { ago: ago(h.checked_at) }), tone: 'ok' };
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

  const listMeta = $derived(!srv ? '' : srv.config.mode === 'off' ? t('aa.meta.off') : t(`aa.meta.${srv.config.mode}`, { n: srv.config[srv.config.mode].length }));
</script>

<div class="set-sec access">
  <h3>{t('aa.title')}</h3>
  <p class="dim">{t('aa.lead')}</p>

  {#if !loaded}
    <p class="dim">{t('dash.loading')}</p>
  {:else if srv && draft}
    {#if error}<div class="err" role="alert">{error}</div>{/if}
    {#if !srv.configured}<p class="warnbox">{t('aa.inherited')}</p>{/if}

    <Fold title={t('aa.restrict')} meta={listMeta} tone={srv.config.mode === 'off' ? 'muted' : 'ok'} bind:open={openList}>
      <div class="radios" role="radiogroup" aria-label={t('aa.mode.label')}>
        {#each MODES as m (m)}
          <label class="opt" class:on={draft.mode === m}>
            <input type="radio" name="app-access-mode" value={m} checked={draft.mode === m} onchange={() => setMode(m)} />
            <span><b>{t(`aa.mode.${m}`)}</b><small class="dim">{t(`aa.mode.${m}.hint`)}</small></span>
          </label>
        {/each}
      </div>

      {#if draft.mode !== 'off'}
        <div class="sec">
          <h4 class="ph">{t(`aa.list.${listKey}`, { n: list.length })}</h4>
          {#if list.length}
            <div class="panel flush"><table class="tbl compact">
              <thead><tr><th>{t('aa.col.addr')}</th><th>{t('aa.col.now')}</th><th>{t('aa.col.note')}</th><th></th></tr></thead>
              <tbody>
                {#each list as rule, i (rule.cidr)}
                  {@const host = isHostRule(rule)}
                  {@const info = hosts[rule.cidr]}
                  {@const line = host ? hostLine(info) : null}
                  <tr>
                    <td class="nw mono">{rule.cidr}{#if host}<span class="badge b-info ddns" title={t('aa.host.badgeHint')}>DDNS</span>{/if}</td>
                    <td>
                      {#if host && line}
                        <div class="mono">{#each info?.ips ?? [] as ip (ip)}<div>{ip}</div>{/each}</div>
                        <div class={line.tone === 'ok' ? 'okt' : 'warnt'}>{line.text}</div>
                      {:else}<span class="dim">—</span>{/if}
                    </td>
                    <td>{rule.note || '—'}</td>
                    <td class="r nw">
                      {#if host}<button class="btn sm" type="button" disabled={hostBusy} aria-label={`${t('aa.host.refresh')} ${rule.cidr}`} onclick={() => void refreshHost(rule.cidr)}>{t('aa.host.refresh')}</button>{' '}{/if}
                      <button class="btn sm" type="button" aria-label={`${t('aa.remove')} ${rule.cidr}`} onclick={() => removeRule(i)}>{t('aa.remove')}</button>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table></div>
            {#if hasHosts}<p class="warnbox warn">{t('aa.host.note')}</p>{/if}
          {:else}
            <p class="dim">{t('aa.empty')}</p>
          {/if}
          <form class="row add" autocomplete="off" onsubmit={addRule}>
            <span class="field grow2"><input placeholder={t('aa.addr.ph')} aria-label={t('aa.addr.aria')} bind:value={newAddr} /></span>
            <span class="field grow"><input placeholder={t('aa.note.ph')} aria-label={t('aa.note.ph')} maxlength="100" bind:value={newNote} /></span>
            <button class="btn primary" type="submit" disabled={hostBusy}>{t('aa.add')}</button>
            {#if draft.mode === 'white'}<button class="btn" type="button" onclick={addRfc}>{t('aa.rfc')}</button>{/if}
          </form>
          {#if addErr}<div class="err" role="alert">{addErr}</div>{/if}
        </div>

        <div class="sec">
          <h4 class="ph">{t('aa.test.title')}</h4>
          <span class="field"><input placeholder="198.51.100.20" aria-label={t('aa.test.aria')} bind:value={testIp} /></span>
          <div aria-live="polite">
            {#if !testIp.trim()}
              <span class="dim">{t('aa.test.enter')}</span>
            {:else if check?.test}
              <span class="badge {check.test.allowed ? 'b-ok' : 'b-crit'}">{t(check.test.allowed ? 'aa.test.allowed' : 'aa.test.denied')}</span>
              {check.test.ip} — {reasonText(check.test.reason)}
            {/if}
          </div>
        </div>

        {#if check?.you && problems.length === 0}
          <p class="okbox">{t('aa.you.ok', { ip: check.you.ip, reason: reasonText(check.you.reason) })}</p>
        {/if}
      {/if}
    </Fold>

    <Fold title={t('aa.log.title')} meta={srv.denied_24h ? t('aa.log.metaDenied', { n: srv.denied_24h }) : t('aa.log.metaNone')} tone={srv.denied_24h ? 'warn' : 'muted'} bind:open={openLog}>
      <div class="row wrapr">
        {#each FILTERS as f (f)}
          <button class="chip" type="button" aria-pressed={filter === f} onclick={() => void setFilter(f)}>{t(`aa.log.f.${f}`)}</button>
        {/each}
      </div>
      <div class="panel flush"><table class="tbl compact">
        <thead><tr><th>{t('aa.log.col.time')}</th><th>{t('aa.log.col.client')}</th><th>{t('aa.log.col.event')}</th><th>{t('aa.log.col.login')}</th><th></th></tr></thead>
        <tbody>
          {#each rows as r (r.id)}
            <tr>
              <td class="nw">{formatTime(r.ts)}</td>
              <td class="nw mono">{r.client_ip}</td>
              <td class="nw"><span class="badge {r.event === 'login.ok' ? 'b-ok' : r.event === 'login.fail' ? 'b-warn' : r.event === 'denied' ? 'b-crit' : 'b-mute'}">{t(`aa.log.e.${r.event}`)}</span></td>
              <td>{r.login || '—'}</td>
              <td class="r">{#if srv.config.mode !== 'off'}<button class="btn sm" type="button" onclick={() => toList(r.client_ip)}>{t(srv.config.mode === 'black' ? 'aa.log.toBlack' : 'aa.log.toWhite')}</button>{/if}</td>
            </tr>
          {:else}
            <tr><td colspan="5" class="dim">{t('aa.log.empty')}</td></tr>
          {/each}
        </tbody>
      </table></div>
      {#if moreRows}<div><button class="btn sm" type="button" onclick={() => void loadLog(true)}>{t('aa.log.more')}</button></div>{/if}
      <p class="dim">{t('aa.log.foot')}</p>
    </Fold>

    {#if dirty || !srv.configured}
      <div class="applybar" role="region" aria-live="polite">
        {#if confirming}
          <div class="msg">
            <b>{t('aa.confirm.title')}</b>
            {t('aa.confirm.text', { mode: t(`aa.mode.${draft.mode}`), n: modeCount })}
            {#if check?.you && draft.mode !== 'off'}<br />{t('aa.confirm.you', { ip: check.you.ip })}{/if}
            <br /><span class="dim">{t('aa.confirm.audit')}</span>
          </div>
          <button class="btn" type="button" disabled={busy} onclick={() => (confirming = false)}>{t('aa.cancel')}</button>
          <button class="btn primary" type="button" disabled={busy} onclick={() => void apply()}>{t('aa.confirm.ok')}</button>
        {:else}
          <div class="msg">
            {#if problems.length}
              <span class="critt">{t('aa.cannotApply')}</span>
              {#each problems as p (p.code + (p.host ?? ''))}<br />{problemText(p)}{/each}
            {:else if dirty}{t('aa.dirty')}
            {:else}{t('aa.separate')}{/if}
          </div>
          {#if dirty}<button class="btn" type="button" onclick={revert}>{t('aa.revert')}</button>{/if}
          <button class="btn primary" type="button" disabled={busy || problems.length > 0 || !check} onclick={() => (confirming = true)}>{t('aa.apply')}</button>
        {/if}
      </div>
    {/if}
  {/if}
</div>
