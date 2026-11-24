<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { ApiError, serverApi2, type ApplyWarning, type FirewallDraft, type FirewallInfo, type FwChain, type FwSet, type ServerDetail } from '../api';
  import CustomSelect from '../CustomSelect.svelte';
  import { confirmAsk } from '../dialogs.svelte';
  import { errorText } from '../errors';
  import { buildRule, normalizeRule, type ListRef, type RuleAction, type RuleProto } from '../fw-rule';
  import { canLog, isSystemRule, parseRule, setLogged } from '../fw-view';
  import Icon from '../Icon.svelte';
  import { t, i18n } from '../i18n.svelte';
  import FwLogModal from './FwLogModal.svelte';
  import FwPorts from './FwPorts.svelte';
  import FwSetsModal from './FwSetsModal.svelte';
  import PendingBanner from './PendingBanner.svelte';
  import SourceInput from './SourceInput.svelte';

  type ChainName = FwChain['name'];
  type Policy = FwChain['policy'];
  type RuleRow = { chain: ChainName; index: number; text: string; last: number };
  const CHAINS: ChainName[] = ['input', 'forward', 'output'];

  let { s }: { s: ServerDetail } = $props();
  const sid = $derived(s.id);

  let info = $state<FirewallInfo | null>(null);
  let loadFailed = $state(false);
  let connLost = $state(false);
  // the local draft: null means "no draft", the view mirrors the live rules
  let draft = $state<FirewallDraft | null>(null);
  let dirty = $state(false);
  let saving = $state(false);
  let saveError = $state('');
  let applying = $state(false);
  let draftError = $state('');
  let note = $state<{ text: string; bad: boolean } | null>(null);
  let noteTimer: ReturnType<typeof setTimeout> | undefined;
  let saveTimer: ReturnType<typeof setTimeout> | undefined;
  let saveSeq = 0;
  let lastPending: string | null = null;
  let confirmed: string | null = null;

  // the add-rule form
  let addChain = $state<ChainName>('input');
  let addAction = $state<RuleAction>('accept');
  let addProto = $state<RuleProto>('tcp');
  let addPort = $state('');
  let addSource = $state('');
  let addComment = $state('');
  let formError = $state('');
  let rawOpen = $state(false);
  let rawText = $state('');
  let rawError = $state('');
  let addBusy = $state(false);
  let showRuleset = $state(false);
  let portsOpen = $state(false);
  let portsMeta = $state<{ n: number; norule: number } | null>(null);
  let activeChain = $state<ChainName | 'all'>('input');
  let asNft = $state(false);
  let setsOpen = $state(false);
  let logOpen = $state(false);
  let logBadge = $state<number | null>(null);

  const pending = $derived(info?.pending ?? null);
  const nf = $derived(i18n.locale === 'ru' ? 'ru-RU' : 'en-US');
  const visible = <T extends { text: string }>(rules: T[]): T[] => rules.filter((r) => !isSystemRule(r.text));
  const copySets = (sets: FwSet[] | undefined): FwSet[] => (sets ?? []).map((x) => ({ name: x.name, family: x.family, elements: [...x.elements] }));
  const fromInfo = (chains: FwChain[], sets: FwSet[] | undefined): FirewallDraft => ({
    sets: copySets(sets),
    chains: CHAINS.map((name) => {
      const c = chains.find((x) => x.name === name);
      return { name, policy: c?.policy ?? 'accept', rules: visible(c?.rules ?? []).map((r) => ({ text: r.text })) };
    }),
  });
  const copyDraft = (src: FirewallDraft): FirewallDraft => ({ sets: copySets(src.sets ?? info?.sets), chains: src.chains.map((c) => ({ name: c.name, policy: c.policy, rules: visible(c.rules).map((r) => ({ text: r.text })) })) });
  const base = (): FirewallDraft => draft ?? fromInfo(info?.chains ?? [], info?.sets);
  const mapChain = (d: FirewallDraft, name: ChainName, fn: (c: FirewallDraft['chains'][number]) => FirewallDraft['chains'][number]): FirewallDraft => ({ ...d, chains: d.chains.map((c) => (c.name === name ? fn(c) : c)) });

  const shown = $derived<FirewallDraft | null>(info ? (draft ?? fromInfo(info.chains, info.sets)) : null);
  const shownSets = $derived<FwSet[]>(shown?.sets ?? []);
  const refs = $derived<ListRef[]>(shownSets.map((x) => ({ name: x.name, family: x.family })));
  const allRows = $derived<RuleRow[]>((shown?.chains ?? []).flatMap((c) => c.rules.map((r, index) => ({ chain: c.name, index, text: r.text, last: c.rules.length - 1 }))));
  const rowsOf = (name: ChainName | 'all'): RuleRow[] => (name === 'all' ? allRows : allRows.filter((r) => r.chain === name));
  const allRuleTexts = $derived(allRows.map((r) => r.text));
  const loggedCount = $derived(allRows.filter((r) => parseRule(r.text).logged).length);
  const known = $derived(allRows.map((r) => ({ chain: r.chain, index: r.index, text: r.text })));

  type Change = { key: string; vars: Record<string, string> };
  const changes = $derived.by<Change[]>(() => {
    if (!draft || !info) return [];
    const out: Change[] = [];
    for (const name of CHAINS) {
      const before = info.chains.find((c) => c.name === name);
      const after = draft.chains.find((c) => c.name === name);
      if (!after) continue;
      const bp = before?.policy ?? after.policy;
      if (bp !== after.policy) out.push({ key: 'fw.change.policy', vars: { chain: name, from: bp, to: after.policy } });
      const was = visible(before?.rules ?? []).map((r) => r.text);
      const next = after.rules.map((r) => r.text);
      const added = next.filter((x) => !was.includes(x));
      const removed = was.filter((x) => !next.includes(x));
      for (const rule of added) out.push({ key: 'fw.change.added', vars: { chain: name, rule } });
      for (const rule of removed) out.push({ key: 'fw.change.removed', vars: { chain: name, rule } });
      if (!added.length && !removed.length && was.join('\n') !== next.join('\n')) out.push({ key: 'fw.change.order', vars: { chain: name } });
    }
    const wasSets = info.sets ?? [];
    const nowSets = draft.sets ?? wasSets;
    for (const set of nowSets) {
      const b = wasSets.find((x) => x.name === set.name);
      if (!b) {
        out.push({ key: 'fw.change.setAdded', vars: { name: set.name } });
        continue;
      }
      const a = set.elements.filter((e) => !b.elements.includes(e)).length;
      const r = b.elements.filter((e) => !set.elements.includes(e)).length;
      if (a || r) out.push({ key: 'fw.change.setChanged', vars: { name: set.name, added: String(a), removed: String(r) } });
    }
    for (const set of wasSets) if (!nowSets.some((x) => x.name === set.name)) out.push({ key: 'fw.change.setRemoved', vars: { name: set.name } });
    return out;
  });

  function say(text: string, bad = false): void {
    note = { text, bad };
    clearTimeout(noteTimer);
    noteTimer = setTimeout(() => (note = null), bad ? 8000 : 4000);
  }

  async function load(): Promise<void> {
    const seqAtStart = saveSeq;
    try {
      const next = await serverApi2.firewall(sid);
      info = next;
      loadFailed = false;
      connLost = false;
      if (next.pending) lastPending = next.pending.id;
      else if (lastPending !== null) {
        say(t(confirmed === lastPending ? 'fw.toast.confirmed' : 'fw.toast.rolledBack'));
        lastPending = confirmed = null;
      }
      // unsaved local edits are never overwritten
      if (!dirty && !saving && seqAtStart === saveSeq) draft = next.draft ? copyDraft(next.draft) : null;
    } catch (e) {
      connLost = e instanceof ApiError && e.status === 0;
      loadFailed = true;
    }
  }
  onMount(() => {
    void load();
    const i = setInterval(() => void load(), 3000);
    return () => {
      clearInterval(i);
      clearTimeout(saveTimer);
      // keep unsaved edits when the tab is left
      untrack(() => {
        if (dirty && draft) void serverApi2.firewallSaveDraft(sid, draft).catch(() => undefined);
      });
    };
  });

  // the counter on the closed "Ports" block, and the badge of the journal
  $effect(() => {
    if (info?.available !== true || untrack(() => portsOpen)) return;
    let active = true;
    void serverApi2.firewallPorts(sid).then((res) => {
      if (active && res.available) portsMeta = { n: res.listeners.length, norule: res.listeners.filter((l) => l.firewall === 'open').length };
    }).catch(() => undefined);
    return () => (active = false);
  });
  async function loadBadge(): Promise<void> {
    try {
      const res = await serverApi2.firewallLog(sid);
      const from = Date.now() / 1000 - 3600;
      logBadge = res.events.filter((e) => e.time >= from && e.action !== 'accept').length;
    } catch {
      // the badge is optional
    }
  }
  $effect(() => {
    if (loggedCount === 0 || logOpen) {
      if (loggedCount === 0) logBadge = null;
      return;
    }
    void loadBadge();
    const i = setInterval(() => void loadBadge(), 60_000);
    return () => clearInterval(i);
  });

  // ---- editing and saving the draft ----
  function commit(next: FirewallDraft): void {
    draft = next;
    dirty = true;
    draftError = '';
    clearTimeout(saveTimer);
    saveTimer = setTimeout(() => void save(), 500);
  }
  async function save(): Promise<void> {
    clearTimeout(saveTimer);
    const snapshot = draft;
    if (!snapshot) return;
    saving = true;
    try {
      await serverApi2.firewallSaveDraft(sid, snapshot);
      if (draft === snapshot) dirty = false;
      saveError = '';
    } catch (e) {
      saveError = errorText(e);
    } finally {
      saving = false;
      saveSeq++;
    }
  }
  const setPolicy = (name: ChainName, policy: Policy): void => commit(mapChain(base(), name, (c) => ({ ...c, policy })));
  const removeRule = (name: ChainName, index: number): void => commit(mapChain(base(), name, (c) => ({ ...c, rules: c.rules.filter((_, i) => i !== index) })));
  function moveRule(name: ChainName, index: number, delta: -1 | 1): void {
    commit(mapChain(base(), name, (c) => {
      const to = index + delta;
      if (to < 0 || to >= c.rules.length) return c;
      const rules = [...c.rules];
      [rules[index], rules[to]] = [rules[to], rules[index]];
      return { ...c, rules };
    }));
  }
  /** Checks the rule against the server draft and appends it to a chain; returns an error text or ''. */
  async function appendRule(name: ChainName, text: string): Promise<string> {
    if (isSystemRule(text)) return t('fw.add.systemRule');
    const next = mapChain(base(), name, (c) => ({ ...c, rules: [...c.rules, { text }] }));
    addBusy = true;
    try {
      const res = await serverApi2.firewallValidate(sid, next);
      if (!res.ok) return res.error || t('fw.add.invalid');
      commit(next);
      return '';
    } catch (e) {
      return errorText(e);
    } finally {
      addBusy = false;
    }
  }
  async function addFromForm(): Promise<void> {
    if (addBusy) return;
    const built = buildRule({ proto: addProto, port: addPort, source: addSource, action: addAction, comment: addComment }, refs);
    if (!built.ok) return void (formError = t(built.error));
    formError = await appendRule(addChain, built.text);
    if (!formError) {
      addPort = addSource = addComment = '';
      activeChain = addChain;
    }
  }
  async function addRaw(): Promise<void> {
    const text = normalizeRule(rawText);
    if (!text || addBusy) return;
    rawError = await appendRule(addChain, text);
    if (!rawError) rawText = '';
  }
  const changeSets = (next: FwSet[]): void => commit({ ...base(), sets: next });
  async function addInputRules(texts: string[]): Promise<string | null> {
    for (const text of texts) {
      const err = await appendRule('input', text);
      if (err) return err;
    }
    activeChain = 'input';
    return null;
  }
  const toggleLog = (chain: ChainName, index: number, on: boolean): void =>
    commit(mapChain(base(), chain, (c) => ({ ...c, rules: c.rules.map((r, i) => (i === index ? { text: setLogged(r.text, chain, on) } : r)) })));
  function logDrops(): void {
    let next = base();
    for (const c of next.chains) c.rules.forEach((r, i) => {
      if (/\b(drop|reject)\b/.test(r.text) && canLog(r.text)) next = mapChain(next, c.name, (x) => ({ ...x, rules: x.rules.map((y, j) => (j === i ? { text: setLogged(y.text, c.name, true) } : y)) }));
    });
    commit(next);
  }
  function addToList(list: string, ip: string): string | null {
    const set = shownSets.find((x) => x.name === list);
    if (!set) return t('fw.log.noList');
    if ((ip.includes(':') ? 'ipv6' : 'ipv4') !== set.family) return t('fw.log.wrongFamily', { list });
    if (!set.elements.includes(ip)) changeSets(shownSets.map((x) => (x.name === list ? { ...x, elements: [...x.elements, ip] } : x)));
    return null;
  }
  /** A rule that drops the address goes first in input (the draft is applied as usual). */
  async function blockAddress(ip: string): Promise<string | null> {
    const built = buildRule({ proto: 'any', port: '', source: ip, action: 'drop', comment: '' }, refs);
    if (!built.ok) return t(built.error);
    const next = mapChain(base(), 'input', (c) => ({ ...c, rules: [{ text: built.text }, ...c.rules] }));
    try {
      const res = await serverApi2.firewallValidate(sid, next);
      if (!res.ok) return res.error || t('fw.add.invalid');
    } catch (e) {
      return errorText(e);
    }
    commit(next);
    return null;
  }
  async function resetDraft(): Promise<void> {
    clearTimeout(saveTimer);
    draftError = '';
    try {
      await serverApi2.firewallResetDraft(sid);
      draft = null;
      dirty = false;
      saveError = '';
      await load();
    } catch (e) {
      draftError = errorText(e);
    }
  }

  // ---- apply: warnings, then the countdown ----
  async function apply(confirm: boolean): Promise<void> {
    applying = true;
    draftError = '';
    try {
      if (dirty) await save();
      if (dirty) {
        draftError = saveError || t('fw.error.unknown');
        return;
      }
      await serverApi2.firewallApply(sid, confirm);
      draft = null;
      dirty = false;
      await load();
    } catch (e) {
      const w = e instanceof ApiError && e.status === 409 ? ((e.body as { warnings?: ApplyWarning[] } | null)?.warnings ?? []) : [];
      if (w.length > 0) {
        applying = false;
        const ok = await confirmAsk({ title: t('fw.warn.title'), text: `${t('fw.warn.explain')}\n\n${w.map((x) => `• ${x.message}`).join('\n')}`, ok: t('fw.warn.apply'), cancel: t('fw.warn.cancel'), danger: true });
        if (ok) await apply(true);
        return;
      }
      draftError = errorText(e);
    } finally {
      applying = false;
    }
  }

  function fmtBytes(n: number): string {
    const units = ['b', 'kb', 'mb', 'gb', 'tb'];
    let v = n;
    let i = 0;
    while (v >= 1024 && i < units.length - 1) {
      v /= 1024;
      i++;
    }
    return `${v.toLocaleString(nf, { maximumFractionDigits: i === 0 ? 0 : 1 })} ${t('fw.unit.' + units[i])}`;
  }
  const live = (chain: ChainName, text: string) => info?.chains.find((c) => c.name === chain)?.rules.find((r) => r.text === text);
  const protoOptions = $derived([{ v: 'tcp', t: 'TCP' }, { v: 'udp', t: 'UDP' }, { v: 'icmp', t: 'ICMP' }, { v: 'any', t: t('fw.add.protoAny') }]);
  const actionOptions = $derived([{ v: 'accept', t: t('fw.act.accept') }, { v: 'drop', t: t('fw.act.drop') }, { v: 'reject', t: t('fw.act.reject') }]);
  const chainOptions = CHAINS.map((c) => ({ v: c, t: c }));
