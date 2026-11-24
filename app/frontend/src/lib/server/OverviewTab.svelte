<script lang="ts">
  import type { ServerDetail } from '../api';
  import { t } from '../i18n.svelte';
  import { fmtBytes, fmtRate, fmtUptime, meter, sparkPath } from '../status';

  let { s }: { s: ServerDetail } = $props();
  const sm = $derived(s.sample);
  const off = $derived(!s.online || !sm);
  const units = $derived({ d: t('unit.d'), h: t('unit.h'), min: t('unit.min') });
  const sp = (a: number[]) => sparkPath(a, 120, 34);
  const cpu = $derived(sp(s.recent.map((r) => r.cpu)));
  const mem = $derived(sp(s.recent.map((r) => r.mem)));
  const rx = $derived(sp((() => { const m = Math.max(1, ...s.recent.map((r) => r.rx)); return s.recent.map((r) => (r.rx / m) * 100); })()));
  const ifaces = $derived(sm?.net ?? []);
</script>

<div class="mcs">
  <div class="mc lv-{off ? 'ok' : meter(sm!.cpu)}">
    <div class="mc-h">CPU</div><div class="mc-v">{off ? '—' : Math.round(sm!.cpu)}{#if !off}<small>%</small>{/if}</div>
    <div class="mc-s">{#if sm?.load}load average: {sm.load.map((x) => x.toFixed(2)).join(' · ')}{/if}</div>
    {#if cpu.line}<svg class="spark" viewBox="0 0 120 34" preserveAspectRatio="none" aria-hidden="true"><path class="sa" d={cpu.area} /><path class="sl" d={cpu.line} vector-effect="non-scaling-stroke" /></svg>{/if}
  </div>
  <div class="mc lv-{off ? 'ok' : meter(sm!.mem_pct)}">
    <div class="mc-h">RAM</div><div class="mc-v">{off ? '—' : Math.round(sm!.mem_pct)}{#if !off}<small>%</small>{/if}</div>
    <div class="mc-s">{#if sm?.mem_total}{t('sp.ram.total', { v: fmtBytes(sm.mem_total) })}{/if}</div>
    {#if mem.line}<svg class="spark" viewBox="0 0 120 34" preserveAspectRatio="none" aria-hidden="true"><path class="sa" d={mem.area} /><path class="sl" d={mem.line} vector-effect="non-scaling-stroke" /></svg>{/if}
  </div>
  <div class="mc">
    <div class="mc-h">{t('sp.net.rx')}</div><div class="mc-v">{off ? '—' : fmtRate(sm!.rx_bps)}</div>
    <div class="mc-s">{#if !off}↑ {fmtRate(sm!.tx_bps)}{/if}</div>
    {#if rx.line}<svg class="spark" viewBox="0 0 120 34" preserveAspectRatio="none" aria-hidden="true"><path class="sa" d={rx.area} /><path class="sl" d={rx.line} vector-effect="non-scaling-stroke" /></svg>{/if}
  </div>
  <div class="mc disks">
    <div class="mc-h">{t('sp.disks')}</div>
    {#each (sm?.disks ?? []).slice(0, 2) as d (d.mount)}
      <div class="dk"><div><span class="mono">{d.mount}</span><b class="mono">{Math.round(d.pct)}%</b></div><span class="bar lv-{meter(d.pct)}"><i style="width:{Math.min(100, d.pct)}%"></i></span><small>{fmtBytes(d.used)} / {fmtBytes(d.total)}</small></div>
    {/each}
  </div>
</div>

<section class="panel flush">
  <h4 class="ph">{t('sp.ifaces')}</h4>
  <table class="tbl compact">
    <thead><tr><th>{t('sp.if.name')}</th><th>{t('sp.if.state')}</th><th>IP</th><th>MAC</th><th class="r">{t('sp.if.rx')}</th><th class="r">{t('sp.if.tx')}</th><th class="r">{t('sp.if.rxTotal')}</th><th class="r">{t('sp.if.txTotal')}</th></tr></thead>
    <tbody>
      {#each ifaces as i (i.iface)}
        <tr><td class="mono"><b>{i.iface}</b></td><td><span class="badge {i.up ? 'b-ok' : 'b-mute'}">{i.up ? 'UP' : 'DOWN'}</span></td><td class="mono">{i.ip}</td><td class="mono dim">{i.mac}</td>
          <td class="mono r">{fmtRate(i.rx_bps)}</td><td class="mono r">{fmtRate(i.tx_bps)}</td><td class="mono r">{fmtBytes(i.rx_total)}</td><td class="mono r">{fmtBytes(i.tx_total)}</td></tr>
      {:else}
        <tr><td colspan="8" class="dim">{t('card.nodata')}</td></tr>
      {/each}
    </tbody>
  </table>
</section>

<section class="panel">
  <h4>{t('sp.system')}</h4>
  <div class="dls">
    <div class="dl"><span>{t('sp.sys.os')}</span><b>{s.system?.os ?? s.os}</b></div>
    <div class="dl"><span>{t('sp.sys.kernel')}</span><b>{s.system?.kernel ?? '—'}</b></div>
    <div class="dl"><span>{t('sp.sys.arch')}</span><b>{s.system?.arch ?? '—'}</b></div>
    <div class="dl"><span>{t('sp.sys.cpu')}</span><b>{s.system ? `${s.system.cpu_model} · ${s.system.cpu_cores}` : '—'}</b></div>
    <div class="dl"><span>{t('sp.sys.mem')}</span><b>{fmtBytes(s.system?.mem_total)}</b></div>
    <div class="dl"><span>Uptime</span><b>{sm ? fmtUptime(sm.uptime, units) : '—'}</b></div>
    <div class="dl"><span>{t('sp.sys.host')}</span><b>{s.hostname}</b></div>
    <div class="dl"><span>{t('sp.sys.agent')}</span><b>{s.agent_version}</b></div>
    <div class="dl"><span>{t('sp.sys.enrolled')}</span><b>{new Date(s.enrolled_at).toLocaleDateString()}</b></div>
  </div>
</section>
