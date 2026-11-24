<script lang="ts">
  import { onMount } from 'svelte';
  import { ApiError, serverApi2, type FwListener, type FwPorts, type FwSet } from '../api';
  import { buildRule, type ListRef } from '../fw-rule';
  import { t } from '../i18n.svelte';
  import SourceInput from './SourceInput.svelte';

  // The listening ports of the server and what the firewall says about each; a port without a rule can be closed or
  // limited to an address: the rules go to the draft of the input chain.
  let { serverId, lists, onrules, onsummary }: { serverId: string; lists: FwSet[]; onrules: (texts: string[]) => Promise<string | null>; onsummary?: (n: number, norule: number) => void } = $props();

  type Filter = 'all' | 'out' | 'local' | 'norule';
  let data = $state<FwPorts | null>(null);
  let failed = $state(false);
  let loading = $state(false);
  let filter = $state<Filter>('all');
  let plan = $state<{ l: FwListener; mode: 'close' | 'restrict' } | null>(null);
  let source = $state('');
  let planError = $state('');
  let planBusy = $state(false);

  const all = $derived(data?.listeners ?? []);
  const outside = (l: FwListener): boolean => l.scope !== 'local';
  const counts = $derived({ all: all.length, out: all.filter(outside).length, local: all.filter((l) => !outside(l)).length, norule: all.filter((l) => l.firewall === 'open').length });
  const rows = $derived(all.filter((l) => (filter === 'out' ? outside(l) : filter === 'local' ? !outside(l) : filter === 'norule' ? l.firewall === 'open' : true)));
  const refs = $derived<ListRef[]>(lists.map((x) => ({ name: x.name, family: x.family })));

  async function load(): Promise<void> {
    loading = true;
    try {
      const next = await serverApi2.firewallPorts(serverId);
      data = next;
      failed = false;
      if (next.available) onsummary?.(next.listeners.length, next.listeners.filter((l) => l.firewall === 'open').length);
    } catch (e) {
      failed = !(e instanceof ApiError && e.status === 501);
    } finally {
      loading = false;
    }
  }
  onMount(() => {
    void load();
    const i = setInterval(() => void load(), 15000);
    return () => clearInterval(i);
  });

  const tone = (l: FwListener): string => (l.firewall === 'allowed' || l.firewall === 'restricted' ? 'b-ok' : l.firewall === 'open' ? (l.risky ? 'b-crit' : 'b-warn') : 'b-mute');
  const label = (l: FwListener): string => (l.firewall === 'open' ? t(l.risky ? 'fw.ports.st.openRisky' : 'fw.ports.st.open') : t(`fw.ports.st.${l.firewall}`));
  const key = (l: FwListener): string => l.proto + l.port + l.scope + l.process + l.addrs.join(',');

  async function submitPlan(): Promise<void> {
    if (!plan || planBusy) return;
    const { l, mode } = plan;
    const texts: string[] = [];
    if (mode === 'restrict') {
      if (!source.trim()) return void (planError = t('fw.ports.needSource'));
      const allow = buildRule({ proto: l.proto, port: String(l.port), source: source.trim(), action: 'accept', comment: '' }, refs);
      if (!allow.ok) return void (planError = t(allow.error));
      texts.push(allow.text);
    }
    const deny = buildRule({ proto: l.proto, port: String(l.port), source: '', action: 'drop', comment: '' }, refs);
    if (!deny.ok) return void (planError = t(deny.error));
    texts.push(deny.text);
    planBusy = true;
    planError = (await onrules(texts)) ?? '';
    planBusy = false;
    if (!planError) plan = null;
  }
</script>