</script>

{#if loadFailed && !info}<div class="err">{t('fw.loadError')}</div>{:else if !info}<p class="dim">{t('dash.loading')}</p>{:else}
  <div class="fw-page">
    {#if note}<div class="fm-note" class:bad={note.bad}><span>{note.text}</span></div>{/if}
    {#if !info.available}<div class="banner">{t('fw.notAvailable')}</div>{:else}
      <p class="dim mono">{t('fw.status.active')}</p>
    {/if}
    {#if info.ufw_active}<div class="banner">{t('fw.ufwWarning')}</div>{/if}
    {#if pending}<PendingBanner serverId={sid} {pending} lost={connLost} onconfirmed={(id) => (confirmed = id)} onchanged={load} />{/if}

    {#if draft}
      <section class="panel">
        <div class="row"><h4 class="ph" style="margin:0">{t('fw.draft.title')}</h4>{#if saving}<span class="dim">{t('fw.draft.saving')}</span>{/if}<span class="grow"></span>
          <button class="btn sm primary" type="button" disabled={applying || pending !== null} onclick={() => apply(false)}>{t('fw.draft.apply')}</button>
          <button class="btn sm" type="button" disabled={applying} onclick={resetDraft}>{t('fw.draft.reset')}</button></div>
        <p class="dim">{t('fw.draft.hint')}</p>
        {#if changes.length}<ul class="changes mono">{#each changes as c, i (i)}<li>{t(c.key, c.vars)}</li>{/each}</ul>{/if}
        {#if saveError || draftError}<div class="err" role="alert">{draftError || saveError}</div>{/if}
      </section>
    {/if}

    {#if info.available}
      <section class="panel">
        <div class="row">
          <h4 class="ph" style="margin:0">{t('fw.card.rules')}</h4><span class="grow"></span>
          <button class="btn sm" type="button" aria-pressed={asNft} onclick={() => (asNft = !asNft)}>{asNft ? t('fw.view.columns') : t('fw.view.nft')}</button>
          <button class="btn sm" type="button" onclick={() => (setsOpen = true)}>{t('fw.sets.open')} <span class="badge b-mute">{shownSets.length}</span></button>
          <button class="btn sm" type="button" onclick={() => (logOpen = true)}>{t('fw.log.open')}{#if logBadge !== null && logBadge > 0} <span class="badge b-crit">{logBadge}</span>{/if}</button>
        </div>
        <div class="row" style="margin-top:10px">
          <button class="chip" type="button" aria-pressed={activeChain === 'all'} onclick={() => (activeChain = 'all')}>{t('fw.chains.all')}<em>{allRows.length}</em></button>
          {#each CHAINS as name (name)}<button class="chip" type="button" aria-pressed={activeChain === name} onclick={() => { activeChain = name; addChain = name; }}>{name}<em>{rowsOf(name).length}</em></button>{/each}
          <span class="grow"></span>
          {#if activeChain !== 'all'}{@const chain = shown?.chains.find((c) => c.name === activeChain)}
            {#if chain}<span class="dim">{t('fw.chain.policyLabel')}</span><CustomSelect small value={chain.policy} options={[{ v: 'accept', t: 'accept' }, { v: 'drop', t: 'drop' }]} onchange={(v) => setPolicy(chain.name, v as Policy)} />{/if}
          {:else}<span class="dim mono">{(shown?.chains ?? []).map((c) => `${c.name}: ${c.policy}`).join(' · ')}</span>{/if}
        </div>
        <div class="tw" style="margin-top:10px;max-height:none"><table class="tbl compact">
          <thead><tr>
            {#if asNft}<th>{t('fw.col.rule')}</th>{:else}{#if activeChain === 'all'}<th>{t('fw.col.chain')}</th>{/if}<th>{t('fw.col.action')}</th><th>{t('fw.col.proto')}</th><th>{t('fw.col.port')}</th><th>{t('fw.col.source')}</th><th>{t('fw.col.note')}</th>{/if}
            <th>{t('fw.col.log')}</th><th class="r">{t('fw.col.packets')}</th><th class="r">{t('fw.col.bytes')}</th><th></th>
          </tr></thead>
          <tbody>
            {#each rowsOf(activeChain) as row (row.chain + ':' + row.index + ':' + row.text)}
              {@const v = parseRule(row.text)}{@const lv = live(row.chain, row.text)}
              <tr title={row.text}>
                {#if asNft}<td class="mono">{activeChain === 'all' ? row.chain + ': ' : ''}{row.text}</td>{:else}
                  {#if activeChain === 'all'}<td class="mono">{row.chain}</td>{/if}
                  <td><span class="act {v.action}">{t(`fw.act.${v.action}`)}</span></td>
                  <td class="mono">{v.proto ? (v.proto === 'ICMPV6' ? 'ICMPv6' : v.proto) : t('fw.any')}</td><td class="mono">{v.port || '—'}</td>
                  <td class="mono">{#if v.source}{v.source}{:else if v.iface}<span class="dim">{t('fw.iface', { name: v.iface })}</span>{:else}<span class="dim">{t('fw.any')}</span>{/if}</td>
                  <td>{#if v.comment}{v.comment}{/if}{#if v.extra} <span class="dim mono">{v.extra}</span>{/if}</td>
                {/if}
                <td><button class="rule-sw" type="button" aria-pressed={v.logged} disabled={!canLog(row.text)} aria-label={t('fw.log.toggle')} title={t('fw.log.toggle')} onclick={() => toggleLog(row.chain, row.index, !v.logged)}></button></td>
                <td class="mono r">{lv?.packets ?? '—'}</td><td class="mono r">{lv?.bytes === undefined ? '—' : fmtBytes(lv.bytes)}</td>
                <td class="r"><span class="r-btns">
                  <button class="btn icon sm" type="button" title={t('fw.rule.up')} disabled={row.index === 0} onclick={() => moveRule(row.chain, row.index, -1)}><Icon name="arrow-up" size={13} /></button>
                  <button class="btn icon sm" type="button" title={t('fw.rule.down')} disabled={row.index === row.last} onclick={() => moveRule(row.chain, row.index, 1)}><Icon name="arrow-down" size={13} /></button>
                  <button class="btn icon sm danger" type="button" title={t('fw.rule.delete')} onclick={() => removeRule(row.chain, row.index)}><Icon name="x" size={13} /></button></span></td>
              </tr>
            {:else}<tr><td colspan="10" class="dim">{t('fw.chain.empty')}</td></tr>{/each}
          </tbody>
        </table></div>
        <p class="dim">{t('fw.chain.established')}</p>
      </section>

      <section class="panel">
        <h4 class="ph">{t('fw.add.title')}</h4>
        <form style="display:grid;gap:10px" onsubmit={(e) => { e.preventDefault(); void addFromForm(); }}>
          <div class="row" style="align-items:flex-end;flex-wrap:wrap">
            <div class="fl"><span>{t('fw.add.chain')}</span><CustomSelect value={addChain} options={chainOptions} onchange={(v) => (addChain = v as ChainName)} /></div>
            <div class="fl"><span>{t('fw.add.action')}</span><CustomSelect value={addAction} options={actionOptions} onchange={(v) => (addAction = v as RuleAction)} /></div>
            <div class="fl"><span>{t('fw.add.proto')}</span><CustomSelect value={addProto} options={protoOptions} onchange={(v) => (addProto = v as RuleProto)} /></div>
            <label class="fl"><span>{t('fw.add.port')}</span><span class="field" style="width:130px"><input class="mono" inputmode="numeric" autocomplete="off" spellcheck="false" bind:value={addPort} oninput={() => (formError = '')} /></span></label>
            <div class="fl"><span>{t('fw.add.source')}</span><SourceInput lists={shownSets} ariaLabel={t('fw.add.source')} bind:value={addSource} oninput={() => (formError = '')} /></div>
            <label class="fl grow"><span>{t('fw.add.comment')}</span><span class="field"><input autocomplete="off" bind:value={addComment} oninput={() => (formError = '')} /></span></label>
            <button class="btn primary" type="submit" disabled={addBusy}>{addBusy ? t('fw.add.checking') : t('fw.add.submit')}</button>
          </div>
          {#if formError}<div class="err" role="alert">{formError}</div>{/if}
          <button class="btn" type="button" style="justify-self:start" aria-expanded={rawOpen} onclick={() => (rawOpen = !rawOpen)}>{t('fw.add.rawToggle')}</button>
          {#if rawOpen}
            <textarea class="ta" rows="3" placeholder={t('fw.add.rawPlaceholder')} aria-label={t('fw.add.rawToggle')} bind:value={rawText} oninput={() => (rawError = '')}></textarea>
            {#if rawError}<div class="err" role="alert">{rawError}</div>{/if}
            <div class="row"><button class="btn" type="button" disabled={addBusy || !rawText.trim()} onclick={addRaw}>{t('fw.add.raw')}</button></div>
          {/if}
        </form>
      </section>

      <section class="panel">
        <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
        <div class="row" style="cursor:pointer" onclick={() => (portsOpen = !portsOpen)}>
          <h4 class="ph" style="margin:0">{t('fw.card.ports')}</h4>
          {#if portsMeta}<span class="badge {portsMeta.norule > 0 ? 'b-warn' : 'b-mute'}">{portsMeta.n}</span>{/if}<span class="grow"></span><Icon name={portsOpen ? 'chevron-down' : 'chevron-right'} size={14} />
        </div>
        {#if portsOpen}<div style="display:grid;gap:10px;margin-top:10px"><FwPorts serverId={sid} lists={shownSets} onrules={addInputRules} onsummary={(n, norule) => (portsMeta = { n, norule })} /></div>{/if}
      </section>

      <section class="panel">
        <div class="row"><h4 class="ph" style="margin:0">{t('fw.ruleset.title')}</h4><span class="grow"></span><button class="btn sm" type="button" aria-expanded={showRuleset} onclick={() => (showRuleset = !showRuleset)}>{showRuleset ? t('fw.ruleset.hide') : t('fw.ruleset.show')}</button></div>
        {#if showRuleset}<pre class="pv">{info.ruleset}</pre>{/if}
        {#if info.other_tables.length}<p class="dim">{t('fw.otherTables', { tables: info.other_tables.join(', ') })}</p>{/if}
      </section>
    {/if}
  </div>

  {#if setsOpen}<FwSetsModal sets={shownSets} rules={allRuleTexts} onchange={changeSets} onclose={() => (setsOpen = false)} />{/if}
  {#if logOpen}<FwLogModal serverId={sid} rules={known} sets={shownSets} onToggle={toggleLog} onToggleDrops={logDrops} onAddToList={addToList} onBlock={blockAddress} onOpenRule={(c) => { activeChain = c; logOpen = false; }} onclose={() => (logOpen = false)} />{/if}
{/if}
