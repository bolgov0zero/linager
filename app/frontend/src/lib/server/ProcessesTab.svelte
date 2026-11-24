<script lang="ts">
  import { onMount } from 'svelte';
  import { serverApi, type ProcessInfo, type ServerDetail } from '../api';
  import CustomSelect from '../CustomSelect.svelte';
  import { hasModule } from '../me.svelte';
  import Modal from '../Modal.svelte';
  import { showToast } from '../toast';
  import { isWindows } from '../status';
  import Icon from '../Icon.svelte';
  import SearchField from '../SearchField.svelte';
  import { t } from '../i18n.svelte';
  import { fmtBytes } from '../status';

  let { s }: { s: ServerDetail } = $props();

  let list = $state<ProcessInfo[]>([]);
  let total = $state(0);
  const ALL = 2000; // the agent's cap
  const COUNTS = [10, 20, 50, 100, ALL];
  let count = $state(50);
  let sort = $state<'cpu' | 'mem' | 'name'>('cpu');
  let victim = $state<ProcessInfo | null>(null);
  let force = $state(false);
  let killing = $state(false);
  const win = $derived(isWindows(s.os));
  const canKill = hasModule('processes');
  const countOptions = $derived(COUNTS.map((n) => ({ v: String(n), t: n === ALL ? t('proc.all') : String(n) })));
  let q = $state('');
  let error = $state('');

  async function load(): Promise<void> {
    try {
      const r = await serverApi.processes(s.id, sort, count);
      list = r.processes;
      total = r.total;
      error = '';
    } catch {
      error = t('sp.err.load');
    }
  }
  $effect(() => {
    void sort;
    void count;
    void load();
  });
  onMount(() => {
    const i = setInterval(() => void load(), 3000);
    return () => clearInterval(i);
  });

  const shown = $derived(q.trim() ? list.filter((p) => `${p.cmd} ${p.user} ${p.pid}`.toLowerCase().includes(q.trim().toLowerCase())) : list);

  function ask(p: ProcessInfo, hard: boolean): void {
    victim = p;
    force = hard;
  }
  async function kill(): Promise<void> {
    const p = victim;
    if (!p || killing) return;
    killing = true;
    try {
      await serverApi.killProcess(s.id, p.pid, force ? 'KILL' : 'TERM');
      showToast(t('proc.sent', { pid: p.pid }));
      victim = null;
      await load();
    } catch {
      showToast(t('proc.killFailed', { pid: p.pid }));
      victim = null;
    } finally {
      killing = false;
    }
  }
  // the agent of Linager keeps the link with the server: stopping it cuts the server off
  const isAgent = (p: ProcessInfo): boolean => /linager/i.test(p.cmd.split(/\s/)[0] ?? '');
</script>

<div class="sp-fill">
<div class="sp-tools">
  <SearchField placeholder={t('sp.filter')} bind:value={q} />
  <div class="seg sm">{#each ['cpu', 'mem', 'name'] as k (k)}<button type="button" class:on={sort === k} aria-pressed={sort === k} onclick={() => (sort = k as typeof sort)}>{k === 'cpu' ? 'CPU' : k === 'mem' ? 'RAM' : t('sp.proc.name')}</button>{/each}</div>
  <span class="dim">{t('proc.show')}</span><CustomSelect small value={String(count)} options={countOptions} onchange={(v) => (count = Number(v))} />
  <span class="grow"></span><span class="dim">{t('sp.proc.total', { n: total })} · {t(count === ALL ? 'proc.refreshAll' : 'proc.refresh', { n: count })}</span>
</div>
{#if error}<div class="err">{error}</div>{/if}
<div class="panel flush fill"><table class="tbl compact fixed">
  <thead><tr><th style="width:72px">PID</th><th style="width:110px">{t('sp.proc.user')}</th><th>{t('sp.proc.cmd')}</th><th class="r" style="width:68px">CPU</th><th class="r" style="width:68px">RAM</th><th class="r" style="width:84px">RSS</th><th style="width:60px"></th></tr></thead>
  <tbody>
    {#each shown as p (p.pid)}
      <tr><td class="mono dim">{p.pid}</td><td>{p.user}</td><td class="mono" title={p.cmd}>{p.cmd}</td>
        <td class="mono r">{p.cpu.toFixed(1)}%</td><td class="mono r">{p.mem_pct.toFixed(1)}%</td><td class="mono r dim">{fmtBytes(p.rss)}</td>
        <td class="r">{#if canKill}<button class="btn icon sm danger" type="button" title={t('sp.proc.kill')} onclick={() => ask(p, false)}><Icon name="x" size={14} /></button>{/if}</td></tr>
    {:else}
      <tr><td colspan="7" class="dim">{t('dash.nothing')}</td></tr>
    {/each}
  </tbody>
</table></div>
</div>

{#if victim}
  <Modal title={t('sp.proc.kill')} onclose={() => (victim = null)}>
    <div class="mdl-stack">
      <p>{win ? t(force ? 'disp.killText' : 'disp.termText', { name: victim.cmd.slice(0, 60), pid: victim.pid }) : t('proc.killText', { cmd: victim.cmd.slice(0, 60), pid: victim.pid })}</p>
      {#if isAgent(victim)}<p class="warnbox warn">{t('disp.agentNote')}</p>{/if}
      <label class="ck"><input type="checkbox" bind:checked={force} />{win ? t('disp.kill') : t('proc.force')}</label>
      {#if force && !win}<p class="dim">{t('proc.forceHint')}</p>{/if}
    </div>
    {#snippet foot()}
      <span class="spacer"></span>
      <button class="btn" type="button" disabled={killing} onclick={() => (victim = null)}>{t('common.cancel')}</button>
      <button class="btn danger" type="button" disabled={killing} onclick={() => void kill()}>{force ? t('proc.confirmKill') : t('proc.confirm')}</button>
    {/snippet}
  </Modal>
{/if}
