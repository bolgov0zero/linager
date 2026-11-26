<script lang="ts">
  import { onMount } from 'svelte';
  import { api, ApiError, subscribeLive, type DeviceRow, type DevicesState } from '../api';
  import { confirmAsk } from '../dialogs.svelte';
  import { isUnauthorized } from '../errors';
  import Icon from '../Icon.svelte';
  import { i18n, t } from '../i18n.svelte';
  import { showToast } from '../toast';
  import { deviceTitle, isMobile, platformKey, versions, waiting } from './devices-util';

  let { onauthlost }: { onauthlost: () => void } = $props();

  let info = $state<DevicesState | null>(null);
  let failed = $state(false);
  let busy = $state(false);
  let renaming = $state<{ id: number; label: string } | null>(null);
  /** Turning the confirmation on asks which of the open sessions stay. */
  let enabling = $state(false);
  let keep = $state(true);

  const when = (iso: string): string => new Date(iso).toLocaleString(i18n.locale);
  const name = (d: DeviceRow): string => deviceTitle(d, t(platformKey(d.platform)));
  const rows = $derived(info?.devices ?? []);
  const pending = $derived(waiting(rows));

  async function load(): Promise<void> {
    try {
      info = await api.devices();
      failed = false;
    } catch (e) {
      if (isUnauthorized(e)) onauthlost();
      else failed = true;
    }
  }
  onMount(() => {
    void load();
    // a new device, a decision of another administrator: the table follows
    const stop = subscribeLive(() => void load(), ['devices']);
    const timer = setInterval(() => void load(), 30_000); // the last IP and the time of the last activity
    return () => {
      stop();
      clearInterval(timer);
    };
  });

  function errText(e: unknown): string {
    if (e instanceof ApiError) {
      if (e.status === 0) return t('st.common.networkError');
      if (e.code === 'device_required') return t('st.dev.err.deviceRequired');
      if (e.status === 409) return t('st.dev.err.self');
    }
    return t('common.genericError');
  }

  async function run(action: () => Promise<unknown>, done = ''): Promise<void> {
    if (busy) return;
    busy = true;
    try {
      await action();
      if (done) showToast(done);
      await load();
    } catch (e) {
      if (isUnauthorized(e)) onauthlost();
      else showToast(errText(e));
    } finally {
      busy = false;
    }
  }

  const approve = (d: DeviceRow): Promise<void> => run(() => api.deviceApprove(d.id), t('st.dev.approved', { name: name(d) }));
  async function deny(d: DeviceRow): Promise<void> {
    if (!(await confirmAsk({ title: t('st.dev.deny'), text: t('st.dev.confirmDeny', { name: name(d), user: d.user }), ok: t('st.dev.deny'), danger: true }))) return;
    await run(() => api.deviceDeny(d.id), t('st.dev.denied', { name: name(d) }));
  }
  async function remove(d: DeviceRow): Promise<void> {
    const text = d.current ? t('st.dev.confirmRemoveSelf') : t('st.dev.confirmRemove', { name: name(d), user: d.user });
    if (!(await confirmAsk({ title: t('st.dev.remove'), text, ok: t('st.dev.remove'), danger: true }))) return;
    await run(() => api.deviceRemove(d.id), t('st.dev.removed', { name: name(d) }));
  }
  async function saveLabel(): Promise<void> {
    const r = renaming;
    if (!r) return;
    renaming = null;
    await run(() => api.deviceLabel(r.id, r.label.trim()));
  }

  async function turnOn(): Promise<void> {
    enabling = false;
    await run(() => api.devicesGate(true, keep), t('st.dev.gateOnDone'));
  }
  async function turnOff(): Promise<void> {
    if (!(await confirmAsk({ title: t('st.dev.gateOffTitle'), text: t('st.dev.gateOffText'), ok: t('st.dev.gateOff'), danger: true }))) return;
    await run(() => api.devicesGate(false, true), t('st.dev.gateOffDone'));
  }
</script>

