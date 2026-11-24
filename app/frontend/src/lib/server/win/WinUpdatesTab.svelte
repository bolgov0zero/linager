<script lang="ts">
  import { onDestroy } from 'svelte';
  import { openStream, winCall, type ServerDetail, type WinUpdate } from '../../api';
  import { confirmAsk } from '../../dialogs.svelte';
  import { errorText } from '../../errors';
  import Icon from '../../Icon.svelte';
  import { t } from '../../i18n.svelte';
  import { fmtBytes } from '../../status';

  let { s }: { s: ServerDetail } = $props();
  const sid = $derived(s.id);

  let list = $state<WinUpdate[] | null>(null);
  let rebootPending = $state(false);
  let searching = $state(false);
  let error = $state('');
  let picked = $state<string[]>([]);
  let log = $state('');
  let running = $state(false);
  let onlySec = $state(false);
  let socket: WebSocket | null = null;

  async function check(): Promise<void> {
    searching = true;
    error = '';
    try {
      const r = await winCall<{ updates: WinUpdate[]; reboot_pending: boolean }>(sid, 'winupdate', 'list');
      list = r.updates;
      rebootPending = r.reboot_pending;
      picked = [];
    } catch (e) {
      error = errorText(e);
    } finally {
      searching = false;
    }
  }

  async function install(ids: string[]): Promise<void> {
    const n = ids.length || list?.length || 0;
    if (!(await confirmAsk({ title: t('win.confirm'), text: t('wup.confirm', { n }), ok: t('win.confirm') }))) return;
    log = '';
    running = true;
    const ws = openStream(sid, 'winupdate', ids.length ? { ids: ids.join(',') } : {});
    socket = ws;
    const dec = new TextDecoder();
    ws.onmessage = (ev) => {
      if (typeof ev.data === 'string') return;
      log += dec.decode(ev.data as ArrayBuffer).replace(/\r\n/g, '\n');
    };
    ws.onclose = () => {
      running = false;
      socket = null;
      void check();
    };
  }
  onDestroy(() => socket?.close());

  const securityList = $derived((list ?? []).filter((u) => u.security));
  const shown = $derived(onlySec ? securityList : (list ?? []));
  const size = (b: number): string => (b > 1 << 20 ? `${(b / (1 << 20)).toFixed(1)} MB` : `${Math.max(1, Math.round(b / 1024))} KB`);
  const toggle = (id: string): void => {
    picked = picked.includes(id) ? picked.filter((x) => x !== id) : [...picked, id];
  };
  const allOn = $derived(shown.length > 0 && shown.every((u) => picked.includes(u.id)));
  const toggleAll = (): void => {
    picked = allOn ? [] : shown.map((u) => u.id);
  };
  void fmtBytes;
</script>

<div class="win-page">
  <div class="sp-tools">
    <button class="btn sm" type="button" disabled={searching || running || !s.online} onclick={check}><Icon name="refresh-cw" size={14} />{t('wup.check')}</button>
    <button class="btn sm" type="button" disabled={running || securityList.length === 0} onclick={() => install(securityList.map((u) => u.id))}>{t('wup.installSec')}</button>
    <button class="btn sm primary" type="button" disabled={running || !list || list.length === 0} onclick={() => install([])}><Icon name="download" size={14} />{t('wup.installAll')}</button>
    <button class="btn sm" type="button" disabled={running || picked.length === 0} onclick={() => install([...picked])}>{t('wup.installSel')}{picked.length ? ` (${picked.length})` : ''}</button>
    <button class="chip" type="button" aria-pressed={onlySec} onclick={() => (onlySec = !onlySec)}>{t('wup.f.security')}</button>
    <span class="grow"></span>
    <span class="dim">{#if list}{t('wup.summary', { n: list.length, s: securityList.length })}{:else if searching}{t('wup.checking')}{:else}{t('wup.first')}{/if}</span>
  </div>
  {#if rebootPending || s.sample?.reboot_required}<div class="banner">{t('wup.rebootPending')}</div>{/if}
  {#if error}<div class="err" role="alert">{error}</div>{/if}
  {#if list && list.length > 0}
    <div class="tw" style="max-height:none"><table class="tbl compact">
      <thead><tr><th class="ck-c"><input type="checkbox" aria-label={t('sp.upd.all')} checked={allOn} disabled={running} onchange={toggleAll} /></th><th>{t('wup.colTitle')}</th><th>KB</th><th class="r">{t('wup.colSize')}</th><th>{t('wup.colKind')}</th></tr></thead>
      <tbody>{#each shown as u (u.id)}
        <tr><td class="ck-c"><input type="checkbox" checked={picked.includes(u.id)} disabled={running} aria-label={u.title} onchange={() => toggle(u.id)} /></td>
          <td>{u.title}{#if u.need_reboot}<div class="dim sub">{t('wup.rebootNote')}</div>{/if}</td><td class="mono dim">{u.kb}</td><td class="mono r">{size(u.size)}</td>
          <td>{#if u.security}<span class="badge b-warn">{t('wup.security')}</span>{/if}{#if u.severity} <span class="dim">{u.severity}</span>{/if}</td></tr>
      {/each}</tbody>
    </table></div>
  {:else if list && list.length === 0}<p class="dim">{t('wup.none')}</p>{/if}
  {#if log || running}
    <section class="panel"><div class="row"><h4 class="ph" style="margin:0">{t('wup.log')}</h4><span class="grow"></span><span class="dim">{running ? t('wup.running') : t('wup.done')}</span></div>
      <pre class="pv" style="max-height:260px;overflow:auto">{log}</pre></section>
  {/if}
</div>
