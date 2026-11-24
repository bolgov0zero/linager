<script lang="ts">
  import { onMount } from 'svelte';
  import { serverApi, type ProcessInfo, type ServerDetail } from '../api';
  import { confirmAsk } from '../dialogs.svelte';
  import Icon from '../Icon.svelte';
  import SearchField from '../SearchField.svelte';
  import { t } from '../i18n.svelte';
  import { fmtBytes } from '../status';

  let { s }: { s: ServerDetail } = $props();

  let list = $state<ProcessInfo[]>([]);
  let total = $state(0);
  let sort = $state<'cpu' | 'mem' | 'name'>('cpu');
  let q = $state('');
  let error = $state('');

  async function load(): Promise<void> {
    try {
      const r = await serverApi.processes(s.id, sort, 50);
      list = r.processes;
      total = r.total;
      error = '';
    } catch {
      error = t('sp.err.load');
    }
  }
  $effect(() => {
    void sort;
    void load();
  });
  onMount(() => {
    const i = setInterval(() => void load(), 3000);
    return () => clearInterval(i);
  });

  const shown = $derived(q.trim() ? list.filter((p) => `${p.cmd} ${p.user} ${p.pid}`.toLowerCase().includes(q.trim().toLowerCase())) : list);

  async function kill(p: ProcessInfo): Promise<void> {
    if (!(await confirmAsk({ title: t('sp.proc.kill'), text: t('sp.proc.kill.text', { cmd: p.cmd.slice(0, 60), pid: p.pid }), ok: t('sp.proc.kill'), danger: true }))) return;
    try {
      await serverApi.killProcess(s.id, p.pid, 'TERM');
      await load();
    } catch {
      error = t('sp.err.action');
    }
  }
</script>

<div class="sp-fill">
<div class="sp-tools">
  <SearchField placeholder={t('sp.filter')} bind:value={q} />
  <div class="seg sm">{#each ['cpu', 'mem', 'name'] as k (k)}<button type="button" class:on={sort === k} aria-pressed={sort === k} onclick={() => (sort = k as typeof sort)}>{k === 'cpu' ? 'CPU' : k === 'mem' ? 'RAM' : t('sp.proc.name')}</button>{/each}</div>
  <span class="grow"></span><span class="dim">{t('sp.proc.total', { n: total })}</span>
</div>
{#if error}<div class="err">{error}</div>{/if}
<div class="panel flush fill"><table class="tbl compact fixed">
  <thead><tr><th style="width:72px">PID</th><th style="width:110px">{t('sp.proc.user')}</th><th>{t('sp.proc.cmd')}</th><th class="r" style="width:68px">CPU</th><th class="r" style="width:68px">RAM</th><th class="r" style="width:84px">RSS</th><th style="width:60px"></th></tr></thead>
  <tbody>
    {#each shown as p (p.pid)}
      <tr><td class="mono dim">{p.pid}</td><td>{p.user}</td><td class="mono" title={p.cmd}>{p.cmd}</td>
        <td class="mono r">{p.cpu.toFixed(1)}%</td><td class="mono r">{p.mem_pct.toFixed(1)}%</td><td class="mono r dim">{fmtBytes(p.rss)}</td>
        <td class="r"><button class="btn icon sm danger" type="button" title={t('sp.proc.kill')} onclick={() => kill(p)}><Icon name="x" size={14} /></button></td></tr>
    {:else}
      <tr><td colspan="7" class="dim">{t('dash.nothing')}</td></tr>
    {/each}
  </tbody>
</table></div>
</div>