<div class="row"><h4 class="ph" style="margin:0">{t('fw.ports.title')}</h4><span class="grow"></span><button class="btn sm" type="button" disabled={loading} onclick={load}>{t('fw.ports.refresh')}</button></div>
{#if !data}<p class="dim">{failed ? t('fw.ports.error') : t('dash.loading')}</p>
{:else if !data.available}<p class="dim">{t('fw.ports.noSs')}</p>
{:else}
  <p>{t('fw.ports.summary', { n: all.length })} {#if counts.norule > 0}<span class="crit-t">{t('fw.ports.exposed', { n: counts.norule })}</span>{/if}</p>
  <div class="row">
    {#each [['all', 'fw.ports.f.all'], ['out', 'fw.ports.f.out'], ['local', 'fw.ports.f.local'], ['norule', 'fw.ports.f.norule']] as [k, lb] (k)}
      <button class="chip" type="button" aria-pressed={filter === k} onclick={() => (filter = k as Filter)}>{t(lb)}<em>{counts[k as keyof typeof counts]}</em></button>
    {/each}
  </div>
  <div class="tw"><table class="tbl compact">
    <thead><tr><th>{t('fw.ports.col.port')}</th><th>{t('fw.ports.col.addr')}</th><th>{t('fw.ports.col.process')}</th><th class="r">{t('fw.ports.col.conns')}</th><th>{t('fw.ports.col.fw')}</th><th></th></tr></thead>
    <tbody>
      {#each rows as l (key(l))}
        <tr><td class="mono">{l.port}/{l.proto}</td><td class="mono dim">{l.addrs.join(', ')}</td><td>{l.process || '—'}{#if l.pid} <span class="dim">({l.pid})</span>{/if}</td><td class="mono r">{l.proto === 'tcp' ? l.conns : '—'}</td>
          <td><span class="badge {tone(l)}">{label(l)}</span></td>
          <td class="r">{#if l.firewall === 'open'}<button class="btn sm" type="button" onclick={() => { plan = { l, mode: l.risky ? 'close' : 'restrict' }; source = ''; planError = ''; }}>{t(l.risky ? 'fw.ports.close' : 'fw.ports.restrict')}</button>{/if}</td></tr>
      {:else}<tr><td colspan="6" class="dim">{t('fw.ports.none')}</td></tr>{/each}
    </tbody>
  </table></div>
  {#if plan}
    <div class="warnbox warn" style="display:grid;gap:8px">
      <p>{t('fw.ports.plan', { port: `${plan.l.port}/${plan.l.proto}`, process: plan.l.process || '?' })}</p>
      <div class="row">
        <button class="chip" type="button" aria-pressed={plan.mode === 'close'} onclick={() => plan && (plan.mode = 'close')}>{t('fw.ports.modeClose')}</button>
        <button class="chip" type="button" aria-pressed={plan.mode === 'restrict'} onclick={() => plan && (plan.mode = 'restrict')}>{t('fw.ports.modeRestrict')}</button>
      </div>
      {#if plan.mode === 'restrict'}<div class="fl"><span>{t('fw.ports.source')}</span><SourceInput {lists} placeholder="10.20.0.0/24" ariaLabel={t('fw.ports.source')} bind:value={source} oninput={() => (planError = '')} /></div>{/if}
      <p class="dim">{t('fw.ports.planNote')}</p>
      {#if planError}<div class="err">{planError}</div>{/if}
      <div class="row end"><button class="btn" type="button" onclick={() => (plan = null)}>{t('common.cancel')}</button><button class="btn primary" type="button" disabled={planBusy} onclick={submitPlan}>{t('fw.ports.toDraft')}</button></div>
    </div>
  {/if}
  <div class="two" style="align-items:start">
    <div><div class="sh">{t('fw.ports.top')}</div>
      {#if data.remotes.length}<div class="tw"><table class="tbl compact"><thead><tr><th>{t('fw.ports.col.remote')}</th><th>{t('fw.ports.col.port')}</th><th class="r">{t('fw.ports.col.conns')}</th></tr></thead>
        <tbody>{#each data.remotes as r (r.addr + r.port)}<tr><td class="mono">{r.addr}</td><td class="mono">{r.port}</td><td class="mono r">{r.conns}</td></tr>{/each}</tbody></table></div>
      {:else}<p class="dim">{t('fw.ports.noRemotes')}</p>{/if}</div>
    <div><div class="sh">{t('fw.ports.howTitle')}</div><p class="dim">{t('fw.ports.how')}</p></div>
  </div>
{/if}