<div class="set-sec">
  <div class="row"><h3 style="margin:0">{t('st.dev.title')}</h3></div>
  <p class="dim">{t('st.dev.hint')}</p>

  {#if failed}<div class="err" role="alert">{t('st.dev.loadFailed')}</div>
  {:else if !info}<p class="dim">{t('dash.loading')}</p>
  {:else}
    <section class="panel dev-gate">
      <div class="row">
        <div class="grow">
          <b>{t('st.dev.gate')}</b>
          <span class="badge {info.enabled ? 'b-ok' : 'b-mute'}">{info.enabled ? t('st.dev.on') : t('st.dev.off')}</span>
          <p class="dim" style="margin:4px 0 0">{info.enabled ? t('st.dev.gateOnHint') : t('st.dev.gateOffHint')}</p>
        </div>
        {#if info.enabled}
          <button class="btn" type="button" disabled={busy} onclick={() => void turnOff()}>{t('st.dev.gateOff')}</button>
        {:else}
          <button class="btn primary" type="button" disabled={busy || !info.can_enable} onclick={() => (enabling = !enabling)}>{t('st.dev.gateOn')}</button>
        {/if}
      </div>
      {#if !info.enabled && !info.can_enable}<p class="dim" style="margin:8px 0 0">{t('st.dev.cannotEnable')}</p>{/if}
      {#if enabling}
        <div class="mdl-stack sub">
          <p style="margin:0">{t('st.dev.enableText')}</p>
          <label class="ck"><input type="radio" name="keep" checked={keep} onchange={() => (keep = true)} />{t('st.dev.keepSessions')}</label>
          <label class="ck"><input type="radio" name="keep" checked={!keep} onchange={() => (keep = false)} />{t('st.dev.cutSessions')}</label>
          <p class="dim" style="margin:0">{t('st.dev.recovery')} <code>linager server devices off</code></p>
          <div class="row end"><button class="btn" type="button" onclick={() => (enabling = false)}>{t('common.cancel')}</button><button class="btn primary" type="button" disabled={busy} onclick={() => void turnOn()}>{t('st.dev.gateOn')}</button></div>
        </div>
      {/if}
    </section>

    {#if pending.length}
      <h4 class="ph">{t('st.dev.waiting', { n: pending.length })}</h4>
      <ul class="set-list col">
        {#each pending as d (d.id)}
          <li>
            <div class="urow">
              <div class="info">
                <Icon name={isMobile(d.platform) ? 'smartphone' : 'monitor'} size={15} />
                <b>{name(d)}</b>
                <span class="badge b-warn">{t('st.dev.status.pending')}</span>
                <span class="dim">{t('st.dev.requestLine', { user: d.user, version: versions(d) || '—', ip: d.last_ip || '—', time: when(d.created_at) })}</span>
              </div>
              <div class="row">
                <button class="btn sm primary" type="button" disabled={busy} onclick={() => void approve(d)}>{t('st.dev.approve')}</button>
                <button class="btn sm danger" type="button" disabled={busy} onclick={() => void deny(d)}>{t('st.dev.deny')}</button>
              </div>
            </div>
          </li>
        {/each}
      </ul>
    {/if}

    <h4 class="ph">{t('st.dev.all')}</h4>
    {#if rows.length === 0}
      <p class="dim">{t('st.dev.empty')}</p>
    {:else}
      <div class="tbl-wrap">
        <table class="tbl dev-tbl">
          <thead>
            <tr>
              <th>{t('st.dev.col.device')}</th><th>{t('st.dev.col.user')}</th><th>{t('st.dev.col.app')}</th><th>{t('st.dev.col.os')}</th>
              <th>{t('st.dev.col.ip')}</th><th>{t('st.dev.col.seen')}</th><th>{t('st.dev.col.status')}</th><th class="r"></th>
            </tr>
          </thead>
          <tbody>
            {#each rows as d (d.id)}
              <tr>
                <td>
                  {#if renaming && renaming.id === d.id}
                    <form class="row" onsubmit={(e) => { e.preventDefault(); void saveLabel(); }}>
                      <span class="field"><input bind:value={renaming.label} maxlength="64" placeholder={d.model || t(platformKey(d.platform))} aria-label={t('st.dev.name')} /></span>
                      <button class="btn sm primary" type="submit">{t('common.save')}</button>
                      <button class="btn sm" type="button" onclick={() => (renaming = null)}>{t('common.cancel')}</button>
                    </form>
                  {:else}
                    <Icon name={isMobile(d.platform) ? 'smartphone' : 'monitor'} size={14} />
                    <b>{name(d)}</b>
                    {#if d.current}<span class="badge b-info">{t('st.dev.thisOne')}</span>{/if}
                    <div class="dim">{t(platformKey(d.platform))}{d.label && d.model ? ` · ${d.model}` : ''}</div>
                  {/if}
                </td>
                <td>{d.user}</td>
                <td>{d.app_version || '—'}</td>
                <td>{d.os_version || '—'}</td>
                <td class="mono">{d.last_ip || '—'}</td>
                <td class="nowrap">{when(d.last_seen)}</td>
                <td><span class="badge {d.status === 'approved' ? 'b-ok' : d.status === 'pending' ? 'b-warn' : 'b-crit'}">{t(`st.dev.status.${d.status}`)}</span></td>
                <td class="r">
                  <div class="row end">
                    {#if d.status === 'pending'}
                      <button class="btn sm primary" type="button" disabled={busy} onclick={() => void approve(d)}>{t('st.dev.approve')}</button>
                    {:else if d.status === 'denied'}
                      <button class="btn sm" type="button" disabled={busy} onclick={() => void approve(d)}>{t('st.dev.approve')}</button>
                    {/if}
                    <button class="btn sm icon ghost" type="button" title={t('st.dev.rename')} aria-label={t('st.dev.rename')} onclick={() => (renaming = { id: d.id, label: d.label })}><Icon name="pencil" size={14} /></button>
                    <button class="btn sm icon ghost" type="button" disabled={busy} title={t('st.dev.remove')} aria-label={t('st.dev.remove')} onclick={() => void remove(d)}><Icon name="trash-2" size={14} /></button>
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  {/if}
</div>
