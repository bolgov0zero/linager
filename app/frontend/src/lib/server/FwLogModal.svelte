<script lang="ts">
  import { onMount } from 'svelte';
  import { serverApi2, type FwLogEvent, type FwSet } from '../api';
  import CustomSelect from '../CustomSelect.svelte';
  import { canLog, ipInList, isLogged, parseRule, ruleLogId, stripLog } from '../fw-view';
  import { t } from '../i18n.svelte';
  import Modal from '../Modal.svelte';

  type Chain = 'input' | 'forward' | 'output';
  interface Known { chain: Chain; index: number; text: string }

  // The journal of the firewall: packets that the rules marked for logging matched (live, filters, grouping, a CSV).
  let { serverId, rules, sets, onToggle, onToggleDrops, onAddToList, onBlock, onOpenRule, onclose }: {
    serverId: string; rules: Known[]; sets: FwSet[];
    onToggle: (chain: Chain, index: number, on: boolean) => void; onToggleDrops: () => void;
    onAddToList: (list: string, ip: string) => string | null; onBlock: (ip: string) => Promise<string | null>;
    onOpenRule: (chain: Chain) => void; onclose: () => void;
  } = $props();

  let events = $state<FwLogEvent[]>([]);
  let loaded = $state(false);
  let available = $state(true);
  let truncated = $state(false);
  let live = $state(true);
  let now = $state(Math.floor(Date.now() / 1000));
  let period = $state(60);
  let chains = $state<Record<string, boolean>>({});
  let actions = $state<Record<string, boolean>>({});
  let protos = $state<Record<string, boolean>>({});
  let port = $state('');
  let source = $state('');
  let group = $state(false);
  let limit = $state(12);
  let openKey = $state<string | null>(null);
  let note = $state('');
  let pickList = $state('');
  let busy = $state(false);

  const loggedRules = $derived(rules.filter((r) => isLogged(r.text)));
  const byId = $derived(new Map(rules.map((r) => [ruleLogId(r.text), r])));
  const elementsOf = $derived(new Map(sets.map((x) => [x.name, x.elements])));

  async function load(): Promise<void> {
    try {
      const res = await serverApi2.firewallLog(serverId);
      events = res.events;
      available = res.available;
      truncated = !!res.truncated;
    } catch {
      // keep what is shown
    } finally {
      loaded = true;
      now = Math.floor(Date.now() / 1000);
    }
  }
  onMount(() => {
    void load();
    const i = setInterval(() => {
      if (live && loggedRules.length > 0) void load();
    }, 5000);
    return () => clearInterval(i);
  });

  function portOk(p: number | undefined, expr: string): boolean {
    const q = expr.trim();
    if (!q) return true;
    if (!p) return false;
    return q.split(/[,\s]+/).filter(Boolean).some((x) => {
      const m = x.match(/^(\d+)-(\d+)$/);
      return m ? p >= Number(m[1]) && p <= Number(m[2]) : String(p) === x;
    });
  }
  const listsOf = (ip: string): string[] => [...elementsOf].filter(([, els]) => ipInList(ip, els)).map(([name]) => name);
  function sourceOk(e: FwLogEvent, q: string): boolean {
    const v = q.trim().toLowerCase();
    if (!v) return true;
    if (v.startsWith('@')) return listsOf(e.src).some((n) => `@${n}`.startsWith(v));
    return e.src.toLowerCase().includes(v);
  }
  const chosen = (o: Record<string, boolean>): string[] => Object.keys(o).filter((k) => o[k]);
  function pass(skip: 'chain' | 'action' | 'proto' | null): FwLogEvent[] {
    const ch = chosen(chains), ac = chosen(actions), pr = chosen(protos);
    const from = now - period * 60;
    return events.filter((e) => e.time >= from && (skip === 'chain' || ch.length === 0 || ch.includes(e.chain)) && (skip === 'action' || ac.length === 0 || ac.includes(e.action)) && (skip === 'proto' || pr.length === 0 || pr.includes(e.proto)) && portOk(e.dport, port) && sourceOk(e, source));
  }
  const inPeriod = $derived(events.filter((e) => e.time >= now - period * 60));
  const rows = $derived(pass(null));
  const activeFilters = $derived(chosen(chains).length + chosen(actions).length + chosen(protos).length + (port.trim() ? 1 : 0) + (source.trim() ? 1 : 0));
  const count = (skip: 'chain' | 'action' | 'proto', key: 'chain' | 'action' | 'proto', value: string): number => pass(skip).filter((e) => e[key] === value).length;

  interface Line { key: string; e: FwLogEvent; n: number }
  const lines = $derived.by<Line[]>(() => {
    if (!group) return rows.map((e, i) => ({ key: `${e.time}:${i}`, e, n: 1 }));
    const map = new Map<string, Line>();
    for (const e of rows) {
      const k = [e.chain, e.action, e.proto, e.src, e.dport ?? 0].join('|');
      const ln = map.get(k);
      if (ln) ln.n++;
      else map.set(k, { key: k, e, n: 1 });
    }
    return [...map.values()].sort((a, b) => b.n - a.n || b.e.time - a.e.time);
  });
  const shownLines = $derived(lines.slice(0, limit));
  const BARS = 48;
  const bars = $derived.by(() => {
    const span = period * 60;
    const cols = Array.from({ length: BARS }, () => ({ a: 0, d: 0 }));
    for (const e of rows) {
      const i = Math.min(BARS - 1, Math.floor(((e.time - (now - span)) / span) * BARS));
      if (i >= 0) cols[i][e.action === 'accept' ? 'a' : 'd']++;
    }
    const max = Math.max(1, ...cols.map((c) => c.a + c.d));
    return cols.map((c) => ({ ...c, ha: (c.a / max) * 100, hd: (c.d / max) * 100 }));
  });
  const stats = $derived({ total: inPeriod.length, blocked: inPeriod.filter((e) => e.action !== 'accept').length, allowed: inPeriod.filter((e) => e.action === 'accept').length, sources: new Set(inPeriod.map((e) => e.src)).size });

  const reset = (): void => {
    chains = {};
    actions = {};
    protos = {};
    port = '';
    source = '';
    limit = 12;
  };
  const toggle = (o: Record<string, boolean>, key: string): void => {
    o[key] = !o[key];
    limit = 12;
  };
  function ago(ts: number): string {
    const m = Math.round((now - ts) / 60);
    if (m < 1) return t('fw.log.justNow');
    if (m < 60) return t('fw.log.minAgo', { n: m });
    if (m < 1440) return t('fw.log.hourAgo', { n: Math.round(m / 60) });
    return t('fw.log.dayAgo', { n: Math.round(m / 1440) });
  }
  function ruleName(e: FwLogEvent): { label: string; known: Known | null } {
    const k = byId.get(e.rule) ?? null;
    return { label: k ? stripLog(k.text) : e.rule, known: k };
  }
  function exportCsv(): void {
    const head = ['time', 'chain', 'action', 'proto', 'src', 'dport', 'dst', 'iface', 'len', 'flags', 'rule'];
    const body = rows.map((e) => [new Date(e.time * 1000).toISOString(), e.chain, e.action, e.proto, e.src, e.dport ?? '', e.dst, e.iface, e.len ?? '', e.flags ?? '', ruleName(e).label]);
    const csv = [head, ...body].map((r) => r.map((c) => `"${String(c).replace(/"/g, '""')}"`).join(',')).join('\n');
    const url = URL.createObjectURL(new Blob([csv], { type: 'text/csv' }));
    const a = document.createElement('a');
    a.href = url;
    a.download = 'firewall-log.csv';
    a.click();
    URL.revokeObjectURL(url);
  }
  async function block(ip: string): Promise<void> {
    if (busy) return;
    busy = true;
    note = (await onBlock(ip)) ?? t('fw.log.blocked', { ip });
    busy = false;
  }
  function addList(ip: string): void {
    const name = pickList || sets[0]?.name;
    if (name) note = onAddToList(name, ip) ?? t('fw.log.addedToList', { ip, list: name });
  }
  const actLabel = (a: string): string => t(`fw.log.act.${a}`);
  const when = (n: number): string => new Date(n * 1000).toLocaleString();
  const listOptions = $derived(sets.map((x) => ({ v: x.name, t: `@${x.name}` })));
