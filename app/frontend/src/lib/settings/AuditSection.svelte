<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type AuditItem } from '../api';
  import CustomSelect from '../CustomSelect.svelte';
  import Icon from '../Icon.svelte';
  import { i18n, t } from '../i18n.svelte';
  import SearchField from '../SearchField.svelte';
  import { ACTION_GROUPS, KNOWN_ACTIONS, RESULT_OPTIONS } from './audit-actions';

  const PAGE_SIZE = 100;
  const REFRESH_MS = 15_000;
  const TEXT_DEBOUNCE_MS = 400;

  // Filter controls (what the person sees) and the applied text filters (what requests use).
  let result = $state('');
  let group = $state('');
  let search = $state('');
  let actor = $state('');
  let appliedSearch = $state('');
  let appliedActor = $state('');

  let items = $state<AuditItem[]>([]);
  let nextBefore = $state(0);
  // Pages loaded since the last reset; the automatic refresh only runs while this is 1.
  let pages = $state(0);
  let loading = $state(false);
  let loaded = $state(false);
  let failed = $state(false);
  // Expanded rows are keyed by audit id, so they survive refreshes and appended pages.
  let expanded = $state<Partial<Record<number, boolean>>>({});

  // Only the newest request may update state, so stale answers are ignored.
  let requestSeq = 0;
  let textTimer: ReturnType<typeof setTimeout> | undefined;

  async function load(reset: boolean): Promise<void> {
    if (!reset && nextBefore <= 0) return;
    const id = ++requestSeq;
    loading = true;
    try {
      const page = await api.audit({ before: reset ? 0 : nextBefore, limit: PAGE_SIZE, action: group, actor: appliedActor, result, q: appliedSearch });
      if (id !== requestSeq) return;
      items = reset ? page.items : [...items, ...page.items];
      nextBefore = page.next_before;
      pages = reset ? 1 : pages + 1;
      failed = false;
    } catch {
      if (id === requestSeq) failed = true;
    } finally {
      if (id === requestSeq) {
        loading = false;
        loaded = true;
      }
    }
  }
  function onTextInput(): void {
    clearTimeout(textTimer);
    textTimer = setTimeout(() => {
      appliedSearch = search.trim();
      appliedActor = actor.trim();
      void load(true);
    }, TEXT_DEBOUNCE_MS);
  }
  // the search field also clears with its cross: the text is watched, not the key presses
  let first = true;
  $effect(() => {
    void search;
    if (first) {
      first = false;
      return;
    }
    onTextInput();
  });
  onMount(() => {
    void load(true);
    const timer = setInterval(() => {
      // refresh page 1 only, and only if nothing is in flight; the older rows loaded are never dropped
      if (pages === 1 && !loading) void load(true);
    }, REFRESH_MS);
    return () => {
      clearInterval(timer);
      clearTimeout(textTimer);
      requestSeq++;
    };
  });

  const resultOptions = $derived(RESULT_OPTIONS.map((o) => ({ v: o.value, t: t(o.key) })));
  const groupOptions = $derived(ACTION_GROUPS.map((o) => ({ v: o.value, t: t(o.key) })));
  const formatTime = (ts: number): string => new Date(ts * 1000).toLocaleString(i18n.locale === 'en' ? 'en-GB' : 'ru-RU', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' });
  const actionText = (a: string): string => (KNOWN_ACTIONS.has(a) ? t(`st.audit.action.${a}`) : a);
  const resultTone = (r: string): string => (r === 'ok' ? 'b-ok' : r === 'failed' ? 'b-crit' : 'b-warn');
  const resultText = (r: string): string => (r === 'ok' ? t('st.audit.result.ok') : r === 'failed' ? t('st.audit.result.failed') : r);
</script>

<div class="set-sec wide">
  <div class="row"><h3 style="margin:0">{t('st.audit.title')}</h3><span class="grow"></span><button class="btn sm" type="button" disabled={loading} onclick={() => void load(true)}>{t('st.audit.refresh')}</button></div>
  <div class="row wrapr">
    <label class="fl small"><span>{t('st.audit.filter.result')}</span><CustomSelect value={result} options={resultOptions} onchange={(v) => { result = v; void load(true); }} /></label>
    <label class="fl small"><span>{t('st.audit.filter.group')}</span><CustomSelect value={group} options={groupOptions} onchange={(v) => { group = v; void load(true); }} /></label>
    <div class="fl grow"><span>{t('st.audit.filter.search')}</span><SearchField placeholder={t('st.audit.filter.searchPlaceholder')} bind:value={search} spellcheck={false} /></div>
    <label class="fl small"><span>{t('st.audit.filter.actor')}</span><span class="field"><input placeholder={t('st.audit.filter.actorPlaceholder')} bind:value={actor} oninput={onTextInput} /></span></label>
  </div>

  {#if failed}<div class="err" role="alert">{t('st.audit.loadFailed')}</div>{/if}
  {#if !loaded}<p class="dim">{t('st.audit.loading')}</p>
  {:else if items.length === 0}<p class="dim">{failed ? '' : t('st.audit.empty')}</p>
  {:else}
    <div class="panel flush"><table class="tbl compact">
      <thead><tr><th>{t('st.audit.col.time')}</th><th>{t('st.audit.col.actor')}</th><th>{t('st.audit.col.action')}</th><th>{t('st.audit.col.target')}</th><th>{t('st.audit.col.result')}</th><th>{t('st.audit.col.ip')}</th><th></th></tr></thead>
      <tbody>
        {#each items as it (it.id)}
          <tr>
            <td class="nw mono">{formatTime(it.ts)}</td>
            <td>{it.actor || '—'}</td>
            <td>{actionText(it.action)}</td>
            <td><div>{it.target_name || '—'}</div>{#if it.target_type}<div class="dim">{it.target_type}</div>{/if}</td>
            <td><span class="badge {resultTone(it.result)}">{resultText(it.result)}</span></td>
            <td class="mono">{it.client_ip || '—'}{#if it.remote_addr && it.remote_addr !== it.client_ip} <span class="dim" title={t('st.audit.remoteTitle', { addr: it.remote_addr })}>{it.remote_addr}</span>{/if}</td>
            <td class="r"><button class="btn icon xs" type="button" disabled={!it.details} aria-expanded={!!expanded[it.id]} aria-label={expanded[it.id] ? t('st.audit.collapse') : t('st.audit.expand')} title={expanded[it.id] ? t('st.audit.collapse') : t('st.audit.expand')} onclick={() => (expanded[it.id] = !expanded[it.id])}><Icon name={expanded[it.id] ? 'chevron-down' : 'chevron-right'} size={13} /></button></td>
          </tr>
          {#if expanded[it.id] && it.details}<tr><td colspan="7"><pre class="json mono">{JSON.stringify(it.details, null, 2)}</pre></td></tr>{/if}
        {/each}
      </tbody>
    </table></div>
  {/if}
  {#if nextBefore > 0}<div class="row end"><button class="btn" type="button" disabled={loading} onclick={() => void load(false)}>{t('st.audit.more')}</button></div>{/if}
</div>