</script>

{#snippet detail(e: FwLogEvent, rn: { label: string; known: Known | null })}
  <div class="detbox">
    <div class="kv">
      <span><b>{t('fw.log.col.time')}</b>{when(e.time)}</span><span><b>{t('fw.log.iface')}</b>{e.iface || '—'}</span><span><b>{t('fw.log.dst')}</b>{e.dst}{e.dport ? `:${e.dport}` : ''}</span>
      {#if e.len}<span><b>{t('fw.log.len')}</b>{e.len} B</span>{/if}{#if e.flags}<span><b>{t('fw.log.flags')}</b>{e.flags}</span>{/if}<span><b>{t('fw.log.col.rule')}</b>{rn.label}</span>
    </div>
    <div class="row">
      {#if sets.length > 0}<CustomSelect small value={pickList || sets[0].name} options={listOptions} onchange={(v) => (pickList = v)} /><button class="btn sm" type="button" onclick={() => addList(e.src)}>{t('fw.log.toList')}</button>{/if}
      <button class="btn sm danger" type="button" disabled={busy} onclick={() => block(e.src)}>{t('fw.log.block')}</button>
      {#if rn.known}<button class="btn sm" type="button" onclick={() => onOpenRule(rn.known!.chain)}>{t('fw.log.openRule')}</button>{/if}
    </div>
  </div>
{/snippet}

<Modal title={t('fw.log.title')} wide {onclose}>
  {#if !loaded}<p class="dim">{t('dash.loading')}</p>
  {:else if !available}<p class="dim">{t('fw.log.unavailable')}</p>
  {:else if events.length === 0}
    <p>{t('fw.log.howto')}</p>
    <ol class="dim" style="margin:0;padding-left:18px"><li>{t('fw.log.step1')}</li><li>{t('fw.log.step2')}</li><li>{t('fw.log.step3')}</li></ol>
    <div class="panel flush" style="margin:0"><table class="tbl compact"><tbody>
      {#each rules as r (r.chain + r.index + r.text)}
        {@const v = parseRule(r.text)}
        <tr><td style="width:44px"><button class="rule-sw" type="button" aria-pressed={v.logged} disabled={!canLog(r.text)} aria-label={t('fw.log.toggle')} onclick={() => onToggle(r.chain, r.index, !v.logged)}></button></td>
          <td class="mono">{r.chain} · {stripLog(r.text)}</td><td class="r"><span class="act {v.action}">{t(`fw.act.${v.action}`)}</span></td></tr>
      {:else}<tr><td class="dim">{t('fw.chain.empty')}</td></tr>{/each}
    </tbody></table></div>
    <div class="row"><span class="dim">{t('fw.log.limitNote')}</span><span class="grow"></span><button class="btn sm" type="button" onclick={onToggleDrops}>{t('fw.log.enableDrops')}</button></div>
    <p class="dim">{loggedRules.length > 0 ? t('fw.log.markedNote') : t('fw.log.applyNote')}</p>
  {:else}
    <div class="row">
      {#each [15, 60, 1440, 10080] as p (p)}<button class="chip" type="button" aria-pressed={period === p} onclick={() => { period = p; limit = 12; }}>{t(`fw.log.p.${p}`)}</button>{/each}
      <button class="chip" type="button" aria-pressed={live} onclick={() => (live = !live)}>{t('fw.log.live')}</button>
      <span class="grow"></span>
      <div class="stats">
        <div class="stat"><b>{stats.total}</b><span>{t('fw.log.stat.total')}</span></div>
        <div class="stat"><b class="crit-t">{stats.blocked}</b><span>{t('fw.log.stat.blocked')}</span></div>
        <div class="stat"><b class="ok-t">{stats.allowed}</b><span>{t('fw.log.stat.allowed')}</span></div>
        <div class="stat"><b>{stats.sources}</b><span>{t('fw.log.stat.sources')}</span></div>
      </div>
    </div>
    <div>
      <div class="hist" role="img" aria-label={t('fw.log.hist')}>{#each bars as c, i (i)}<div class="col" title={String(c.a + c.d)}><i class="d" style="height:{c.hd}%"></i><i class="a" style="height:{c.ha}%"></i></div>{/each}</div>
      <div class="axis"><span>{t(`fw.log.ago.${period}`)}</span><span><span class="ok-t">■ {t('fw.log.legend.allowed')}</span> <span class="crit-t">■ {t('fw.log.legend.blocked')}</span></span><span>{t('fw.log.now')}</span></div>
    </div>
    <div class="filters">
      <div class="fg"><span class="lbl">{t('fw.col.chain')}</span><div class="row">{#each ['input', 'forward', 'output'] as c (c)}<button class="chip" type="button" aria-pressed={!!chains[c]} onclick={() => toggle(chains, c)}>{c}<em>{count('chain', 'chain', c)}</em></button>{/each}</div></div>
      <div class="fg"><span class="lbl">{t('fw.col.action')}</span><div class="row">{#each ['accept', 'drop', 'reject'] as a (a)}<button class="chip" type="button" aria-pressed={!!actions[a]} onclick={() => toggle(actions, a)}>{actLabel(a)}<em>{count('action', 'action', a)}</em></button>{/each}</div></div>
      <div class="fg"><span class="lbl">{t('fw.col.proto')}</span><div class="row">{#each ['TCP', 'UDP', 'ICMP'] as p (p)}<button class="chip" type="button" aria-pressed={!!protos[p]} onclick={() => toggle(protos, p)}>{p}<em>{count('proto', 'proto', p)}</em></button>{/each}</div></div>
      <label class="fg"><span class="lbl">{t('fw.log.port')}</span><span class="field"><input class="mono" placeholder="22, 80, 443, 1000-2000" spellcheck="false" autocomplete="off" bind:value={port} oninput={() => (limit = 12)} /></span></label>
      <label class="fg"><span class="lbl">{t('fw.col.source')}</span><span class="field"><input class="mono" placeholder={t('fw.log.sourcePh')} spellcheck="false" autocomplete="off" bind:value={source} oninput={() => (limit = 12)} /></span></label>
      <button class="btn sm" type="button" disabled={activeFilters === 0} onclick={reset}>{t('fw.log.reset')}</button>
    </div>
    <div class="row">
      <span>{group ? t('fw.log.foundGroups', { n: lines.length }) : t('fw.log.foundOf', { n: lines.length, total: inPeriod.length })}{#if activeFilters > 0} <span class="dim">· {t('fw.log.filters', { n: activeFilters })}</span>{/if}</span><span class="grow"></span>
      <button class="chip" type="button" aria-pressed={group} title={t('fw.log.groupHint')} onclick={() => { group = !group; limit = 12; openKey = null; }}>{t('fw.log.group')}</button>
      <button class="btn sm" type="button" disabled={rows.length === 0} onclick={exportCsv}>{t('fw.log.csv')}</button>
    </div>
    {#if note}<div class="ok-t">{note}</div>{/if}
    <div class="tw"><table class="tbl compact">
      <thead><tr><th>{t('fw.log.col.time')}</th><th>{t('fw.col.chain')}</th><th>{t('fw.col.action')}</th><th>{t('fw.col.proto')}</th><th>{t('fw.col.source')}</th><th>{t('fw.col.port')}</th><th>{t('fw.log.col.rule')}</th><th class="r">{group ? t('fw.log.col.times') : t('fw.log.col.packets')}</th></tr></thead>
      <tbody>
        {#each shownLines as l (l.key)}
          {@const e = l.e}{@const rn = ruleName(e)}{@const op = openKey === l.key}
          <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
          <tr class="evrow" onclick={() => (openKey = op ? null : l.key)}>
            <td class="dim" title={when(e.time)}>{group ? t('fw.log.last', { when: ago(e.time) }) : ago(e.time)}</td><td class="mono">{e.chain}</td><td><span class="act {e.action}">{actLabel(e.action)}</span></td><td class="mono">{e.proto}</td>
            <td class="mono">{e.src}{#each listsOf(e.src).slice(0, 2) as n (n)}<span class="logtag">@{n}</span>{/each}</td><td class="mono">{e.dport ?? '—'}</td>
            <td title={rn.label}>{rn.known ? `#${rn.known.index + 1} ` : ''}{rn.label.length > 30 ? rn.label.slice(0, 29) + '…' : rn.label}</td><td class="mono r">{l.n}</td></tr>
          {#if op}<tr><td colspan="8">{@render detail(e, rn)}</td></tr>{/if}
        {:else}<tr><td colspan="8" class="dim">{t('fw.log.nothing')} <button class="btn sm" type="button" onclick={reset}>{t('fw.log.reset')}</button></td></tr>{/each}
      </tbody>
    </table></div>
    <div class="row"><span class="dim">{t('fw.log.shown', { n: shownLines.length, total: lines.length })}{#if truncated} · {t('fw.log.truncated')}{/if}</span><span class="grow"></span>
      {#if shownLines.length < lines.length}<button class="btn sm" type="button" onclick={() => (limit += 15)}>{t('fw.log.more')}</button>{/if}</div>
  {/if}
  {#snippet foot()}<span class="dim">{t('fw.log.source')}</span><span class="spacer"></span><button class="btn" type="button" onclick={onclose}>{t('common.close')}</button>{/snippet}
</Modal>
